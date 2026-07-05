package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version      string             `yaml:"version"`
	Node         NodeConfig         `yaml:"node"`
	API          APIConfig          `yaml:"api"`
	Logging      LoggingConfig      `yaml:"logging"`
	Commands     CommandPaths       `yaml:"commands"`
	Resources    ResourcesConfig    `yaml:"resources"`
	Systemd      SystemdConfig      `yaml:"systemd"`
	SSH          SSHConfig          `yaml:"ssh"`
	Journal      JournalConfig      `yaml:"journal"`
	Filewatch    FilewatchConfig    `yaml:"filewatch"`
	Audit        AuditConfig        `yaml:"audit"`
	Notification NotificationConfig `yaml:"notification"`
	Receivers    []ReceiverConfig   `yaml:"receivers"`
	Storage      StorageConfig      `yaml:"storage"`
}

type NodeConfig struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Hostname    string `yaml:"hostname"`
	Region      string `yaml:"region"`
	Role        string `yaml:"role"`
	Environment string `yaml:"environment"`
	Ring        string `yaml:"ring"`
}

type APIConfig struct {
	Enabled bool   `yaml:"enabled"`
	Bind    string `yaml:"bind"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type CommandPaths struct {
	Systemctl  string `yaml:"systemctl"`
	Journalctl string `yaml:"journalctl"`
	SSHD       string `yaml:"sshd"`
	SS         string `yaml:"ss"`
	UFW        string `yaml:"ufw"`
	Visudo     string `yaml:"visudo"`
	Auditctl   string `yaml:"auditctl"`
	Ausearch   string `yaml:"ausearch"`
}

type TimedConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Interval Duration `yaml:"interval"`
	Timeout  Duration `yaml:"timeout"`
}

type ResourcesConfig struct {
	Enabled    bool                 `yaml:"enabled"`
	Interval   Duration             `yaml:"interval"`
	Timeout    Duration             `yaml:"timeout"`
	CPU        ResourceToggleConfig `yaml:"cpu"`
	Memory     ResourceToggleConfig `yaml:"memory"`
	Pressure   PressureConfig       `yaml:"pressure"`
	Filesystem FilesystemConfig     `yaml:"filesystem"`
	DiskIO     DiskIOConfig         `yaml:"diskio"`
	Network    NetworkConfig        `yaml:"network"`
	Uptime     ResourceToggleConfig `yaml:"uptime"`
}

type ResourceToggleConfig struct {
	Enabled bool `yaml:"enabled"`
}

type PressureConfig struct {
	Enabled     bool `yaml:"enabled"`
	MissingIsOK bool `yaml:"missing_is_ok"`
}

type FilesystemConfig struct {
	Enabled bool     `yaml:"enabled"`
	Mounts  []string `yaml:"mounts"`
}

type DiskIOConfig struct {
	Enabled      bool     `yaml:"enabled"`
	AutoDiscover bool     `yaml:"auto_discover"`
	Exclude      []string `yaml:"exclude"`
}

type NetworkConfig struct {
	Enabled      bool     `yaml:"enabled"`
	AutoDiscover bool     `yaml:"auto_discover"`
	Include      []string `yaml:"include"`
	Exclude      []string `yaml:"exclude"`
}

type SystemdConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Interval         Duration `yaml:"interval"`
	CriticalServices []string `yaml:"critical_services"`
}

type SSHConfig struct {
	Enabled     bool              `yaml:"enabled"`
	Interval    Duration          `yaml:"interval"`
	EventDriven bool              `yaml:"event_driven"`
	Expected    SSHExpectedConfig `yaml:"expected"`
}

type SSHExpectedConfig struct {
	Ports                        []int  `yaml:"ports"`
	ForbiddenPorts               []int  `yaml:"forbidden_ports"`
	PermitRootLogin              string `yaml:"permitrootlogin"`
	PasswordAuthentication       string `yaml:"passwordauthentication"`
	KbdInteractiveAuthentication string `yaml:"kbdinteractiveauthentication"`
	PermitEmptyPasswords         string `yaml:"permitemptypasswords"`
	PubkeyAuthentication         string `yaml:"pubkeyauthentication"`
	StrictModes                  string `yaml:"strictmodes"`
	PermitUserEnvironment        string `yaml:"permituserenvironment"`
}

type JournalConfig struct {
	Auth     TimedConfig `yaml:"auth"`
	Services TimedConfig `yaml:"services"`
	Kernel   TimedConfig `yaml:"kernel"`
}

type FilewatchConfig struct {
	Enabled                bool     `yaml:"enabled"`
	Debounce               Duration `yaml:"debounce"`
	PeriodicVerifyInterval Duration `yaml:"periodic_verify_interval"`
}

type AuditConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Mode        string `yaml:"mode"`
	ManageRules bool   `yaml:"manage_rules"`
}

type NotificationConfig struct {
	PaidReceiversEnabledByDefault bool `yaml:"paid_receivers_enabled_by_default"`
}

type ReceiverConfig struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	CostClass string `yaml:"cost_class"`
	Enabled   bool   `yaml:"enabled"`

	WebhookEnv string `yaml:"webhook_env,omitempty"`
	URLEnv     string `yaml:"url_env,omitempty"`
	TokenEnv   string `yaml:"token_env,omitempty"`
	APIKeyEnv  string `yaml:"api_key_env,omitempty"`
}

type StorageConfig struct {
	StateDir           string       `yaml:"state_dir"`
	LogDir             string       `yaml:"log_dir"`
	DatabaseFile       string       `yaml:"database_file"`
	CurrentMetricsFile string       `yaml:"current_metrics_file"`
	SQLite             SQLiteConfig `yaml:"sqlite"`
}

type SQLiteConfig struct {
	BusyTimeout Duration `yaml:"busy_timeout"`
	WAL         bool     `yaml:"wal"`
}

type Duration struct {
	time.Duration
}

func (d Duration) String() string {
	return d.Duration.String()
}

func (d Duration) MarshalYAML() (any, error) {
	if d.Duration == 0 {
		return "", nil
	}
	return d.Duration.String(), nil
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Value == "" {
			d.Duration = 0
			return nil
		}
		parsed, err := time.ParseDuration(value.Value)
		if err != nil {
			return fmt.Errorf("must be a Go duration such as 30s, 5m, or 1h")
		}
		d.Duration = parsed
		return nil
	default:
		return fmt.Errorf("must be a duration string")
	}
}
