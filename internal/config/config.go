package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/raha-io/joghd/internal/domain"
)

// Config holds all application configuration.
type Config struct {
	App      AppConfig      `koanf:"app"`
	HTTP     HTTPConfig     `koanf:"http"`
	Retry    RetryConfig    `koanf:"retry"`
	Alerters AlertersConfig `koanf:"alerters"`
	Targets  []domain.Target `koanf:"targets"`
}

// AppConfig holds application-level settings.
type AppConfig struct {
	Mode        string `koanf:"mode"`
	LogLevel    string `koanf:"log_level"`
	Concurrency int    `koanf:"concurrency"`
}

// HTTPConfig holds HTTP client settings.
type HTTPConfig struct {
	Timeout             time.Duration `koanf:"timeout"`
	UserAgent           string        `koanf:"user_agent"`
	SkipTLSVerification bool          `koanf:"skip_tls_verification"`
}

// RetryConfig holds retry behavior settings.
type RetryConfig struct {
	MaxAttempts int           `koanf:"max_attempts"`
	InitialWait time.Duration `koanf:"initial_wait"`
	MaxWait     time.Duration `koanf:"max_wait"`
	Multiplier  float64       `koanf:"multiplier"`
}

// AlertersConfig holds alerter configurations.
type AlertersConfig struct {
	Telegram TelegramConfig `koanf:"telegram"`
}

// TelegramConfig holds Telegram alerter settings.
type TelegramConfig struct {
	Enabled  bool   `koanf:"enabled"`
	BotToken string `koanf:"bot_token"`
	ChatID   string `koanf:"chat_id"`
}

// defaults returns a koanf instance with default values.
func defaults() *koanf.Koanf {
	k := koanf.New(".")

	_ = k.Set("app.mode", "oneshot")
	_ = k.Set("app.log_level", "info")
	_ = k.Set("app.concurrency", 10)

	_ = k.Set("http.timeout", "10s")
	_ = k.Set("http.user_agent", "Joghd/1.0")
	_ = k.Set("http.skip_tls_verification", false)

	_ = k.Set("retry.max_attempts", 3)
	_ = k.Set("retry.initial_wait", "1s")
	_ = k.Set("retry.max_wait", "10s")
	_ = k.Set("retry.multiplier", 2.0)

	_ = k.Set("alerters.telegram.enabled", false)

	return k
}

// Load loads configuration from file and environment variables.
func Load(configPath string) (*Config, error) {
	k := defaults()

	// Load from TOML file if path is provided
	if configPath != "" {
		if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	// Load from environment variables (JOGHD_ prefix)
	err := k.Load(env.Provider("JOGHD_", ".", func(s string) string {
		return strings.Replace(
			strings.ToLower(strings.TrimPrefix(s, "JOGHD_")),
			"_",
			".",
			-1,
		)
	}), nil)
	if err != nil {
		return nil, fmt.Errorf("loading env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	// Apply defaults to targets
	for i := range cfg.Targets {
		if cfg.Targets[i].Method == "" {
			cfg.Targets[i].Method = "GET"
		}
		if cfg.Targets[i].Timeout == 0 {
			cfg.Targets[i].Timeout = cfg.HTTP.Timeout
		}
		if cfg.Targets[i].Interval == 0 {
			cfg.Targets[i].Interval = 30 * time.Second
		}
		if cfg.Targets[i].ExpectedStatus == 0 {
			cfg.Targets[i].ExpectedStatus = 200
		}
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if cfg.App.Mode != "oneshot" && cfg.App.Mode != "continuous" {
		return fmt.Errorf("invalid app.mode: %s (must be 'oneshot' or 'continuous')", cfg.App.Mode)
	}

	if cfg.Alerters.Telegram.Enabled {
		if cfg.Alerters.Telegram.BotToken == "" {
			return fmt.Errorf("telegram.bot_token is required when telegram is enabled")
		}
		if cfg.Alerters.Telegram.ChatID == "" {
			return fmt.Errorf("telegram.chat_id is required when telegram is enabled")
		}
	}

	for i, t := range cfg.Targets {
		if t.URL == "" {
			return fmt.Errorf("target[%d]: url is required", i)
		}
		if t.Name == "" {
			return fmt.Errorf("target[%d]: name is required", i)
		}
	}

	return nil
}
