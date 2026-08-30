package reddit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// DefaultUserAgent is used when none is configured. Reddit filters blank or
// generic UAs aggressively; a descriptive one is the minimum for reliability.
const DefaultUserAgent = "guddit/0.1 (by /u/guddit) — github.com/minoa/guddit"

// Hosts that serve identical .json paths. old.reddit.com is sometimes more
// lenient with unauthenticated traffic and serves as the fallback host.
const (
	HostWWW  = "www.reddit.com"
	HostOLD  = "old.reddit.com"
	HostNEW  = "new.reddit.com"
	HostOAuth = "oauth.reddit.com"
)

// Client is an HTTP client for Reddit's .json endpoints. It carries cookies
// (anonymous, cookie, or OAuth bearer), sets a descriptive User-Agent, falls
// back to old.reddit.com on repeated 429s, and backs off with Retry-After.
type Client struct {
	HTTP       *http.Client
	UserAgent  string
	Host       string // primary host (www|old|new|oauth)
	OAuthToken string // if set, sent as Authorization: bearer
	Verbose    bool
	MaxRetries int
	maxRetries int
	// lastListingPath/Opts record the most recent listing fetch so the TUI
	// can paginate without callers passing state explicitly.
	lastListingPath string
	lastListingOpts ListingOptions
}

// Option configures a Client.
type Option func(*Client)

func WithHTTPClient(h *http.Client) Option          { return func(c *Client) { c.HTTP = h } }
func WithUserAgent(ua string) Option                 { return func(c *Client) { if ua != "" { c.UserAgent = ua } } }
func WithHost(h string) Option                       { return func(c *Client) { c.Host = h } }
func WithVerbose(v bool) Option                      { return func(c *Client) { c.Verbose = v } }
func WithOAuthToken(t string) Option                 { return func(c *Client) { c.OAuthToken = t } }
func WithCookieJar(j *cookiejar.Jar) Option          { return func(c *Client) { c.HTTP.Jar = j } }
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// New returns a Client with sensible defaults.
func New(opts ...Option) *Client {
	jar, _ := cookiejar.New(nil)
	c := &Client{
		HTTP:       &http.Client{Jar: jar, Timeout: 30 * time.Second},
		UserAgent:  DefaultUserAgent,
		Host:       HostWWW,
		MaxRetries: 3,
		maxRetries: 3,
	}
	for _, o := range opts {
		o(c)
	}
	if c.MaxRetries > 0 {
		c.maxRetries = c.MaxRetries
	}
	return c
}

// hostOrder returns the primary host followed by fallbacks.
// "html" is a special marker meaning "scrape old.reddit.com HTML".
func (c *Client) hostOrder() []string {
	switch c.Host {
	case HostOLD:
		return []string{HostOLD, "html"}
	case HostNEW:
		return []string{HostNEW, HostOLD, "html"}
	case HostOAuth:
		return []string{HostOAuth}
	default:
		return []string{HostWWW, HostOLD, "html"}
	}
}

// errHTMLResponse signals a 2xx response that carried HTML where JSON was
// expected (Reddit serves block/error pages with 200). do() treats it as a
// fallback trigger instead of a hard failure.
var errHTMLResponse = errors.New("reddit: expected JSON, got HTML")

// APIError carries a non-2xx Reddit response.
type APIError struct {
	Host       string
	Path       string
	Status     int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("reddit %s: %d %s (retry-after %s)", e.Host, e.Status, e.Path, e.RetryAfter)
	}
	return fmt.Sprintf("reddit %s: %d %s", e.Host, e.Status, e.Path)
}

// IsRateLimited reports a 429 / 503 retryable response.
func (e *APIError) IsRateLimited() bool { return e.Status == 429 || e.Status == 503 }

// NotFound reports a 404 / 403 gone.
func (e *APIError) NotFound() bool { return e.Status == 404 || e.Status == 410 }

// AuthRequired reports 401/403 needing credentials.
func (e *APIError) AuthRequired() bool { return e.Status == 401 || e.Status == 403 }

// Get fetches a .json path and decodes into target. path may or may not start
// with "/"; ".json" suffix is added if missing. query is optional.
func (c *Client) Get(path string, query url.Values, target any) error {
	return c.do("GET", path, query, "", target)
}

// PostForm sends a form POST and decodes into target.
func (c *Client) PostForm(path string, form url.Values, target any) error {
	return c.do("POST", path, nil, form.Encode(), target)
}

