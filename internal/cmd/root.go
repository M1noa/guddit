// Package cmd holds the cobra command tree for guddit. The root command owns
// the global/persistent flags (auth, host, format, output, limits) and exposes
// helpers used by every subcommand to build a configured reddit.Client and
// detect terminal capabilities.
package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/minoa/guddit/internal/auth"
	"github.com/minoa/guddit/internal/config"
	"github.com/minoa/guddit/internal/output"
	"github.com/minoa/guddit/internal/reddit"
	"github.com/minoa/guddit/internal/render"
)

// Build-time variables, injected by the Makefile via -ldflags through
// SetBuildInfo.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetBuildInfo records version/commit/date injected at link time.
func SetBuildInfo(v, c, d string) {
	if v != "" {
		version = v
	}
	if c != "" {
		commit = c
	}
	if d != "" {
		date = d
	}
}

func versionString() string {
	s := "guddit " + version
	if len(commit) >= 7 {
		s += " (build " + commit[:7]
		if date != "" && date != "unknown" {
			s += ", " + date
		}
		s += ")"
	}
	return s
}

// globals set during root PreRun from flags + config.
var (
	cfgViper *viper.Viper
)

// rootCmd is the guddit entrypoint.
var rootCmd = &cobra.Command{
	Use:   "guddit",
	Short: "Terminal Reddit browser — search, read, and scrape Reddit with no API key",
	Long: `guddit is a terminal-first Reddit client that talks directly to Reddit's public
.json endpoints (with old.reddit.com fallback). No paid API, no API key —
just a descriptive User-Agent and, for authenticated features, your cookies or
OAuth script-app credentials.

  guddit home --limit 10
  guddit search "go cobra cli" --sub golang
  guddit sub aww top -t week --limit 25
  guddit post https://www.reddit.com/r/golang/comments/xxxx
  guddit me saved --cookie "reddit_session=..."

When piped, guddit defaults to JSON so agents and scripts get machine data.
Override with --format: pretty|json|csv|agent|table|markdown|txt. --agent is a
shorthand for csv (token-cheap, no color). Writes to a file with --output.
Settings persist to ~/.config/guddit/config.yaml.

` + "guddit/" + versionString() + ``,
	SilenceUsage: true,
	RunE: func(c *cobra.Command, args []string) error {
		if v, _ := c.Flags().GetBool("version"); v {
			fmt.Println(versionString())
			return nil
		}
		return c.Help()
	},
}

// Execute runs the root command.
func Execute() error {
	// Check if first arg is a URL/ID and route to appropriate command, but
	// never hijack a real subcommand name.
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") && !isCommand(os.Args[1]) {
		if isRedditURL(os.Args[1]) {
			routeURL(os.Args[1])
		}
	}
	return rootCmd.Execute()
}

// isCommand reports whether s names one of the registered subcommands (or help).
func isCommand(s string) bool {
	if s == "help" || s == "completion" {
		return true
	}
	for _, c := range rootCmd.Commands() {
		if c.Name() == s {
			return true
		}
		for _, a := range c.Aliases {
			if a == s {
				return true
			}
		}
	}
	return false
}

// isRedditURL checks if a string looks like a Reddit URL or ID
func isRedditURL(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	// Check for Reddit domains
	if strings.Contains(s, "reddit.com") || strings.Contains(s, "redd.it") {
		return true
	}
	// Check for bare IDs (t3_xxx, t1_xxx, or 5-8 char alphanumeric)
	if strings.HasPrefix(s, "t3_") || strings.HasPrefix(s, "t1_") || strings.HasPrefix(s, "t2_") || strings.HasPrefix(s, "t5_") {
		return true
	}
	// 5-8 char alphanumeric (post/comment ID)
	if matched, _ := regexp.MatchString(`^[a-z0-9]{5,8}$`, s); matched {
		return true
	}
	// subreddit names (r/name or bare name - assume lowercase alphanumeric/underscore is subreddit)
	if strings.HasPrefix(s, "r/") || strings.HasPrefix(s, "/r/") {
		return true
	}
	// Bare subreddit name (alphanumeric + underscore, 3-21 chars)
	if matched, _ := regexp.MatchString(`^[a-z0-9_]{3,21}$`, s); matched {
		return true
	}
	// user names (u/name or bare username with hyphens)
	if strings.HasPrefix(s, "u/") || strings.HasPrefix(s, "/u/") {
		return true
	}
	// Bare username (alphanumeric + underscore/hyphen, 3-20 chars)
	if matched, _ := regexp.MatchString(`^[a-z0-9_-]{3,20}$`, s); matched {
		return true
	}
	return false
}

