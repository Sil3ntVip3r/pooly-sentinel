package config

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"gopkg.in/yaml.v3"
)

func LoadFile(ctx context.Context, path string) (Config, error) {
	if ctx == nil {
		return Config{}, fmt.Errorf("config load context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Config{}, redaction.Error(err)
	}
	if path == "" {
		return Config{}, fmt.Errorf("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", redaction.Error(err))
	}
	if err := ctx.Err(); err != nil {
		return Config{}, redaction.Error(err)
	}
	return LoadBytes(ctx, data)
}

func LoadBytes(ctx context.Context, data []byte) (Config, error) {
	if ctx == nil {
		return Config{}, fmt.Errorf("config load context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Config{}, redaction.Error(err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Config{}, fmt.Errorf("config is empty")
	}

	cfg := Default()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", redaction.Error(err))
	}
	if err := ctx.Err(); err != nil {
		return Config{}, redaction.Error(err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
