package reddit

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// UserKind selects a user feed.
type UserKind string

const (
	UserOverview  UserKind = "overview"
	UserSubmitted UserKind = "submitted"
	UserComments  UserKind = "comments"
	UserPosts      UserKind = "submitted" // alias; Reddit only has "submitted"
)

// UserAbout fetches /user/x/about.json.
func (c *Client) UserAbout(name string) (*User, error) {
	name = trimUserPrefix(name)
	var u User
	if err := c.Get("/user/"+name+"/about.json", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// scrapeUserProfile fetches old.reddit.com/user/{name}/ and parses the sidebar
// into a User. Used as the HTML fallback when about.json is unavailable.
func (c *Client) scrapeUserProfile(name string) (*User, error) {
	name = trimUserPrefix(name)
	u := "https://old.reddit.com/user/" + name + "/"

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
		return nil, &APIError{Host: "old.reddit.com", Path: "/user/" + name, Status: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	out := &User{Kind: "t2"}
	box := doc.Find(".titlebox").First()
	if box.Length() == 0 {
		// Page exists but has no titlebox (suspended/empty). Return minimal.
		out.Data.Name = name
		return out, nil
	}

	// Name from h1 (may include an admin/mod badge span we must skip).
	h1 := box.Find("h1").First()
	if h1.Length() > 0 {
		// Clone text excluding nested .user-distinction spans.
		clone := h1.Clone()
		clone.Find(".user-distinction").Remove()
		out.Data.Name = strings.TrimSpace(clone.Text())
	}
	if out.Data.Name == "" {
		out.Data.Name = name
	}

	// Karma: first span.karma = post/link karma; span.karma.comment-karma = comment karma.
	if k := box.Find("span.karma").First(); k.Length() > 0 {
		out.Data.LinkKarma = parseScore(strings.TrimSpace(k.Text()))
	}
	if k := box.Find("span.karma.comment-karma").First(); k.Length() > 0 {
		out.Data.CommentKarma = parseScore(strings.TrimSpace(k.Text()))
	}

	// Created date from <time datetime="...">.
	if dt, ok := box.Find("time").First().Attr("datetime"); ok {
		if t, err := time.Parse(time.RFC3339, dt); err == nil {
			out.Data.CreatedUTC = float64(t.Unix())
			out.Data.Created = float64(t.Unix())
		}
	}

	// Admin / employee badge. User.Data has no IsAdmin field; IsEmployee covers it.
	if box.Find(".user-distinction .admin").Length() > 0 {
		out.Data.IsEmployee = true
	}
	// Mod badge (gold "M").
	if box.Find(".user-distinction .moderator").Length() > 0 {
		out.Data.IsMod = true
	}
	// Gold (trophies area implies gilded history; conservative: leave false).

	return out, nil
}

// UserListing fetches a user's posts/comments feed.
func (c *Client) UserListing(name string, kind UserKind, opt ListingOptions) ([]Post, *Paginator, error) {
	name = trimUserPrefix(name)
	if kind == "" {
		kind = UserOverview
	}
	return c.fetchListing("/user/"+name+"/"+string(kind), opt)
}

// trimUserPrefix strips "u/" and "/u/" prefixes.
func trimUserPrefix(s string) string {
	s = trimLeading(s, "/u/")
	s = trimLeading(s, "u/")
	return trimLeading(s, "/")
}

func trimLeading(s, prefix string) string {
	for {
		if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
			s = s[len(prefix):]
			continue
		}
		break
	}
	return s
}
