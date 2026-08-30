package reddit

import (
	"net/url"
)

// Me fetches /api/v1/me.json (requires authentication — cookies or OAuth).
// Uses oauth.reddit.com which accepts bearer tokens for auth endpoints.
func (c *Client) Me() (*Me, error) {
	var m Me
	if err := c.getWithHost(HostOAuth, "/api/v1/me.json", nil, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Karma fetches /api/v1/me/karma.json (requires authentication).
func (c *Client) Karma() (*KarmaBreakdown, error) {
	var k KarmaBreakdown
	if err := c.getWithHost(HostOAuth, "/api/v1/me/karma.json", nil, &k); err != nil {
		return nil, err
	}
	return &k, nil
}

// MyListing fetches an authenticated user feed: saved, upvoted, hidden, or
// submitted. Uses oauth.reddit.com for bearer auth.
func (c *Client) MyListing(kind MyListingKind, opt ListingOptions) ([]Post, *Paginator, error) {
	return c.fetchListingWithHost(HostOAuth, "/user/me/"+string(kind)+".json", opt)
}

// getWithHost performs a GET request using a specific host (e.g., oauth.reddit.com).
func (c *Client) getWithHost(host, path string, query url.Values, target any) error {
	oldHost := c.Host
	c.Host = host
	defer func() { c.Host = oldHost }()
	return c.Get(path, query, target)
}

// fetchListingWithHost performs a listing fetch using a specific host.
func (c *Client) fetchListingWithHost(host, path string, opt ListingOptions) ([]Post, *Paginator, error) {
	oldHost := c.Host
	c.Host = host
	defer func() { c.Host = oldHost }()
	return c.fetchListing(path, opt)
}

// MyListingKind selects the authenticated user's personal feed.
type MyListingKind string

const (
	MySaved    MyListingKind = "saved"
	MyUpvoted  MyListingKind = "upvoted"
	MyHidden   MyListingKind = "hidden"
	MySubmitted MyListingKind = "submitted"
	MyOverview MyListingKind = "overview"
)
