package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/config"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/logging"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
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
	default:
		return fmt.Errorf("unknown pooly-agent command %q", args[0])
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
		return fmt.Errorf("usage: pooly-agent collectors list OR pooly-agent collectors run resources --config <path>")
	}
	switch args[0] {
	case "list":
		return collectorsListCommand(args[1:])
	case "run":
		if len(args) < 2 || args[1] != "resources" {
			return fmt.Errorf("usage: pooly-agent collectors run resources --config <path>")
		}
		return collectorsRunResourcesCommand(args[2:])
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
	return nil
}

func collectorsRunResourcesCommand(args []string) error {
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
	opts := resourceOptionsFromConfig(cfg, persist, store)
	observations := resources.Collect(ctx, opts)
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
		return fmt.Errorf("resource collection had required failures")
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

Task status:
  Core foundation, storage foundation, and one-shot Linux resource collectors are present.
  Production monitoring, alerting, rules, incidents, and service monitoring are not implemented.
`)
}
