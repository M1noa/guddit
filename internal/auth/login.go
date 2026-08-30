package auth

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// LoginResult is what /api/login gives back: a set of cookies (notably
// reddit_session and token_v2) harvested from the response jar.
type LoginResult struct {
	Username string
	Cookies  []*http.Cookie
	CookieJar *cookiejar.Jar
}

// LoginWithPassword POSTs Reddit's legacy /api/login endpoint to harvest the
// reddit_session + token_v2 cookies. This is the unauthenticated password tier
// — still free, no API key. Returns the cookie jar so the caller can attach it
// to a reddit.Client.
//
// Note: Reddit has weakened this flow over the years and may require 2FA or
// present a challenge; the error captures whatever Reddit responds with.
func LoginWithPassword(username, password, userAgent string) (*LoginResult, error) {
	if username == "" || password == "" {
		return nil, errors.New("auth: username and password required")
	}
	ua := userAgent
	if ua == "" {
		ua = "guddit/0.1"
	}
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}
	form := url.Values{
		"op":           {"login-main"},
		"user":         {username},
		"passwd":       {password},
		"api_type":     {"json"},
		"rem":          {"on"},
	}
	req, err := http.NewRequest("POST", "https://www.reddit.com/api/login", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain;q=0.5")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: login request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("auth: login HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}
	// /api/login returns JSON with a "json" wrapper; errors appear there.
	if looksLikeLoginError(body) {
		return nil, fmt.Errorf("auth: login rejected by Reddit: %s", truncate(body, 300))
	}
	u, _ := url.Parse("https://www.reddit.com")
	cookies := jar.Cookies(u)
	if !hasCookie(cookies, "reddit_session") && !hasCookie(cookies, "token_v2") {
		return nil, fmt.Errorf("auth: login produced no session cookies: %s", truncate(body, 200))
	}
	return &LoginResult{Username: username, Cookies: cookies, CookieJar: jar}, nil
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, c := range cookies {
		if c.Name == name && c.Value != "" {
			return true
		}
	}
	return false
}

// looksLikeLoginError scans a /api/login response for the "wrong password" /
// "RATELIMIT" / "BAD_CAPTCHA" error shape Reddit returns.
func looksLikeLoginError(body []byte) bool {
	s := string(body)
	for _, marker := range []string{`"errors"`, `"WRONG_PASSWORD"`, `"RATELIMIT"`, `"BAD_CAPTCHA"`, `"BAD_USERNAME"`, `"INCORRECT_USERNAME_PASSWORD"`} {
		if strings.Contains(s, marker) {
			// errors only count if the array is non-empty — quick heuristic.
			if strings.Contains(s, `"errors": [["`) || strings.Contains(s, `"errors":[["`) {
				return true
			}
		}
	}
	return false
}
