// Package reddit HTML scraper for old.reddit.com fallback when .json endpoints fail.
package reddit

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ScrapedListing holds posts parsed from old.reddit.com HTML.
type ScrapedListing struct {
	Posts    []Post
	After    string
	Before   string
	HTMLOnly bool // true if we only have HTML (no .json)
}

// scrapeListing fetches and parses old.reddit.com HTML listing.
func (c *Client) scrapeListing(path string, query url.Values) ([]Post, *Paginator, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Convert .json path to HTML path for old.reddit.com
	htmlPath := jsonPathToHTMLPath(path)
	
	// Copy query and remove raw_json for HTML endpoint
	htmlQuery := make(url.Values)
	for k, v := range query {
		if k != "raw_json" {
			htmlQuery[k] = v
		}
	}
	
	u := "https://old.reddit.com" + htmlPath + "?" + htmlQuery.Encode()
	
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	// Use client's cookie jar
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, nil, &APIError{Host: "old.reddit.com", Path: path, Status: resp.StatusCode}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, err
	}

	posts := c.parseHTMLListing(doc)
	after := c.extractAfterCursor(doc)

	p := &Paginator{After: after, Count: len(posts)}
	return posts, p, nil
}

// parseHTMLListing extracts posts from old.reddit.com listing HTML.
// Handles both subreddit listings (.linklisting) and search results (.search-result-link).
func (c *Client) parseHTMLListing(doc *goquery.Document) []Post {
	var posts []Post

	// Subreddit/frontpage listing: .linklisting > .link
	doc.Find(".linklisting > .link, .search-result-link").Each(func(_ int, s *goquery.Selection) {
		p := c.parseHTMLPost(s)
		if p != nil && p.ID != "" {
			posts = append(posts, *p)
		}
	})

	return posts
}

// parseHTMLPost extracts a single post from a .link or .search-result-link element.
func (c *Client) parseHTMLPost(s *goquery.Selection) *Post {
	p := &Post{}

	// Fullname from data-fullname (t3_xxx)
	if fullname, ok := s.Attr("data-fullname"); ok {
		p.Name = fullname
		if strings.HasPrefix(fullname, "t3_") {
			p.ID = strings.TrimPrefix(fullname, "t3_")
		}
	}

	// Title + permalink
	titleSel := s.Find("a.title.may-blank, .search-title.may-blank").First()
	if titleSel.Length() > 0 {
		p.Title = strings.TrimSpace(titleSel.Text())
		// Extract ID from permalink if not in data-fullname
		if p.ID == "" {
			if href, ok := titleSel.Attr("href"); ok {
				if m := rePermalinkID.FindStringSubmatch(href); m != nil {
					p.ID = m[1]
					p.Name = "t3_" + p.ID
				}
			}
		}
	}

	// Permalink + subreddit come from the comments link: for link posts the
	// title href points at the external URL, not the thread.
	commentsLink := s.Find("a.comments.may-blank, .search-comments.may-blank").First()
	if commentsLink.Length() > 0 {
		if href, ok := commentsLink.Attr("href"); ok {
			p.Permalink = c.absURL(href)
			if m := rePermalinkPath.FindStringSubmatch(href); m != nil {
				if p.Subreddit == "" {
					p.Subreddit = m[1]
				}
				if p.ID == "" {
					p.ID = m[2]
					p.Name = "t3_" + p.ID
				}
			}
		}
	}
	// Fallback permalink: the title href (self posts link to the thread).
	if p.Permalink == "" {
		if href, ok := titleSel.Attr("href"); ok {
			p.Permalink = c.absURL(href)
		}
	}

	// Author
	authorSel := s.Find("a.author.may-blank, .search-author a.author.may-blank").First()
	if authorSel.Length() > 0 {
		p.Author = strings.TrimSpace(authorSel.Text())
	}

	// Subreddit
	subSel := s.Find("a.subreddit.may-blank, .search-subreddit-link.may-blank").First()
	if subSel.Length() > 0 {
		p.Subreddit = strings.TrimSpace(subSel.Text())
		p.Subreddit = strings.TrimPrefix(p.Subreddit, "r/")
	}

	// Score
	scoreSel := s.Find(".score.unvoted, .search-score").First()
	if scoreSel.Length() > 0 {
		scoreText := strings.TrimSpace(scoreSel.Text())
		// "19 points" or "3.1k"
		p.Score = parseScore(scoreText)
		p.Ups = p.Score
	}

	// Comments count
	commentsSel := s.Find("a.comments.may-blank, .search-comments.may-blank").First()
	if commentsSel.Length() > 0 {
		p.NumComments = parseScore(strings.TrimSpace(commentsSel.Text()))
	}

	// Time
	timeSel := s.Find("time.live-timestamp, .search-time time").First()
	if timeSel.Length() > 0 {
		if dt, ok := timeSel.Attr("datetime"); ok {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				p.CreatedUTC = float64(t.Unix())
			}
		}
		// Fallback: parse relative time text
		if p.CreatedUTC == 0 {
			p.CreatedUTC = parseRelativeTime(strings.TrimSpace(timeSel.Text()))
		}
	}

	// Domain
	domainSel := s.Find("span.domain a, .domain").First()
	if domainSel.Length() > 0 {
		p.Domain = strings.Trim(strings.TrimSpace(domainSel.Text()), "()")
	}

	// Thumbnail: listing items wrap it in a.thumbnail; search results just
	// embed the img with a thumbs.redditmedia / redd.it src.
	thumbSel := s.Find("a.thumbnail img, .search-result-link.has-thumbnail img, img[src*='thumbs.redditmedia.com'], img[src*='redd.it']").First()
	if thumbSel.Length() > 0 {
		if src, ok := thumbSel.Attr("src"); ok {
			p.Thumbnail = src
		}
	}

	// NSFW / Spoiler / Stickied
	if s.HasClass("over18") || s.Find(".nsfw-stamp").Length() > 0 {
		p.Over18 = true
	}
	if s.HasClass("spoiler") || s.Find(".spoiler-stamp").Length() > 0 {
		p.Spoiler = true
	}
	if s.HasClass("stickied") || s.Find(".stickied-tag").Length() > 0 {
		p.Stickied = true
	}

	// Self-post detection
	if s.HasClass("self") || s.Find(".selftext").Length() > 0 {
		p.IsSelf = true
	}

	// URL for link posts - get the external link
	if !p.IsSelf {
		// Try multiple selectors for the external URL
		urlSel := s.Find("a.title.may-blank[href^=http], a.search-title.may-blank[href^=http]").First()
		if urlSel.Length() > 0 {
			if href, ok := urlSel.Attr("href"); ok && !strings.HasPrefix(href, "/r/") && !strings.HasPrefix(href, "/comments/") {
				p.URL = href
			}
		}
		// Fallback: check for data-url attribute (old.reddit.com uses this)
		if p.URL == "" {
			if dataURL, ok := s.Attr("data-url"); ok && dataURL != "" && strings.HasPrefix(dataURL, "http") {
				p.URL = dataURL
			}
		}
		// Fallback: search result external link in footer (.search-link.may-blank)
		if p.URL == "" {
			extLink := s.Find(".search-link.may-blank[href^=http]").First()
			if extLink.Length() > 0 {
				if href, ok := extLink.Attr("href"); ok && !strings.Contains(href, "reddit.com") {
					p.URL = href
				}
			}
		}
		// Fallback: try the title link if it's an external link
		if p.URL == "" {
			titleSel := s.Find("a.title.may-blank, .search-title.may-blank").First()
			if titleSel.Length() > 0 {
				if href, ok := titleSel.Attr("href"); ok && strings.HasPrefix(href, "http") && !strings.Contains(href, "reddit.com") {
					p.URL = href
				}
			}
		}
	}

	// Selftext (for expanded view - not in listing)
	// We'll leave Selftext empty for listing; full post fetch gets it.

	// Extract subreddit ID if available
	if subSel.Length() > 0 {
		if dataSubID, ok := subSel.Attr("data-subreddit-id"); ok {
			p.SubredditID = dataSubID
		}
	}

	return p
}

