package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level: got %q, want %q", cfg.Log.Level, "warn")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format: got %q, want %q", cfg.Log.Format, "text")
	}
	if cfg.Cache.MaxAge != "8h" {
		t.Errorf("Cache.MaxAge: got %q, want %q", cfg.Cache.MaxAge, "8h")
	}
	if cfg.Timeouts.Secrets != "30s" {
		t.Errorf("Timeouts.Secrets: got %q, want %q", cfg.Timeouts.Secrets, "30s")
	}
	if !cfg.UI.Border {
		t.Error("UI.Border: got false, want true (default on)")
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// Should return defaults unchanged.
	want := Defaults()
	if cfg.Log.Level != want.Log.Level {
		t.Errorf("Log.Level: got %q, want %q", cfg.Log.Level, want.Log.Level)
	}
	if cfg.Cache.MaxAge != want.Cache.MaxAge {
		t.Errorf("Cache.MaxAge: got %q, want %q", cfg.Cache.MaxAge, want.Cache.MaxAge)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
log:
  level: debug
  format: json
cache:
  max_age: 2h
timeouts:
  secrets: 60s
`
	if err := os.WriteFile(cfgFile, []byte(content), 0600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level: got %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format: got %q, want %q", cfg.Log.Format, "json")
	}
	if cfg.Cache.MaxAge != "2h" {
		t.Errorf("Cache.MaxAge: got %q, want %q", cfg.Cache.MaxAge, "2h")
	}
	if cfg.Timeouts.Secrets != "60s" {
		t.Errorf("Timeouts.Secrets: got %q, want %q", cfg.Timeouts.Secrets, "60s")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(":\tinvalid: yaml: ["), 0600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}
	_, err := Load(cfgFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_EmptyPathUsesDefault(t *testing.T) {
	// With an empty path, Load should not error even when the default config
	// file doesn't exist.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with empty path: %v", err)
	}
	// Should at minimum return valid defaults.
	if cfg.Log.Level == "" {
		t.Error("expected non-empty Log.Level from defaults")
	}
}

func TestLoad_EnvVarOverrides(t *testing.T) {
	t.Run("ENVOKE_LOG_LEVEL overrides config", func(t *testing.T) {
		t.Setenv("ENVOKE_LOG_LEVEL", "debug")
		cfg := Defaults()
		applyEnv(&cfg)
		if cfg.Log.Level != "debug" {
			t.Errorf("got %q, want %q", cfg.Log.Level, "debug")
		}
	})

	t.Run("ENVOKE_LOG_FORMAT overrides config", func(t *testing.T) {
		t.Setenv("ENVOKE_LOG_FORMAT", "json")
		cfg := Defaults()
		applyEnv(&cfg)
		if cfg.Log.Format != "json" {
			t.Errorf("got %q, want %q", cfg.Log.Format, "json")
		}
	})

	t.Run("ENVOKE_CACHE_MAX_AGE overrides config", func(t *testing.T) {
		t.Setenv("ENVOKE_CACHE_MAX_AGE", "2h")
		cfg := Defaults()
		applyEnv(&cfg)
		if cfg.Cache.MaxAge != "2h" {
			t.Errorf("got %q, want %q", cfg.Cache.MaxAge, "2h")
		}
	})

	t.Run("ENVOKE_TIMEOUT_SECRETS overrides config", func(t *testing.T) {
		t.Setenv("ENVOKE_TIMEOUT_SECRETS", "60s")
		cfg := Defaults()
		applyEnv(&cfg)
		if cfg.Timeouts.Secrets != "60s" {
			t.Errorf("got %q, want %q", cfg.Timeouts.Secrets, "60s")
		}
	})

	t.Run("ENVOKE_UI_BORDER overrides config", func(t *testing.T) {
		t.Setenv("ENVOKE_UI_BORDER", "false")
		cfg := Defaults()
		applyEnv(&cfg)
		if cfg.UI.Border {
			t.Errorf("got %v, want false", cfg.UI.Border)
		}
	})
}

func TestCacheMaxAge(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"8h", 8 * time.Hour},
		{"2h30m", 2*time.Hour + 30*time.Minute},
		{"", 8 * time.Hour},        // fallback
		{"invalid", 8 * time.Hour}, // fallback on parse error
	}
	for _, tt := range tests {
		cfg := Defaults()
		cfg.Cache.MaxAge = tt.input
		got := cfg.CacheMaxAge()
		if got != tt.want {
			t.Errorf("CacheMaxAge(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSecretsTimeout(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"30s", 30 * time.Second},
		{"1m", time.Minute},
		{"", 30 * time.Second},
		{"oops", 30 * time.Second},
	}
	for _, tt := range tests {
		cfg := Defaults()
		cfg.Timeouts.Secrets = tt.input
		got := cfg.SecretsTimeout()
		if got != tt.want {
			t.Errorf("SecretsTimeout(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestDefaultConfigFile(t *testing.T) {
	t.Run("uses XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")
		got := DefaultConfigFile()
		want := "/custom/config/envoke/config.yaml"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("falls back to home dir", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		got := DefaultConfigFile()
		if got == "" {
			t.Skip("could not determine home dir in test environment")
		}
		if filepath.Base(got) != "config.yaml" {
			t.Errorf("expected filename config.yaml, got %q", filepath.Base(got))
		}
		if filepath.Base(filepath.Dir(got)) != "envoke" {
			t.Errorf("expected parent dir envoke, got %q", filepath.Base(filepath.Dir(got)))
		}
	})
}
