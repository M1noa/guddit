package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/minoa/guddit/internal/reddit"
)

// writeTable emits aligned columns for posts and comments. Width is not used —
// columns size to content, which keeps tables intact when piped.
func writeTable(result any, w io.Writer) error {
	switch v := result.(type) {
	case withPaginator:
		return writePostsTable(w, v.posts)
	case []reddit.Post:
		return writePostsTable(w, v)
	case *reddit.PostWithComments:
		if err := writePostsTable(w, []reddit.Post{v.Post}); err != nil {
			return err
		}
		return writeCommentsTable(w, v.Comments, 0)
	case []reddit.Comment:
		return writeCommentsTable(w, v, 0)
	case *reddit.Subreddit:
		return writeKeyValueTable(w, []kv{
			{"name", v.Data.Name}, {"title", v.Data.Title},
			{"subscribers", strconv.Itoa(v.Data.Subscribers)},
			{"active", strconv.Itoa(v.Data.ActiveUsers)},
			{"created", ftoa(v.Data.CreatedUTC)},
			{"nsfw", strconv.FormatBool(v.Data.Over18)},
			{"type", v.Data.SubredditType},
			{"description", v.Data.Description},
		})
	case *reddit.User:
		return writeKeyValueTable(w, []kv{
			{"name", v.Data.Name},
			{"link_karma", strconv.Itoa(v.Data.LinkKarma)},
			{"comment_karma", strconv.Itoa(v.Data.CommentKarma)},
			{"total_karma", strconv.Itoa(v.Data.TotalKarma)},
			{"created", ftoa(v.Data.CreatedUTC)},
			{"is_gold", strconv.FormatBool(v.Data.IsGold)},
			{"is_mod", strconv.FormatBool(v.Data.IsMod)},
			{"desc", v.Data.Subreddit.PublicDesc},
		})
	case *reddit.Me:
		return writeKeyValueTable(w, []kv{
			{"name", v.Name},
			{"link_karma", strconv.Itoa(v.LinkKarma)},
			{"comment_karma", strconv.Itoa(v.CommentKarma)},
			{"total_karma", strconv.Itoa(v.TotalKarma)},
			{"created", ftoa(v.CreatedUTC)},
			{"is_gold", strconv.FormatBool(v.IsGold)},
			{"is_mod", strconv.FormatBool(v.IsMod)},
		})
	case *reddit.KarmaBreakdown:
		return writeRows(w, []string{"subreddit", "karma", "comment_karma", "link_karma"}, func(emit func([]string) error) error {
			for _, sr := range v.Data.Subreddits {
				if err := emit([]string{sr.SR, strconv.Itoa(sr.Karma), strconv.Itoa(sr.CommentKarma), strconv.Itoa(sr.LinkKarma)}); err != nil {
					return err
				}
			}
			return nil
		})
	case *reddit.WikiPage:
		fmt.Fprintln(w, v.Data.ContentMD)
		return nil
	default:
		return writeJSON(result, w)
	}
}

type kv struct{ K, V string }

func writeKeyValueTable(w io.Writer, rows []kv) error {
	width := 0
	for _, r := range rows {
		if c := utf8.RuneCountInString(r.K); c > width {
			width = c
		}
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-*s  %s\n", width, r.K, r.V)
	}
	return nil
}

var postTableHeader = []string{"score", "cmts", "sub", "title", "author", "time"}

func writePostsTable(w io.Writer, posts []reddit.Post) error {
	rows := make([][]string, 0, len(posts))
	for i := range posts {
		p := &posts[i]
		title := p.Title
		if len([]rune(title)) > 60 {
			title = string([]rune(title)[:60]) + "…"
		}
		rows = append(rows, []string{
			strconv.Itoa(p.Score), strconv.Itoa(p.NumComments),
			"r/" + p.Subreddit, title, "u/" + p.Author,
			relativeTime(p.CreatedUTC),
		})
	}
	return writeRows(w, postTableHeader, func(emit func([]string) error) error {
		for _, r := range rows {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	})
}

var commentTableHeader = []string{"score", "depth", "author", "body"}

func writeCommentsTable(w io.Writer, comments []reddit.Comment, depth int) error {
	return writeRows(w, commentTableHeader, func(emit func([]string) error) error {
		var rec func([]reddit.Comment, int) error
		rec = func(cs []reddit.Comment, d int) error {
			for i := range cs {
				c := &cs[i]
				if c.IsMore() {
					if err := emit([]string{"", strconv.Itoa(d), "", fmt.Sprintf("…load %d more", c.Data.Count)}); err != nil {
						return err
					}
					continue
				}
				body := strings.ReplaceAll(c.Data.Body, "\n", " ")
				if len([]rune(body)) > 80 {
					body = string([]rune(body)[:80]) + "…"
				}
				if err := emit([]string{
					strconv.Itoa(c.Data.Score), strconv.Itoa(d),
					"u/" + c.Data.Author, body,
				}); err != nil {
					return err
				}
				if err := rec(c.Replies, d+1); err != nil {
					return err
				}
			}
			return nil
		}
		return rec(comments, depth)
	})
}

// writeRows prints an aligned column table with vertical borders.
func writeRows(w io.Writer, header []string, body func(emit func([]string) error) error) error {
	cols := len(header)
	widths := make([]int, cols)
	for i, h := range header {
		widths[i] = utf8.RuneCountInString(h)
	}
	var collected [][]string
	collect := func(row []string) error {
		collected = append(collected, row)
		for i, cell := range row {
			if i < cols {
				if c := utf8.RuneCountInString(cell); c > widths[i] {
					widths[i] = c
				}
			}
		}
		return nil
	}
	if err := body(collect); err != nil {
		return err
	}
	writeRow := func(row []string) {
		var b strings.Builder
		b.WriteString("│ ")
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(cell)
			pad := widths[i] - utf8.RuneCountInString(cell)
			if pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
			b.WriteString(" │ ")
		}
		fmt.Fprintln(w, b.String())
	}
	// Header
	writeRow(header)
	// Separator
	sep := make([]string, cols)
	for i := range sep {
		sep[i] = strings.Repeat("─", widths[i])
	}
	writeRow(sep)
	// Data rows
	for _, r := range collected {
		writeRow(r)
	}
	return nil
}

// relativeTime is a local copy to avoid importing render here.
func relativeTime(utc float64) string {
	if utc <= 0 {
		return ""
	}
	d := time.Since(time.Unix(int64(utc), 0))
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}
