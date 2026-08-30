package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthClient holds Reddit script-app credentials (free, registered at
// https://www.reddit.com/prefs/apps — "script" type).
type OAuthClient struct {
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
	UserAgent    string
	HTTP         *http.Client
}

// Token is an OAuth access token + its lifetime.
type Token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
	// AcquiredAt lets callers compute remaining lifetime.
	AcquiredAt time.Time `json:"-"`
}

// ExpiresAt returns when the token expires.
func (t *Token) ExpiresAt() time.Time {
	if t.AcquiredAt.IsZero() {
		return time.Now()
	}
	return t.AcquiredAt.Add(time.Duration(t.ExpiresIn) * time.Second)
}

// Valid reports whether the token is still usable.
func (t *Token) Valid() bool {
	return t != nil && t.AccessToken != "" && time.Until(t.ExpiresAt()) > 10*time.Second
}

// TokenExchange mints a token via grant_type=password (script-app flow).
func (o *OAuthClient) TokenExchange() (*Token, error) {
	if o.ClientID == "" || o.ClientSecret == "" {
		return nil, errors.New("auth: oauth requires client_id and client_secret")
	}
	if o.Username == "" || o.Password == "" {
		return nil, errors.New("auth: oauth password grant requires username and password")
	}
	form := url.Values{
		"grant_type": {"password"},
		"username":   {o.Username},
		"password":   {o.Password},
	}
	return o.postToken(form)
}

// TokenFromAuthorizationCode mints a token from a one-time authorization code
// (used by the browser/Google login capture flow).
func (o *OAuthClient) TokenFromAuthorizationCode(code, redirectURI string) (*Token, error) {
	if o.ClientID == "" || o.ClientSecret == "" {
		return nil, errors.New("auth: oauth requires client_id and client_secret")
	}
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	return o.postToken(form)
}

func (o *OAuthClient) postToken(form url.Values) (*Token, error) {
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequest("POST", "https://www.reddit.com/api/v1/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(o.ClientID, o.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ua := o.UserAgent
	if ua == "" {
		ua = "guddit/0.1"
	}
	req.Header.Set("User-Agent", ua)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: oauth token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("auth: oauth token request: %d %s", resp.StatusCode, truncate(body, 200))
	}
	var t Token
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("auth: oauth decode: %w", err)
	}
	if t.AccessToken == "" {
		return nil, fmt.Errorf("auth: oauth returned no access_token: %s", truncate(body, 200))
	}
	t.AcquiredAt = time.Now()
	return &t, nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
