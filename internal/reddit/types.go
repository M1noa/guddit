// Package reddit provides an unauthenticated-or-cookie HTTP client for Reddit's
// public .json endpoints with old.reddit.com fallback, 429 backoff, and a
// small typed model of listings, posts, comments, users and search results.
package reddit

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Kind prefixes Reddit uses for its "thing" identifiers.
const (
	KindComment  = "t1"
	KindAccount  = "t2"
	KindLink     = "t3" // a post/link
	KindMessage  = "t4"
	KindSubreddit = "t5"
	KindAward    = "t6"
	KindListing  = "Listing"
	KindMore     = "more"
)

// Listing wraps the outer Reddit listing envelope.
type Listing[T any] struct {
	Kind string `json:"kind"`
	Data struct {
		After    string      `json:"after"`
		Before   string      `json:"before"`
		Dist     int         `json:"dist"`
		Modhash  string      `json:"modhash"`
		Children []T         `json:"children"`
	} `json:"data"`
}

// Paginator is a cursor for "next page" requests.
type Paginator struct {
	After  string
	Before string
	Count  int
}

// Post is a t3 link/submission. Only fields we render are kept; everything else
// is dropped to keep the model small.
type Post struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"` // fullname t3_xxx
	Title            string  `json:"title"`
	Author           string  `json:"author"`
	AuthorFullname   string  `json:"author_fullname"`
	Subreddit        string  `json:"subreddit"`
	SubredditID      string  `json:"subreddit_id"`
	Permalink        string  `json:"permalink"`
	URL              string  `json:"url"`
	Domain           string  `json:"domain"`
	Selftext         string  `json:"selftext"`
	SelftextHTML     string  `json:"selftext_html"`
	Score            int     `json:"score"`
	Ups              int     `json:"ups"`
	Downs            int     `json:"downs"`
	UpvoteRatio      float64 `json:"upvote_ratio"`
	NumComments      int     `json:"num_comments"`
	NumCrossposts    int     `json:"num_crossposts"`
	NumReports        int     `json:"num_reports"`
	ViewCount        *int    `json:"view_count"`
	Created          float64 `json:"created"`
	CreatedUTC       float64 `json:"created_utc"`
	Edited           any     `json:"edited"` // bool or float64
	Over18           bool    `json:"over_18"`
	NSFW             bool    `json:"over_18"`
	Spoiler          bool    `json:"spoiler"`
	Locked           bool    `json:"locked"`
	Archived         bool    `json:"archived"`
	Stickied         bool    `json:"stickied"`
	Pinned           bool    `json:"pinned"`
	Saved            bool    `json:"saved"`
	Hidden           bool    `json:"hidden"`
	Clicked          bool    `json:"clicked"`
	Visited          bool    `json:"visited"`
	Gilded           int     `json:"gilded"`
	TotalAwards      int     `json:"total_awards_received"`
	IsSelf           bool    `json:"is_self"`
	IsVideo          bool    `json:"is_video"`
	IsOriginalContent bool   `json:"is_original_content"`
	IsMeta           bool    `json:"is_meta"`
	IsCrosspostable  bool    `json:"is_crosspostable"`
	IsRedditMediaDomain bool `json:"is_reddit_media_domain"`
	Thumbnail        string  `json:"thumbnail"`
	ThumbnailWidth   *int    `json:"thumbnail_width"`
	ThumbnailHeight  *int    `json:"thumbnail_height"`
	Preview          *Preview `json:"preview"`
	LinkFlairText    string  `json:"link_flair_text"`
	LinkFlairCSSClass string `json:"link_flair_css_class"`
	LinkFlairType    string  `json:"link_flair_type"`
	AuthorFlairText  string  `json:"author_flair_text"`
	SuggestedSort    string  `json:"suggested_sort"`
	DiscussionType   string  `json:"discussion_type"`
	Quarantine       bool    `json:"quarantine"`
	RemovedByCategory string `json:"removed_by_category"`
	Media            json.RawMessage `json:"media"`
	MediaEmbed       json.RawMessage `json:"media_embed"`
	SecureMedia      json.RawMessage `json:"secure_media"`
	GalleryData      json.RawMessage `json:"gallery_data"`
	MediaMetadata    json.RawMessage `json:"media_metadata"`
	CrosspostParent  string  `json:"crosspost_parent"`
	SubredditSubscribers int   `json:"subreddit_subscribers"`
}

