package render

import (
	"github.com/charmbracelet/lipgloss"
)

// ThemeOptions holds customizable theme colors from config.
type ThemeOptions struct {
	Primary      string // main accent color (titles, links)
	Secondary    string // secondary color (subtitles, meta)
	Accent       string // accent color (scores, highlights)
	Muted        string // muted color (dimmed text)
	Bg           string // background color
	Fg           string // foreground color
	CustomDomain string // custom domain for links when link_domain=custom
}

// Theme holds the lipgloss styles used across renderers. When color is
// disabled, all styles are zero-value no-ops, so renderers can call them
// unconditionally without producing ANSI codes.
type Theme struct {
	Caps     Capabilities
	Opts     ThemeOptions
	LinkOpts LinkOptions

	// Title is a bold, slightly emphatic heading.
	Title lipgloss.Style
	// Subtitle is a dimmed secondary line.
	Subtitle lipgloss.Style
	// Meta is small meta text (scores, times, counts).
	Meta lipgloss.Style
	// Author colors the username.
	Author lipgloss.Style
	// Subreddit colors the r/x prefix.
	Subreddit lipgloss.Style
	// Flair colors post/user flair.
	Flair lipgloss.Style
	// URL is a plain (non-linked) URL style.
	URL lipgloss.Style
	// Score highlights positive scores.
	Score lipgloss.Style
	// CommentScore for comment scores.
	CommentScore lipgloss.Style
	// Stickied / distinguished markers.
	Stickied lipgloss.Style
	Mod       lipgloss.Style
	Admin     lipgloss.Style
	NSFW      lipgloss.Style
	Spoiler   lipgloss.Style
	// Border for cards.
	Border lipgloss.Style
	// Error / warn.
	Error lipgloss.Style
	Warn  lipgloss.Style
	// Indent per depth for comment trees.
	Indent string
	// Tree branch chars.
	Branch string
}

// DefaultColors returns the default color values based on dark/light bg.
// Reddit-branded: orange primary, blue accent.
func DefaultColors(darkBG bool) (primary, secondary, accent, muted, bg, fg string) {
	c := Capabilities{DarkBG: darkBG}
	return defaultThemeColors(c)
}

// defaultThemeColors returns the default color values based on dark/light bg.
// When the terminal's actual fg/bg colors are known (via OSC 10/11 queries),
// those are used directly, giving a theme that matches the user's terminal
// (e.g. Ghostty's palette) rather than a hardcoded orange. The accent is
// Reddit-orange (brand identity) but primary/secondary/muted/bg/fg follow
// the terminal.
func defaultThemeColors(c Capabilities) (primary, secondary, accent, muted, bg, fg string) {
	// Accent is always Reddit-orange (brand identity).
	if c.DarkBG {
		accent = "117" // bright blue on dark
	} else {
		accent = "25" // blue on light
	}

	// If we queried the terminal's actual fg/bg, use them.
	if c.BgColor != "" {
		bg = c.BgColor
	}
	if c.FgColor != "" {
		fg = c.FgColor
	}

	// Primary: Reddit-orange for titles/links (brand identity).
	if c.DarkBG {
		primary = "208" // bright orange
		secondary = "250"
		muted = "245"
		if bg == "" {
			bg = "235"
		}
		if fg == "" {
			fg = "255"
		}
	} else {
		primary = "166" // darker orange
		secondary = "238"
		muted = "240"
		if bg == "" {
			bg = "255"
		}
		if fg == "" {
			fg = "234"
		}
	}
	return
}

// NewTheme builds a Theme, applying color only when the terminal supports it.
// opts can be nil to use defaults.
func NewTheme(c Capabilities, opts *ThemeOptions) Theme {
	t := Theme{Caps: c, Indent: "  ", Branch: "└ ", Opts: defaultOpts(opts)}
	if !c.Color {
		return t // all styles stay zero-value no-ops
	}
	primary, secondary, accent, muted, bg, fg := defaultThemeColors(c)
	if t.Opts.Primary != "" {
		primary = t.Opts.Primary
	}
	if t.Opts.Secondary != "" {
		secondary = t.Opts.Secondary
	}
	if t.Opts.Accent != "" {
		accent = t.Opts.Accent
	}
	if t.Opts.Muted != "" {
		muted = t.Opts.Muted
	}
	if t.Opts.Bg != "" {
		bg = t.Opts.Bg
	}
	if t.Opts.Fg != "" {
		fg = t.Opts.Fg
	}

	t.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(primary))
	t.Subtitle = lipgloss.NewStyle().Foreground(lipgloss.Color(secondary))
	t.Meta = lipgloss.NewStyle().Foreground(lipgloss.Color(muted))
	t.Author = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent))
	t.Subreddit = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(primary))
	t.Flair = lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg))
	t.URL = lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Underline(true)
	t.Score = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("green"))
	t.CommentScore = lipgloss.NewStyle().Foreground(lipgloss.Color(accent))
	t.Stickied = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("green"))
	t.Mod = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color(primary)).Foreground(lipgloss.Color(fg))
	t.Admin = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("124")).Foreground(lipgloss.Color(fg))
	t.NSFW = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("124")).Foreground(lipgloss.Color(fg))
	t.Spoiler = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color(muted)).Foreground(lipgloss.Color(fg))
	t.Border = lipgloss.NewStyle().BorderForeground(lipgloss.Color(muted))
	t.Error = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("red"))
	t.Warn = lipgloss.NewStyle().Foreground(lipgloss.Color("yellow"))
	return t
}

func defaultOpts(opts *ThemeOptions) ThemeOptions {
	if opts == nil {
		return ThemeOptions{}
	}
	return *opts
}

// Width returns the available column width, falling back to 80.
func (t Theme) Width() int {
	if t.Caps.Width > 0 {
		return t.Caps.Width
	}
	return 80
}
