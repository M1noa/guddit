// Package config persists guddit settings to ~/.config/guddit/config.yaml
// via viper. Keys are flat (user_agent, host, cookie_string, client_id, ...).
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ConfigDir returns the guddit config directory (~/.config/guddit or
// $XDG_CONFIG_HOME/guddit), creating it if missing.
func ConfigDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	dir = filepath.Join(dir, "guddit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("config: mkdir %s: %w", dir, err)
	}
	return dir, nil
}

// ConfigPath returns the full path to config.yaml.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Defaults applied on every Load.
var Defaults = map[string]any{
	"user_agent":     "",
	"host":           "www",
	"format":         "",
	"limit":          25,
	"raw_json":       true,
	"no_color":       false,
	"no_links":       false,
	"no_images":      false,
	"pager":          "",
	"no_pager":       false,
	"verbose":        false,
	"link_domain":    "old",
	"link_format":    "medium",
	"custom_domain":  "",
	"theme_primary":   "",
	"theme_secondary": "",
	"theme_accent":    "",
	"theme_muted":     "",
	"theme_bg":        "",
	"theme_fg":        "",
	"cookie":          "",
	"cookie_file":     "",
	"output":          "",
	"after":           "",
	"before":          "",
	"nsfw":            false,
	"safe_search":     false,
	"client_id":       "",
	"client_secret":   "",
}

// Load reads config.yaml (creating it with defaults if absent) and returns the
// bound viper instance. Missing file is not an error.
func Load() (*viper.Viper, error) {
	v := viper.New()
	for k, val := range Defaults {
		v.SetDefault(k, val)
	}
	// Environment overrides: GUDDIT_<UPPER_KEY>.
	v.SetEnvPrefix("GUDDIT")
	v.AutomaticEnv()

	dir, err := ConfigDir()
	if err != nil {
		return v, nil // non-fatal: env/flags can still drive config
	}
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return v, nil
		}
		return v, fmt.Errorf("config: read: %w", err)
	}
	return v, nil
}

// Save writes the current viper state back to config.yaml.
func Save(v *viper.Viper) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	v.SetConfigFile(filepath.Join(dir, "config.yaml"))
	v.SetConfigType("yaml")
	return v.WriteConfig()
}

// Set writes a single key and persists.
func Set(v *viper.Viper, key string, val any) error {
	v.Set(key, val)
	return Save(v)
}

// Unset removes a key and persists. viper has no native delete, so we read the
// raw map, delete the key, and rewrite.
func Unset(v *viper.Viper, key string) error {
	all := v.AllSettings()
	delete(all, key)
	// Re-bind a fresh viper to avoid stale key residue.
	fresh := viper.New()
	for k, val := range Defaults {
		fresh.SetDefault(k, val)
	}
	for k, val := range all {
		fresh.Set(k, val)
	}
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	fresh.SetConfigFile(filepath.Join(dir, "config.yaml"))
	fresh.SetConfigType("yaml")
	return fresh.WriteConfig()
}
