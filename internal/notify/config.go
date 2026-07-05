package notify

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/config"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type Options struct {
	Enabled   bool
	DryRun    bool
	Receivers []ReceiverSpec
}

type EnvLookup func(string) (string, bool)

func OptionsFromConfig(cfg config.Config, env EnvLookup) (Options, error) {
	if env == nil {
		env = func(string) (string, bool) { return "", false }
	}
	opts := Options{
		Enabled: cfg.Notify.Enabled,
		DryRun:  cfg.Notify.DryRun,
	}
	for _, receiver := range cfg.Notify.Receivers {
		spec := ReceiverSpec{
			ID:                 receiver.ID,
			DisplayName:        receiver.DisplayName,
			Enabled:            receiver.Enabled,
			Type:               receiver.Type,
			Timeout:            receiver.Timeout.Duration,
			Events:             notifyEvents(receiver.Events),
			Severities:         notifySeverities(receiver.Severities),
			AllowInsecureLocal: receiver.AllowInsecureLocal,
			CostClass:          "free_core",
			URLConfigured:      receiver.URLEnv != "",
		}
		if spec.Timeout == 0 {
			spec.Timeout = 5 * time.Second
		}
		if spec.Type == "webhook" {
			spec.CostClass = "free_external"
			rawURL, ok := env(receiver.URLEnv)
			if receiver.Enabled && !cfg.Notify.DryRun && !ok {
				return Options{}, fmt.Errorf("notify receiver %s URL environment variable is not set", receiver.ID)
			}
			if rawURL != "" {
				parsed, err := validateWebhookURL(rawURL, receiver.AllowInsecureLocal)
				if err != nil {
					return Options{}, fmt.Errorf("notify receiver %s URL is invalid: %w", receiver.ID, err)
				}
				spec.URL = parsed
			}
		}
		opts.Receivers = append(opts.Receivers, spec)
	}
	return opts, nil
}

func BuildReceivers(specs []ReceiverSpec) ([]Receiver, error) {
	receivers := make([]Receiver, 0, len(specs))
	for _, spec := range specs {
		switch spec.Type {
		case "webhook":
			receivers = append(receivers, NewWebhookReceiver(spec, nil))
		case "noop":
			receivers = append(receivers, NewNoopReceiver(spec))
		default:
			return nil, fmt.Errorf("unsupported notify receiver type %q", redaction.Redact(spec.Type))
		}
	}
	return receivers, nil
}

func notifyEvents(values []string) []Event {
	if len(values) == 0 {
		return []Event{EventOpened, EventEscalated, EventResolved}
	}
	events := make([]Event, 0, len(values))
	for _, value := range values {
		events = append(events, Event(value))
	}
	return events
}

func notifySeverities(values []string) []incidents.Severity {
	if len(values) == 0 {
		return []incidents.Severity{incidents.SeverityWarning, incidents.SeverityFailure, incidents.SeverityCritical}
	}
	severities := make([]incidents.Severity, 0, len(values))
	for _, value := range values {
		severities = append(severities, incidents.Severity(value))
	}
	return severities
}

func validateWebhookURL(rawURL string, allowInsecureLocal bool) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("could not be parsed")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("host is required")
	}
	if parsed.Scheme != "https" {
		if parsed.Scheme == "http" && allowInsecureLocal && isLocalHost(parsed.Hostname()) {
			return rawURL, nil
		}
		return "", fmt.Errorf("must use HTTPS unless local testing is explicitly allowed")
	}
	return rawURL, nil
}

func isLocalHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
