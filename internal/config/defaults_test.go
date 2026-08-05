package config

import (
	"net/http"
	"testing"
)

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.App.Concurrency != DefaultConcurrency {
		t.Errorf("app.concurrency = %d, want %d", cfg.App.Concurrency, DefaultConcurrency)
	}
	if cfg.HTTP.Timeout != DefaultHTTPTimeout {
		t.Errorf("http.timeout = %s, want %s", cfg.HTTP.Timeout, DefaultHTTPTimeout)
	}

	target := cfg.Targets[0]
	if target.Method != DefaultTargetMethod {
		t.Errorf("target method = %q, want %q", target.Method, DefaultTargetMethod)
	}
	if target.Interval != DefaultTargetInterval {
		t.Errorf("target interval = %s, want %s", target.Interval, DefaultTargetInterval)
	}
	if target.ExpectedStatus != DefaultExpectedStatus {
		t.Errorf("target expected_status = %d, want %d", target.ExpectedStatus, DefaultExpectedStatus)
	}
	if target.Timeout != cfg.HTTP.Timeout {
		t.Errorf("target timeout = %s, want the http.timeout default %s", target.Timeout, cfg.HTTP.Timeout)
	}
}

// An alerter without an explicit timeout must not inherit "no timeout", which
// would let a hung alert block health checking.
func TestLoadDefaultsAlerterTimeout(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalConfig+`
[alerters.raha]
type = "telegram"
enabled = true
bot_token = "token"
chat_id = "-100"
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Alerters["raha"].Timeout; got != DefaultAlerterTimeout {
		t.Errorf("alerter timeout = %s, want %s", got, DefaultAlerterTimeout)
	}
}

func TestParseLogLevel(t *testing.T) {
	if _, ok := ParseLogLevel("DEBUG"); !ok {
		t.Error(`ParseLogLevel("DEBUG") should be case-insensitive`)
	}
	if _, ok := ParseLogLevel("trace"); ok {
		t.Error(`ParseLogLevel("trace") should not be recognised`)
	}
}

// The built-in defaults plus a single target must pass validation, otherwise
// the shipped defaults are unusable.
func TestDefaultIsValid(t *testing.T) {
	loaded, err := Load(writeConfig(t, targetsBlock))
	if err != nil {
		t.Fatalf("defaults are not self-consistent: %v", err)
	}

	if loaded.App.Mode != ModeOneshot {
		t.Errorf("default mode = %q, want %q", loaded.App.Mode, ModeOneshot)
	}
	if loaded.Retry != DefaultRetry() {
		t.Errorf("retry defaults = %+v, want %+v", loaded.Retry, DefaultRetry())
	}
	if loaded.Targets[0].ExpectedStatus != http.StatusOK {
		t.Errorf("expected_status = %d, want %d", loaded.Targets[0].ExpectedStatus, http.StatusOK)
	}
}