// routeURL determines the type of Reddit URL and routes to appropriate command
func routeURL(url string) {
	// Normalize the URL
	url = strings.TrimSpace(url)
	
	// Special case: "search" as a bare command should route to search
	if url == "search" {
		os.Args = append([]string{"guddit", "search"}, os.Args[1:]...)
		return
	}
	
	// Check for search URLs FIRST (before subreddit detection)
	if strings.Contains(url, "/search?") || strings.Contains(url, "/search?q=") {
		os.Args = append([]string{"guddit", "search"}, os.Args[1:]...)
		return
	}
	
	// Check for wiki URLs
	if strings.Contains(url, "/wiki/") {
		os.Args = append([]string{"guddit", "wiki"}, os.Args[1:]...)
		return
	}
	
	// Check for post/comment URLs (full URLs with /comments/ or redd.it/)
	if strings.Contains(url, "/comments/") || strings.Contains(url, "redd.it/") || 
	   strings.HasPrefix(url, "t3_") {
		os.Args = append([]string{"guddit", "post"}, os.Args[1:]...)
		return
	}
	
	// Check for subreddit URLs (full URL)
	if strings.Contains(url, "reddit.com/r/") {
		// Extract subreddit name from URL
		re := regexp.MustCompile(`reddit\.com/r/([A-Za-z0-9_]+)`)
		if m := re.FindStringSubmatch(url); m != nil {
			os.Args = append([]string{"guddit", "sub", m[1]}, os.Args[2:]...)
			return
		}
		os.Args = append([]string{"guddit", "sub"}, os.Args[1:]...)
		return
	}
	if strings.HasPrefix(url, "r/") || strings.HasPrefix(url, "/r/") {
		os.Args = append([]string{"guddit", "sub"}, os.Args[1:]...)
		return
	}
	
	// Bare subreddit name (letters + underscore only, 3-21 chars, no numbers/dots/slashes)
	if matched, _ := regexp.MatchString(`^[a-z_]{3,21}$`, url); matched {
		os.Args = append([]string{"guddit", "sub", url}, os.Args[2:]...)
		return
	}
	
	// Check for user URLs
	if strings.Contains(url, "reddit.com/user/") {
		re := regexp.MustCompile(`reddit\.com/user/([A-Za-z0-9_-]+)`)
		if m := re.FindStringSubmatch(url); m != nil {
			os.Args = append([]string{"guddit", "user", m[1]}, os.Args[2:]...)
			return
		}
		os.Args = append([]string{"guddit", "user"}, os.Args[1:]...)
		return
	}
	if strings.Contains(url, "reddit.com/u/") {
		re := regexp.MustCompile(`reddit\.com/u/([A-Za-z0-9_-]+)`)
		if m := re.FindStringSubmatch(url); m != nil {
			os.Args = append([]string{"guddit", "user", m[1]}, os.Args[2:]...)
			return
		}
		os.Args = append([]string{"guddit", "user"}, os.Args[1:]...)
		return
	}
	if strings.HasPrefix(url, "u/") || strings.HasPrefix(url, "/u/") || strings.HasPrefix(url, "t2_") {
		os.Args = append([]string{"guddit", "user"}, os.Args[1:]...)
		return
	}
	
	// Bare post/comment ID (5-8 char alphanumeric) - check BEFORE bare
	// username and subreddits-with-digits
	if regexp.MustCompile(`^[a-z0-9]{5,8}$`).MatchString(url) {
		os.Args = append([]string{"guddit", "post"}, os.Args[1:]...)
		return
	}

	// Bare subreddit with digits (e.g. "3dprinting") - no hyphen allowed.
	if matched, _ := regexp.MatchString(`^[a-z0-9_]{3,21}$`, url); matched {
		os.Args = append([]string{"guddit", "sub", url}, os.Args[2:]...)
		return
	}

	// Bare username (alphanumeric + underscore/hyphen, 3-20 chars)
	if matched, _ := regexp.MatchString(`^[a-z0-9_-]{3,20}$`, url); matched {
		os.Args = append([]string{"guddit", "user", url}, os.Args[2:]...)
		return
	}

	// Default to post
	os.Args = append([]string{"guddit", "post"}, os.Args[1:]...)
}