// postAlias bypasses the Unmarshaler interface to avoid infinite recursion.
type postAlias Post

// UnmarshalJSON handles Reddit's "thing" wrapper: {"kind": "t3", "data": {...}}.
func (p *Post) UnmarshalJSON(data []byte) error {
	var raw struct {
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	inner := data
	if raw.Kind != "" && len(raw.Data) > 0 {
		inner = raw.Data
	}
	var a postAlias
	if err := json.Unmarshal(inner, &a); err != nil {
		return err
	}
	*p = Post(a)
	return nil
}

// Preview holds Reddit's image/video preview metadata.
type Preview struct {
	Images    []PreviewImage `json:"images"`
	RedditVideo *PreviewVideo `json:"reddit_video,omitempty"`
	Enabled   bool            `json:"enabled"`
}

type PreviewImage struct {
	ID      string         `json:"id"`
	Resolutions []ImageVariant `json:"resolutions"`
	Source  ImageVariant   `json:"source"`
	Variants map[string]any `json:"variants"`
}

type ImageVariant struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type PreviewVideo struct {
	BitratePerSecond int    `json:"bitrate_kbps"`
	FallbackURL      string `json:"fallback_url"`
	DashURL          string `json:"dash_url"`
	HLSURL           string `json:"hls_url"`
	Duration         int    `json:"duration"`
	Height           int    `json:"height"`
	Width            int    `json:"width"`
	IsGif            bool   `json:"is_gif"`
}

// IsEdited reports whether the post has been edited (Edited is bool false or a
// timestamp).
func (p *Post) IsEdited() bool {
	switch v := p.Edited.(type) {
	case bool:
		return v
	case float64:
		return v > 0
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f > 0
		}
	}
	return false
}

