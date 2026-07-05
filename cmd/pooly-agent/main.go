package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/filewatch"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/journal"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/ssh"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/systemd"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/config"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/logging"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/rules"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/version"
)

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, redaction.Redact(err.Error()))
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		printHelp()
		return nil
	}

	switch args[0] {
	case "version":
		fmt.Println(version.Current().String())
		return nil
	case "run":
		return runCommand(args[1:])
	case "check":
		return checkCommand(args[1:])
	case "status":
		fmt.Println("Pooly Sentinel status: production monitoring is not implemented yet.")
		return nil
	case "doctor":
		return doctorCommand(args[1:])
	case "collectors":
		return collectorsCommand(args[1:])
	case "rules":
		return rulesCommand(args[1:])
	case "incidents":
		return incidentsCommand(args[1:])
	default:
		return fmt.Errorf("unknown pooly-agent command %q", args[0])
	}
}

func rulesCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pooly-agent rules validate --config <path> OR pooly-agent rules test --config <path> --fixture <path>")
	}
	switch args[0] {
	case "validate":
		configPath, err := parseConfigFlag(args[1:])
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg, err := config.LoadFile(ctx, configPath)
		if err != nil {
			return err
		}
		loadedRules, err := rules.FromConfig(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("rules OK: %d configured\n", len(loadedRules))
		return nil
	case "test":
		return rulesTestCommand(args[1:])
	default:
		return fmt.Errorf("unknown rules command %q", args[0])
	}
}

