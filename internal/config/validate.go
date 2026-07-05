package config

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type ValidationError struct {
	Field   string
	Message string
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "configuration validation failed"
	}
	parts := make([]string, 0, len(e))
	for _, item := range e {
		parts = append(parts, fmt.Sprintf("%s: %s", item.Field, item.Message))
	}
	return redaction.Redact("configuration validation failed: " + strings.Join(parts, "; "))
}

func (e *ValidationErrors) add(field, message string) {
	*e = append(*e, ValidationError{Field: field, Message: message})
}

func (c Config) Validate() error {
	var errs ValidationErrors

	if c.Version == "" {
		errs.add("version", "is required")
	} else if c.Version != CurrentConfigVersion {
		errs.add("version", "must match supported configuration version "+CurrentConfigVersion)
	}

	requireString(&errs, "node.id", c.Node.ID)
	requireString(&errs, "node.name", c.Node.Name)
	requireString(&errs, "node.hostname", c.Node.Hostname)
	requireString(&errs, "node.region", c.Node.Region)
	requireString(&errs, "node.role", c.Node.Role)
	requireString(&errs, "node.environment", c.Node.Environment)
	requireString(&errs, "node.ring", c.Node.Ring)
	validateNoSecretLiteral(&errs, "node.id", c.Node.ID)
	validateNoSecretLiteral(&errs, "node.name", c.Node.Name)
	validateNoSecretLiteral(&errs, "node.hostname", c.Node.Hostname)
	validateNoSecretLiteral(&errs, "node.region", c.Node.Region)
	validateNoSecretLiteral(&errs, "node.role", c.Node.Role)
	validateNoSecretLiteral(&errs, "node.environment", c.Node.Environment)
	validateNoSecretLiteral(&errs, "node.ring", c.Node.Ring)

	if c.API.Bind == "" {
		errs.add("api.bind", "is required")
	} else {
		validateLoopbackBind(&errs, "api.bind", c.API.Bind)
	}

	validateOneOf(&errs, "logging.format", c.Logging.Format, "text", "json")
	validateOneOf(&errs, "logging.level", c.Logging.Level, "debug", "info", "warn", "error")

	validateCommandPaths(&errs, c.Commands)
	validateTimedConfig(&errs, "resources", c.Resources)
	validateDuration(&errs, "systemd.interval", c.Systemd.Interval.Duration)
	for i, service := range c.Systemd.CriticalServices {
		field := fmt.Sprintf("systemd.critical_services[%d]", i)
		requireString(&errs, field, service)
		validateNoSecretLiteral(&errs, field, service)
	}
	validateDuration(&errs, "ssh.interval", c.SSH.Interval.Duration)
	validatePorts(&errs, "ssh.expected.ports", c.SSH.Expected.Ports)
	validatePorts(&errs, "ssh.expected.forbidden_ports", c.SSH.Expected.ForbiddenPorts)
	validateYesNo(&errs, "ssh.expected.permitrootlogin", c.SSH.Expected.PermitRootLogin)
	validateYesNo(&errs, "ssh.expected.passwordauthentication", c.SSH.Expected.PasswordAuthentication)
	validateYesNo(&errs, "ssh.expected.kbdinteractiveauthentication", c.SSH.Expected.KbdInteractiveAuthentication)
	validateYesNo(&errs, "ssh.expected.permitemptypasswords", c.SSH.Expected.PermitEmptyPasswords)
	validateYesNo(&errs, "ssh.expected.pubkeyauthentication", c.SSH.Expected.PubkeyAuthentication)
	validateYesNo(&errs, "ssh.expected.strictmodes", c.SSH.Expected.StrictModes)
	validateYesNo(&errs, "ssh.expected.permituserenvironment", c.SSH.Expected.PermitUserEnvironment)

	validateTimedConfig(&errs, "journal.auth", c.Journal.Auth)
	validateTimedConfig(&errs, "journal.services", c.Journal.Services)
	validateTimedConfig(&errs, "journal.kernel", c.Journal.Kernel)
	validateDuration(&errs, "filewatch.debounce", c.Filewatch.Debounce.Duration)
	validateDuration(&errs, "filewatch.periodic_verify_interval", c.Filewatch.PeriodicVerifyInterval.Duration)

	validateOneOf(&errs, "audit.mode", c.Audit.Mode, "observe_only")
	if c.Audit.ManageRules {
		errs.add("audit.manage_rules", "must be false during alpha")
	}
	if c.Notification.PaidReceiversEnabledByDefault {
		errs.add("notification.paid_receivers_enabled_by_default", "must be false")
	}

	validateReceivers(&errs, c.Receivers)
	validateAbsolutePath(&errs, "storage.state_dir", c.Storage.StateDir)
	validateAbsolutePath(&errs, "storage.log_dir", c.Storage.LogDir)
	validateFileName(&errs, "storage.database_file", c.Storage.DatabaseFile)
	validateFileName(&errs, "storage.current_metrics_file", c.Storage.CurrentMetricsFile)
	validateDuration(&errs, "storage.sqlite.busy_timeout", c.Storage.SQLite.BusyTimeout.Duration)

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func requireString(errs *ValidationErrors, field, value string) {
	if strings.TrimSpace(value) == "" {
		errs.add(field, "is required")
	}
}

func validateNoSecretLiteral(errs *ValidationErrors, field, value string) {
	if value == "" {
		return
	}
	if redaction.Redact(value) != value {
		errs.add(field, "must not contain a secret literal")
	}
}

func validateOneOf(errs *ValidationErrors, field, value string, allowed ...string) {
	if !slices.Contains(allowed, value) {
		errs.add(field, "must be one of "+strings.Join(allowed, ", "))
	}
}

func validateDuration(errs *ValidationErrors, field string, d time.Duration) {
	if d <= 0 {
		errs.add(field, "must be greater than zero")
	}
}

func validateTimedConfig(errs *ValidationErrors, prefix string, cfg TimedConfig) {
	validateDuration(errs, prefix+".interval", cfg.Interval.Duration)
	validateDuration(errs, prefix+".timeout", cfg.Timeout.Duration)
}

func validateLoopbackBind(errs *ValidationErrors, field, bind string) {
	host, port, err := net.SplitHostPort(bind)
	if err != nil {
		errs.add(field, "must be host:port")
		return
	}
	if port == "" {
		errs.add(field, "must include a port")
	}
	if host == "localhost" {
		return
	}
	ip := net.ParseIP(host)
	if ip == nil {
		errs.add(field, "host must be localhost or a loopback IP")
		return
	}
	if !ip.IsLoopback() {
		errs.add(field, "must bind to localhost or loopback only")
	}
}

func validateCommandPaths(errs *ValidationErrors, paths CommandPaths) {
	entries := map[string]string{
		"commands.systemctl":  paths.Systemctl,
		"commands.journalctl": paths.Journalctl,
		"commands.sshd":       paths.SSHD,
		"commands.ss":         paths.SS,
		"commands.ufw":        paths.UFW,
		"commands.visudo":     paths.Visudo,
		"commands.auditctl":   paths.Auditctl,
		"commands.ausearch":   paths.Ausearch,
	}
	for field, value := range entries {
		validateAbsolutePath(errs, field, value)
	}
}

func validateAbsolutePath(errs *ValidationErrors, field, value string) {
	requireString(errs, field, value)
	if value != "" && !filepath.IsAbs(value) {
		errs.add(field, "must be an absolute path")
	}
	validateNoSecretLiteral(errs, field, value)
}

func validateFileName(errs *ValidationErrors, field, value string) {
	requireString(errs, field, value)
	if value == "" {
		return
	}
	if filepath.IsAbs(value) || filepath.Base(value) != value || value == "." || value == ".." {
		errs.add(field, "must be a plain filename")
	}
	validateNoSecretLiteral(errs, field, value)
}

func validatePorts(errs *ValidationErrors, field string, ports []int) {
	if len(ports) == 0 {
		errs.add(field, "must include at least one port")
		return
	}
	for i, port := range ports {
		if port < 1 || port > 65535 {
			errs.add(fmt.Sprintf("%s[%d]", field, i), "must be between 1 and 65535")
		}
	}
}

func validateYesNo(errs *ValidationErrors, field, value string) {
	validateOneOf(errs, field, value, "yes", "no")
}

func validateReceivers(errs *ValidationErrors, receivers []ReceiverConfig) {
	seen := map[string]struct{}{}
	for i, receiver := range receivers {
		prefix := fmt.Sprintf("receivers[%d]", i)
		requireString(errs, prefix+".name", receiver.Name)
		requireString(errs, prefix+".type", receiver.Type)
		requireString(errs, prefix+".cost_class", receiver.CostClass)
		validateNoSecretLiteral(errs, prefix+".name", receiver.Name)
		if receiver.Name != "" {
			if _, ok := seen[receiver.Name]; ok {
				errs.add(prefix+".name", "must be unique")
			}
			seen[receiver.Name] = struct{}{}
		}

		validateOneOf(errs, prefix+".type", receiver.Type, "file", "discord", "gotify", "ntfy", "noop", "pushover", "twilio_sms", "aws_sns_sms")
		validateOneOf(errs, prefix+".cost_class", receiver.CostClass, "free_core", "free_self_hosted", "free_external", "paid_external")
		validateReceiverCostClass(errs, prefix, receiver)
		validateEnvField(errs, prefix+".webhook_env", receiver.WebhookEnv)
		validateEnvField(errs, prefix+".url_env", receiver.URLEnv)
		validateEnvField(errs, prefix+".token_env", receiver.TokenEnv)
		validateEnvField(errs, prefix+".api_key_env", receiver.APIKeyEnv)

		if receiver.Type == "discord" && receiver.Enabled && receiver.WebhookEnv == "" {
			errs.add(prefix+".webhook_env", "is required when an enabled Discord receiver is configured")
		}
		if receiver.CostClass == "paid_external" && receiver.Enabled {
			errs.add(prefix+".enabled", "paid receivers must be disabled by default")
		}
	}
}

func validateReceiverCostClass(errs *ValidationErrors, prefix string, receiver ReceiverConfig) {
	want := map[string]string{
		"file":        "free_core",
		"noop":        "free_core",
		"discord":     "free_external",
		"gotify":      "free_self_hosted",
		"ntfy":        "free_self_hosted",
		"pushover":    "paid_external",
		"twilio_sms":  "paid_external",
		"aws_sns_sms": "paid_external",
	}
	if expected, ok := want[receiver.Type]; ok && receiver.CostClass != expected {
		errs.add(prefix+".cost_class", "must be "+expected+" for receiver type "+receiver.Type)
	}
}

var envNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func validateEnvField(errs *ValidationErrors, field, value string) {
	if value == "" {
		return
	}
	if !envNamePattern.MatchString(value) {
		errs.add(field, "must name an environment variable, not contain a secret value")
	}
	validateNoSecretLiteral(errs, field, value)
}
