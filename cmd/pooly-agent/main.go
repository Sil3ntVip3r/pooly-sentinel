package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/api"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/filewatch"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/journal"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/ssh"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/systemd"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/config"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/logging"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/notify"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/reports"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/rules"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/systemdnotify"
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
		fmt.Println("Pooly Sentinel status: use `pooly-agent scheduler status --config <path>` for scheduler state.")
		return nil
	case "doctor":
		return doctorCommand(args[1:])
	case "collectors":
		return collectorsCommand(args[1:])
	case "rules":
		return rulesCommand(args[1:])
	case "incidents":
		return incidentsCommand(args[1:])
	case "notifications":
		return notificationsCommand(args[1:])
	case "api":
		return apiCommand(args[1:])
	case "reports":
		return reportsCommand(args[1:])
	case "scheduler":
		return schedulerCommand(args[1:])
	default:
		return fmt.Errorf("unknown pooly-agent command %q", args[0])
	}
}

func schedulerCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pooly-agent scheduler <status|run-once> --config <path>")
	}
	switch args[0] {
	case "status":
		return schedulerStatusCommand(args[1:])
	case "run-once":
		return schedulerRunOnceCommand(args[1:])
	default:
		return fmt.Errorf("unknown scheduler command %q", args[0])
	}
}

func schedulerStatusCommand(args []string) error {
	configPath, err := parseConfigFlag(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	dbPath := filepath.Join(cfg.Storage.StateDir, cfg.Storage.DatabaseFile)
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			printSchedulerStatus(schedulerConfiguredStatus(cfg))
			return nil
		}
		return err
	}
	store, err := openConfiguredStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()
	status, ok, err := agent.LoadPersistedSchedulerStatus(ctx, store)
	if err != nil {
		return err
	}
	if !ok {
		status = schedulerConfiguredStatus(cfg)
	}
	printSchedulerStatus(status)
	return nil
}

func schedulerRunOnceCommand(args []string) error {
	configPath, jsonOutput, persist, err := parseSchedulerRunOnceFlags(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	store, cleanup, err := openSchedulerRunOnceStore(ctx, cfg, persist)
	if err != nil {
		return err
	}
	defer cleanup()
	scheduler, err := schedulerFromConfig(cfg, store, persist, !persist, nil)
	if err != nil {
		return err
	}
	result, runErr := scheduler.RunOnce(ctx)
	if jsonOutput {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("scheduler run-once observations=%d rule_results=%d transitions=%d delivered=%d failed=%d status_error=%s summary=%s\n",
			len(result.Observations), len(result.Evaluation.Results), len(result.Evaluation.Transitions),
			result.NotificationReport.Delivered, result.NotificationReport.Failed,
			redaction.Redact(result.Status.LastSafeErrorClass), redaction.Redact(result.Status.LastSafeErrorSummary))
	}
	return runErr
}

func apiCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pooly-agent api check --config <path>")
	}
	switch args[0] {
	case "check":
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
		server, err := api.NewServer(apiOptionsFromConfig(cfg, nil, nil))
		if err != nil {
			return err
		}
		fmt.Printf("api OK: enabled=%t listen=%s reports=%t\n", cfg.API.Enabled, config.EffectiveAPIListen(cfg.API), server != nil && cfg.Reports.Enabled)
		return nil
	default:
		return fmt.Errorf("unknown api command %q", args[0])
	}
}

func reportsCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pooly-agent reports preview --config <path> [--json]")
	}
	switch args[0] {
	case "preview":
		return reportsPreviewCommand(args[1:])
	default:
		return fmt.Errorf("unknown reports command %q", args[0])
	}
}