// init registers persistent (global) flags inherited by all subcommands.
func init() {
	rootCmd.PersistentFlags().String("cookie", "", "raw cookie header (reddit_session=...; token_v2=...)")
	rootCmd.PersistentFlags().String("cookie-file", "", "path to a Netscape cookies.txt file")
	rootCmd.PersistentFlags().String("user-agent", "", "override the descriptive User-Agent sent to Reddit")
	rootCmd.PersistentFlags().String("host", "", "reddit host to use (www|old|new|oauth)")
	rootCmd.PersistentFlags().StringP("format", "f", "", "output format (pretty|json|csv|agent|table|markdown|txt)")
	rootCmd.PersistentFlags().Bool("agent", false, "agent mode: compact csv output, no color (shorthand for --format agent)")
	rootCmd.PersistentFlags().StringP("output", "o", "", "write output to a file instead of stdout")
	rootCmd.PersistentFlags().Bool("no-color", false, "disable ANSI color")
	rootCmd.PersistentFlags().Bool("no-links", false, "disable OSC 8 hyperlinks")
	rootCmd.PersistentFlags().Bool("no-images", false, "disable inline images")
	rootCmd.PersistentFlags().String("pager", "", "pager command (default $PAGER or 'less -RFX')")
	rootCmd.PersistentFlags().Bool("no-pager", false, "never pipe output through a pager")
	rootCmd.PersistentFlags().IntP("limit", "L", 0, "limit number of items (max 100)")
	rootCmd.PersistentFlags().String("after", "", "pagination cursor: items after this fullname")
	rootCmd.PersistentFlags().String("before", "", "pagination cursor: items before this fullname")
	rootCmd.PersistentFlags().Bool("raw-json", true, "pass raw_json=1 to avoid HTML-escaped selftext")
	rootCmd.PersistentFlags().Bool("verbose", false, "verbose output")

	// NSFW / safe search flags
	rootCmd.PersistentFlags().Bool("nsfw", false, "include NSFW (18+) content in results")
	rootCmd.PersistentFlags().Bool("safe-search", false, "enable safe search (exclude NSFW content)")

	// Link formatting flags
	rootCmd.PersistentFlags().String("link-format", "medium", "link format: short (ID only), medium (redd.it/ID), long (full permalink)")
	rootCmd.PersistentFlags().String("link-domain", "old", "link domain: old (old.reddit.com), www (www.reddit.com), custom")
	rootCmd.PersistentFlags().String("custom-domain", "", "custom domain for links when link-domain=custom")

	// Theme color flags
	rootCmd.PersistentFlags().String("theme-primary", "", "primary theme color (hex or ANSI color code)")
	rootCmd.PersistentFlags().String("theme-secondary", "", "secondary theme color (hex or ANSI color code)")
	rootCmd.PersistentFlags().String("theme-accent", "", "accent theme color (hex or ANSI color code)")
	rootCmd.PersistentFlags().String("theme-muted", "", "muted theme color (hex or ANSI color code)")
	rootCmd.PersistentFlags().String("theme-bg", "", "background theme color (hex or ANSI color code)")
	rootCmd.PersistentFlags().String("theme-fg", "", "foreground theme color (hex or ANSI color code)")

	// OAuth script-app credentials (override config).
	rootCmd.PersistentFlags().String("client-id", "", "OAuth script-app client id")
	rootCmd.PersistentFlags().String("client-secret", "", "OAuth script-app client secret")

	// Version flag (cobra doesn't auto-add this).
	rootCmd.Flags().BoolP("version", "v", false, "print version and exit")

	// Version subcommand so `guddit version` never routes to /r/version.
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		Run: func(c *cobra.Command, args []string) {
			fmt.Println(versionString())
		},
	})

	// Bind persistent flags to viper so config can supply defaults. Lower
	// precedence than flags; env vars sit in between (GUDDIT_<UPPER>).
	_ = viper.BindPFlags(rootCmd.PersistentFlags())

	// Add completion command (cobra built-in).
	rootCmd.AddCommand(&cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Args:  cobra.MaximumNArgs(1),
		Run: func(c *cobra.Command, args []string) {
			shell := "bash"
			if len(args) > 0 {
				shell = args[0]
			}
			switch shell {
			case "bash":
				rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				rootCmd.GenPowerShellCompletion(os.Stdout)
			default:
				die(fmt.Errorf("unknown shell %q (want bash|zsh|fish|powershell)", shell))
			}
		},
	})
}

