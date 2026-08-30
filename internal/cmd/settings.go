package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/minoa/guddit/internal/auth"
	"github.com/minoa/guddit/internal/config"
)

var settingsCmd = &cobra.Command{
	Use:   "settings [file|list|get|set|unset|edit] [key] [value]",
	Short: "Manage persistent configuration",
	Long: `Manage guddit's persistent settings stored in ~/.config/guddit/config.yaml.

  guddit settings              # show all settings (same as 'list')
  guddit settings file         # print the config file path
  guddit settings edit         # open the config in $EDITOR
  guddit settings list         # show all settings with current values
  guddit settings get <key>    # show one setting
  guddit settings set <key> <val>   # set and validate, then persist
  guddit settings unset <key>  # remove a setting`,
	Args: cobra.MaximumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		if len(args) == 0 {
			return settingsList()
		}
		switch args[0] {
		case "get":
			if len(args) < 2 {
				return errInvalid("settings get", "missing", "key")
			}
			return settingsGet(args[1])
		case "set":
			if len(args) < 3 {
				return errInvalid("settings set", "missing", "key value")
			}
			return settingsSet(args[1], args[2])
		case "unset":
			if len(args) < 2 {
				return errInvalid("settings unset", "missing", "key")
			}
			return settingsUnset(args[1])
		case "list":
			return settingsList()
		case "file", "path":
			return settingsFile()
		case "edit":
			return settingsEdit()
		default:
			return errInvalid("settings action", args[0], "file|list|get|set|unset|edit")
		}
	},
}

func settingsGet(key string) error {
	if !isKnownKey(key) {
		return fmt.Errorf("unknown setting %q — run `guddit settings list` to see available keys", key)
	}
	if cfgViper == nil {
		return fmt.Errorf("config not loaded")
	}
	fmt.Printf("%s = %v\n", key, cfgViper.Get(key))
	return nil
}

func settingsSet(key, value string) error {
	if !isKnownKey(key) {
		return fmt.Errorf("unknown setting %q — run `guddit settings list` to see available keys", key)
	}
	if cfgViper == nil {
		return fmt.Errorf("config not loaded")
	}
	parsed, err := parseValue(key, value)
	if err != nil {
		return err
	}
	if err := validateSetting(key, parsed); err != nil {
		return fmt.Errorf("invalid value for %q: %w", key, err)
	}
	cfgViper.Set(key, parsed)
	if err := configSave(cfgViper); err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}
	fmt.Printf("  ✓ %s = %v\n", key, parsed)
	return nil
}

func settingsUnset(key string) error {
	if !isKnownKey(key) {
		return fmt.Errorf("unknown setting %q", key)
	}
	if cfgViper == nil {
		return fmt.Errorf("config not loaded")
	}
	all := cfgViper.AllSettings()
	delete(all, key)
	v := viper.New()
	for k, val := range config.Defaults {
		v.SetDefault(k, val)
	}
	for k, val := range all {
		v.Set(k, val)
	}
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	v.SetConfigFile(filepath.Join(dir, "config.yaml"))
	v.SetConfigType("yaml")
	cfgViper = v
	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}
	fmt.Printf("  ✓ unset %s\n", key)
	return nil
}

func settingsList() error {
	if cfgViper == nil {
		return fmt.Errorf("config not loaded")
	}
	dir, _ := config.ConfigDir()
	path := filepath.Join(dir, "config.yaml")
	fmt.Printf("# config: %s\n\n", path)

	keys := make([]string, 0, len(config.Defaults))
	for k := range config.Defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		val := cfgViper.Get(k)
		def := config.Defaults[k]
		marker := ""
		if cfgViper.IsSet(k) && val != def {
			marker = " *"
		}
		fmt.Printf("  %-18s %v%s\n", k, val, marker)
	}
	return nil
}

func settingsFile() error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	fmt.Println(filepath.Join(dir, "config.yaml"))
	return nil
}

func settingsEdit() error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isKnownKey reports whether key is a recognized setting.
func isKnownKey(key string) bool {
	_, ok := config.Defaults[key]
	return ok
}

// parseValue converts a string value to the appropriate Go type for the key.
func parseValue(key, value string) (any, error) {
	switch key {
	case "limit":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
			return nil, fmt.Errorf("expected an integer, got %q", value)
		}
		return n, nil
	case "no_color", "no_links", "no_images", "no_pager", "verbose",
		"raw_json", "nsfw", "safe_search":
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		}
		return nil, fmt.Errorf("expected true/false, got %q", value)
	default:
		return value, nil
	}
}

// validateSetting checks that a value is valid for the key before persisting.
func validateSetting(key string, val any) error {
	s, _ := val.(string)
	switch key {
	case "host":
		switch s {
		case "www", "old", "new", "oauth", "":
			return nil
		}
		return fmt.Errorf("must be www|old|new|oauth, got %q", s)
	case "format":
		switch s {
		case "", "pretty", "json", "csv", "agent", "table", "markdown", "txt":
			return nil
		}
		return fmt.Errorf("must be pretty|json|csv|agent|table|markdown|txt, got %q", s)
	case "link_domain":
		switch s {
		case "old", "www", "custom", "":
			return nil
		}
		return fmt.Errorf("must be old|www|custom, got %q", s)
	case "link_format":
		switch s {
		case "short", "medium", "long", "":
			return nil
		}
		return fmt.Errorf("must be short|medium|long, got %q", s)
	case "cookie":
		if s == "" {
			return nil
		}
		if _, err := auth.ParseCookieString(s); err != nil {
			return fmt.Errorf("cookie string is not parseable: %w", err)
		}
		return nil
	case "cookie_file":
		if s == "" {
			return nil
		}
		if _, err := os.Stat(s); err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		if _, err := auth.ParseNetscapeFile(s); err != nil {
			return fmt.Errorf("file is not a valid Netscape cookies.txt: %w", err)
		}
		return nil
	}
	return nil
}

func init() {
	rootCmd.AddCommand(settingsCmd)
}
