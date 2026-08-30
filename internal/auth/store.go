package auth

import (
	"net/http"
	"net/http/cookiejar"
	"time"
)

// StoredAuth is the serializable credential bundle persisted in config.yaml.
// Only one tier is populated at a time; the active one is determined by the
// order of preference oauth > cookies > username/password.
type StoredAuth struct {
	// Cookie string (raw header) — for the --cookie tier.
	CookieString string `yaml:"cookie_string,omitempty"`

	// Cookies harvested from password login or browser capture, as k=v pairs
	// that can be re-fed into ParseCookieString.
	Cookies []StoredCookie `yaml:"cookies,omitempty"`

	// Username + password — re-used to harvest fresh cookies or mint tokens.
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`

	// OAuth script-app credentials + last known token.
	ClientID     string `yaml:"client_id,omitempty"`
	ClientSecret string `yaml:"client_secret,omitempty"`
	Token        *Token `yaml:"token,omitempty"`
}

// StoredCookie is a flattened http.Cookie for persistence.
type StoredCookie struct {
	Name    string `yaml:"name"`
	Value   string `yaml:"value"`
	Path    string `yaml:"path,omitempty"`
	Domain  string `yaml:"domain,omitempty"`
	Expires int64  `yaml:"expires,omitempty"` // unix epoch; 0 = session
	Secure  bool   `yaml:"secure,omitempty"`
}

// ToHTTPCookies converts stored cookies back to http.Cookies.
func (s *StoredAuth) ToHTTPCookies() []*http.Cookie {
	var out []*http.Cookie
	for _, c := range s.Cookies {
		hc := &http.Cookie{
			Name: c.Name, Value: c.Value,
			Path: c.Path, Domain: c.Domain, Secure: c.Secure,
		}
		if c.Expires > 0 {
			hc.Expires = time.Unix(c.Expires, 0)
		} else {
			hc.Expires = time.Now().Add(365 * 24 * time.Hour)
		}
		out = append(out, hc)
	}
	return out
}

// FromHTTPCookies flattens http.Cookies (e.g. from a jar) into stored form.
func FromHTTPCookies(cookies []*http.Cookie) []StoredCookie {
	var out []StoredCookie
	for _, c := range cookies {
		sc := StoredCookie{Name: c.Name, Value: c.Value, Path: c.Path, Domain: c.Domain, Secure: c.Secure}
		if !c.Expires.IsZero() {
			sc.Expires = c.Expires.Unix()
		}
		out = append(out, sc)
	}
	return out
}

// ApplyTo installs this auth onto a cookie jar (for the cookie/password tiers).
func (s *StoredAuth) ApplyTo(jar *cookiejar.Jar) {
	if jar == nil {
		return
	}
	cookies := s.ToHTTPCookies()
	if s.CookieString != "" {
		if parsed, err := ParseCookieString(s.CookieString); err == nil {
			cookies = append(cookies, parsed...)
		}
	}
	ApplyCookies(jar, cookies)
}

// Tier reports the active authentication tier, in preference order.
func (s *StoredAuth) Tier() AuthTier {
	switch {
	case s.Token != nil && s.Token.Valid():
		return TierOAuth
	case s.Token != nil && s.ClientID != "" && s.ClientSecret != "":
		return TierOAuth
	case len(s.Cookies) > 0 || s.CookieString != "":
		return TierCookie
	case s.Username != "" && s.Password != "":
		return TierPassword
	}
	return TierNone
}
