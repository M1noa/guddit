// Package auth handles Reddit authentication: cookie strings, Netscape
// cookies.txt files, username/password login, and OAuth script-app tokens.
// All flows are free Reddit paths — no paid API.
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// ParseCookieString parses a raw "k=v; k2=v2" header into http.Cookies for
// the reddit hosts. Values are URL-decoded where appropriate.
func ParseCookieString(raw string) ([]*http.Cookie, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "Cookie:"))
	if raw == "" {
		return nil, errors.New("auth: empty cookie string")
	}
	// Reddit cookies are typically unquoted; tolerate quoted values.
	var out []*http.Cookie
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		name := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		val = strings.Trim(val, `"`)
		out = append(out, &http.Cookie{
			Name:    name,
			Value:   val,
			Path:    "/",
			Domain:  "reddit.com",
			Expires: time.Now().Add(365 * 24 * time.Hour),
		})
	}
	if len(out) == 0 {
		return nil, errors.New("auth: no cookies parsed")
	}
	return out, nil
}

// ApplyCookies installs the cookies onto a cookie jar covering the reddit
// hosts so the client carries them on subsequent requests.
func ApplyCookies(jar *cookiejar.Jar, cookies []*http.Cookie) {
	if len(cookies) == 0 || jar == nil {
		return
	}
	for _, host := range []string{"https://www.reddit.com", "https://old.reddit.com", "https://oauth.reddit.com", "https://new.reddit.com"} {
		u, _ := url.Parse(host)
		jar.SetCookies(u, cookies)
	}
}

// CookiesFromJar returns the cookies the jar currently holds for a host, as a
// "k=v; k2=v2" header string suitable for logging or re-storing.
func CookiesFromJar(jar *cookiejar.Jar, host string) string {
	u, _ := url.Parse(host)
	if u == nil {
		return ""
	}
	var b strings.Builder
	for i, c := range jar.Cookies(u) {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s=%s", c.Name, c.Value)
	}
	return b.String()
}

// AuthTier describes how the client is authenticating.
type AuthTier string

const (
	TierNone     AuthTier = "anonymous"
	TierCookie   AuthTier = "cookie"
	TierPassword AuthTier = "password" // harvested cookies via /api/login
	TierOAuth     AuthTier = "oauth"     // script-app bearer token
)
