package config

import (
	"fmt"
	"math"
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
	ruleMetricPattern     = regexp.MustCompile(`^pooly_[a-z0-9_]+$`)
	pressureMetricPattern = regexp.MustCompile(`^pooly_pressure_(cpu|memory|io)_(some|full)_(avg10|avg60|avg300|total_microseconds)$`)
	ruleTargetPattern     = regexp.MustCompile(`^[A-Za-z0-9_.@:/-]+$`)
)

var knownRuleMetrics = map[string]struct{}{
	"pooly_cpu_count":                                   {},
	"pooly_cpu_iowait_ratio":                            {},
	"pooly_cpu_load1":                                   {},
	"pooly_cpu_load15":                                  {},
	"pooly_cpu_load15_per_cpu":                          {},
	"pooly_cpu_load1_per_cpu":                           {},
	"pooly_cpu_load5":                                   {},
	"pooly_cpu_load5_per_cpu":                           {},
	"pooly_cpu_steal_ratio":                             {},
	"pooly_cpu_used_ratio":                              {},
	"pooly_disk_daily_read_bytes":                       {},
	"pooly_disk_daily_write_bytes":                      {},
	"pooly_disk_io_in_progress":                         {},
	"pooly_disk_io_time_seconds_total":                  {},
	"pooly_disk_read_bytes_total":                       {},
	"pooly_disk_read_time_seconds_total":                {},
	"pooly_disk_reads_total":                            {},
	"pooly_disk_weighted_io_time_seconds_total":         {},
	"pooly_disk_write_bytes_total":                      {},
	"pooly_disk_write_time_seconds_total":               {},
	"pooly_disk_writes_total":                           {},
	"pooly_filesystem_available_bytes":                  {},
	"pooly_filesystem_free_bytes":                       {},
	"pooly_filesystem_inodes_free":                      {},
	"pooly_filesystem_inodes_total":                     {},
	"pooly_filesystem_inodes_used_ratio":                {},
	"pooly_filesystem_readonly":                         {},
	"pooly_filesystem_size_bytes":                       {},
	"pooly_filesystem_used_bytes":                       {},
	"pooly_filesystem_used_ratio":                       {},
	"pooly_filewatch_manifest_complete":                 {},
	"pooly_filewatch_manifest_truncated":                {},
	"pooly_filewatch_target_changed":                    {},
	"pooly_filewatch_target_exists":                     {},
	"pooly_filewatch_target_oversized":                  {},
	"pooly_filewatch_target_size_bytes":                 {},
	"pooly_filewatch_target_symlink":                    {},
	"pooly_journal_events_total":                        {},
	"pooly_journal_truncated":                           {},
	"pooly_memory_available_bytes":                      {},
	"pooly_memory_available_ratio":                      {},
	"pooly_memory_buffers_bytes":                        {},
	"pooly_memory_cached_bytes":                         {},
	"pooly_memory_dirty_bytes":                          {},
	"pooly_memory_free_bytes":                           {},
	"pooly_memory_kernel_stack_bytes":                   {},
	"pooly_memory_page_tables_bytes":                    {},
	"pooly_memory_slab_bytes":                           {},
	"pooly_memory_sreclaimable_bytes":                   {},
	"pooly_memory_sunreclaim_bytes":                     {},
	"pooly_memory_total_bytes":                          {},
	"pooly_memory_used_ratio":                           {},
	"pooly_memory_writeback_bytes":                      {},
	"pooly_network_daily_receive_bytes":                 {},
	"pooly_network_daily_transmit_bytes":                {},
	"pooly_network_interface_carrier":                   {},
	"pooly_network_interface_mtu_bytes":                 {},
	"pooly_network_interface_up":                        {},
	"pooly_network_receive_bytes_total":                 {},
	"pooly_network_receive_dropped_total":               {},
	"pooly_network_receive_errors_total":                {},
	"pooly_network_receive_packets_total":               {},
	"pooly_network_transmit_bytes_total":                {},
	"pooly_network_transmit_dropped_total":              {},
	"pooly_network_transmit_errors_total":               {},
	"pooly_network_transmit_packets_total":              {},
	"pooly_ssh_directive_expected_match":                {},
	"pooly_ssh_expected_port_listening":                 {},
	"pooly_ssh_forbidden_port_listening":                {},
	"pooly_swap_free_bytes":                             {},
	"pooly_swap_total_bytes":                            {},
	"pooly_swap_used_ratio":                             {},
	"pooly_system_boot_id_changed":                      {},
	"pooly_system_boot_time_timestamp_seconds":          {},
	"pooly_system_uptime_seconds":                       {},
	"pooly_systemd_unit_activating":                     {},
	"pooly_systemd_unit_active":                         {},
	"pooly_systemd_unit_active_enter_monotonic_seconds": {},
	"pooly_systemd_unit_deactivating":                   {},
	"pooly_systemd_unit_exec_main_code":                 {},
	"pooly_systemd_unit_exec_main_status":               {},
	"pooly_systemd_unit_failed":                         {},
	"pooly_systemd_unit_main_pid":                       {},
	"pooly_systemd_unit_present":                        {},
	"pooly_systemd_unit_restart_count":                  {},
	"pooly_tasks_runnable":                              {},
	"pooly_tasks_total":                                 {},
}

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
	validateRules(&errs, c.Rules)
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

