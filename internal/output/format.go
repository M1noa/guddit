// Package output renders Reddit results in the user-chosen format:
// pretty, json, csv, table, markdown, txt. It is the format layer that sits
// above render (terminal features) and reddit (data). When stdout is not a TTY
// the default flips to json so pipes/ai get machine data.
package output

import (
	"fmt"
	"io"
	"os"

	"github.com/minoa/guddit/internal/render"
	"github.com/minoa/guddit/internal/reddit"
)

// Format selects the output shape.
type Format string

const (
	FormatPretty   Format = "pretty"
	FormatJSON     Format = "json"
	FormatCSV      Format = "csv"
	FormatAgent    Format = "agent"
	FormatTable    Format = "table"
	FormatMarkdown Format = "markdown"
	FormatTxt      Format = "txt"
)

// Parse resolves a format name, defaulting to pretty for TTY and json
// otherwise.
func Parse(s string, isTTY bool) Format {
	switch Format(s) {
	case FormatPretty, FormatJSON, FormatCSV, FormatAgent, FormatTable, FormatMarkdown, FormatTxt:
		return Format(s)
	case "":
		if isTTY {
			return FormatPretty
		}
		return FormatJSON
	}
	return Format(s) // let unknown through; defaulters above handle blanks
}

// Write renders result in the given format to w. result is one of the Reddit
// types ([]Post, *PostWithComments, *Subreddit, *User, *Me, *WikiPage,
// *KarmaBreakdown). Unknown types produce a json dump.
func Write(format Format, caps render.Capabilities, result any, w io.Writer, themeOpts *render.ThemeOptions, linkOpts render.LinkOptions) error {
	switch format {
	case FormatPretty:
		return writePretty(caps, result, w, themeOpts, linkOpts)
	case FormatJSON:
		return writeJSON(result, w)
	case FormatCSV:
		return writeCSV(result, w)
	case FormatAgent:
		return writeAgent(result, w)
	case FormatTable:
		return writeTable(result, w)
	case FormatMarkdown:
		return writeMarkdown(result, w, linkOpts)
	case FormatTxt:
		return writeTxt(result, w)
	default:
		return writeJSON(result, w)
	}
}

// Output resolves the destination: a file (--output path) or stdout. It opens
// the file for truncation+write and returns a closer.
func Output(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("output: open %s: %w", path, err)
	}
	return f, f.Close, nil
}

// Paginator wrapper carried alongside listings so the table/csv formats can
// show pagination cursors. The pretty renderer ignores it.
type withPaginator struct {
	posts     []reddit.Post
	paginator *reddit.Paginator
}

// WithPaginator tags a listing result with its paginator.
func WithPaginator(posts []reddit.Post, p *reddit.Paginator) any {
	return withPaginator{posts: posts, paginator: p}
}

// WithPaginatorer is implemented by results that carry a paginator.
type WithPaginatorer interface {
	Paginator() *reddit.Paginator
}

func (w withPaginator) Paginator() *reddit.Paginator { return w.paginator }

// PostsOf extracts the post list from a WithPaginator-wrapped result, a bare
// []Post, or a *PostWithComments. Returns true if posts were found.
func PostsOf(result any) ([]reddit.Post, bool) {
	switch v := result.(type) {
	case withPaginator:
		return v.posts, true
	case []reddit.Post:
		return v, true
	case *reddit.PostWithComments:
		return []reddit.Post{v.Post}, true
	}
	return nil, false
}