// extractAfterCursor finds the "next" pagination cursor from HTML.
func (c *Client) extractAfterCursor(doc *goquery.Document) string {
	// .nav-buttons a[rel="next"]
	nextSel := doc.Find(".nav-buttons a[rel=next], .nextprev a[rel=nofollow]").First()
	if nextSel.Length() > 0 {
		if href, ok := nextSel.Attr("href"); ok {
			u, err := url.Parse(href)
			if err == nil {
				return u.Query().Get("after")
			}
		}
	}
	return ""
}

// absURL makes a relative Reddit URL absolute.
func (c *Client) absURL(href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return "https://www.reddit.com" + href
}

var rePermalinkID = regexp.MustCompile(`/comments/([a-z0-9]+)`)

// rePermalinkPath matches the thread path and captures subreddit + post id.
var rePermalinkPath = regexp.MustCompile(`/r/([A-Za-z0-9_]+)/comments/([a-z0-9]+)`)

// parseScore parses "19 points", "3.1k", "1.2M" etc.
func parseScore(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " points", "")
	s = strings.ReplaceAll(s, " point", "")
	s = strings.ReplaceAll(s, " comments", "")
	s = strings.ReplaceAll(s, " comment", "")
	s = strings.ReplaceAll(s, ",", "")

	mult := 1
	if strings.HasSuffix(s, "k") {
		mult = 1000
		s = strings.TrimSuffix(s, "k")
	} else if strings.HasSuffix(s, "m") {
		mult = 1000000
		s = strings.TrimSuffix(s, "m")
	}

	var val float64
	if _, err := fmt.Sscanf(s, "%f", &val); err == nil {
		return int(val * float64(mult))
	}
	return 0
}