// loadConfig loads config.yaml + env into the global viper.
func loadConfig() error {
	v, err := config.Load()
	cfgViper = v
	return err
}

// buildClient assembles a reddit.Client from flags + config + auth sources,
// applying cookies / OAuth bearer as configured.
func buildClient() (*reddit.Client, error) {
	if cfgViper == nil {
		_ = loadConfig() // best effort
	}

	ua := flagStr("user-agent")
	if ua == "" {
		ua = reddit.DefaultUserAgent
	}

	host := flagStr("host")
	if host == "" {
		host = "www"
	}
	switch host {
	case "www", "old", "new", "oauth":
	default:
		return nil, fmt.Errorf("invalid --host %q (want www|old|new|oauth)", host)
	}

	opts := []reddit.Option{
		reddit.WithUserAgent(ua),
		reddit.WithHost(hostToConst(host)),
		reddit.WithVerbose(flagBool("verbose")),
	}

	// OAuth bearer token (highest precedence).
	if clientID, clientSecret := flagStr("client-id"), flagStr("client-secret"); clientID != "" && clientSecret != "" {
		// Try a stored token first; mint if missing/invalid.
		stored := loadStoredAuth()
		if stored.Token != nil && stored.Token.Valid() {
			opts = append(opts, reddit.WithOAuthToken(stored.Token.AccessToken))
			opts = append(opts, reddit.WithHost(reddit.HostOAuth))
			return reddit.New(opts...), nil
		}
		token, err := mintOAuthToken(clientID, clientSecret, ua)
		if err != nil {
			return nil, err
		}
		stored.Token = token
		_ = saveStoredAuth(stored)
		opts = append(opts, reddit.WithOAuthToken(token.AccessToken))
		opts = append(opts, reddit.WithHost(reddit.HostOAuth))
		c := reddit.New(opts...)
		return c, nil
	}

	c := reddit.New(opts...)

	// Cookie tier: --cookie string, --cookie-file, or stored cookies.
	var oauthToken string
	if raw := flagStr("cookie"); raw != "" {
		cookies, err := auth.ParseCookieString(raw)
		if err != nil {
			return nil, err
		}
		c.SetCookies(cookies)
		if t := findTokenV2(cookies); t != "" {
			oauthToken = t
		}
	} else if path := flagStr("cookie-file"); path != "" {
		cookies, err := auth.ParseNetscapeFile(path)
		if err != nil {
			return nil, err
		}
		c.SetCookies(cookies)
		if t := findTokenV2(cookies); t != "" {
			oauthToken = t
		}
	} else {
		stored := loadStoredAuth()
		cookies := stored.ToHTTPCookies()
		if len(cookies) > 0 {
			c.SetCookies(cookies)
			if t := findTokenV2(cookies); t != "" {
				oauthToken = t
			}
		}
	}
	// Store oauthToken on the client for authenticated endpoints (Me, MyListing).
	// The client keeps its primary host (www/old) for public listings.
	c.OAuthToken = oauthToken
	currentClient = c
	return c, nil
}

