package render

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// Markdown renders a markdown string to ANSI-pretty text via glamour, choosing
// a dark or light stylesheet from the terminal's detected background. When
// color is disabled, returns the input unchanged (callers writing to a file or
// piping should get raw markdown).
func (t Theme) Markdown(md string) string {
	if md == "" {
		return ""
	}
	if !t.Caps.Color {
		return md
	}
	width := t.Width()
	if width < 40 {
		width = 80
	}
	theme := "dark"
	if !t.Caps.DarkBG {
		theme = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(theme),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return strings.TrimRight(out, "\n")
}