func (c *Client) do(method, path string, query url.Values, body string, target any) error {
	if path == "" {
		return fmt.Errorf("reddit: empty path")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Default to raw_json=1 to avoid HTML-escaped selftext, unless caller
	// explicitly set raw_json in query.
	if query == nil {
		query = url.Values{}
	}
	if query.Get("raw_json") == "" {
		query.Set("raw_json", "1")
	}

	hosts := c.hostOrder()

	// Fast path: race all JSON hosts in parallel, use the first success.
	// This eliminates the sequential penalty when www.json 403s and we must
	// wait for old.json. The HTML scrape fallback runs only if all JSON hosts
	// fail.
	jsonHosts := make([]string, 0, len(hosts))
	var htmlHost string
	for _, h := range hosts {
		if h == "html" {
			htmlHost = h
		} else {
			jsonHosts = append(jsonHosts, h)
		}
	}

	if len(jsonHosts) > 0 && method == "GET" {
		type result struct {
			err  error
			host string
			idx  int
		}
		ch := make(chan result, len(jsonHosts))
		if target == nil {
			// No decode needed; race-free by construction.
			for _, h := range jsonHosts {
				go func(host string) {
					ch <- result{err: c.doOnce(method, host, path, query, body, nil), host: host}
				}(h)
			}
			var firstErr error
			for range jsonHosts {
				r := <-ch
				if r.err == nil {
					return nil
				}
				var ae *APIError
				_ = asAPIError(r.err, &ae)
			fallback := r.err == errHTMLResponse ||
				(ae != nil && (ae.AuthRequired() || ae.Status == 404 || ae.IsRateLimited()))
			if firstErr == nil || !fallback {
				firstErr = r.err
			}
			if !fallback {
				return r.err
			}
		}
		if htmlHost != "" {
			return c.scrapeAndDecode(path, query, target)
		}
		return firstErr
	}
		// Each goroutine decodes into its own buffer; the winner's buffer is
		// copied into target. Decoding the shared target concurrently would
		// race (slow hosts keep writing after we return).
		reflectBufs := make([]reflect.Value, len(jsonHosts))
		for i, h := range jsonHosts {
			go func(i int, host string) {
				buf := reflect.New(reflect.TypeOf(target).Elem())
				reflectBufs[i] = buf
				ch <- result{err: c.doOnce(method, host, path, query, body, buf.Interface()), host: host, idx: i}
			}(i, h)
		}
		var firstErr error
		for range jsonHosts {
			r := <-ch
			if r.err == nil {
				// Copy the winner's decoded value into target.
				reflect.ValueOf(target).Elem().Set(reflectBufs[r.idx].Elem())
				return nil
			}
		// errHTMLResponse and 403/404 are fallback-eligible.
		var ae *APIError
		_ = asAPIError(r.err, &ae)
		fallback := r.err == errHTMLResponse ||
			(ae != nil && (ae.AuthRequired() || ae.Status == 404 || ae.IsRateLimited()))
			if firstErr == nil || !fallback {
				firstErr = r.err
			}
			if !fallback {
				return r.err // non-recoverable error (decode, network, etc.)
			}
		}
		// All JSON hosts failed with fallback-eligible errors → try HTML.
		if htmlHost != "" {
			err := c.scrapeAndDecode(path, query, target)
			if err == nil {
				return nil
			}
			if c.Verbose {
				fmt.Fprintf(os.Stderr, "guddit: HTML fallback also failed: %v\n", err)
			}
			return err
		}
		return firstErr
	}

	// Non-GET (POST) or OAuth-only: sequential fallback.
	var lastErr error
	for _, host := range hosts {
		var err error
		if host == "html" {
			if method != "GET" {
				continue
			}
			err = c.scrapeAndDecode(path, query, target)
		} else {
			err = c.doOnce(method, host, path, query, body, target)
		}
		if err == nil {
			return nil
		}
		if err == errHTMLResponse && host != hosts[len(hosts)-1] {
			lastErr = err
			continue
		}
		var ae *APIError
		_ = asAPIError(err, &ae)
		if ae != nil && ae.IsRateLimited() && host != hosts[len(hosts)-1] {
			lastErr = err
			continue
		}
		if ae != nil && (ae.Status == 403 || ae.Status == 404) && host != hosts[len(hosts)-1] {
			lastErr = err
			continue
		}
		return err
	}
	return lastErr
}

// scrapeAndDecode fetches HTML from old.reddit.com and decodes into target.
// Target must be a pointer to []Post, Listing[Post], PostWithComments, or User.
func (c *Client) scrapeAndDecode(path string, query url.Values, target any) error {
	// User profile (about) — path is /user/{name}/about.json
	if _, ok := target.(*User); ok {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "/user/"), "/about.json")
		name = strings.TrimSuffix(name, ".json")
		u, err := c.scrapeUserProfile(name)
		if err != nil {
			return err
		}
		*(target.(*User)) = *u
		return nil
	}

	// Check if target is PostWithComments for post page scraping
	if _, ok := target.(*PostWithComments); ok {
		// Check if the path is for a post (contains /comments/)
		if strings.Contains(path, "/comments/") {
			result, err := c.scrapePostPage(path, query, true)
			if err != nil {
				return err
			}
			*(target.(*PostWithComments)) = *result
			return nil
		}
	}

	posts, pager, err := c.scrapeListing(path, query)
	if err != nil {
		return err
	}
	// Decode into target based on its type
	switch t := target.(type) {
	case *[]Post:
		*t = posts
	case *Listing[Post]:
		t.Data.Children = posts
		t.Data.After = pager.After
		t.Data.Before = pager.Before
		t.Data.Dist = pager.Count
	case *ScrapedListing:
		t.Posts = posts
		t.After = pager.After
		t.Before = pager.Before
		t.HTMLOnly = true
	default:
		return fmt.Errorf("scrapeAndDecode: unsupported target type %T", target)
	}
	return nil
}