func findTokenV2(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if c.Name == "token_v2" && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

// currentClient is the reddit client for the active command.
var currentClient *reddit.Client

// hostToConst maps a flag value to a reddit host constant.
func hostToConst(h string) string {
	switch h {
	case "old":
		return reddit.HostOLD
	case "new":
		return reddit.HostNEW
	case "oauth":
		return reddit.HostOAuth
	default:
		return reddit.HostWWW
	}
}

// caps detects terminal capabilities from the chosen output destination.
func caps() render.Capabilities {
	noColor := flagBool("no-color")
	noLinks := flagBool("no-links")
	noImages := flagBool("no-images")
	if cfgViper != nil {
		if cfgViper.GetBool("no_color") {
			noColor = true
		}
		if cfgViper.GetBool("no_links") {
			noLinks = true
		}
		if cfgViper.GetBool("no_images") {
			noImages = true
		}
	}
	out := os.Stdout
	if path := flagStr("output"); path != "" && path != "-" {
		// File output is never a TTY; detect from stdout anyway so a TTY
		// writer still gets color when the user pipes to a pager that supports
		// it (CLICOLOR_FORCE).
		caps := render.Detect(out, noColor, noLinks)
		caps.Out = nil // set by writeOutput to the actual writer
		return caps
	}
	detected := render.Detect(out, noColor, noLinks)
	if noImages {
		detected.Images = false
	}
	return detected
}

// format resolves the output format from flag/config, defaulting per TTY.
func format() output.Format {
	if flagBool("agent") {
		return output.FormatAgent
	}
	f := flagStr("format")
	if f == "" && cfgViper != nil {
		f = cfgViper.GetString("format")
	}
	isTTY := isStdoutTTY()
	return output.Parse(f, isTTY)
}

func isStdoutTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd()))
}

// themeOptions builds render.ThemeOptions from config.
func themeOptions() *render.ThemeOptions {
	if cfgViper == nil {
		return nil
	}
	opts := &render.ThemeOptions{
		Primary:      cfgViper.GetString("theme_primary"),
		Secondary:    cfgViper.GetString("theme_secondary"),
		Accent:       cfgViper.GetString("theme_accent"),
		Muted:        cfgViper.GetString("theme_muted"),
		Bg:           cfgViper.GetString("theme_bg"),
		Fg:           cfgViper.GetString("theme_fg"),
		CustomDomain: cfgViper.GetString("custom_domain"),
	}
	// Return nil if all empty (use defaults)
	if opts.Primary == "" && opts.Secondary == "" && opts.Accent == "" && 
	   opts.Muted == "" && opts.Bg == "" && opts.Fg == "" && opts.CustomDomain == "" {
		return nil
	}
	return opts
}

// linkOptions builds render.LinkOptions from config.
func linkOptions() render.LinkOptions {
	if cfgViper == nil {
		return render.LinkOptions{}
	}
	// Parse link_format - try flag first, then config
	formatStr := flagStr("link-format")
	var format render.LinkFormat
	switch formatStr {
	case "short":
		format = render.LinkFormatShort
	case "long":
		format = render.LinkFormatLong
	default:
		format = render.LinkFormatMedium
	}
	
	// Parse link_domain
	domainStr := flagStr("link-domain")
	var domain render.LinkDomain
	switch domainStr {
	case "www":
		domain = render.LinkDomainWWW
	case "custom":
		domain = render.LinkDomainCustom
	default:
		domain = render.LinkDomainOld
	}
	
	return render.LinkOptions{
		Format:       format,
		Domain:       domain,
		CustomDomain: flagStr("custom-domain"),
	}
}

// listingOpts builds ListingOptions from the common pagination/limit flags.
func listingOpts() reddit.ListingOptions {
	return reddit.ListingOptions{
		Limit:       flagInt("limit"),
		After:       flagStr("after"),
		Before:      flagStr("before"),
		RawJSON:     flagBool("raw-json"),
		NSFW:        flagBool("nsfw"),
		SafeSearch:  flagBool("safe-search"),
	}
}

