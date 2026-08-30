// Package render detects terminal capabilities and provides the pretty
// renderers (color, OSC 8 hyperlinks, markdown) for Reddit listings, posts,
// comments, subreddits and users. Output formatting (json/csv/table/etc.) lives
// in the sibling output package.
package render

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Capabilities describes what the current output terminal supports.
type Capabilities struct {
	Out        io.Writer
	IsTTY      bool
	Color      bool   // emit ANSI color at all
	TrueColor  bool   // 24-bit color
	Hyperlinks bool   // OSC 8 clickable links
	DarkBG     bool   // true if terminal background is dark
	Width      int    // columns (0 if unknown)
	Height     int    // rows
	// Image support
	Images         bool // any image protocol supported
	ImageProtocol  ImageProtocol // which protocol
	// TerminalName is the detected terminal program (ghostty, iTerm.app, etc.)
	TerminalName string
	// BgColor is the terminal's background color as "#rrggbb" (empty if unknown)
	BgColor string
	// FgColor is the terminal's foreground color as "#rrggbb" (empty if unknown)
	FgColor string
}

type ImageProtocol int

const (
	ImageProtocolNone ImageProtocol = iota
	ImageProtocolITerm2
	ImageProtocolKitty
	ImageProtocolSixel
)

// Detect probes the terminal backing w (defaults to os.Stdout). noColor and
// noLinks force-disable features regardless of detection.
func Detect(w io.Writer, noColor, noLinks bool) Capabilities {
	if w == nil {
		w = os.Stdout
	}
	cap := Capabilities{Out: w}
	if f, ok := w.(*os.File); ok {
		cap.IsTTY = term.IsTerminal(int(f.Fd()))
		if w, h, err := term.GetSize(int(f.Fd())); err == nil {
			cap.Width, cap.Height = w, h
		}
	}
	// Respect CLICOLOR_FORCE=1 even when not a TTY (e.g. piped to a pager that
	// can render color). Otherwise color requires a TTY.
	if os.Getenv("CLICOLOR_FORCE") != "0" {
		cap.Color = cap.IsTTY || os.Getenv("CLICOLOR_FORCE") != ""
	}
	cap.TrueColor = supportsTrueColor()

	// OSC 8 hyperlinks: widely adopted in modern terminals.
	// https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda
	if cap.IsTTY && !noLinks {
		cap.Hyperlinks = supportsHyperlinks()
	}

	// Force-disable.
	if noColor {
		cap.Color = false
		cap.TrueColor = false
	}
	if noLinks {
		cap.Hyperlinks = false
	}

	// Image protocol detection: only when TTY (inline images need a real
	// terminal; piped output must stay text).
	if cap.IsTTY && os.Getenv("GUDDIT_NO_IMAGES") == "" {
		cap.ImageProtocol, cap.Images = detectImageProtocol()
	}

	// Background: COLORFGBG is "fg;bg" with 0=black..15=white. Default dark.
	// lipgloss itself also queries the terminal; we keep this env-based guess
	// as a stable cross-version fallback.
	cap.DarkBG = true
	cap.TerminalName = os.Getenv("TERM_PROGRAM")
	if fgBg := os.Getenv("COLORFGBG"); fgBg != "" {
		parts := strings.Split(fgBg, ";")
		if len(parts) >= 2 {
			if bg, err := atoiSafe(parts[1]); err == nil && bg >= 7 {
				cap.DarkBG = false // light background
			}
		}
	}
	// Query the terminal for its actual background + foreground colors (OSC 11/10).
	// Overrides the env guess when the terminal answers. Both queries and the
	// drain run under one raw-mode session: replies can straddle termios
	// windows, so anything arriving late is consumed before echo re-enables
	// (otherwise it lands in the shell's input queue and fish prints it as
	// "^[]11;rgb:…^G" slop).
	if cap.IsTTY && !noColor {
		detectColors(&cap)
	}
	return cap
}

// detectColors runs the OSC 11/10 queries plus an input drain inside a single
// raw-mode window so no response bytes are ever left echoed or queued.
func detectColors(cap *Capabilities) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return
	}
	defer term.Restore(fd, old)

	// Pre-drain: the shell (fish) fires its own OSC 11 query right before
	// exec'ing us, and lipgloss/termenv's package-init query can desync on it,
	// leaving replies queued before we send anything.
	drainStdin(fd, 80*time.Millisecond)

	cap.BgColor = queryTerminalColor(cap.Out, "11")
	if cap.BgColor != "" {
		cap.DarkBG = !isLightColor(cap.BgColor)
	}
	cap.FgColor = queryTerminalColor(cap.Out, "10")

	// Drain late/stray replies (ours or ones leaked by lipgloss/termenv at
	// package init) while ECHO is still off. ~150ms of silence = done.
	drainStdin(fd, 150*time.Millisecond)
}

// drainStdin reads stdin until quiet for d or 2s elapse, discarding bytes.
func drainStdin(fd int, d time.Duration) {
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return
	}
	unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags|unix.O_NONBLOCK)
	defer unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags)

	buf := make([]byte, 256)
	quiet := time.Now().Add(d)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m, _ := unix.Read(fd, buf)
		if m > 0 {
			quiet = time.Now().Add(d)
			continue
		}
		if time.Now().After(quiet) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// queryTerminalColor asks the terminal for a color (bg=11, fg=10) and returns
