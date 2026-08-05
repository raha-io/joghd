package config

import "testing"

func TestProvideConfigExposesEverySection(t *testing.T) {
	sections, err := ProvideConfig(CLIParams{ConfigPath: writeConfig(t, minimalConfig)})
	if err != nil {
		t.Fatalf("ProvideConfig: %v", err)
	}

	if sections.App.Mode != "oneshot" {
		t.Errorf("app.mode = %q, want %q", sections.App.Mode, "oneshot")
	}
	if sections.HTTP.Timeout == 0 {
		t.Error("http.timeout is unset")
	}
	if sections.Retry.MaxAttempts == 0 {
		t.Error("retry.max_attempts is unset")
	}
	if len(sections.Targets) != 1 {
		t.Errorf("got %d targets, want 1", len(sections.Targets))
	}
	if sections.Alerters == nil {
		t.Error("alerters map is nil")
	}
}

func TestProvideConfigModeFlagOverridesFile(t *testing.T) {
	sections, err := ProvideConfig(CLIParams{ConfigPath: writeConfig(t, minimalConfig), Mode: "continuous"})
	if err != nil {
		t.Fatalf("ProvideConfig: %v", err)
	}
	if sections.App.Mode != "continuous" {
		t.Errorf("app.mode = %q, want %q", sections.App.Mode, "continuous")
	}
}
