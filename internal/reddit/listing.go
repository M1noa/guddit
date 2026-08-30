package reddit

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ListingKind selects which feed of a subreddit/homepage to fetch.
type ListingKind string

const (
	ListingHot           ListingKind = "hot"
	ListingNew           ListingKind = "new"
	ListingTop           ListingKind = "top"
	ListingRising        ListingKind = "rising"
	ListingControversial ListingKind = "controversial"
	ListingBest          ListingKind = "best"
)

// TimeWindow for top/controversial listings.
type TimeWindow string

const (
	TimeHour  TimeWindow = "hour"
	TimeDay   TimeWindow = "day"
	TimeWeek  TimeWindow = "week"
	TimeMonth TimeWindow = "month"
	TimeYear  TimeWindow = "year"
	TimeAll   TimeWindow = "all"
)

// SortKind for search results.
type SortKind string

const (
	SortRelevance   SortKind = "relevance"
	SortHot          SortKind = "hot"
	SortNew          SortKind = "new"
	SortTop          SortKind = "top"
	SortComments     SortKind = "comments"
)

// ListingOptions controls a listing fetch.
type ListingOptions struct {
	Limit       int
	After       string
	Before      string
	Count       int
	Show        string // "all"
	Time        TimeWindow // for top/controversial
	RawJSON     bool
	NSFW        bool // include NSFW (18+) content
	SafeSearch  bool // enable safe search (exclude NSFW)
}

func (o ListingOptions) query() url.Values {
	q := url.Values{}
	if o.Limit > 0 {
		q.Set("limit", clamp(o.Limit, 1, 100))
	}
	if o.After != "" {
		q.Set("after", o.After)
	}
	if o.Before != "" {
		q.Set("before", o.Before)
	}
	if o.Count > 0 {
		q.Set("count", fmtInt(o.Count))
	}
	if o.Show != "" {
		q.Set("show", o.Show)
	}
	if o.Time != "" {
		q.Set("t", string(o.Time))
	}
	if o.NSFW {
		q.Set("include_over_18", "true")
	}
	if o.SafeSearch {
		q.Set("include_over_18", "false")
	}
	if !o.RawJSON {
		q.Set("raw_json", "1")
	}
	return q
}

func clamp(n, lo, hi int) string {
	if n < lo {
		n = lo
	}
	if n > hi {
		n = hi
	}
	return fmtInt(n)
}
func fmtInt(n int) string { return fmt.Sprintf("%d", n) }

// Home fetches the frontpage listing for the given kind.
func (c *Client) Home(kind ListingKind, opt ListingOptions) ([]Post, *Paginator, error) {
	path := "/" + string(kind) + ".json"
	if kind == ListingBest {
		path = "/best.json"
	}
	return c.fetchListing(path, opt)
}

// Subreddit fetches a subreddit's listing.
func (c *Client) Subreddit(sub string, kind ListingKind, opt ListingOptions) ([]Post, *Paginator, error) {
	sub = sanitizeSub(sub)
	var path string
	switch kind {
	case ListingBest:
		return nil, nil, fmt.Errorf("reddit: 'best' is not a subreddit listing")
	case ListingHot:
		path = "/r/" + sub + "/hot.json"
	case ListingNew:
		path = "/r/" + sub + "/new.json"
	case ListingTop:
		path = "/r/" + sub + "/top.json"
	case ListingRising:
		path = "/r/" + sub + "/rising.json"
	case ListingControversial:
		path = "/r/" + sub + "/controversial.json"
	default:
		path = "/r/" + sub + "/" + string(kind) + ".json"
	}
	return c.fetchListing(path, opt)
}

