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
	if cfg.Timeouts.Bitwarden != "30s" {
		t.Errorf("Timeouts.Bitwarden: got %q, want %q", cfg.Timeouts.Bitwarden, "30s")
	}
	if cfg.Timeouts.Vault != "30s" {
		t.Errorf("Timeouts.Vault: got %q, want %q", cfg.Timeouts.Vault, "30s")
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
  isolated: true
timeouts:
  bitwarden: 60s
  vault: 45s
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
	if !cfg.Cache.Isolated {
		t.Error("Cache.Isolated: expected true")
	}
	if cfg.Timeouts.Bitwarden != "60s" {
		t.Errorf("Timeouts.Bitwarden: got %q, want %q", cfg.Timeouts.Bitwarden, "60s")
	}
	if cfg.Timeouts.Vault != "45s" {
		t.Errorf("Timeouts.Vault: got %q, want %q", cfg.Timeouts.Vault, "45s")
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

func TestApplyEnv(t *testing.T) {
	// Isolate env var changes to this test.
	vars := map[string]string{
		"RENV_LOG_LEVEL":            "debug",
		"RENV_LOG_FORMAT":           "json",
		"RENV_CACHE_MAX_AGE":        "4h",
		"RENV_ISOLATED":             "true",
		"RENV_PASSWORD_GRACE_PERIOD": "5m",
		"RENV_TIMEOUT_BITWARDEN":    "10s",
		"RENV_TIMEOUT_VAULT":        "15s",
	}
	for k, v := range vars {
		t.Setenv(k, v)
	}

	cfg := Defaults()
	applyEnv(&cfg)

	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level: got %q, want debug", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format: got %q, want json", cfg.Log.Format)
	}
	if cfg.Cache.MaxAge != "4h" {
		t.Errorf("Cache.MaxAge: got %q, want 4h", cfg.Cache.MaxAge)
	}
	if !cfg.Cache.Isolated {
		t.Error("Cache.Isolated: expected true")
	}
	if cfg.Cache.PasswordGracePeriod != "5m" {
		t.Errorf("Cache.PasswordGracePeriod: got %q, want 5m", cfg.Cache.PasswordGracePeriod)
	}
	if cfg.Timeouts.Bitwarden != "10s" {
		t.Errorf("Timeouts.Bitwarden: got %q, want 10s", cfg.Timeouts.Bitwarden)
	}
	if cfg.Timeouts.Vault != "15s" {
		t.Errorf("Timeouts.Vault: got %q, want 15s", cfg.Timeouts.Vault)
	}
}

func TestApplyEnv_IsolatedValues(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false}, // env var unset — handled by empty-string guard in applyEnv
	}
	for _, tt := range tests {
		if tt.val == "" {
			continue // applyEnv only applies when val != ""
		}
		t.Run(tt.val, func(t *testing.T) {
			t.Setenv("RENV_ISOLATED", tt.val)
			cfg := Defaults()
			applyEnv(&cfg)
			if cfg.Cache.Isolated != tt.want {
				t.Errorf("RENV_ISOLATED=%q → Isolated=%v, want %v", tt.val, cfg.Cache.Isolated, tt.want)
			}
		})
	}
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

func TestCachePasswordGracePeriod(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"", 0},        // fallback is 0
		{"bad", 0},     // fallback on parse error
	}
	for _, tt := range tests {
		cfg := Defaults()
		cfg.Cache.PasswordGracePeriod = tt.input
		got := cfg.CachePasswordGracePeriod()
		if got != tt.want {
			t.Errorf("CachePasswordGracePeriod(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBitwardenTimeout(t *testing.T) {
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
		cfg.Timeouts.Bitwarden = tt.input
		got := cfg.BitwardenTimeout()
		if got != tt.want {
			t.Errorf("BitwardenTimeout(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestVaultTimeout(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"30s", 30 * time.Second},
		{"2m", 2 * time.Minute},
		{"", 30 * time.Second},
		{"oops", 30 * time.Second},
	}
	for _, tt := range tests {
		cfg := Defaults()
		cfg.Timeouts.Vault = tt.input
		got := cfg.VaultTimeout()
		if got != tt.want {
			t.Errorf("VaultTimeout(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestDefaultConfigFile(t *testing.T) {
	t.Run("uses XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")
		got := DefaultConfigFile()
		want := "/custom/config/renv/config.yaml"
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
		if filepath.Base(filepath.Dir(got)) != "renv" {
			t.Errorf("expected parent dir renv, got %q", filepath.Base(filepath.Dir(got)))
		}
	})
}
