package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/minoa/guddit/internal/reddit"
)

// writeTxt emits plain text: no ANSI color, no markdown glyphs, no links.
// Optimized for `guddit ... --format txt > file.txt` and for AI ingestion.
func writeTxt(result any, w io.Writer) error {
	switch v := result.(type) {
	case withPaginator:
		return writePostsTxt(w, v.posts)
	case []reddit.Post:
		return writePostsTxt(w, v)
	case *reddit.PostWithComments:
		if err := writePostsTxt(w, []reddit.Post{v.Post}); err != nil {
			return err
		}
		fmt.Fprintln(w)
		return writeCommentsTxt(w, v.Comments, 0)
	case []reddit.Comment:
		return writeCommentsTxt(w, v, 0)
	case *reddit.Subreddit:
		fmt.Fprintf(w, "r/%s\n", v.Data.Name)
		if v.Data.Title != "" {
			fmt.Fprintf(w, "%s\n", v.Data.Title)
		}
		fmt.Fprintf(w, "%d subscribers, %d active, created %s\n", v.Data.Subscribers, v.Data.ActiveUsers, ftoa(v.Data.CreatedUTC))
		if v.Data.Description != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, v.Data.Description)
		}
		return nil
	case *reddit.User:
		fmt.Fprintf(w, "u/%s\n", v.Data.Name)
		fmt.Fprintf(w, "%d link karma, %d comment karma, %d total, created %s\n",
			v.Data.LinkKarma, v.Data.CommentKarma, v.Data.TotalKarma, ftoa(v.Data.CreatedUTC))
		if v.Data.Subreddit.PublicDesc != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, v.Data.Subreddit.PublicDesc)
		}
		return nil
	case *reddit.Me:
		fmt.Fprintf(w, "u/%s\n", v.Name)
		fmt.Fprintf(w, "%d link karma, %d comment karma, %d total, created %s\n",
			v.LinkKarma, v.CommentKarma, v.TotalKarma, ftoa(v.CreatedUTC))
		return nil
	case *reddit.WikiPage:
		fmt.Fprintln(w, v.Data.ContentMD)
		return nil
	case *reddit.KarmaBreakdown:
		for _, sr := range v.Data.Subreddits {
			fmt.Fprintf(w, "r/%s: %d (comment %d, link %d)\n", sr.SR, sr.Karma, sr.CommentKarma, sr.LinkKarma)
		}
		return nil
	default:
		return writeJSON(result, w)
	}
}

func writePostsTxt(w io.Writer, posts []reddit.Post) error {
	for i := range posts {
		p := &posts[i]
		fmt.Fprintf(w, "%s\n", p.Title)
		fmt.Fprintf(w, "r/%s | u/%s | %d upvotes | %d comments | %s | %s\n",
			p.Subreddit, p.Author, p.Score, p.NumComments, ftoa(p.CreatedUTC), p.Domain)
		if p.IsSelf && p.Selftext != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, p.Selftext)
		} else if p.URL != "" && p.URL != p.Permalink {
			fmt.Fprintf(w, "%s\n", p.URL)
		}
		fmt.Fprintf(w, "%s\n", p.Permalink)
		if i < len(posts)-1 {
			fmt.Fprintln(w)
		}
	}
	return nil
}

func writeCommentsTxt(w io.Writer, comments []reddit.Comment, depth int) error {
	for i := range comments {
		c := &comments[i]
		if c.IsMore() {
			fmt.Fprintf(w, "%s[... %d more replies]\n", strings.Repeat("  ", depth), c.Data.Count)
			continue
		}
		indent := strings.Repeat("  ", depth)
		author := c.Data.Author
		if author == "" {
			author = "[deleted]"
		}
		fmt.Fprintf(w, "%su/%s | %d points\n", indent, author, c.Data.Score)
		fmt.Fprintf(w, "%s%s\n", indent, c.Data.Body)
		fmt.Fprintln(w)
		if err := writeCommentsTxt(w, c.Replies, depth+1); err != nil {
			return err
		}
	}
	return nil
}