// "#rrggbb", or "" if it doesn't answer within ~600ms. Caller must hold the
// tty in raw mode (see detectColors).
func queryTerminalColor(w io.Writer, osc string) string {
	if w == nil {
		w = os.Stdout
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return ""
	}

	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return ""
	}
	unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags|unix.O_NONBLOCK)
	defer unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags)

	fmt.Fprintf(w, "\x1b]%s;?\x07", osc)

	buf := make([]byte, 256)
	n := 0
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) && n < len(buf) {
		m, _ := unix.Read(fd, buf[n:n+1])
		if m == 1 {
			n++
			if bytes.Contains(buf[:n], []byte(";rgb:")) ||
				bytes.Contains(buf[:n], []byte("SetBackgroundColor")) ||
				bytes.Contains(buf[:n], []byte("SetForegroundColor")) {
				for time.Now().Before(deadline) && n < len(buf) {
					m, _ := unix.Read(fd, buf[n:n+1])
					if m == 1 {
						n++
						if buf[n-1] == '\\' || buf[n-1] == 0x07 {
							return parseBGReply(buf[:n])
						}
					}
				}
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ""
}

// parseBGReply extracts a hex color from an OSC 11 reply ("rgb:rrrr/gggg/bbbb"
// or "rgb:rr/gg/bb") or an iTerm2 reply ("SetBackgroundColor=r,g,b").
func parseBGReply(b []byte) string {
	if i := bytes.Index(b, []byte(";rgb:")); i >= 0 {
		parts := strings.SplitN(string(b[i+5:]), "/", 3)
		if len(parts) != 3 {
			return ""
		}
		hex := "#"
		for _, p := range parts {
			p = strings.TrimRight(p, "\x1b\\\x07\n\r ")
			p = strings.TrimSpace(p)
			// "ffff" -> "ff"; "f" -> "0f"
			if len(p) >= 2 {
				hex += p[:2]
			} else if len(p) == 1 {
				hex += "0" + p
			} else {
				return ""
			}
		}
		return hex
	}
	if i := bytes.Index(b, []byte("SetBackgroundColor=")); i >= 0 {
		parts := strings.SplitN(string(b[i+len("SetBackgroundColor="):]), ",", 3)
		if len(parts) == 3 {
			hex := "#"
			for _, p := range parts {
				p = strings.TrimRight(p, "\x1b\\\x07\n\r ")
				n, err := atoiSafe(strings.TrimSpace(p))
				if err != nil || n > 255 {
					return ""
				}
				hex += fmt.Sprintf("%02x", n)
			}
			return hex
		}
	}
	return ""
}

func atoiSafe(s string) (int, error) {
	n := 0
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			return 0, errNotInt
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var errNotInt = errors.New("not an integer")

func supportsTrueColor() bool {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return true
	}
	return false
}

// supportsHyperlinks reports OSC 8 support via TERM_PROGRAM and a known set of
// terminals that advertise it.
func supportsHyperlinks() bool {
	prog := os.Getenv("TERM_PROGRAM")
	switch prog {
	case "iTerm.app", "WezTerm", "ghostty", "kitty", "mintty", "fish":
		return true
	}
	// GNOME Terminal, Windows Terminal, konsole, etc. also support OSC 8 but
	// don't always set TERM_PROGRAM. Fall back to a positive default when
	// running in a TTY — OSC 8 is a no-op where unsupported.
	return true
}

// detectImageProtocol picks the best inline-image protocol the terminal
// advertises via environment. Precedence: iTerm2 > Kitty > sixel.
func detectImageProtocol() (ImageProtocol, bool) {
	prog := os.Getenv("TERM_PROGRAM")
	kittyProg := os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty"
	// Ghostty supports Kitty graphics protocol natively
	isGhostty := prog == "ghostty"
	switch prog {
	case "iTerm.app":
		// iTerm2 inline images: old (protocol) and new (file-based) both OK.
		return ImageProtocolITerm2, true
	case "WezTerm":
		// WezTerm supports iTerm2 protocol.
		return ImageProtocolITerm2, true
	case "ghostty", "kitty":
		// Ghostty and Kitty both support Kitty graphics protocol.
		return ImageProtocolKitty, true
	}
	if kittyProg || isGhostty {
		return ImageProtocolKitty, true
	}
	// sixel: TERM=*-sixel or explicit env, plus xterm w/ sixel support.
	if strings.Contains(os.Getenv("TERM"), "sixel") || os.Getenv("WT_SESSION") != "" {
		return ImageProtocolSixel, true
	}
	return ImageProtocolNone, false
}

// isLightColor makes a crude luminance guess from a hex color like "#rrggbb".
func isLightColor(hex string) bool {
	h := strings.TrimPrefix(hex, "#")
	if len(h) < 6 {
		return false
	}
	r := hexByte(h[0:2])
	g := hexByte(h[2:4])
	b := hexByte(h[4:6])
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	return lum > 128
}

func hexByte(s string) int {
	var n int
	for _, c := range s {
		n <<= 4
		switch {
		case c >= '0' && c <= '9':
			n |= int(c - '0')
		case c >= 'a' && c <= 'f':
			n |= int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n |= int(c-'A') + 10
		}
	}
	return n
}