// parseRelativeTime parses "11 days ago", "2 months ago", etc. to unix epoch.
// Returns 0 if unparseable.
func parseRelativeTime(s string) float64 {
	s = strings.ToLower(s)
	now := time.Now()
	var d time.Duration

	switch {
	case strings.Contains(s, "second"):
		return float64(now.Unix())
	case strings.Contains(s, "minute"):
		if n := extractNumber(s); n > 0 {
			d = time.Duration(n) * time.Minute
		}
	case strings.Contains(s, "hour"):
		if n := extractNumber(s); n > 0 {
			d = time.Duration(n) * time.Hour
		}
	case strings.Contains(s, "day"):
		if n := extractNumber(s); n > 0 {
			d = time.Duration(n) * 24 * time.Hour
		}
	case strings.Contains(s, "month"):
		if n := extractNumber(s); n > 0 {
			d = time.Duration(n) * 30 * 24 * time.Hour
		}
	case strings.Contains(s, "year"):
		if n := extractNumber(s); n > 0 {
			d = time.Duration(n) * 365 * 24 * time.Hour
		}
	}
	if d > 0 {
		return float64(now.Add(-d).Unix())
	}
	return 0
}

func extractNumber(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// jsonPathToHTMLPath converts a .json API path to the corresponding old.reddit.com HTML path.
func jsonPathToHTMLPath(path string) string {
	// Remove .json suffix
	path = strings.TrimSuffix(path, ".json")
	// Ensure it starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// scrapePostPage fetches and parses a single post page from old.reddit.com HTML.
// This includes the post content and optionally comments.
func (c *Client) scrapePostPage(path string, query url.Values, includeComments bool) (*PostWithComments, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Convert .json path to HTML path for old.reddit.com post page
	// /comments/abc123.json -> /comments/abc123
	htmlPath := strings.TrimSuffix(path, ".json")
	
	// Copy query and remove raw_json for HTML endpoint
	htmlQuery := make(url.Values)
	for k, v := range query {
		if k != "raw_json" {
			htmlQuery[k] = v
		}
	}
	
	u := "https://old.reddit.com" + htmlPath + "?" + htmlQuery.Encode()
	
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, &APIError{Host: "old.reddit.com", Path: path, Status: resp.StatusCode}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	result := &PostWithComments{}

	// Parse the post (first .entry or .thing with data-fullname)
	postEntry := doc.Find(".entry, .thing[id^=thing_t3_]").First()
	if postEntry.Length() > 0 {
		result.Post = *c.parseHTMLPost(postEntry)
	}

	// Parse comments if requested
	if includeComments {
		result.Comments = c.parseHTMLComments(doc.Selection)
	}

	return result, nil
}

// parseHTMLComments extracts comments from old.reddit.com post page HTML.
func (c *Client) parseHTMLComments(doc *goquery.Selection) []Comment {
	var comments []Comment

	// Find comment area
	doc.Find(".comment").Each(func(_ int, s *goquery.Selection) {
		c := c.parseHTMLComment(s)
		if c != nil && c.Data.ID != "" {
			comments = append(comments, *c)
		}
	})

	return comments
}

// parseHTMLComment extracts a single comment from old.reddit.com HTML.
func (c *Client) parseHTMLComment(s *goquery.Selection) *Comment {
	cmt := &Comment{}

	// ID from data-fullname or id attribute
	if fullname, ok := s.Attr("data-fullname"); ok {
		cmt.Kind = "t1"
		cmt.Data.ID = strings.TrimPrefix(fullname, "t1_")
		cmt.Data.Name = fullname
	} else if id, ok := s.Attr("id"); ok && strings.HasPrefix(id, "thing_t1_") {
		cmt.Data.ID = strings.TrimPrefix(id, "thing_t1_")
		cmt.Data.Name = "t1_" + cmt.Data.ID
		cmt.Kind = "t1"
	}

	// Author
	authorSel := s.Find("a.author.may-blank").First()
	if authorSel.Length() > 0 {
		cmt.Data.Author = strings.TrimSpace(authorSel.Text())
	}

	// Body
	bodySel := s.Find(".usertext-body .md").First()
	if bodySel.Length() > 0 {
		cmt.Data.Body = strings.TrimSpace(bodySel.Text())
	}

	// Score
	scoreSel := s.Find(".score.unvoted").First()
	if scoreSel.Length() > 0 {
		cmt.Data.Score = parseScore(strings.TrimSpace(scoreSel.Text()))
	}

	// Time
	timeSel := s.Find("time.live-timestamp").First()
	if timeSel.Length() > 0 {
		if dt, ok := timeSel.Attr("datetime"); ok {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				cmt.Data.CreatedUTC = float64(t.Unix())
			}
		}
	}

	// Subreddit (from permalink)
	permaSel := s.Find("a.bylink.may-blank").First()
	if permaSel.Length() > 0 {
		if href, ok := permaSel.Attr("href"); ok {
			if m := rePermalinkID.FindStringSubmatch(href); m != nil {
				cmt.Data.LinkID = "t3_" + m[1]
			}
		}
	}

	// Check for replies (nested comments)
	replies := c.parseHTMLComments(s)
	if len(replies) > 0 {
		cmt.Replies = replies
	}

	// OP marker
	if s.HasClass("submitter") {
		cmt.Data.IsSubmitter = true
	}

	return cmt
}