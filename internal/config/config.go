package config

const CurrentConfigVersion = "1"

const (
	DefaultAPIBind  = "127.0.0.1:9587"
	DefaultStateDir = "/var/lib/pooly-sentinel"
	DefaultLogDir   = "/var/log/pooly-sentinel"
)

func EffectiveAPIListen(cfg APIConfig) string {
	if cfg.Bind != "" {
		return cfg.Bind
	}
	return cfg.Listen
}
