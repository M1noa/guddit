package cmd

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/spf13/viper"

	"github.com/minoa/guddit/internal/auth"
	"github.com/minoa/guddit/internal/config"
)

// loadStoredAuth reads the persisted "auth" block from config.yaml into a
// auth.StoredAuth. Missing block returns a zero value (not an error).
func loadStoredAuth() auth.StoredAuth {
	var s auth.StoredAuth
	if cfgViper == nil {
		return s
	}
	if cfgViper.IsSet("auth") {
		if err := cfgViper.UnmarshalKey("auth", &s); err == nil {
			return s
		}
	}
	// Also tolerate flat keys (cookie_string, client_id, ...) for hand-edited
	// configs.
	if v := cfgViper.GetString("cookie_string"); v != "" {
		s.CookieString = v
	}
	if id := flagStr("client-id"); id != "" {
		s.ClientID = id
	}
	if sec := flagStr("client-secret"); sec != "" {
		s.ClientSecret = sec
	}
	return s
}

// saveStoredAuth writes the auth block back to config.yaml.
func saveStoredAuth(s auth.StoredAuth) error {
	if cfgViper == nil {
		_ = loadConfig()
	}
	cfgViper.Set("auth", s)
	return saveConfig()
}

func saveConfig() error {
	if cfgViper == nil {
		return fmt.Errorf("config not loaded")
	}
	return configSave(cfgViper)
}

// configSave is a thin wrapper to break a package cycle (config is imported
// directly by cmd; this lets tests swap it). It pins the config file path so
// WriteConfig works even before the file has been created (fresh installs).
var configSave = func(v *viper.Viper) error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	v.SetConfigFile(filepath.Join(dir, "config.yaml"))
	v.SetConfigType("yaml")
	return v.WriteConfig()
}

// mintOAuthToken exchanges script-app credentials + username/password for an
// OAuth access token via the password grant. Username/password come from the
// stored auth block (set via `guddit auth login --method password`).
func mintOAuthToken(clientID, clientSecret, ua string) (*auth.Token, error) {
	stored := loadStoredAuth()
	if stored.Username == "" || stored.Password == "" {
		return nil, fmt.Errorf("oauth requires stored username/password — run `guddit auth login --method password`")
	}
	o := &auth.OAuthClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Username:     stored.Username,
		Password:     stored.Password,
		UserAgent:    ua,
		HTTP:         &http.Client{Timeout: 30 * time.Second},
	}
	return o.TokenExchange()
}