// writeOut renders result and writes to --output or stdout. Long human-readable
// output on a TTY is piped through a pager (less) unless disabled.
func writeOut(format output.Format, caps render.Capabilities, result any) error {
	w, closer, err := output.Output(flagStr("output"))
	if err != nil {
		return err
	}
	defer closer()
	// Point caps at the actual writer so lipgloss renders to the right sink.
	if caps.Out == nil {
		caps.Out = w
	}

	if canPage(format, caps) {
		var buf bytes.Buffer
		if err := output.Write(format, caps, result, &buf, themeOptions(), linkOptions()); err != nil {
			return err
		}
		return writePaged(w, caps, buf.String())
	}
	return output.Write(format, caps, result, w, themeOptions(), linkOptions())
}

// canPage reports whether the result should be considered for paging: a
// human-readable format, written to a TTY (not a file), not disabled.
func canPage(format output.Format, caps render.Capabilities) bool {
	if flagBool("no-pager") {
		return false
	}
	if flagStr("output") != "" && flagStr("output") != "-" {
		return false
	}
	if !caps.IsTTY {
		return false
	}
	switch format {
	case output.FormatPretty, output.FormatMarkdown, output.FormatTxt:
		return true
	}
	return false
}

// writePaged writes content directly if it fits the screen, otherwise through
// the pager command ($PAGER, --pager, or less -RFX).
func writePaged(w io.Writer, caps render.Capabilities, content string) error {
	lines := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines++
		}
	}
	cmdStr := flagStr("pager")
	if cmdStr == "" {
		cmdStr = os.Getenv("PAGER")
	}
	if cmdStr == "" {
		cmdStr = "less -RFX"
	}
	if lines <= caps.Height || cmdStr == "cat" || cmdStr == "" {
		_, err := io.WriteString(w, content)
		return err
	}
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// emit is the convenience wrapper subcommands use: render + write, honoring
// --format and --output.
func emit(result any) error {
	c := caps()
	if posts, ok := output.PostsOf(result); ok && len(posts) == 0 && isStdoutTTY() {
		fmt.Fprintf(os.Stderr, "  ⚠ no posts found — the subreddit may be private, quarantined, or doesn't exist\n")
	}
	return writeOut(format(), c, result)
}

// --- flag helpers (flag > config > default) -------------------------------

func flagStr(name string) string {
	if rootCmd.PersistentFlags().Changed(name) {
		v, _ := rootCmd.PersistentFlags().GetString(name)
		return v
	}
	if cfgViper != nil {
		if v := cfgViper.GetString(name); v != "" {
			return v
		}
		// Config stores underscore keys (cookie_file), flags use hyphens.
		if v := cfgViper.GetString(strings.ReplaceAll(name, "-", "_")); v != "" {
			return v
		}
	}
	v, _ := rootCmd.PersistentFlags().GetString(name)
	return v
}

func flagBool(name string) bool {
	if rootCmd.PersistentFlags().Changed(name) {
		v, _ := rootCmd.PersistentFlags().GetBool(name)
		return v
	}
	if cfgViper != nil {
		if cfgViper.IsSet(name) {
			return cfgViper.GetBool(name)
		}
		if cfgViper.IsSet(strings.ReplaceAll(name, "-", "_")) {
			return cfgViper.GetBool(strings.ReplaceAll(name, "-", "_"))
		}
	}
	v, _ := rootCmd.PersistentFlags().GetBool(name)
	return v
}

func flagInt(name string) int {
	if rootCmd.PersistentFlags().Changed(name) {
		v, _ := rootCmd.PersistentFlags().GetInt(name)
		return v
	}
	if cfgViper != nil {
		if cfgViper.IsSet(name) {
			return cfgViper.GetInt(name)
		}
		if cfgViper.IsSet(strings.ReplaceAll(name, "-", "_")) {
			return cfgViper.GetInt(strings.ReplaceAll(name, "-", "_"))
		}
	}
	v, _ := rootCmd.PersistentFlags().GetInt(name)
	return v
}

// die prints a styled error to stderr and exits 1.
func die(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "guddit:", err)
	os.Exit(1)
}

// printBanner writes a small contextual header to stderr (kept off stdout so
// JSON/CSV output stays clean for pipes).
func printBanner(msg string) {
	if isStdoutTTY() {
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("─", 60))
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		fmt.Fprintf(os.Stderr, "%s\n\n", strings.Repeat("─", 60))
	}
}

var _ io.Writer = os.Stdout
var _ = http.StatusOK