func reportsPreviewCommand(args []string) error {
	configPath, jsonOutput, err := parseReportsPreviewFlags(args)
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
	summary, err := reports.Generate(ctx, store, reportsOptionsFromConfig(cfg, nil))
	if err != nil {
		return err
	}
	if jsonOutput {
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("report generated=%s storage=%t schema=%d open_incidents=%d resolved_incidents=%d deliveries=%d\n",
		summary.GeneratedAt.Format(time.RFC3339), summary.StorageAvailable, summary.SchemaVersion,
		summary.IncidentStatusCounts["open"], summary.IncidentStatusCounts["resolved"], sumCounts(summary.NotificationDeliveryCounts))
	for severity, count := range summary.OpenIncidentsBySeverity {
		fmt.Printf("open severity=%s count=%d\n", redaction.Redact(severity), count)
	}
	return nil
}

func notificationsCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pooly-agent notifications <validate|test|send|deliveries> --config <path>")
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
		fmt.Printf("notifications OK: enabled=%t dry_run=%t receivers=%d\n", cfg.Notify.Enabled, cfg.Notify.DryRun, len(cfg.Notify.Receivers))
		return nil
	case "test":
		return notificationsTestCommand(args[1:])
	case "send":
		return notificationsSendCommand(args[1:])
	case "deliveries":
		return notificationsDeliveriesCommand(args[1:])
	default:
		return fmt.Errorf("unknown notifications command %q", args[0])
	}
}

