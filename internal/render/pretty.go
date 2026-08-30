package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/minoa/guddit/internal/reddit"
)

// Posts renders a listing of posts as a series of cards.
func (t Theme) Posts(posts []reddit.Post) string {
	var b strings.Builder
	for i := range posts {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(t.PostCard(&posts[i]))
	}
	return b.String()
}

// PostCard renders a single post as a compact card.
func (t Theme) PostCard(p *reddit.Post) string {
	return t.postCard(p, 140)
}

// postCard renders a post card. thumbW > 0 renders a small inline thumbnail;
// PostFull passes -1 so it can render the full-size image itself.
func (t Theme) postCard(p *reddit.Post, thumbW int) string {
	var b strings.Builder

	// Title line, clickable if hyperlinks are available.
	title := p.Title
	linkOpts := t.LinkOpts
	if t.Caps.Hyperlinks {
		title = t.LinkWithOptions(p.Title, t.PermalinkURL(p.Permalink, linkOpts), linkOpts)
	}
	b.WriteString(t.Title.Render(title))
	b.WriteString("\n")

	// Flair + badges row.
	var badges []string
	if p.LinkFlairText != "" {
		badges = append(badges, t.Flair.Render(p.LinkFlairText))
	}
	if p.Over18 {
		badges = append(badges, t.NSFW.Render("NSFW"))
	}
	if p.Spoiler {
		badges = append(badges, t.Spoiler.Render("SPOILER"))
	}
	if p.Stickied {
		badges = append(badges, t.Stickied.Render("📌 stickied"))
	}
	if p.Locked {
		badges = append(badges, t.Meta.Render("🔒 locked"))
	}
	if p.Archived {
		badges = append(badges, t.Meta.Render("archived"))
	}
	if p.IsOriginalContent {
		badges = append(badges, t.Meta.Render("OC"))
	}
	if len(badges) > 0 {
		b.WriteString(strings.Join(badges, " "))
		b.WriteString("\n")
	}

	// Meta line: score • comments • r/sub • u/author • time • domain
	meta := fmt.Sprintf("▲ %d  💬 %d  %s  %s  %s  %s",
		p.Score, p.NumComments,
		t.Subreddit.Render("r/"+p.Subreddit),
		t.Author.Render("u/"+p.Author),
		relativeTime(p.CreatedUTC),
		t.Meta.Render(p.Domain),
	)
	b.WriteString(meta)
	b.WriteString("\n")

	// URL line: link to the post's external url if not a self post.
	if !p.IsSelf && p.URL != "" {
		url := p.URL
		if t.Caps.Hyperlinks {
			url = t.Link(p.URL, p.URL)
		} else {
			url = t.URL.Render(p.URL)
		}
		b.WriteString(url)
		b.WriteString("\n")
	}

	// Permalink line
	permalink := t.PermalinkURL(p.Permalink, linkOpts)
	if t.Caps.Hyperlinks {
		b.WriteString(t.LinkWithOptions("permalink", permalink, linkOpts))
	} else {
		b.WriteString(permalink)
	}
	b.WriteString("\n")

	// Inline thumbnail when the terminal supports images.
	if thumbW > 0 && t.Caps.Images {
		if u := p.MediaImageURL(thumbW); u != "" {
			if err := RenderImage(&b, t.Caps, u, thumbW, 0); err == nil {
				b.WriteString("\n")
			}
		}
	}

	// Selftext preview (first few lines) — only the first ~300 chars; full
	// body is shown by PostFull.
	if p.IsSelf && p.Selftext != "" {
		preview := p.Selftext
		if len(preview) > 300 {
			preview = preview[:300] + "…"
		}
		b.WriteString(t.Meta.Render(preview))
		b.WriteString("\n")
	}
	return b.String()
}

// PostFull renders a post plus its comment tree.
func (t Theme) PostFull(thread *reddit.PostWithComments, depth int) string {
	var b strings.Builder
	b.WriteString(t.postCard(&thread.Post, -1))
	b.WriteString("\n")

	// Full-size inline image(s) when the terminal supports them.
	if t.Caps.Images {
		widthPx := (t.Caps.Width - 8) * 9
		if widthPx < 200 {
			widthPx = 200
		}
		if urls := thread.Post.GalleryImageURLs(widthPx); len(urls) > 1 {
			for _, u := range urls {
				if err := RenderImage(&b, t.Caps, u, widthPx, 0); err == nil {
					b.WriteString("\n")
				}
			}
		} else if u := thread.Post.MediaImageURL(widthPx); u != "" {
			if err := RenderImage(&b, t.Caps, u, widthPx, 0); err == nil {
				b.WriteString("\n")
			}
		}
	}

	if thread.Post.IsSelf && thread.Post.Selftext != "" {
		b.WriteString(t.Markdown(thread.Post.Selftext))
		b.WriteString("\n\n")
	}
	if len(thread.Comments) == 0 {
		b.WriteString(t.Meta.Render("no comments"))
		return b.String()
	}
	b.WriteString(t.Subtitle.Render(fmt.Sprintf("%d comments (showing depth %d)", thread.Post.NumComments, depth)))
	b.WriteString("\n\n")
	maxDepth := depth
	if maxDepth <= 0 {
		maxDepth = 6
	}
	for i := range thread.Comments {
		b.WriteString(t.Comment(&thread.Comments[i], 0, maxDepth))
	}
	return b.String()
}