func (c *Client) doOnce(method, host, path string, query url.Values, body string, target any) error {
	u := &url.URL{Scheme: "https", Host: host, Path: path, RawQuery: query.Encode()}
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	var resp *http.Response
	var err error
	backoff := time.Second
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, rerr := http.NewRequest(method, u.String(), bodyReader)
		if rerr != nil {
			return rerr
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/json, text/plain;q=0.5")
		if body != "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if c.OAuthToken != "" {
			req.Header.Set("Authorization", "bearer "+c.OAuthToken)
		}

		resp, err = c.HTTP.Do(req)
		if err != nil {
			if attempt == c.maxRetries {
				return err
			}
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			if attempt == c.maxRetries {
				return &APIError{Host: host, Path: path, Status: resp.StatusCode, RetryAfter: retryAfter}
			}
			if retryAfter == 0 {
				retryAfter = backoff
			}
			time.Sleep(retryAfter)
			backoff *= 2
			continue
		}
		break
	}
	if resp == nil {
		return err
	}
	defer resp.Body.Close()

	raw, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return rerr
	}
	if resp.StatusCode >= 400 {
		return &APIError{Host: host, Path: path, Status: resp.StatusCode, Body: string(raw)}
	}
	if target == nil {
		return nil
	}
	// Reddit sometimes answers a blocked/error page with 200 + text/html.
	// Treat that as a fallback trigger rather than feeding HTML to the
	// JSON decoder.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return errHTMLResponse
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("reddit: decode %s: %w", path, err)
	}
	return nil
}

// parseRetryAfter parses either a seconds integer or an HTTP-date.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func asAPIError(err error, target **APIError) bool {
	if ae, ok := err.(*APIError); ok {
		*target = ae
		return true
	}
	return false
}

// SetCookies replaces the jar with one carrying the given cookies for reddit.
// Convenience for callers that built a cookie string or parsed a cookies.txt.
func (c *Client) SetCookies(cookies []*http.Cookie) {
	jar, _ := cookiejar.New(nil)
	if len(cookies) > 0 {
		u, _ := url.Parse("https://www.reddit.com")
		jar.SetCookies(u, cookies)
		u2, _ := url.Parse("https://old.reddit.com")
		jar.SetCookies(u2, cookies)
		u3, _ := url.Parse("https://oauth.reddit.com")
		jar.SetCookies(u3, cookies)
	}
	c.HTTP.Jar = jar
}

// HasCookies reports whether the jar carries any cookies for the current host.
func (c *Client) HasCookies() bool {
	if c.HTTP.Jar == nil {
		return false
	}
	u, _ := url.Parse("https://" + c.Host)
	return len(c.HTTP.Jar.Cookies(u)) > 0
}
