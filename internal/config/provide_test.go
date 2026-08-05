package config

import "testing"

func TestProvideConfigExposesEverySection(t *testing.T) {
	sections, err := ProvideConfig(CLIParams{ConfigPath: writeConfig(t, minimalConfig)})
	if err != nil {
		t.Fatalf("ProvideConfig: %v", err)
	}

	if sections.App.Mode != ModeOneshot {
		t.Errorf("app.mode = %q, want %q", sections.App.Mode, ModeOneshot)
	}
	if sections.HTTP.Timeout != DefaultHTTPTimeout {
		t.Errorf("http.timeout = %s, want %s", sections.HTTP.Timeout, DefaultHTTPTimeout)
	}
	if sections.Retry != DefaultRetry() {
		t.Errorf("retry = %+v, want %+v", sections.Retry, DefaultRetry())
	}
	if len(sections.Targets) != 1 {
		t.Errorf("got %d targets, want 1", len(sections.Targets))
	}
	if sections.Alerters == nil {
		t.Error("alerters map is nil")
	}
}

// The -mode flag is applied after the file is validated, so it needs checking
// in its own right.
func TestProvideConfigModeFlag(t *testing.T) {
	path := writeConfig(t, minimalConfig)

	sections, err := ProvideConfig(CLIParams{ConfigPath: path, Mode: ModeContinuous})
	if err != nil {
		t.Fatalf("ProvideConfig: %v", err)
	}
	if sections.App.Mode != ModeContinuous {
		t.Errorf("app.mode = %q, want %q", sections.App.Mode, ModeContinuous)
	}

	if _, err := ProvideConfig(CLIParams{ConfigPath: path, Mode: "nonsense"}); err == nil {
		t.Error("expected an error for an invalid -mode value, got nil")
	}
}

func TestProvideConfigPropagatesLoadErrors(t *testing.T) {
	if _, err := ProvideConfig(CLIParams{ConfigPath: writeConfig(t, "[app]\nmode = \"oneshot\"\n")}); err == nil {
		t.Error("expected the underlying validation error to propagate, got nil")
	}
}