// Comment renders one comment and its recursive replies, indented by depth.
func (t Theme) Comment(c *reddit.Comment, depth, maxDepth int) string {
	if c == nil {
		return ""
	}
	// "more" placeholder.
	if c.IsMore() {
		indent := strings.Repeat(t.Indent, depth)
		return indent + t.Meta.Render(fmt.Sprintf("▸ load %d more replies", c.Data.Count)) + "\n"
	}
	var b strings.Builder
	indent := strings.Repeat(t.Indent, depth)

	// Author + score + markers.
	author := c.Data.Author
	if author == "" {
		author = "[deleted]"
	}
	authorStr := t.Author.Render("u/" + author)
	if c.Data.IsSubmitter {
		authorStr += " " + t.Subreddit.Render("OP")
	}
	if c.Data.Distinguished == "moderator" {
		authorStr += " " + t.Mod.Render("M")
	} else if c.Data.Distinguished == "admin" {
		authorStr += " " + t.Admin.Render("A")
	}
	if c.Data.Stickied {
		authorStr += " " + t.Stickied.Render("📌")
	}
	score := t.CommentScore.Render(fmt.Sprintf("%d", c.Data.Score))
	if c.Data.Gilded > 0 {
		score += " " + t.Warn.Render(fmt.Sprintf("★×%d", c.Data.Gilded))
	}
	fmt.Fprintf(&b, "%s%s • %s • %s\n", indent, authorStr, score, relativeTime(c.Data.CreatedUTC))

	// Body (markdown rendered). Indent each body line.
	body := c.Data.Body
	if body == "" {
		body = c.Data.BodyHTML
	}
	if t.Caps.Color {
		body = t.Markdown(body)
	}
	for _, line := range strings.Split(body, "\n") {
		b.WriteString(indent)
		b.WriteString(t.Indent)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Recurse into replies up to maxDepth.
	if depth+1 < maxDepth {
		for i := range c.Replies {
			b.WriteString(t.Comment(&c.Replies[i], depth+1, maxDepth))
		}
	} else if len(c.Replies) > 0 {
		b.WriteString(strings.Repeat(t.Indent, depth+1))
		b.WriteString(t.Meta.Render(fmt.Sprintf("… %d more replies (use --depth %d)", len(c.Replies), depth+2)))
		b.WriteString("\n")
	}
	return b.String()
}

// SubredditAbout renders /r/x/about info.
func (t Theme) SubredditAbout(s *reddit.Subreddit) string {
	var b strings.Builder
	b.WriteString(t.Title.Render("r/" + s.Data.Name))
	b.WriteString("\n")
	if s.Data.Title != "" {
		b.WriteString(t.Subtitle.Render(s.Data.Title))
		b.WriteString("\n")
	}
	b.WriteString(t.Meta.Render(fmt.Sprintf("%d subscribers · %d active · created %s",
		s.Data.Subscribers, s.Data.ActiveUsers, relativeTime(s.Data.CreatedUTC))))
	b.WriteString("\n")
	if s.Data.Over18 {
		b.WriteString(t.NSFW.Render("NSFW"))
		b.WriteString("\n")
	}
	if s.Data.Description != "" {
		b.WriteString("\n")
		b.WriteString(t.Markdown(s.Data.Description))
	}
	return b.String()
}

// User renders a user profile.
func (t Theme) User(u *reddit.User) string {
	var b strings.Builder
	name := u.Data.Name
	linkOpts := t.LinkOpts
	if t.Caps.Hyperlinks {
		name = t.LinkWithOptions("u/"+u.Data.Name, t.AuthorURL(u.Data.Name, linkOpts), linkOpts)
	}
	b.WriteString(t.Title.Render(name))
	b.WriteString("\n")
	if u.Data.IsGold {
		b.WriteString(t.Warn.Render("★ gold"))
		b.WriteString("\n")
	}
	if u.Data.IsMod {
		b.WriteString(t.Meta.Render("moderator"))
		b.WriteString("\n")
	}
	if u.Data.IsVerified {
		b.WriteString(t.Meta.Render("verified email"))
		b.WriteString("\n")
	}
	b.WriteString(t.Meta.Render(fmt.Sprintf("%d link karma · %d comment karma · %d total · created %s",
		u.Data.LinkKarma, u.Data.CommentKarma, u.Data.TotalKarma, relativeTime(u.Data.CreatedUTC))))
	b.WriteString("\n")
	if u.Data.Subreddit.PublicDesc != "" {
		b.WriteString("\n")
		b.WriteString(t.Markdown(u.Data.Subreddit.PublicDesc))
	}
	return b.String()
}

// Me renders the authenticated user.
func (t Theme) Me(m *reddit.Me) string {
	var b strings.Builder
	b.WriteString(t.Title.Render("u/" + m.Name))
	b.WriteString("\n")
	if m.IsGold {
		b.WriteString(t.Warn.Render("★ gold"))
		b.WriteString("\n")
	}
	b.WriteString(t.Meta.Render(fmt.Sprintf("%d link karma · %d comment karma · %d total · created %s",
		m.LinkKarma, m.CommentKarma, m.TotalKarma, relativeTime(m.CreatedUTC))))
	b.WriteString("\n")
	if m.IsMod {
		b.WriteString(t.Meta.Render("moderator"))
		b.WriteString("\n")
	}
	return b.String()
}

// Wiki renders a wiki page.
func (t Theme) Wiki(w *reddit.WikiPage) string {
	return t.Markdown(w.Data.ContentMD)
}

// relativeTime formats a unix epoch float as "3h ago" etc.
func relativeTime(utc float64) string {
	if utc <= 0 {
		return ""
	}
	d := time.Since(time.Unix(int64(utc), 0))
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s ago"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24/30)) + "mo ago"
	default:
		return strconv.Itoa(int(d.Hours()/24/365)) + "y ago"
	}
}