func validateRules(errs *ValidationErrors, rules []RuleConfig) {
	if len(rules) > 128 {
		errs.add("rules", "must contain no more than 128 entries")
	}
	seen := map[string]struct{}{}
	for i, rule := range rules {
		prefix := fmt.Sprintf("rules[%d]", i)
		requireString(errs, prefix+".id", rule.ID)
		validateNoSecretLiteral(errs, prefix+".id", rule.ID)
		if rule.ID != "" {
			if !safeIdentifierPattern.MatchString(rule.ID) {
				errs.add(prefix+".id", "must contain only letters, numbers, dot, underscore, or dash")
			}
			if _, ok := seen[rule.ID]; ok {
				errs.add(prefix+".id", "duplicates another rule id")
			}
			seen[rule.ID] = struct{}{}
		}
		requireString(errs, prefix+".collector", rule.Collector)
		validateOneOf(errs, prefix+".collector", rule.Collector,
			"resources", "cpu", "load", "memory", "pressure", "filesystem", "diskio", "network", "uptime",
			"systemd", "journal", "ssh", "ssh_effective_config", "ssh_listeners", "filewatch")
		validateNoSecretLiteral(errs, prefix+".collector", rule.Collector)
		if rule.Metric == "" && rule.EventCategory == "" {
			errs.add(prefix+".metric", "metric or event_category is required")
		}
		if rule.Metric != "" {
			if !ruleMetricPattern.MatchString(rule.Metric) {
				errs.add(prefix+".metric", "must be a supported pooly_ metric name")
			} else if !isKnownRuleMetric(rule.Metric) {
				errs.add(prefix+".metric", "is not emitted by the implemented collectors")
			}
			validateNoSecretLiteral(errs, prefix+".metric", rule.Metric)
		}
		if rule.EventCategory != "" {
			if !safeIdentifierPattern.MatchString(rule.EventCategory) {
				errs.add(prefix+".event_category", "must contain only letters, numbers, dot, underscore, or dash")
			}
			validateNoSecretLiteral(errs, prefix+".event_category", rule.EventCategory)
		}
		if rule.Target != "" && rule.Target != "any" {
			if len(rule.Target) > 128 {
				errs.add(prefix+".target", "must be 128 bytes or shorter")
			}
			if !ruleTargetPattern.MatchString(rule.Target) {
				errs.add(prefix+".target", "must use only safe target characters")
			}
			validateNoSecretLiteral(errs, prefix+".target", rule.Target)
		}
		validateNonNegativeDuration(errs, prefix+".recover_for", rule.RecoverFor.Duration)
		validateNoSecretLiteral(errs, prefix+".summary", rule.Summary)
		if len(rule.Summary) > 240 {
			errs.add(prefix+".summary", "must be 240 bytes or shorter")
		}
		validateRuleLabels(errs, prefix+".labels", rule.Labels)
		thresholds := 0
		if rule.Warn != nil {
			thresholds++
			validateRuleThreshold(errs, prefix+".warn", rule.Metric, *rule.Warn)
		}
		if rule.Fail != nil {
			thresholds++
			validateRuleThreshold(errs, prefix+".fail", rule.Metric, *rule.Fail)
		}
		if rule.Critical != nil {
			thresholds++
			validateRuleThreshold(errs, prefix+".critical", rule.Metric, *rule.Critical)
		}
		if thresholds == 0 {
			errs.add(prefix, "at least one warn, fail, or critical threshold is required")
		}
		validateRulePolicy(errs, prefix+".missing_data", rule.MissingData)
		validateRulePolicy(errs, prefix+".stale_data", rule.StaleData)
	}
}