// EditedTime returns the edit timestamp as a float64 epoch (0 if not edited).
func (p *Post) EditedTime() float64 {
	switch v := p.Edited.(type) {
	case bool:
		return 0
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

// Comment is a t1 comment. Replies are decoded recursively from the nested
// Listing; "more" children (load-more placeholders) are captured as
// Continued entries.
type Comment struct {
	Kind      string `json:"kind"` // "t1" or "more"
	Data      CommentData `json:"data"`
	Replies   []Comment `json:"-"` // populated by UnmarshalJSON
}

// CommentData holds the t1 comment fields we render.
type CommentData struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Author        string  `json:"author"`
	AuthorFullname string `json:"author_fullname"`
	Body          string  `json:"body"`
	BodyHTML      string  `json:"body_html"`
	Score         int     `json:"score"`
	Ups           int     `json:"ups"`
	Downs         int     `json:"downs"`
	Controversial int     `json:"controversiality"`
	Created       float64 `json:"created"`
	CreatedUTC    float64 `json:"created_utc"`
	Edited        any     `json:"edited"`
	IsSubmitter   bool    `json:"is_submitter"`
	Saved         bool    `json:"saved"`
	Gilded        int     `json:"gilded"`
	TotalAwards   int     `json:"total_awards_received"`
	Stickied      bool    `json:"stickied"`
	Locked        bool    `json:"locked"`
	Distinguished string  `json:"distinguished"` // "moderator" or "admin"
	ParentID      string  `json:"parent_id"`
	LinkID        string  `json:"link_id"`
	Subreddit     string  `json:"subreddit"`
	SubredditID   string  `json:"subreddit_id"`
	Permalink     string  `json:"permalink"`
	Depth         int     `json:"depth"`
	Count         int     `json:"count"`     // for "more" children
	Children      []string `json:"children"` // for "more" children: list of ids to load
}

// UnmarshalJSON decodes a comment, expanding the polymorphic "replies" field
// into the Replies slice. "replies" is either "" (no replies), a string error,
// or a nested Listing of comments.
func (c *Comment) UnmarshalJSON(data []byte) error {
	type raw struct {
		Kind  string          `json:"kind"`
		Data  json.RawMessage `json:"data"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	c.Kind = r.Kind
	if len(r.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(r.Data, &c.Data); err != nil {
		return err
	}
	// "more" children have no replies field.
	if r.Kind != KindComment {
		return nil
	}
	var rd struct {
		Replies json.RawMessage `json:"replies"`
	}
	if err := json.Unmarshal(r.Data, &rd); err != nil {
		return err
	}
	// Replies may be "" or a string. If it's an object, it's a Listing.
	if len(rd.Replies) == 0 || rd.Replies[0] != '{' {
		return nil
	}
	var l Listing[Comment]
	if err := json.Unmarshal(rd.Replies, &l); err != nil {
		return err
	}
	c.Replies = l.Data.Children
	return nil
}

// IsMore reports whether this is a "load more comments" placeholder.
func (c *Comment) IsMore() bool { return c.Kind == KindMore || c.Data.Children != nil }

// User wraps a t2 account; only fields we render.
type User struct {
	Kind string `json:"kind"`
	Data struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		IsFriend       bool    `json:"is_friend"`
		IsBlocked      bool    `json:"is_blocked"`
		IsMod          bool    `json:"is_mod"`
		IsGold         bool    `json:"is_gold"`
		IsEmployee     bool    `json:"is_employee"`
		IsVerified     bool    `json:"verified"`
		HasVerifiedMail bool   `json:"has_verified_email"`
		Created        float64 `json:"created"`
		CreatedUTC     float64 `json:"created_utc"`
		CommentKarma   int     `json:"comment_karma"`
		LinkKarma      int     `json:"link_karma"`
		AwarderKarma   int     `json:"awarder_karma"`
		AwardeeKarma   int     `json:"awardee_karma"`
		TotalKarma     int     `json:"total_karma"`
		Subreddit      struct {
			Title       string `json:"title"`
			PublicDesc  string `json:"public_description"`
			Subscribers int    `json:"subscribers"`
			IconImg     string `json:"icon_img"`
			BannerImg   string `json:"banner_img"`
			Over18      bool   `json:"over_18"`
		} `json:"subreddit"`
IconImg       string `json:"icon_img"`
		SnoovatarSize any    `json:"snoovatar_size"`
	} `json:"data"`
}

// Subreddit wraps a t5 subreddit; only fields we render.
type Subreddit struct {
	Kind string `json:"kind"`
	Data struct {
		ID              string  `json:"id"`
		Name            string  `json:"display_name"`
		Title           string  `json:"title"`
		Description     string  `json:"description"`
		PublicDesc      string  `json:"public_description"`
		Subscribers     int     `json:"subscribers"`
		ActiveUsers      int     `json:"active_user_count"`
		Created         float64 `json:"created"`
		CreatedUTC      float64 `json:"created_utc"`
		Over18          bool    `json:"over_18"`
		SubredditType   string  `json:"subreddit_type"`
		SubmissionType  string  `json:"submission_type"`
		Quarantine      bool    `json:"quarantine"`
		Lang            string  `json:"lang"`
		IconImg         string  `json:"icon_img"`
		BannerImg       string  `json:"banner_img"`
		BannerBgColor   string  `json:"banner_background_color"`
		KeyColor        string  `json:"key_color"`
		URL             string  `json:"url"`
		HeaderImg       string  `json:"header_img"`
		DescriptionHTML string  `json:"description_html"`
		SubmitText      string  `json:"submit_text"`
		WikiEnabled     bool    `json:"wiki_enabled"`
	} `json:"data"`
}

// SearchResult reuses Post for search hits; Reddit returns t3 children.
type SearchResult = Post

// WikiPage holds a wiki page body.
type WikiPage struct {
	Kind string `json:"kind"`
	Data struct {
		ContentMD   string  `json:"content_md"`
		ContentHTML string  `json:"content_html"`
		RevisionBy  struct {
			Data struct {
				Author string `json:"author"`
			} `json:"data"`
		} `json:"revision_by"`
		RevisionDate float64 `json:"revision_date"`
		MayRevise    bool     `json:"may_revise"`
	} `json:"data"`
}

// Me wraps /api/v1/me.json (authenticated).
type Me struct {
	IsMod         bool    `json:"is_mod"`
	IsGold        bool    `json:"is_gold"`
	IsEmployee    bool    `json:"is_employee"`
	IsSuspended   bool    `json:"is_suspended"`
	HasVerifiedEmail bool  `json:"has_verified_email"`
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	CommentKarma  int     `json:"comment_karma"`
	LinkKarma     int     `json:"link_karma"`
	TotalKarma    int     `json:"total_karma"`
	AwarderKarma  int     `json:"awarder_karma"`
	AwardeeKarma  int     `json:"awardee_karma"`
	Created       float64 `json:"created"`
	CreatedUTC    float64 `json:"created_utc"`
IconImg       string `json:"icon_img"`
	SnoovatarSize any     `json:"snoovatar_size"`
	Subreddit     struct {
		Title      string `json:"title"`
		PublicDesc string `json:"public_description"`
		Subscribers int   `json:"subscribers"`
		IconImg    string `json:"icon_img"`
	} `json:"subreddit"`
	Prefs json.RawMessage `json:"prefs"`
}

// KarmaBreakdown from /api/v1/me/karma.json
type KarmaBreakdown struct {
	Data struct {
		Subreddits []struct {
			SR       string `json:"sr"`
			Karma     int    `json:"karma"`
			CommentKarma int `json:"comment_karma"`
			LinkKarma int    `json:"link_karma"`
		} `json:"subreddits"`
	} `json:"data"`
}

// --- media helpers ---------------------------------------------------------

// galleryMetadata is the parsed shape of Post.MediaMetadata.
type galleryMetadata map[string]struct {
	Status string `json:"status"`
	E      string `json:"e"` // "Image" or "AnimatedImage"
	M      string `json:"m"` // mime type
	S      struct {
		X int    `json:"x"`
		Y int    `json:"y"`
		U string `json:"u"`
	} `json:"s"`             // source (full size)
	P []galleryMetaRes `json:"p"` // preview resolutions
}

// galleryMetaRes is one resolution entry in media_metadata ("u" is the URL,
// "x"/"y" the dimensions).
type galleryMetaRes struct {
	U string `json:"u"`
	X int    `json:"x"`
	Y int    `json:"y"`
}

type galleryData struct {
	Items []struct {
		MediaID string `json:"media_id"`
		ID      int    `json:"id"`
		Caption string `json:"caption"`
	} `json:"items"`
}

// GalleryImageURLs returns the ordered image URLs of a gallery post, choosing
// the largest resolution no wider than maxWidth (or the smallest available).
// Returns nil for non-gallery posts.
func (p *Post) GalleryImageURLs(maxWidth int) []string {
	if len(p.MediaMetadata) == 0 && len(p.GalleryData) == 0 {
		return nil
	}
	var gd galleryData
	if err := json.Unmarshal(p.GalleryData, &gd); err != nil || len(gd.Items) == 0 {
		return nil
	}
	var md galleryMetadata
	if err := json.Unmarshal(p.MediaMetadata, &md); err != nil {
		return nil
	}
	out := make([]string, 0, len(gd.Items))
	for _, item := range gd.Items {
		m, ok := md[item.MediaID]
		if !ok || m.S.U == "" {
			continue
		}
		u := m.S.U
		for _, r := range m.P {
			if r.U != "" && (r.X <= maxWidth || u == m.S.U) {
				u = r.U
			}
		}
		out = append(out, httpsURL(u))
	}
	return out
}

// MediaImageURL returns the best single image URL for the post (preview image,
// gallery first image, or direct image link), no wider than maxWidth. Returns
// "" if the post has no inline-renderable image.
func (p *Post) MediaImageURL(maxWidth int) string {
	if p.Preview != nil && len(p.Preview.Images) > 0 {
		img := p.Preview.Images[0]
		return pickResolution(img.Resolutions, img.Source.URL, maxWidth)
	}
	if urls := p.GalleryImageURLs(maxWidth); len(urls) > 0 {
		return urls[0]
	}
	if isImageURL(p.URL) {
		return p.URL
	}
	if p.Thumbnail != "" && !isThumbPlaceholder(p.Thumbnail) {
		return p.Thumbnail
	}
	return ""
}

// pickResolution returns the largest URL no wider than maxWidth, falling back
// to the smallest when all exceed it; empty if no URL available.
func pickResolution(res []ImageVariant, fallback string, maxWidth int) string {
	if len(res) == 0 {
		return httpsURL(fallback)
	}
	best := res[len(res)-1]
	for _, r := range res {
		if r.Width > 0 && r.Width <= maxWidth {
			best = r
		}
	}
	if best.URL == "" {
		best.URL = fallback
	}
	return httpsURL(best.URL)
}

func httpsURL(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}

func isImageURL(u string) bool {
	lower := strings.ToLower(u)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif"} {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}

func isThumbPlaceholder(t string) bool {
	return t == "self" || t == "spoiler" || t == "nsfw" || t == "default" || t == "image" || t == ""
}
