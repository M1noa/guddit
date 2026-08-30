package auth

import (
	"bytes"
	"testing"
)

func TestSelfCheck(t *testing.T) {
	// ParseCookieString
	cookies, err := ParseCookieString("reddit_session=abc; token_v2=def")
	if err != nil {
		t.Fatalf("ParseCookieString: %v", err)
	}
	if len(cookies) != 2 {
		t.Errorf("expected 2 cookies, got %d", len(cookies))
	}

	// Netscape format
	netscape := `# Netscape HTTP Cookie File
.reddit.com	TRUE	/	FALSE	1893456000	reddit_session	xyz
.reddit.com	TRUE	/	FALSE	1893456000	token_v2	uvw
`
	parsed, err := ParseNetscape(bytes.NewReader([]byte(netscape)))
	if err != nil {
		t.Fatalf("ParseNetscape: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 cookies, got %d", len(parsed))
	}

	// StoredAuth tier detection
	s := StoredAuth{CookieString: "x=y"}
	if s.Tier() != TierCookie {
		t.Errorf("CookieString tier = %v", s.Tier())
	}
	s = StoredAuth{Username: "u", Password: "p"}
	if s.Tier() != TierPassword {
		t.Errorf("Username/Password tier = %v", s.Tier())
	}
	s = StoredAuth{ClientID: "id", ClientSecret: "sec", Token: &Token{AccessToken: "tok", ExpiresIn: 3600}}
	if s.Tier() != TierOAuth {
		t.Errorf("OAuth tier = %v", s.Tier())
	}
	s = StoredAuth{}
	if s.Tier() != TierNone {
		t.Errorf("empty tier = %v", s.Tier())
	}
}