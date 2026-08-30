package reddit

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// PostWithComments holds the result of a thread fetch: the original post plus
// the flattened-then-tree comments.
type PostWithComments struct {
	Post     Post
	Comments []Comment
}

// UnmarshalJSON decodes the /comments/{id}.json array shape, which is
// [Listing[Post], Listing[Comment]].
func (p *PostWithComments) UnmarshalJSON(data []byte) error {
	var listings []json.RawMessage
	if err := json.Unmarshal(data, &listings); err != nil {
		return err
	}
	if len(listings) > 0 {
		var pl Listing[Post]
		if err := json.Unmarshal(listings[0], &pl); err != nil {
			return err
		}
		if len(pl.Data.Children) > 0 {
			p.Post = pl.Data.Children[0]
		}
	}
	if len(listings) > 1 {
		var cl Listing[Comment]
		if err := json.Unmarshal(listings[1], &cl); err != nil {
			return err
		}
		p.Comments = cl.Data.Children
	}
	return nil
}

// Post fetches a thread by ID, fullname, shortlink, or permalink. With
// comments=true, returns the comment tree. With comments=false, returns just
// the post (uses /api/info.json or /by_id).
func (c *Client) Post(ref string, comments bool) (*PostWithComments, error) {
	id, err := normalizePostRef(ref)
	if err != nil {
		return nil, err
	}

	if !comments {
		var l Listing[Post]
		if err := c.Get("/api/info.json", url.Values{"id": {id}}, &l); err != nil {
			return nil, err
		}
		if len(l.Data.Children) == 0 {
			return nil, &APIError{Status: 404, Path: id, Body: "no such post"}
		}
		return &PostWithComments{Post: l.Data.Children[0]}, nil
	}

	// For comments, use /comments/{id}.json which returns [Listing[Post], Listing[Comment]]
	// The client's do() method will handle fallback chain including HTML scraping.
	// We need to use a target that scrapeAndDecode understands: PostWithComments.
	var result PostWithComments
	err = c.Get("/comments/"+strings.TrimPrefix(id, "t3_")+".json", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// jsonRaw is a helper for decoding a generic JSON array element lazily.
type jsonRaw struct {
	raw []byte
}

func (j *jsonRaw) UnmarshalJSON(b []byte) error {
	j.raw = b
	return nil
}
func (j *jsonRaw) decode(target any) error {
	return json.Unmarshal(j.raw, target)
}

// CommentBy fetches a single comment and its surrounding context by id
// (t1_xxx or 7-char id) via /api/info.json.
func (c *Client) CommentBy(ref string) (*Comment, error) {
	id := normalizeThingRef(ref, "t1")
	var l Listing[Comment]
	if err := c.Get("/api/info.json", url.Values{"id": {id}}, &l); err != nil {
		return nil, err
	}
	if len(l.Data.Children) == 0 {
		return nil, &APIError{Status: 404, Path: id, Body: "no such comment"}
	}
	return &l.Data.Children[0], nil
}

// --- ref normalization -----------------------------------------------------

var (
	reFullname = regexp.MustCompile(`^t3_[a-z0-9]+$`)
	reShortID  = regexp.MustCompile(`^[a-z0-9]{5,8}$`)
	reShortlink = regexp.MustCompile(`(?i)redd\.it/([a-z0-9]+)`)
	rePermalink = regexp.MustCompile(`(?i)reddit\.com/r/([A-Za-z0-9_]+)/comments/([a-z0-9]+)`)
)

// normalizePostRef accepts: t3_xxx fullname, bare id, redd.it/xxx shortlink,
// full permalink (with or without slug/comment id), https url, /r/x/comments/id/...
func normalizePostRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("reddit: empty post reference")
	}

	if reFullname.MatchString(ref) {
		return ref, nil
	}
	if strings.HasPrefix(strings.ToLower(ref), "http") {
		u, err := url.Parse(ref)
		if err == nil && u != nil {
			return postRefFromURL(u)
		}
	}
	if m := reShortlink.FindStringSubmatch(ref); m != nil {
		return "t3_" + strings.ToLower(m[1]), nil
	}
	if strings.Contains(ref, "reddit.com/r/") {
		if m := rePermalink.FindStringSubmatch(ref); m != nil {
			return "t3_" + strings.ToLower(m[2]), nil
		}
	}
	if strings.HasPrefix(ref, "/r/") {
		if m := rePermalink.FindStringSubmatch("https://reddit.com" + ref); m != nil {
			return "t3_" + strings.ToLower(m[2]), nil
		}
	}
	if reShortID.MatchString(ref) {
		return "t3_" + ref, nil
	}
	return "", fmt.Errorf("reddit: unrecognized post reference %q (want t3_xxx, id, redd.it/xxx, or permalink)", ref)
}

func postRefFromURL(u *url.URL) (string, error) {
	host := strings.ToLower(u.Host)
	switch {
	case host == "redd.it" || host == "www.redd.it":
		return "t3_" + strings.ToLower(strings.TrimPrefix(u.Path, "/")), nil
	case strings.Contains(host, "reddit.com"):
		if m := rePermalink.FindStringSubmatch(u.String()); m != nil {
			return "t3_" + strings.ToLower(m[2]), nil
		}
		if id := strings.ToLower(strings.TrimPrefix(u.Path, "/")); reShortID.MatchString(id) {
			return "t3_" + id, nil
		}
	}
	return "", fmt.Errorf("reddit: cannot extract post id from %s", u)
}

// normalizeThingRef prefixes a bare id with the kind prefix if missing.
func normalizeThingRef(ref, kind string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, kind+"_") {
		return ref
	}
	return kind + "_" + ref
}