func notificationsTestCommand(args []string) error {
	configPath, receiverID, jsonOutput, send, err := parseNotificationsTestFlags(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadFile(ctx, configPath)
	if err != nil {
		return err
	}
	dryRun := !send
	service, err := notificationServiceFromConfig(cfg, nil, receiverID, dryRun, true)
	if err != nil {
		return err
	}
	incident := notificationPreviewIncident()
	if dryRun {
		report, err := service.DeliverIncident(ctx, incident, notify.EventOpened)
		if err != nil {
			return err
		}
		return printNotificationReport(report, jsonOutput)
	}
	payload := notify.RenderPayload(incident, notify.EventOpened)
	var report notify.Report
	for _, receiver := range service.Receivers {
		outcome := receiver.Deliver(ctx, payload)
		result := notify.DeliveryResult{IncidentID: incident.ID, ReceiverID: receiver.ID(), Event: notify.EventOpened}
		if outcome.Success {
			result.Status = notify.StatusDelivered
			result.Summary = redaction.Redact(outcome.Summary)
		} else {
			result.Status = notify.StatusFailed
			result.ErrorClass = redaction.Redact(outcome.ErrorClass)
			result.Summary = redaction.Redact(outcome.Summary)
		}
		report.Results = append(report.Results, result)
		if result.Status == notify.StatusDelivered {
			report.Delivered++
		} else {
			report.Failed++
		}
	}
	if err := printNotificationReport(report, jsonOutput); err != nil {
		return err
	}
	if notify.Failed(report) {
		return notify.ErrDeliveryFailed
	}
	return nil
}

func notificationsSendCommand(args []string) error {
	configPath, incidentID, jsonOutput, dryRun, err := parseNotificationsSendFlags(args)
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
	incident, err := store.GetIncident(ctx, incidentID)
	if err != nil {
		return err
	}
	effectiveDryRun := dryRun || cfg.Notify.DryRun
	service, err := notificationServiceFromConfig(cfg, store, "", effectiveDryRun, false)
	if err != nil {
		return err
	}
	report, err := service.DeliverIncident(ctx, incident, notify.EventFromIncident(incident))
	if err != nil {
		return err
	}
	if err := printNotificationReport(report, jsonOutput); err != nil {
		return err
	}
	if notify.Failed(report) {
		return notify.ErrDeliveryFailed
	}
	return nil
}

func notificationsDeliveriesCommand(args []string) error {
	configPath, incidentID, err := parseNotificationsDeliveriesFlags(args)
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
	deliveries, err := store.ListNotificationDeliveries(ctx, incidentID)
	if err != nil {
		return err
	}
	if len(deliveries) == 0 {
		fmt.Println("no notification deliveries recorded")
		return nil
	}
	for _, delivery := range deliveries {
		deliveredAt := ""
		if delivery.DeliveredAt != nil {
			deliveredAt = delivery.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
		}
		fmt.Printf("%s receiver=%s status=%s attempt=%d attempted=%s delivered=%s error=%s summary=%s\n",
			delivery.ID, delivery.Receiver, delivery.Status, delivery.Attempt,
			delivery.AttemptedAt.Format("2006-01-02T15:04:05Z07:00"), deliveredAt,
			redaction.Redact(delivery.ErrorClass), redaction.Redact(delivery.ErrorSummary))
	}
	return nil
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
	store, err := openConfiguredStore(ctx, cfg)
	if err != nil {
		return err
	}
	scheduler, err := schedulerFromConfig(cfg, store, true, cfg.Notify.DryRun, logger)
	if err != nil {
		_ = store.Close()
		return err
	}
	var apiServer *api.Server
	if cfg.API.Enabled {
		apiServer, err = api.NewServer(apiOptionsFromConfig(cfg, store, scheduler.Status))
		if err != nil {
			_ = store.Close()
			return err
		}
	}
	var notifier agent.Notifier
	var notifyClient systemdnotify.Client
	watchdogEnabled := false
	watchdogInterval := cfg.Systemd.WatchdogInterval.Duration
	if cfg.Systemd.Notify {
		notifyClient = systemdnotify.NewFromEnv(os.Getenv)
		notifier = notifyClient
		watchdogEnabled = cfg.Systemd.Watchdog && notifyClient.WatchdogEnabled()
		if watchdogEnabled {
			watchdogInterval = notifyClient.WatchdogInterval(cfg.Systemd.WatchdogInterval.Duration)
		}
	}
	if cfg.Agent.Scheduler.Enabled {
		fmt.Println("Pooly Sentinel run active. Scheduler enabled; remediation and report delivery are not implemented. Press Ctrl+C to exit.")
	} else {
		fmt.Println("Pooly Sentinel run infrastructure active. Scheduler disabled by config. Press Ctrl+C to exit.")
	}
	return agent.RunInfrastructure(ctx, agent.RuntimeOptions{
		Logger:           logger,
		Store:            store,
		API:              apiServer,
		Scheduler:        scheduler,
		Notifier:         notifier,
		ShutdownTimeout:  cfg.API.ShutdownTimeout.Duration,
		WatchdogEnabled:  watchdogEnabled,
		WatchdogInterval: watchdogInterval,
		RunWatchdog: func(watchdogCtx context.Context) {
			systemdnotify.RunWatchdog(watchdogCtx, notifyClient, watchdogInterval, logger)
		},
	})
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
	fmt.Println("Pooly Sentinel doctor: safe alpha checks only. Scheduler, collectors, and notifications are not run.")
	checks := storage.RunDoctor(ctx, storage.DoctorOptions{
		StateDir:           cfg.Storage.StateDir,
		LogDir:             cfg.Storage.LogDir,
		DatabaseFile:       cfg.Storage.DatabaseFile,
		CurrentMetricsFile: cfg.Storage.CurrentMetricsFile,
		BusyTimeout:        cfg.Storage.SQLite.BusyTimeout.Duration,
		WAL:                cfg.Storage.SQLite.WAL,
	})
	checks = append(checks, alphaDoctorChecks(ctx, cfg, configPath)...)
	for _, check := range checks {
		fmt.Printf("%s %s: %s\n", check.Status, check.Name, redaction.Redact(check.Message))
	}
	if storage.DoctorFailed(checks) {
		return fmt.Errorf("storage doctor failed")
	}
	fmt.Printf("PASS storage database: %s\n", redaction.Redact(filepath.Join(cfg.Storage.StateDir, cfg.Storage.DatabaseFile)))
	return nil
}

func alphaDoctorChecks(ctx context.Context, cfg config.Config, configPath string) []storage.DoctorCheck {
	var checks []storage.DoctorCheck
	add := func(name string, status storage.DoctorStatus, message string) {
		checks = append(checks, storage.DoctorCheck{Name: name, Status: status, Message: message})
	}
	if err := ctx.Err(); err != nil {
		add("context", storage.DoctorFail, err.Error())
		return checks
	}
	add("config validation", storage.DoctorPass, "configuration loaded and validated")
	if server, err := api.NewServer(apiOptionsFromConfig(cfg, nil, nil)); err != nil {
		add("api configuration", storage.DoctorFail, err.Error())
	} else {
		_ = server
		add("api configuration", storage.DoctorPass, "api configuration is valid")
	}
	if _, err := rules.FromConfig(cfg); err != nil {
		add("rules configuration", storage.DoctorFail, err.Error())
	} else {
		add("rules configuration", storage.DoctorPass, fmt.Sprintf("%d rules configured", len(cfg.Rules)))
	}
	if _, err := notify.OptionsFromConfig(cfg, os.LookupEnv); err != nil {
		add("notification configuration", storage.DoctorFail, err.Error())
	} else {
		add("notification configuration", storage.DoctorPass, fmt.Sprintf("enabled=%t dry_run=%t receivers=%d", cfg.Notify.Enabled, cfg.Notify.DryRun, len(cfg.Notify.Receivers)))
	}
	add("scheduler configuration", storage.DoctorPass, fmt.Sprintf("enabled=%t interval=%s", cfg.Agent.Scheduler.Enabled, cfg.Agent.Scheduler.Interval.Duration))
	if os.Getenv("NOTIFY_SOCKET") == "" {
		add("systemd notify environment", storage.DoctorWarn, "NOTIFY_SOCKET is not set; systemd readiness will be a no-op outside systemd")
	} else {
		client := systemdnotify.NewFromEnv(os.Getenv)
		if client.Socket == "" {
			add("systemd notify environment", storage.DoctorWarn, "systemd notify socket is unavailable")
		} else {
			add("systemd notify environment", storage.DoctorPass, "systemd notify socket is configured")
		}
	}
	if exe, err := os.Executable(); err != nil {
		add("executable path", storage.DoctorWarn, err.Error())
	} else {
		add("executable path", storage.DoctorPass, exe)
	}
	if info, err := os.Stat(configPath); err != nil {
		add("config permissions", storage.DoctorWarn, err.Error())
	} else {
		mode := info.Mode().Perm()
		if mode&0o022 != 0 {
			add("config permissions", storage.DoctorWarn, fmt.Sprintf("config mode %04o is writable by group or others", mode))
		} else if mode&0o004 != 0 {
			add("config permissions", storage.DoctorWarn, fmt.Sprintf("config mode %04o is world-readable; installed configs should normally be 0640", mode))
		} else {
			add("config permissions", storage.DoctorPass, fmt.Sprintf("config mode %04o", mode))
		}
	}
	return checks
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
		Enabled:       stream.Enabled,
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

func apiOptionsFromConfig(cfg config.Config, store api.Store, schedulerStatus func() agent.SchedulerStatus) api.Options {
	return api.Options{
		Enabled:          cfg.API.Enabled,
		Listen:           config.EffectiveAPIListen(cfg.API),
		AllowNonLoopback: cfg.API.AllowNonLoopback,
		ReadTimeout:      cfg.API.ReadTimeout.Duration,
		WriteTimeout:     cfg.API.WriteTimeout.Duration,
		ShutdownTimeout:  cfg.API.ShutdownTimeout.Duration,
		Store:            store,
		Reports:          reportsOptionsFromConfig(cfg, schedulerStatus),
		SchedulerStatus:  schedulerStatus,
	}
}

func reportsOptionsFromConfig(cfg config.Config, schedulerStatus func() agent.SchedulerStatus) reports.Options {
	return reports.Options{
		Enabled:         cfg.Reports.Enabled,
		MaxIncidents:    cfg.Reports.MaxIncidents,
		IncludeResolved: cfg.Reports.IncludeResolved,
		SchedulerStatus: schedulerStatus,
	}
}

func schedulerFromConfig(cfg config.Config, store *storage.Store, persist bool, dryRun bool, logger *slog.Logger) (*agent.Scheduler, error) {
	loadedRules, err := rules.FromConfig(cfg)
	if err != nil {
		return nil, err
	}
	notifyService, err := notificationServiceFromConfig(cfg, store, "", dryRun || cfg.Notify.DryRun, false)
	if err != nil {
		return nil, err
	}
	schedulerOpts := agent.SchedulerOptions{
		Enabled:                cfg.Agent.Scheduler.Enabled,
		Interval:               cfg.Agent.Scheduler.Interval.Duration,
		RunOnStart:             cfg.Agent.Scheduler.RunOnStart,
		CycleTimeout:           cfg.Agent.Scheduler.CycleTimeout.Duration,
		MaxConsecutiveFailures: cfg.Agent.Scheduler.MaxConsecutiveFailures,
		Logger:                 logger,
		StatusStore:            agent.MetadataStatusStore{Store: store},
	}
	schedulerOpts.Collector = agent.CollectorFunc(func(ctx context.Context) ([]resources.Observation, error) {
		return collectSchedulerObservations(ctx, cfg, persist, store), nil
	})
	schedulerOpts.Evaluator = agent.EvaluatorFunc(func(ctx context.Context, observations []resources.Observation) (rules.Evaluation, error) {
		engine := rules.Engine{Rules: loadedRules, NodeID: cfg.Node.ID}
		return engine.Evaluate(ctx, store, observations)
	})
	schedulerOpts.Notifier = agent.TransitionNotifierFunc(notifyService.DeliverTransitions)
	return agent.NewScheduler(schedulerOpts), nil
}

func collectSchedulerObservations(ctx context.Context, cfg config.Config, persist bool, store *storage.Store) []resources.Observation {
	var observations []resources.Observation
	if cfg.Resources.Enabled {
		observations = append(observations, resources.Collect(ctx, resourceOptionsFromConfig(cfg, persist, store))...)
	}
	if cfg.Systemd.Enabled {
		observations = append(observations, systemd.Collect(ctx, systemdOptionsFromConfig(cfg))...)
	}
	if cfg.Journal.Auth.Enabled || cfg.Journal.Services.Enabled || cfg.Journal.Kernel.Enabled {
		observations = append(observations, journal.Collect(ctx, journalOptionsFromConfig(cfg, persist, store))...)
	}
	if cfg.SSH.Enabled {
		observations = append(observations, ssh.Collect(ctx, sshOptionsFromConfig(cfg))...)
	}
	if cfg.Filewatch.Enabled {
		observations = append(observations, filewatch.Collect(ctx, filewatchOptionsFromConfig(cfg, persist, store))...)
	}
	return observations
}

func schedulerConfiguredStatus(cfg config.Config) agent.SchedulerStatus {
	return agent.SchedulerStatus{
		Enabled:                cfg.Agent.Scheduler.Enabled,
		Running:                false,
		Interval:               cfg.Agent.Scheduler.Interval.Duration.String(),
		MaxConsecutiveFailures: cfg.Agent.Scheduler.MaxConsecutiveFailures,
	}
}

func printSchedulerStatus(status agent.SchedulerStatus) {
	fmt.Printf("scheduler enabled=%t running=%t interval=%s last_attempt=%s last_success=%s cycles=%d failed=%d active=%t error=%s summary=%s\n",
		status.Enabled, status.Running, status.Interval, formatOptionalTime(status.LastAttemptAt), formatOptionalTime(status.LastSuccessfulCycleAt),
		status.CycleCount, status.FailedCycleCount, status.CurrentlyRunningCycle,
		redaction.Redact(status.LastSafeErrorClass), redaction.Redact(status.LastSafeErrorSummary))
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func notificationServiceFromConfig(cfg config.Config, store notify.Store, receiverID string, dryRunOverride bool, allowDisabled bool) (notify.Service, error) {
	cfg.Notify.DryRun = dryRunOverride
	opts, err := notify.OptionsFromConfig(cfg, os.LookupEnv)
	if err != nil {
		return notify.Service{}, err
	}
	filtered := opts.Receivers[:0]
	for _, receiver := range opts.Receivers {
		if receiverID != "" && receiver.ID != receiverID {
			continue
		}
		if allowDisabled && opts.DryRun && receiverID != "" {
			receiver.Enabled = true
		}
		filtered = append(filtered, receiver)
	}
	if receiverID != "" && len(filtered) == 0 {
		return notify.Service{}, fmt.Errorf("notification receiver %q was not found", receiverID)
	}
	opts.Receivers = filtered
	receivers, err := notify.BuildReceivers(opts.Receivers)
	if err != nil {
		return notify.Service{}, err
	}
	return notify.NewService(opts, store, receivers), nil
}

func notificationPreviewIncident() storage.IncidentRecord {
	return storage.IncidentRecord{
		ID:              "preview-notification",
		Fingerprint:     "preview:notification:diagnostic:opened",
		NodeID:          "preview-node",
		Type:            "diagnostic",
		Target:          "notification-test",
		Condition:       "manual-test",
		Severity:        "warning",
		Status:          "open",
		Summary:         "safe notification test payload",
		FirstSeen:       fixedPreviewTime(),
		LastSeen:        fixedPreviewTime(),
		OccurrenceCount: 1,
		LastTransition:  ptrTime(fixedPreviewTime()),
	}
}

func fixedPreviewTime() time.Time {
	return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func printNotificationReport(report notify.Report, jsonOutput bool) error {
	if jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	for _, result := range report.Results {
		fmt.Printf("%s receiver=%s event=%s status=%s attempt=%d summary=%s\n",
			result.IncidentID, result.ReceiverID, result.Event, result.Status, result.Attempt, redaction.Redact(result.Summary))
	}
	fmt.Printf("notification results: delivered=%d failed=%d skipped=%d dry_run=%d\n", report.Delivered, report.Failed, report.Skipped, report.DryRun)
	return nil
}

func sumCounts(counts map[string]int64) int64 {
	var total int64
	for _, count := range counts {
		total += count
	}
	return total
}

func parseNotificationsTestFlags(args []string) (configPath string, receiverID string, jsonOutput bool, send bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", false, false, fmt.Errorf("--config requires a path")
			}
			configPath = args[i+1]
			i++
		case "--receiver":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", false, false, fmt.Errorf("--receiver requires an id")
			}
			receiverID = args[i+1]
			i++
		case "--json":
			jsonOutput = true
		case "--send":
			send = true
		case "--dry-run":
			send = false
		default:
			return "", "", false, false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", "", false, false, fmt.Errorf("--config <path> is required")
	}
	if receiverID == "" {
		return "", "", false, false, fmt.Errorf("--receiver <id> is required")
	}
	return configPath, receiverID, jsonOutput, send, nil
}

func parseNotificationsSendFlags(args []string) (configPath string, incidentID string, jsonOutput bool, dryRun bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", false, false, fmt.Errorf("--config requires a path")
			}
			configPath = args[i+1]
			i++
		case "--incident":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", false, false, fmt.Errorf("--incident requires an id")
			}
			incidentID = args[i+1]
			i++
		case "--json":
			jsonOutput = true
		case "--dry-run":
			dryRun = true
		default:
			return "", "", false, false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", "", false, false, fmt.Errorf("--config <path> is required")
	}
	if incidentID == "" {
		return "", "", false, false, fmt.Errorf("--incident <id> is required")
	}
	return configPath, incidentID, jsonOutput, dryRun, nil
}