func isKnownRuleMetric(metric string) bool {
	if _, ok := knownRuleMetrics[metric]; ok {
		return true
	}
	return pressureMetricPattern.MatchString(metric)
}

func validateRuleThreshold(errs *ValidationErrors, field string, metric string, threshold RuleThresholdConfig) {
	validateOneOf(errs, field+".operator", threshold.Operator,
		"greater_than", "greater_than_or_equal", "less_than", "less_than_or_equal",
		"equal", "not_equal", "boolean_true", "boolean_false", "state_match", "event_category_match")
	validateNonNegativeDuration(errs, field+".for", threshold.For.Duration)
	requiresNumeric := threshold.Operator == "greater_than" || threshold.Operator == "greater_than_or_equal" ||
		threshold.Operator == "less_than" || threshold.Operator == "less_than_or_equal"
	requiresString := threshold.Operator == "state_match" || threshold.Operator == "event_category_match"
	if requiresNumeric {
		value, ok := numericRuleValue(threshold.Value)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			errs.add(field+".value", "must be a finite number")
			return
		}
		if strings.Contains(metric, "_ratio") && (value < 0 || value > 1) {
			errs.add(field+".value", "ratio thresholds must be between 0 and 1")
		}
		if value < 0 && !strings.Contains(metric, "temperature") {
			errs.add(field+".value", "must not be negative for this metric")
		}
		return
	}
	if requiresString {
		value, ok := threshold.Value.(string)
		if !ok || strings.TrimSpace(value) == "" {
			errs.add(field+".value", "must be a non-empty string")
			return
		}
		validateNoSecretLiteral(errs, field+".value", value)
		if len(value) > 128 || !ruleTargetPattern.MatchString(value) {
			errs.add(field+".value", "must use only safe bounded characters")
		}
		return
	}
	if threshold.Operator == "boolean_true" || threshold.Operator == "boolean_false" {
		return
	}
	switch value := threshold.Value.(type) {
	case int, int64, float64, float32:
		number, ok := numericRuleValue(value)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			errs.add(field+".value", "must be finite")
		}
	case bool:
	case string:
		if strings.TrimSpace(value) == "" {
			errs.add(field+".value", "must not be empty")
		}
		validateNoSecretLiteral(errs, field+".value", value)
	default:
		errs.add(field+".value", "must be a number, boolean, or string")
	}
}

func numericRuleValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	default:
		return 0, false
	}
}

func validateNonNegativeDuration(errs *ValidationErrors, field string, d time.Duration) {
	if d < 0 {
		errs.add(field, "must not be negative")
	}
}

func validateRulePolicy(errs *ValidationErrors, field string, value string) {
	if value == "" {
		return
	}
	validateOneOf(errs, field, value, "ignore", "stale", "warn", "fail")
	validateNoSecretLiteral(errs, field, value)
}

func validateRuleLabels(errs *ValidationErrors, field string, labels map[string]string) {
	if len(labels) > 16 {
		errs.add(field, "must contain no more than 16 labels")
	}
	allowed := map[string]struct{}{
		"collector": {}, "cpu": {}, "mount": {}, "device": {}, "interface": {},
		"pressure_type": {}, "window": {}, "unit": {}, "stream": {}, "directive": {},
		"port": {}, "watch": {}, "event_category": {},
	}
	for key, value := range labels {
		if _, ok := allowed[key]; !ok {
			errs.add(field+"."+key, "is not an allowlisted rule label")
		}
		if strings.TrimSpace(value) == "" {
			errs.add(field+"."+key, "must not be empty")
		}
		if len(value) > 128 || strings.ContainsAny(value, "\n\r\t") {
			errs.add(field+"."+key, "must be bounded and single-line")
		}
		validateNoSecretLiteral(errs, field+"."+key, value)
	}
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