func rulesTestCommand(args []string) error {
	configPath, fixturePath, jsonOutput, persist, err := parseRulesTestFlags(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	loadedRules, err := rules.FromConfig(cfg)
	if err != nil {
		return err
	}
	observations, err := loadObservationFixture(fixturePath)
	if err != nil {
		return err
	}
	store, cleanup, err := openRuleTestStore(ctx, cfg, persist)
	if err != nil {
		return err
	}
	defer cleanup()
	engine := rules.Engine{Rules: loadedRules, NodeID: cfg.Node.ID}
	evaluation, err := engine.Evaluate(ctx, store, observations)
	if err != nil {
		return err
	}
	if jsonOutput {
		data, err := json.MarshalIndent(evaluation, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("rule test evaluated %d rule targets and produced %d incident transitions\n", len(evaluation.Results), len(evaluation.Transitions))
	for _, result := range evaluation.Results {
		fmt.Printf("%s target=%s state=%s severity=%s summary=%s\n", result.RuleID, result.Target, result.State, result.Severity, redaction.Redact(result.Summary))
	}
	return nil
}

func incidentsCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pooly-agent incidents list --config <path> OR pooly-agent incidents show --config <path> --id <id>")
	}
	switch args[0] {
	case "list":
		configPath, err := parseConfigFlag(args[1:])
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg, err := config.LoadFile(ctx, configPath)
		if err != nil {
			return err
		}
		store, err := openConfiguredStore(ctx, cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		records, err := store.ListIncidents(ctx)
		if err != nil {
			return err
		}
		for _, record := range records {
			fmt.Printf("%s status=%s severity=%s target=%s condition=%s summary=%s\n",
				record.ID, record.Status, record.Severity, record.Target, record.Condition, redaction.Redact(record.Summary))
		}
		if len(records) == 0 {
			fmt.Println("no incidents recorded")
		}
		return nil
	case "show":
		configPath, id, err := parseIncidentShowFlags(args[1:])
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg, err := config.LoadFile(ctx, configPath)
		if err != nil {
			return err
		}
		store, err := openConfiguredStore(ctx, cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		record, err := store.GetIncident(ctx, id)
		if err != nil {
			return err
		}
		fmt.Printf("id: %s\nfingerprint: %s\nstatus: %s\nseverity: %s\ntarget: %s\ncondition: %s\nsummary: %s\nfirst_seen: %s\nlast_seen: %s\n",
			record.ID, record.Fingerprint, record.Status, record.Severity, record.Target, record.Condition,
			redaction.Redact(record.Summary), record.FirstSeen.Format("2006-01-02T15:04:05Z07:00"), record.LastSeen.Format("2006-01-02T15:04:05Z07:00"))
		return nil
	default:
		return fmt.Errorf("unknown incidents command %q", args[0])
	}
}

func runCommand(args []string) error {
	configPath, err := parseConfigFlag(args)
	if err != nil {
		return err
	}
	ctx, stop := agent.SignalContext(context.Background())
	defer stop()

	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	logger, err := logging.New(os.Stdout, logging.Options{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	if err != nil {
		return err
	}

	logger.InfoContext(ctx, "configuration loaded",
		logging.Component("agent"),
		slog.String("config_version", cfg.Version),
		slog.String("node_id", cfg.Node.ID),
	)
	fmt.Println("Pooly Sentinel run placeholder active. Production monitoring is not implemented yet. Press Ctrl+C to exit.")
	return agent.RunPlaceholder(ctx, logger)
}

func checkCommand(args []string) error {
	if len(args) == 0 || args[0] != "config" {
		return fmt.Errorf("usage: pooly-agent check config --config <path>")
	}
	configPath, err := parseConfigFlag(args[1:])
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	fmt.Printf("configuration OK: version=%s node_id=%s\n", cfg.Version, redaction.Redact(cfg.Node.ID))
	return nil
}

func doctorCommand(args []string) error {
	configPath, err := parseConfigFlag(args)
	if err != nil {
		return fmt.Errorf("usage: pooly-agent doctor --config <path>: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	fmt.Println("Pooly Sentinel doctor: storage foundation checks only. Production monitoring is not implemented.")
	checks := storage.RunDoctor(ctx, storage.DoctorOptions{
		StateDir:           cfg.Storage.StateDir,
		LogDir:             cfg.Storage.LogDir,
		DatabaseFile:       cfg.Storage.DatabaseFile,
		CurrentMetricsFile: cfg.Storage.CurrentMetricsFile,
		BusyTimeout:        cfg.Storage.SQLite.BusyTimeout.Duration,
		WAL:                cfg.Storage.SQLite.WAL,
	})
	for _, check := range checks {
		fmt.Printf("%s %s: %s\n", check.Status, check.Name, redaction.Redact(check.Message))
	}
	if storage.DoctorFailed(checks) {
		return fmt.Errorf("storage doctor failed")
	}
	fmt.Printf("PASS storage database: %s\n", redaction.Redact(filepath.Join(cfg.Storage.StateDir, cfg.Storage.DatabaseFile)))
	return nil
}

func collectorsCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pooly-agent collectors list OR pooly-agent collectors run <resources|systemd|journal|ssh|filewatch> --config <path>")
	}
	switch args[0] {
	case "list":
		return collectorsListCommand(args[1:])
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: pooly-agent collectors run <resources|systemd|journal|ssh|filewatch> --config <path>")
		}
		return collectorsRunCommand(args[1], args[2:])
	default:
		return fmt.Errorf("unknown collectors command %q", args[0])
	}
}

func collectorsListCommand(args []string) error {
	cfg := config.Default()
	if len(args) > 0 {
		configPath, err := parseConfigFlag(args)
		if err != nil {
			return err
		}
		loaded, err := config.LoadFile(context.Background(), configPath)
		if err != nil {
			return err
		}
		cfg = loaded
	}
	opts := resourceOptionsFromConfig(cfg, false, nil)
	for _, info := range resources.ListCollectors(opts) {
		fmt.Printf("%s enabled=%t supported=%t\n", info.Name, info.Enabled, info.Supported)
	}
	supported := platform.Supported(nil)
	fmt.Printf("systemd enabled=%t supported=%t\n", cfg.Systemd.Enabled, supported)
	fmt.Printf("journal.auth enabled=%t supported=%t\n", cfg.Journal.Auth.Enabled, supported)
	fmt.Printf("journal.services enabled=%t supported=%t\n", cfg.Journal.Services.Enabled, supported)
	fmt.Printf("journal.kernel enabled=%t supported=%t\n", cfg.Journal.Kernel.Enabled, supported)
	fmt.Printf("ssh enabled=%t supported=%t\n", cfg.SSH.Enabled, supported)
	fmt.Printf("filewatch enabled=%t supported=%t targets=%d\n", cfg.Filewatch.Enabled, supported, len(cfg.Filewatch.Targets))
	return nil
}

func collectorsRunCommand(family string, args []string) error {
	configPath, jsonOutput, persist, err := parseCollectorRunFlags(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	var store *storage.Store
	if persist {
		store, err = storage.Open(ctx, storage.SQLiteOptions{
			Path:             filepath.Join(cfg.Storage.StateDir, cfg.Storage.DatabaseFile),
			CreateParentDirs: true,
			BusyTimeout:      cfg.Storage.SQLite.BusyTimeout.Duration,
			WAL:              cfg.Storage.SQLite.WAL,
			Synchronous:      "NORMAL",
		})
		if err != nil {
			return err
		}
		defer store.Close()
	}
	var observations []resources.Observation
	switch family {
	case "resources":
		observations = resources.Collect(ctx, resourceOptionsFromConfig(cfg, persist, store))
	case "systemd":
		observations = systemd.Collect(ctx, systemdOptionsFromConfig(cfg))
	case "journal":
		observations = journal.Collect(ctx, journalOptionsFromConfig(cfg, persist, store))
	case "ssh":
		observations = ssh.Collect(ctx, sshOptionsFromConfig(cfg))
	case "filewatch":
		observations = filewatch.Collect(ctx, filewatchOptionsFromConfig(cfg, persist, store))
	default:
		return fmt.Errorf("unknown collector family %q", family)
	}
	return printCollectorObservations(family, observations, jsonOutput)
}

func printCollectorObservations(family string, observations []resources.Observation, jsonOutput bool) error {
	if jsonOutput {
		data, err := json.MarshalIndent(observations, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		for _, observation := range observations {
			status := "PASS"
			if !observation.Supported {
				status = "UNSUPPORTED"
			} else if !observation.Success {
				status = "FAIL"
			} else if observation.Stale {
				status = "STALE"
			}
			fmt.Printf("%s %s target=%s metrics=%d summary=%s\n", status, observation.Collector, observation.Target, len(observation.Metrics), redaction.Redact(observation.Summary))
		}
	}
	if resources.RequiredFailed(observations) {
		return fmt.Errorf("%s collection had required failures", family)
	}
	return nil
}

func resourceOptionsFromConfig(cfg config.Config, persist bool, store *storage.Store) resources.Options {
	opts := resources.DefaultOptions()
	opts.Persist = persist
	if store != nil {
		opts.State = resources.StorageStateStore{Store: store}
	}
	opts.CPUEnabled = cfg.Resources.Enabled && cfg.Resources.CPU.Enabled
	opts.LoadEnabled = cfg.Resources.Enabled && cfg.Resources.CPU.Enabled
	opts.MemoryEnabled = cfg.Resources.Enabled && cfg.Resources.Memory.Enabled
	opts.PressureEnabled = cfg.Resources.Enabled && cfg.Resources.Pressure.Enabled
	opts.PressureMissingOK = cfg.Resources.Pressure.MissingIsOK
	opts.FilesystemEnabled = cfg.Resources.Enabled && cfg.Resources.Filesystem.Enabled
	opts.FilesystemMounts = cfg.Resources.Filesystem.Mounts
	opts.DiskIOEnabled = cfg.Resources.Enabled && cfg.Resources.DiskIO.Enabled
	opts.DiskAutoDiscover = cfg.Resources.DiskIO.AutoDiscover
	opts.DiskExclude = cfg.Resources.DiskIO.Exclude
	opts.NetworkEnabled = cfg.Resources.Enabled && cfg.Resources.Network.Enabled
	opts.NetworkAutoDiscover = cfg.Resources.Network.AutoDiscover
	opts.NetworkInclude = cfg.Resources.Network.Include
	opts.NetworkExclude = cfg.Resources.Network.Exclude
	opts.UptimeEnabled = cfg.Resources.Enabled && cfg.Resources.Uptime.Enabled
	return opts
}

func systemdOptionsFromConfig(cfg config.Config) systemd.Options {
	opts := systemd.DefaultOptions()
	opts.SystemctlPath = cfg.Commands.Systemctl
	opts.Services = append([]string(nil), cfg.Systemd.CriticalServices...)
	opts.Timeout = cfg.Systemd.Timeout.Duration
	return opts
}

func journalOptionsFromConfig(cfg config.Config, persist bool, store *storage.Store) journal.Options {
	opts := journal.DefaultOptions()
	opts.JournalctlPath = cfg.Commands.Journalctl
	opts.Persist = persist
	if store != nil {
		opts.State = resources.StorageStateStore{Store: store}
	}
	opts.Streams = []journal.StreamConfig{
		journalStreamFromConfig("auth", cfg.Journal.Auth),
		journalStreamFromConfig("services", cfg.Journal.Services),
		journalStreamFromConfig("kernel", cfg.Journal.Kernel),
	}
	return opts
}

func journalStreamFromConfig(name string, stream config.JournalStreamConfig) journal.StreamConfig {
	return journal.StreamConfig{
		Name:          name,
		Enabled:       true,
		Timeout:       stream.Timeout.Duration,
		MaxRecords:    stream.MaxRecords,
		MaxBytes:      stream.MaxBytes,
		MaxFieldBytes: stream.MaxFieldBytes,
	}
}

func sshOptionsFromConfig(cfg config.Config) ssh.Options {
	opts := ssh.DefaultOptions()
	opts.SSHDPath = cfg.Commands.SSHD
	opts.SSPath = cfg.Commands.SS
	opts.Timeout = cfg.SSH.Timeout.Duration
	opts.Expected = ssh.ExpectedConfig{
		Ports:                        append([]int(nil), cfg.SSH.Expected.Ports...),
		ForbiddenPorts:               append([]int(nil), cfg.SSH.Expected.ForbiddenPorts...),
		PermitRootLogin:              cfg.SSH.Expected.PermitRootLogin,
		PasswordAuthentication:       cfg.SSH.Expected.PasswordAuthentication,
		KbdInteractiveAuthentication: cfg.SSH.Expected.KbdInteractiveAuthentication,
		PermitEmptyPasswords:         cfg.SSH.Expected.PermitEmptyPasswords,
		PubkeyAuthentication:         cfg.SSH.Expected.PubkeyAuthentication,
		StrictModes:                  cfg.SSH.Expected.StrictModes,
		PermitUserEnvironment:        cfg.SSH.Expected.PermitUserEnvironment,
	}
	return opts
}

func filewatchOptionsFromConfig(cfg config.Config, persist bool, store *storage.Store) filewatch.Options {
	opts := filewatch.DefaultOptions()
	opts.Timeout = cfg.Filewatch.Timeout.Duration
	opts.MaxFileBytes = cfg.Filewatch.MaxFileBytes
	opts.MaxDirectoryEntries = cfg.Filewatch.MaxDirectoryEntries
	opts.Persist = persist
	if store != nil {
		opts.State = resources.StorageStateStore{Store: store}
	}
	for _, target := range cfg.Filewatch.Targets {
		opts.Targets = append(opts.Targets, filewatch.Target{
			Name:                target.Name,
			Path:                target.Path,
			Type:                target.Type,
			Hash:                target.Hash,
			Manifest:            target.Manifest,
			AllowPrivateKeyHash: target.AllowPrivateKeyHash,
		})
	}
	return opts
}

func parseCollectorRunFlags(args []string) (configPath string, jsonOutput bool, persist bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", false, false, fmt.Errorf("--config requires a path")
			}
			if configPath != "" {
				return "", false, false, fmt.Errorf("--config was provided more than once")
			}
			configPath = args[i+1]
			i++
		case "--json":
			jsonOutput = true
		case "--persist":
			persist = true
		case "--dry-run", "--no-persist":
			persist = false
		default:
			return "", false, false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", false, false, fmt.Errorf("--config <path> is required")
	}
	return configPath, jsonOutput, persist, nil
}

func parseRulesTestFlags(args []string) (configPath string, fixturePath string, jsonOutput bool, persist bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", false, false, fmt.Errorf("--config requires a path")
			}
			configPath = args[i+1]
			i++
		case "--fixture":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", false, false, fmt.Errorf("--fixture requires a path")
			}
			fixturePath = args[i+1]
			i++
		case "--json":
			jsonOutput = true
		case "--persist":
			persist = true
		case "--dry-run", "--no-persist":
			persist = false
		default:
			return "", "", false, false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", "", false, false, fmt.Errorf("--config <path> is required")
	}
	if fixturePath == "" {
		return "", "", false, false, fmt.Errorf("--fixture <path> is required")
	}
	return configPath, fixturePath, jsonOutput, persist, nil
}

func parseIncidentShowFlags(args []string) (configPath string, id string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", fmt.Errorf("--config requires a path")
			}
			configPath = args[i+1]
			i++
		case "--id":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", fmt.Errorf("--id requires a value")
			}
			id = args[i+1]
			i++
		default:
			return "", "", fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", "", fmt.Errorf("--config <path> is required")
	}
	if id == "" {
		return "", "", fmt.Errorf("--id <id> is required")
	}
	return configPath, id, nil
}

