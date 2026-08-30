package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/minoa/guddit/internal/reddit"
	"github.com/minoa/guddit/internal/render"
)

// writeMarkdown exports Reddit content as Reddit-flavored markdown, suitable
// for piping into another tool or saving to a .md file.
func writeMarkdown(result any, w io.Writer, linkOpts render.LinkOptions) error {
	switch v := result.(type) {
	case withPaginator:
		return writePostsMarkdown(w, v.posts, linkOpts)
	case []reddit.Post:
		return writePostsMarkdown(w, v, linkOpts)
	case *reddit.PostWithComments:
		return writeThreadMarkdown(w, v, linkOpts)
	case *reddit.Subreddit:
		fmt.Fprintf(w, "# r/%s\n\n", v.Data.Name)
		if v.Data.Title != "" {
			fmt.Fprintf(w, ">%s\n\n", v.Data.Title)
		}
		fmt.Fprintf(w, "**%d subscribers** · %d active · created %s\n\n",
			v.Data.Subscribers, v.Data.ActiveUsers, ftoa(v.Data.CreatedUTC))
		if v.Data.Over18 {
			fmt.Fprintln(w, "**NSFW**")
		}
		if v.Data.Description != "" {
			fmt.Fprintln(w, v.Data.Description)
		}
		return nil
	case *reddit.User:
		fmt.Fprintf(w, "# u/%s\n\n", v.Data.Name)
		fmt.Fprintf(w, "**%d link karma** · **%d comment karma** · **%d total** · created %s\n\n",
			v.Data.LinkKarma, v.Data.CommentKarma, v.Data.TotalKarma, ftoa(v.Data.CreatedUTC))
		if v.Data.Subreddit.PublicDesc != "" {
			fmt.Fprintln(w, v.Data.Subreddit.PublicDesc)
		}
		return nil
	case *reddit.Me:
		fmt.Fprintf(w, "# u/%s (you)\n\n", v.Name)
		fmt.Fprintf(w, "**%d link karma** · **%d comment karma** · **%d total** · created %s\n\n",
			v.LinkKarma, v.CommentKarma, v.TotalKarma, ftoa(v.CreatedUTC))
		return nil
	case *reddit.WikiPage:
		fmt.Fprintf(w, "# %s\n\n", "wiki page")
		fmt.Fprintln(w, v.Data.ContentMD)
		return nil
	case *reddit.KarmaBreakdown:
		fmt.Fprintln(w, "| subreddit | karma | comment | link |")
		fmt.Fprintln(w, "|---|---|---|---|")
		for _, sr := range v.Data.Subreddits {
			fmt.Fprintf(w, "| r/%s | %d | %d | %d |\n", sr.SR, sr.Karma, sr.CommentKarma, sr.LinkKarma)
		}
		return nil
	default:
		return writeJSON(result, w)
	}
}

func writePostsMarkdown(w io.Writer, posts []reddit.Post, linkOpts render.LinkOptions) error {
	for i := range posts {
		p := &posts[i]
		fmt.Fprintf(w, "## %s\n\n", p.Title)
		fmt.Fprintf(w, "r/%s · u/%s · %d ↑ · %d 💬 · %s\n\n",
			p.Subreddit, p.Author, p.Score, p.NumComments, ftoa(p.CreatedUTC))
		if p.LinkFlairText != "" {
			fmt.Fprintf(w, "Flair: %s  \n", p.LinkFlairText)
		}
		if p.IsSelf && p.Selftext != "" {
			fmt.Fprintln(w, p.Selftext)
		} else if p.URL != "" {
			fmt.Fprintf(w, "[%s](%s)\n", p.Domain, p.URL)
		}
		// Apply link format to permalink
		permalink := formatPermalink(p.Permalink, linkOpts)
		fmt.Fprintf(w, "[permalink](%s)\n\n", permalink)
		if i < len(posts)-1 {
			fmt.Fprintln(w, "---")
		}
	}
	return nil
}