// SubredditAbout fetches /r/x/about.json (sidebar info). Reddit returns a
// single t5 thing, not a listing.
func (c *Client) SubredditAbout(sub string) (*Subreddit, error) {
	sub = sanitizeSub(sub)
	var s Subreddit
	if err := c.Get("/r/"+sub+"/about.json", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Random fetches /r/random.json (or /r/randnsfw.json).
func (c *Client) Random(nsfw bool, opt ListingOptions) ([]Post, *Paginator, error) {
	path := "/r/random.json"
	if nsfw {
		path = "/r/randnsfw.json"
	}
	return c.fetchListing(path, opt)
}

// Popular fetches /r/popular.json (logged-out frontpage).
func (c *Client) Popular(kind ListingKind, opt ListingOptions) ([]Post, *Paginator, error) {
	return c.fetchListing("/r/popular/"+string(kind)+".json", opt)
}

// All fetches /r/all.json.
func (c *Client) All(kind ListingKind, opt ListingOptions) ([]Post, *Paginator, error) {
	return c.fetchListing("/r/all/"+string(kind)+".json", opt)
}

// Search performs a site-wide or subreddit-scoped search.
func (c *Client) Search(q string, opt SearchOptions) ([]Post, *Paginator, error) {
	if strings.TrimSpace(q) == "" {
		return nil, nil, fmt.Errorf("reddit: empty search query")
	}
	values := opt.query()
	values.Set("q", q)
	path := "/search.json"
	if opt.Subreddit != "" {
		path = "/r/" + sanitizeSub(opt.Subreddit) + "/search.json"
		values.Set("restrict_sr", "1")
	}
	return c.fetchListingRaw(path, values)
}

// SearchOptions controls a search.
type SearchOptions struct {
	Subreddit string
	Sort      SortKind
	Time      TimeWindow
	ListingOptions
}

func (o SearchOptions) query() url.Values {
	q := o.ListingOptions.query()
	if o.Sort != "" {
		q.Set("sort", string(o.Sort))
	}
	return q
}

// fetchListing builds a .json path and paginates the standard listing shape.
func (c *Client) fetchListing(path string, opt ListingOptions) ([]Post, *Paginator, error) {
	c.lastListingPath = path
	c.lastListingOpts = opt
	return c.fetchListingRaw(path, opt.query())
}

// FetchListing is the exported alias for the TUI's endless-scroll pagination.
func (c *Client) FetchListing(path string, opt ListingOptions) ([]Post, *Paginator, error) {
	return c.fetchListing(path, opt)
}

// LastListingPath returns the path of the most recent listing fetch.
func (c *Client) LastListingPath() string { return c.lastListingPath }

// LastListingOpts returns the options of the most recent listing fetch.
func (c *Client) LastListingOpts() ListingOptions { return c.lastListingOpts }

func (c *Client) fetchListingRaw(path string, q url.Values) ([]Post, *Paginator, error) {
	ensureJSON(path, &q)
	var l Listing[Post]
	if err := c.Get(path, q, &l); err != nil {
		return nil, nil, err
	}
	p := &Paginator{After: l.Data.After, Before: l.Data.Before, Count: len(l.Data.Children)}
	return l.Data.Children, p, nil
}

// SanitizeSub strips leading "r/" and stray slashes. Exported for callers.
func SanitizeSub(s string) string {
	s = strings.TrimSpace(s)
	// Handle full URLs like https://www.reddit.com/r/golang or https://old.reddit.com/r/golang
	if strings.Contains(s, "reddit.com/r/") {
		// Extract subreddit name from URL
		re := regexp.MustCompile(`reddit\.com/r/([A-Za-z0-9_]+)`)
		if m := re.FindStringSubmatch(s); m != nil {
			s = m[1]
		}
	}
	s = strings.TrimPrefix(s, "/r/")
	s = strings.TrimPrefix(s, "r/")
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	return s
}

// sanitizeSub is the internal alias.
func sanitizeSub(s string) string { return SanitizeSub(s) }

// ensureJSON makes sure path ends with .json.
func ensureJSON(path string, q *url.Values) {
	if !strings.HasSuffix(path, ".json") {
		*q = *q // no-op to use q
		// The caller is expected to add .json to path before calling.
	}
}
