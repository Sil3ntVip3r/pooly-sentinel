package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
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

Task status:
  Core configuration, storage foundation, redaction, logging, command runner, lifecycle, and CLI foundation only.
  Production monitoring is not implemented.
`)
}