func parseNotificationsDeliveriesFlags(args []string) (configPath string, incidentID string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", fmt.Errorf("--config requires a path")
			}
			configPath = args[i+1]
			i++
		case "--incident":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", "", fmt.Errorf("--incident requires an id")
			}
			incidentID = args[i+1]
			i++
		default:
			return "", "", fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", "", fmt.Errorf("--config <path> is required")
	}
	if incidentID == "" {
		return "", "", fmt.Errorf("--incident <id> is required")
	}
	return configPath, incidentID, nil
}

func parseReportsPreviewFlags(args []string) (configPath string, jsonOutput bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) || args[i+1] == "" {
				return "", false, fmt.Errorf("--config requires a path")
			}
			if configPath != "" {
				return "", false, fmt.Errorf("--config was provided more than once")
			}
			configPath = args[i+1]
			i++
		case "--json":
			jsonOutput = true
		default:
			return "", false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if configPath == "" {
		return "", false, fmt.Errorf("--config <path> is required")
	}
	return configPath, jsonOutput, nil
}

func parseSchedulerRunOnceFlags(args []string) (configPath string, jsonOutput bool, persist bool, err error) {
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

func openSchedulerRunOnceStore(ctx context.Context, cfg config.Config, persist bool) (*storage.Store, func(), error) {
	if persist {
		store, err := openConfiguredStore(ctx, cfg)
		return store, func() {
			if store != nil {
				_ = store.Close()
			}
		}, err
	}
	dir, err := os.MkdirTemp("", "pooly-scheduler-run-once-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	store, err := storage.Open(ctx, storage.SQLiteOptions{
		Path:             filepath.Join(dir, "scheduler-run-once.db"),
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
  pooly-agent notifications validate --config <path>
  pooly-agent notifications test --config <path> --receiver <id> [--json] [--dry-run|--send]
  pooly-agent notifications send --config <path> --incident <id> [--json] [--dry-run]
  pooly-agent notifications deliveries --config <path> --incident <id>
  pooly-agent api check --config <path>
  pooly-agent reports preview --config <path> [--json]
  pooly-agent scheduler status --config <path>
  pooly-agent scheduler run-once --config <path> [--json] [--dry-run|--persist]

Task status:
  Core foundation, storage foundation, one-shot Linux collectors, rule evaluation,
  incident lifecycle persistence, single-cycle notification delivery, localhost API,
  report preview, systemd readiness wiring, and disabled-by-default scheduler are present.
  Remediation, updater, dashboard, public API, and report delivery are not implemented.
`)
}
