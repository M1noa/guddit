package reddit

// Wiki fetches a subreddit wiki page. If page is empty, returns the index.
func (c *Client) Wiki(sub, page string) (*WikiPage, error) {
	sub = sanitizeSub(sub)
	if page == "" {
		page = "index"
	}
	var w WikiPage
	if err := c.Get("/r/"+sub+"/wiki/"+page+".json", nil, &w); err != nil {
		return nil, err
	}
	return &w, nil
}
