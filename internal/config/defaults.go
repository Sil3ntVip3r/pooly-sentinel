package config

import "time"

func Default() Config {
	return Config{
		Version: CurrentConfigVersion,
		Node: NodeConfig{
			Environment: "production",
			Ring:        "alpha",
		},
		API: APIConfig{
			Enabled: true,
			Bind:    DefaultAPIBind,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		Commands: CommandPaths{
			Systemctl:  "/bin/systemctl",
			Journalctl: "/bin/journalctl",
			SSHD:       "/usr/sbin/sshd",
			SS:         "/usr/sbin/ss",
			UFW:        "/usr/sbin/ufw",
			Visudo:     "/usr/sbin/visudo",
			Auditctl:   "/usr/sbin/auditctl",
			Ausearch:   "/usr/sbin/ausearch",
		},
		Resources: ResourcesConfig{
			Enabled:  true,
			Interval: Duration{Duration: 30 * time.Second},
			Timeout:  Duration{Duration: 3 * time.Second},
			CPU:      ResourceToggleConfig{Enabled: true},
			Memory:   ResourceToggleConfig{Enabled: true},
			Pressure: PressureConfig{Enabled: true, MissingIsOK: true},
			Filesystem: FilesystemConfig{
				Enabled: true,
				Mounts:  []string{"/", "/home", "/var", "/var/log", "/var/lib", "/var/lib/pooly-sentinel", "/var/log/pooly-sentinel"},
			},
			DiskIO: DiskIOConfig{
				Enabled:      true,
				AutoDiscover: true,
				Exclude:      []string{"loop*", "ram*", "fd*", "sr*"},
			},
			Network: NetworkConfig{
				Enabled:      true,
				AutoDiscover: true,
				Include:      []string{},
				Exclude:      []string{"lo", "docker*", "veth*", "br-*"},
			},
			Uptime: ResourceToggleConfig{Enabled: true},
		},
		Systemd: SystemdConfig{
			Enabled:  false,
			Interval: Duration{Duration: 30 * time.Second},
			CriticalServices: []string{
				"ssh.service",
				"fail2ban.service",
				"pooly-sentinel-agent.service",
				"miningcore.service",
			},
		},
		SSH: SSHConfig{
			Enabled:     false,
			Interval:    Duration{Duration: 5 * time.Minute},
			EventDriven: true,
			Expected: SSHExpectedConfig{
				Ports:                        []int{6200},
				ForbiddenPorts:               []int{22},
				PermitRootLogin:              "no",
				PasswordAuthentication:       "no",
				KbdInteractiveAuthentication: "no",
				PermitEmptyPasswords:         "no",
				PubkeyAuthentication:         "yes",
				StrictModes:                  "yes",
				PermitUserEnvironment:        "no",
			},
		},
		Journal: JournalConfig{
			Auth: TimedConfig{
				Enabled:  false,
				Interval: Duration{Duration: 10 * time.Second},
				Timeout:  Duration{Duration: 3 * time.Second},
			},
			Services: TimedConfig{
				Enabled:  false,
				Interval: Duration{Duration: 30 * time.Second},
				Timeout:  Duration{Duration: 3 * time.Second},
			},
			Kernel: TimedConfig{
				Enabled:  false,
				Interval: Duration{Duration: 60 * time.Second},
				Timeout:  Duration{Duration: 3 * time.Second},
			},
		},
		Filewatch: FilewatchConfig{
			Enabled:                false,
			Debounce:               Duration{Duration: 2 * time.Second},
			PeriodicVerifyInterval: Duration{Duration: 5 * time.Minute},
		},
		Audit: AuditConfig{
			Enabled:     false,
			Mode:        "observe_only",
			ManageRules: false,
		},
		Notification: NotificationConfig{
			PaidReceiversEnabledByDefault: false,
		},
		Receivers: []ReceiverConfig{
			{
				Name:      "local_file",
				Type:      "file",
				CostClass: "free_core",
				Enabled:   true,
			},
		},
		Storage: StorageConfig{
			StateDir:           DefaultStateDir,
			LogDir:             DefaultLogDir,
			DatabaseFile:       "state.db",
			CurrentMetricsFile: "metrics-current.json",
			SQLite: SQLiteConfig{
				BusyTimeout: Duration{Duration: 5 * time.Second},
				WAL:         true,
			},
		},
	}
}
