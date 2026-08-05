package config

import "time"

// Default returns a Config with default values.
func Default() Config {
	return Config{
		App: AppConfig{
			Mode:               ModeOneshot,
			LogLevel:           DefaultLogLevel,
			Concurrency:        DefaultConcurrency,
			ReminderMultiplier: DefaultReminderMultiplier,
		},
		HTTP: HTTPConfig{
			Timeout:             DefaultHTTPTimeout,
			UserAgent:           DefaultUserAgent,
			SkipTLSVerification: false,
		},
		Retry:    DefaultRetry(),
		Alerters: map[string]AlerterConfig{},
	}
}

// DefaultRetry returns the default retry behaviour. The checker shares it so
// its fallback cannot drift from the configuration defaults.
func DefaultRetry() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		InitialWait: 1 * time.Second,
		MaxWait:     10 * time.Second,
		Multiplier:  2.0,
	}
}
