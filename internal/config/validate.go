package config

import (
	"fmt"
	"net"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

var (
	unitNamePattern       = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+\.(?:service|socket|timer|target|mount|path|slice|scope)$`)
	safeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
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
	validateResources(&errs, c.Resources)
	validateDuration(&errs, "systemd.interval", c.Systemd.Interval.Duration)
	validateDuration(&errs, "systemd.timeout", c.Systemd.Timeout.Duration)
	if c.Systemd.Interval.Duration > 0 && c.Systemd.Timeout.Duration >= c.Systemd.Interval.Duration {
		errs.add("systemd.timeout", "must be less than systemd.interval")
	}
	validateStringListLimit(&errs, "systemd.critical_services", c.Systemd.CriticalServices, 64)
	for i, service := range c.Systemd.CriticalServices {
		field := fmt.Sprintf("systemd.critical_services[%d]", i)
		requireString(&errs, field, service)
		validateUnitName(&errs, field, service)
		validateNoSecretLiteral(&errs, field, service)
	}
	validateDuplicateStrings(&errs, "systemd.critical_services", c.Systemd.CriticalServices)
	validateDuration(&errs, "ssh.interval", c.SSH.Interval.Duration)
	validateDuration(&errs, "ssh.timeout", c.SSH.Timeout.Duration)
	if c.SSH.Interval.Duration > 0 && c.SSH.Timeout.Duration >= c.SSH.Interval.Duration {
		errs.add("ssh.timeout", "must be less than ssh.interval")
	}
	validatePorts(&errs, "ssh.expected.ports", c.SSH.Expected.Ports)
	validatePorts(&errs, "ssh.expected.forbidden_ports", c.SSH.Expected.ForbiddenPorts)
	validateYesNo(&errs, "ssh.expected.permitrootlogin", c.SSH.Expected.PermitRootLogin)
	validateYesNo(&errs, "ssh.expected.passwordauthentication", c.SSH.Expected.PasswordAuthentication)
	validateYesNo(&errs, "ssh.expected.kbdinteractiveauthentication", c.SSH.Expected.KbdInteractiveAuthentication)
	validateYesNo(&errs, "ssh.expected.permitemptypasswords", c.SSH.Expected.PermitEmptyPasswords)
	validateYesNo(&errs, "ssh.expected.pubkeyauthentication", c.SSH.Expected.PubkeyAuthentication)
	validateYesNo(&errs, "ssh.expected.strictmodes", c.SSH.Expected.StrictModes)
	validateYesNo(&errs, "ssh.expected.permituserenvironment", c.SSH.Expected.PermitUserEnvironment)

	validateJournalStream(&errs, "journal.auth", c.Journal.Auth)
	validateJournalStream(&errs, "journal.services", c.Journal.Services)
	validateJournalStream(&errs, "journal.kernel", c.Journal.Kernel)
	validateDuration(&errs, "filewatch.debounce", c.Filewatch.Debounce.Duration)
	validateDuration(&errs, "filewatch.periodic_verify_interval", c.Filewatch.PeriodicVerifyInterval.Duration)
	validateDuration(&errs, "filewatch.timeout", c.Filewatch.Timeout.Duration)
	validateFilewatch(&errs, c.Filewatch)

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

func validateJournalStream(errs *ValidationErrors, prefix string, cfg JournalStreamConfig) {
	validateDuration(errs, prefix+".interval", cfg.Interval.Duration)
	validateDuration(errs, prefix+".timeout", cfg.Timeout.Duration)
	if cfg.Interval.Duration > 0 && cfg.Timeout.Duration >= cfg.Interval.Duration {
		errs.add(prefix+".timeout", "must be less than "+prefix+".interval")
	}
	validateIntRange(errs, prefix+".max_records", cfg.MaxRecords, 1, 1000)
	validateInt64Range(errs, prefix+".max_bytes", cfg.MaxBytes, 4096, 4*1024*1024)
	validateIntRange(errs, prefix+".max_field_bytes", cfg.MaxFieldBytes, 64, 4096)
}

func validateResources(errs *ValidationErrors, cfg ResourcesConfig) {
	validateDuration(errs, "resources.interval", cfg.Interval.Duration)
	validateDuration(errs, "resources.timeout", cfg.Timeout.Duration)
	if cfg.Interval.Duration > 0 && cfg.Timeout.Duration >= cfg.Interval.Duration {
		errs.add("resources.timeout", "must be less than resources.interval")
	}
	validateStringListLimit(errs, "resources.filesystem.mounts", cfg.Filesystem.Mounts, 64)
	seenMounts := map[string]struct{}{}
	for i, mount := range cfg.Filesystem.Mounts {
		field := fmt.Sprintf("resources.filesystem.mounts[%d]", i)
		requireString(errs, field, mount)
		if mount != "" {
			if !filepath.IsAbs(mount) {
				errs.add(field, "must be an absolute path")
			}
			clean := filepath.Clean(mount)
			if _, ok := seenMounts[clean]; ok {
				errs.add(field, "duplicates another mount after normalization")
			}
			seenMounts[clean] = struct{}{}
		}
		validateNoSecretLiteral(errs, field, mount)
	}
	validateGlobList(errs, "resources.diskio.exclude", cfg.DiskIO.Exclude, 64)
	validateGlobList(errs, "resources.network.include", cfg.Network.Include, 64)
	validateGlobList(errs, "resources.network.exclude", cfg.Network.Exclude, 64)
	validateDuplicateStrings(errs, "resources.network.include", cfg.Network.Include)
	validateDuplicateStrings(errs, "resources.network.exclude", cfg.Network.Exclude)
}

func validateStringListLimit(errs *ValidationErrors, field string, values []string, limit int) {
	if len(values) > limit {
		errs.add(field, fmt.Sprintf("must contain no more than %d entries", limit))
	}
}

func validateGlobList(errs *ValidationErrors, field string, values []string, limit int) {
	validateStringListLimit(errs, field, values, limit)
	for i, value := range values {
		itemField := fmt.Sprintf("%s[%d]", field, i)
		requireString(errs, itemField, value)
		if value == "" {
			continue
		}
		if _, err := path.Match(value, "candidate"); err != nil {
			errs.add(itemField, "must be a valid glob pattern")
		}
		validateNoSecretLiteral(errs, itemField, value)
	}
}

func validateDuplicateStrings(errs *ValidationErrors, field string, values []string) {
	seen := map[string]struct{}{}
	for i, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			errs.add(fmt.Sprintf("%s[%d]", field, i), "duplicates another entry")
		}
		seen[value] = struct{}{}
	}
}

func validateFilewatch(errs *ValidationErrors, cfg FilewatchConfig) {
	if cfg.PeriodicVerifyInterval.Duration > 0 && cfg.Timeout.Duration >= cfg.PeriodicVerifyInterval.Duration {
		errs.add("filewatch.timeout", "must be less than filewatch.periodic_verify_interval")
	}
	validateInt64Range(errs, "filewatch.max_file_bytes", cfg.MaxFileBytes, 1, 128*1024*1024)
	validateIntRange(errs, "filewatch.max_directory_entries", cfg.MaxDirectoryEntries, 1, 10000)
	if len(cfg.Targets) > 128 {
		errs.add("filewatch.targets", "must contain no more than 128 entries")
	}
	seen := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	for i, target := range cfg.Targets {
		prefix := fmt.Sprintf("filewatch.targets[%d]", i)
		requireString(errs, prefix+".name", target.Name)
		requireString(errs, prefix+".path", target.Path)
		validateNoSecretLiteral(errs, prefix+".name", target.Name)
		validateNoSecretLiteral(errs, prefix+".path", target.Path)
		validateOneOf(errs, prefix+".type", target.Type, "file", "directory", "any")
		if target.Path != "" {
			if !filepath.IsAbs(target.Path) {
				errs.add(prefix+".path", "must be an absolute path")
			}
			clean := filepath.Clean(target.Path)
			if _, ok := seen[clean]; ok {
				errs.add(prefix+".path", "duplicates another target after normalization")
			}
			seen[clean] = struct{}{}
		}
		if target.Name != "" {
			if !safeIdentifierPattern.MatchString(target.Name) {
				errs.add(prefix+".name", "must contain only letters, numbers, dot, underscore, or dash")
			}
			if _, ok := seenNames[target.Name]; ok {
				errs.add(prefix+".name", "duplicates another target name")
			}
			seenNames[target.Name] = struct{}{}
		}
		if target.AllowPrivateKeyHash && !target.Hash {
			errs.add(prefix+".allow_private_key_hash", "requires hash to be true")
		}
	}
}

func validateUnitName(errs *ValidationErrors, field string, value string) {
	if value == "" {
		return
	}
	if !unitNamePattern.MatchString(value) {
		errs.add(field, "must be a safe systemd unit name")
	}
}

func validateIntRange(errs *ValidationErrors, field string, value int, min int, max int) {
	if value < min || value > max {
		errs.add(field, fmt.Sprintf("must be between %d and %d", min, max))
	}
}

func validateInt64Range(errs *ValidationErrors, field string, value int64, min int64, max int64) {
	if value < min || value > max {
		errs.add(field, fmt.Sprintf("must be between %d and %d", min, max))
	}
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