func loadObservationFixture(path string) ([]resources.Observation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var observations []resources.Observation
	if err := json.Unmarshal(data, &observations); err == nil {
		return observations, nil
	}
	var wrapped struct {
		Observations []resources.Observation `json:"observations"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Observations, nil
}

func openRuleTestStore(ctx context.Context, cfg config.Config, persist bool) (*storage.Store, func(), error) {
	if persist {
		store, err := openConfiguredStore(ctx, cfg)
		return store, func() {}, err
	}
	dir, err := os.MkdirTemp("", "pooly-rules-test-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	store, err := storage.Open(ctx, storage.SQLiteOptions{
		Path:             filepath.Join(dir, "rules-test.db"),
		CreateParentDirs: true,
		BusyTimeout:      cfg.Storage.SQLite.BusyTimeout.Duration,
		WAL:              cfg.Storage.SQLite.WAL,
		Synchronous:      "NORMAL",
	})
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return store, func() {
		_ = store.Close()
		cleanup()
	}, nil
}

func openConfiguredStore(ctx context.Context, cfg config.Config) (*storage.Store, error) {
	return storage.Open(ctx, storage.SQLiteOptions{
		Path:             filepath.Join(cfg.Storage.StateDir, cfg.Storage.DatabaseFile),
		CreateParentDirs: true,
		BusyTimeout:      cfg.Storage.SQLite.BusyTimeout.Duration,
		WAL:              cfg.Storage.SQLite.WAL,
		Synchronous:      "NORMAL",
	})
}

func parseConfigFlag(args []string) (string, error) {
	var configPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", fmt.Errorf("--config requires a path")
			}
			if configPath != "" {
				return "", fmt.Errorf("--config was provided more than once")
			}
			configPath = args[i+1]
			i++
		case "-config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", fmt.Errorf("-config requires a path")
			}
			if configPath != "" {
				return "", fmt.Errorf("-config was provided more than once")
			}
			configPath = args[i+1]
			i++
		case "--help", "-h", "help":
			return "", fmt.Errorf("usage requires --config <path>")
		default:
			return "", fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", fmt.Errorf("--config <path> is required")
	}
	return configPath, nil
}

func isHelp(arg string) bool {
	return arg == "help" || arg == "--help" || arg == "-h"
}

func printHelp() {
	fmt.Print(`pooly-agent

Pooly Sentinel agent core foundation.

Usage:
  pooly-agent help
  pooly-agent version
  pooly-agent run --config <path>
  pooly-agent check config --config <path>
  pooly-agent status
  pooly-agent doctor --config <path>
  pooly-agent collectors list [--config <path>]
  pooly-agent collectors run resources --config <path> [--json] [--persist|--dry-run]
  pooly-agent collectors run systemd --config <path> [--json] [--dry-run]
  pooly-agent collectors run journal --config <path> [--json] [--persist|--dry-run]
  pooly-agent collectors run ssh --config <path> [--json] [--dry-run]
  pooly-agent collectors run filewatch --config <path> [--json] [--persist|--dry-run]
  pooly-agent rules validate --config <path>
  pooly-agent rules test --config <path> --fixture <path> [--json] [--persist|--dry-run]
  pooly-agent incidents list --config <path>
  pooly-agent incidents show --config <path> --id <id>

Task status:
  Core foundation, storage foundation, one-shot Linux collectors, rule evaluation,
  and incident lifecycle persistence are present.
  Production monitoring, notification delivery, remediation, and scheduling are not implemented.
`)
}