func formatPermalink(permalink string, opts render.LinkOptions) string {
	if permalink == "" {
		return ""
	}
	if strings.HasPrefix(permalink, "http") {
		// Extract ID and apply format
		id := extractPostID(permalink)
		if id != "" {
			return formatPermalinkID(id, opts)
		}
		return permalink
	}
	if !strings.HasPrefix(permalink, "/") {
		permalink = "/" + permalink
	}
	// Apply format to relative permalink
	id := extractPostID(permalink)
	if id != "" {
		return formatPermalinkID(id, opts)
	}
	return "https://www.reddit.com" + permalink
}

func formatPermalinkID(id string, opts render.LinkOptions) string {
	domain := getMarkdownDomain(opts)
	switch opts.Format {
	case render.LinkFormatShort:
		return id
	case render.LinkFormatMedium:
		return "https://redd.it/" + id
	case render.LinkFormatLong:
		return "https://" + domain + "/comments/" + id
	default:
		return "https://www.reddit.com/comments/" + id
	}
}

func getMarkdownDomain(opts render.LinkOptions) string {
	switch opts.Domain {
	case render.LinkDomainWWW:
		return "www.reddit.com"
	case render.LinkDomainCustom:
		if opts.CustomDomain != "" {
			return opts.CustomDomain
		}
		return "old.reddit.com"
	default:
		return "old.reddit.com"
	}
}

func extractPostID(url string) string {
	// Try to extract from /comments/ID/ pattern
	if idx := strings.Index(url, "/comments/"); idx != -1 {
		start := idx + len("/comments/")
		end := start
		for end < len(url) && url[end] != '/' {
			end++
		}
		if end > start {
			return url[start:end]
		}
	}
	
	// Try redd.it/ID
	if idx := strings.Index(url, "redd.it/"); idx != -1 {
		start := idx + len("redd.it/")
		end := start
		for end < len(url) && url[end] != '/' && url[end] != '?' && url[end] != '#' {
			end++
		}
		if end > start {
			return url[start:end]
		}
	}
	
	// If it looks like a bare ID (alphanumeric, 5-8 chars)
	url = strings.TrimSpace(url)
	if len(url) >= 5 && len(url) <= 8 {
		isID := true
		for _, c := range url {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				isID = false
				break
			}
		}
		if isID {
			return url
		}
	}
	
	return ""
}

func writeThreadMarkdown(w io.Writer, t *reddit.PostWithComments, linkOpts render.LinkOptions) error {
	if err := writePostsMarkdown(w, []reddit.Post{t.Post}, linkOpts); err != nil {
		return err
	}
	fmt.Fprintf(w, "## Comments\n\n")
	return writeCommentsMarkdown(w, t.Comments, 0, linkOpts)
}

func writeCommentsMarkdown(w io.Writer, comments []reddit.Comment, depth int, linkOpts render.LinkOptions) error {
	for i := range comments {
		c := &comments[i]
		if c.IsMore() {
			fmt.Fprintf(w, "%s* …load %d more\n", strings.Repeat("  ", depth), c.Data.Count)
			continue
		}
		prefix := strings.Repeat("  ", depth)
		author := c.Data.Author
		if author == "" {
			author = "[deleted]"
		}
		OP := ""
		if c.Data.IsSubmitter {
			OP = " *(OP)*"
		}
		fmt.Fprintf(w, "%s* **u/%s**%s · %d ↑\n\n", prefix, author, OP, c.Data.Score)
		for _, line := range strings.Split(strings.TrimSpace(c.Data.Body), "\n") {
			fmt.Fprintf(w, "%s  %s\n", prefix, line)
		}
		fmt.Fprintln(w)
		if err := writeCommentsMarkdown(w, c.Replies, depth+1, linkOpts); err != nil {
			return err
		}
	}
	return nil
}