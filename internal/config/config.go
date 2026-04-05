// Package config provides structured configuration for renv/kctx.
//
// Loading order (later sources override earlier ones):
//  1. Built-in defaults
//  2. Config file ($XDG_CONFIG_HOME/renv/config.yaml, or --config path)
//  3. Environment variables (RENV_*)
//  4. CLI flags (applied by the caller after Load returns)
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all tunable parameters shared by renv and kctx.
type Config struct {
	Log      LogConfig     `yaml:"log"`
	Cache    CacheConfig   `yaml:"cache"`
	Timeouts TimeoutConfig `yaml:"timeouts"`
}

// LogConfig controls log verbosity and output format.
type LogConfig struct {
	// Level is the minimum log level: debug, info, warn, error.
	Level string `yaml:"level"`
	// Format is the output format: text or json.
	Format string `yaml:"format"`
}

// CacheConfig controls secret cache behaviour.
type CacheConfig struct {
	// MaxAge is the maximum age of cached Bitwarden folder items (Go duration string).
	MaxAge string `yaml:"max_age"`
}

// TimeoutConfig controls subprocess call timeouts.
type TimeoutConfig struct {
	// Bitwarden is the timeout for bw CLI subprocess calls (Go duration string).
	Bitwarden string `yaml:"bitwarden"`
	// Vault is the timeout for vault CLI subprocess calls (Go duration string).
	Vault string `yaml:"vault"`
}

// Defaults returns a Config with all fields set to safe defaults.
func Defaults() Config {
	return Config{
		Log: LogConfig{
			Level:  "warn",
			Format: "text",
		},
		Cache: CacheConfig{
			MaxAge: "8h",
		},
		Timeouts: TimeoutConfig{
			Bitwarden: "30s",
			Vault:     "30s",
		},
	}
}

// Load reads configuration from cfgFile (empty → default path), then overlays
// environment variables. CLI flag overrides are the caller's responsibility.
func Load(cfgFile string) (Config, error) {
	cfg := Defaults()

	if cfgFile == "" {
		cfgFile = DefaultConfigFile()
	}

	if cfgFile != "" {
		data, err := os.ReadFile(cfgFile)
		if err == nil {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return cfg, fmt.Errorf("parsing config file %s: %w", cfgFile, err)
			}
		} else if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("reading config file %s: %w", cfgFile, err)
		}
	}

	applyEnv(&cfg)
	return cfg, nil
}

// DefaultConfigFile returns the default config file path based on XDG conventions.
func DefaultConfigFile() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "renv", "config.yaml")
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("RENV_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("RENV_LOG_FORMAT"); v != "" {
		cfg.Log.Format = v
	}
	if v := os.Getenv("RENV_CACHE_MAX_AGE"); v != "" {
		cfg.Cache.MaxAge = v
	}
	if v := os.Getenv("RENV_TIMEOUT_BITWARDEN"); v != "" {
		cfg.Timeouts.Bitwarden = v
	}
	if v := os.Getenv("RENV_TIMEOUT_VAULT"); v != "" {
		cfg.Timeouts.Vault = v
	}
}

// CacheMaxAge parses and returns the cache max-age duration.
func (c *Config) CacheMaxAge() time.Duration {
	return parseDuration(c.Cache.MaxAge, 8*time.Hour)
}

// BitwardenTimeout parses and returns the Bitwarden subprocess timeout.
func (c *Config) BitwardenTimeout() time.Duration {
	return parseDuration(c.Timeouts.Bitwarden, 30*time.Second)
}

// VaultTimeout parses and returns the Vault subprocess timeout.
func (c *Config) VaultTimeout() time.Duration {
	return parseDuration(c.Timeouts.Vault, 30*time.Second)
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
