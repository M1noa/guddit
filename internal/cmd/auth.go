package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/auth"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication (cookies, OAuth, password login)",
	Long: `Auth methods (all free, no paid API):
  password  — harvest reddit_session/token_v2 via /api/login (legacy)
  oauth     — mint bearer token via script-app password grant
  browser   — open reddit.com/login, capture cookies via local callback
  google    — same as browser; works for "Continue with Google" redirects
  status    — show current auth tier and stored credentials`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login [--method password|oauth|browser|google]",
	Short: "Authenticate and store credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		method, _ := cmd.Flags().GetString("method")
		switch method {
		case "password":
			return authLoginPassword(cmd)
		case "oauth":
			return authLoginOAuth(cmd)
		case "browser":
			return authLoginBrowser(cmd, "https://www.reddit.com/login")
		case "google":
			return authLoginBrowser(cmd, "https://www.reddit.com/login?provider=google")
		default:
			return errInvalid("auth method", method, "password|oauth|browser|google")
		}
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored authentication",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		s := loadStoredAuth()
		s = auth.StoredAuth{} // zero
		return saveStoredAuth(s)
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication tier and cookie/token state",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		s := loadStoredAuth()
		tier := s.Tier()
		fmt.Printf("tier: %s\n", tier)
		switch tier {
		case auth.TierOAuth:
			fmt.Printf("  oauth: client_id=%s token_valid=%v\n", s.ClientID, s.Token != nil && s.Token.Valid())
		case auth.TierCookie:
			if s.CookieString != "" {
				fmt.Printf("  cookie_string: set (%d chars)\n", len(s.CookieString))
			}
			if len(s.Cookies) > 0 {
				fmt.Printf("  cookies: %d\n", len(s.Cookies))
				for _, c := range s.Cookies {
					fmt.Printf("    %s=%s\n", c.Name, c.Value[:min(8, len(c.Value))]+"...")
				}
			}
		case auth.TierPassword:
			fmt.Printf("  username: %s\n", s.Username)
		}
		return nil
	},
}

func authLoginPassword(cmd *cobra.Command) error {
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	if username == "" || password == "" {
		return errInvalid("password login", "missing", "--username and --password required")
	}
	res, err := auth.LoginWithPassword(username, password, flagStr("user-agent"))
	if err != nil {
		return err
	}
	stored := auth.StoredAuth{
		Username: username,
		Password: password,
		Cookies:  auth.FromHTTPCookies(res.Cookies),
	}
	return saveStoredAuth(stored)
}

func authLoginOAuth(cmd *cobra.Command) error {
	clientID, _ := cmd.Flags().GetString("client-id")
	clientSecret, _ := cmd.Flags().GetString("client-secret")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	if clientID == "" || clientSecret == "" {
		return errInvalid("oauth", "missing", "--client-id and --client-secret required")
	}
	if username == "" || password == "" {
		return errInvalid("oauth", "missing", "--username and --password required (for password grant)")
	}
	ua := flagStr("user-agent")
	if ua == "" {
		ua = "guddit/0.1"
	}
	o := &auth.OAuthClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Username:     username,
		Password:     password,
		UserAgent:    ua,
	}
	token, err := o.TokenExchange()
	if err != nil {
		return err
	}
	stored := auth.StoredAuth{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Username:     username,
		Password:     password,
		Token:        token,
	}
	return saveStoredAuth(stored)
}

func authLoginBrowser(cmd *cobra.Command, loginURL string) error {
	// Minimal local callback server to capture the post-login redirect.
	// This is a stub - a full implementation would spin up an HTTP server
	// on localhost and open the browser. For v0.1, we just print instructions.
	printBanner("browser login")
	fmt.Println("Opening: " + loginURL)
	fmt.Println("1. Log in to Reddit in the browser (including Google if you chose that)")
	fmt.Println("2. After logging in, export your cookies (use a browser extension like 'Get cookies.txt')")
	fmt.Println("3. Save to a file, then run: guddit auth login --method password --cookie-file <file>")
	fmt.Println("   OR: guddit settings set cookie_string \"<cookie header>\"")
	return nil
}

func init() {
	authLoginCmd.Flags().String("method", "password", "auth method: password|oauth|browser|google")
	authLoginCmd.Flags().String("username", "", "Reddit username")
	authLoginCmd.Flags().String("password", "", "Reddit password")
	authLoginCmd.Flags().String("client-id", "", "OAuth client ID")
	authLoginCmd.Flags().String("client-secret", "", "OAuth client secret")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}