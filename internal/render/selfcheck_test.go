package render

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/minoa/guddit/internal/reddit"
)

func TestSelfCheck(t *testing.T) {
	// Theme with no color
	caps := Capabilities{Out: bytes.NewBuffer(nil), Color: false}
	theme := NewTheme(caps, nil)
	// All styles should be zero-value (no-op)
	if theme.Title.String() != "" {
		t.Errorf("Theme.Title should be empty when color disabled")
	}
	// Link should return plain text
	if theme.Link("text", "http://x") != "text" {
		t.Errorf("Link should return plain when hyperlinks disabled")
	}

	// Theme with color + hyperlinks
	caps2 := Capabilities{Out: bytes.NewBuffer(nil), Color: true, Hyperlinks: true}
	theme2 := NewTheme(caps2, nil)
	linked := theme2.Link("text", "http://x")
	if linked == "text" {
		t.Errorf("Link should wrap when hyperlinks enabled")
	}
	// osc8Start = "\x1b]8;;" (5 bytes: \x1b ] 8 ; ;)
	if len(linked) < 5 || linked[:5] != "\x1b]8;;" {
		preview := linked
		if len(preview) > 10 {
			preview = preview[:10]
		}
		t.Errorf("Link should start with OSC 8 prefix: got %q", preview)
	}

	// PermalinkURL (method on Theme)
	linkOpts := LinkOptions{}
	if theme2.PermalinkURL("/r/x/comments/abc", linkOpts) != "https://old.reddit.com/r/x/comments/abc" {
		t.Errorf("PermalinkURL failed")
	}
	if theme2.PermalinkURL("https://reddit.com/x", linkOpts) != "https://reddit.com/x" {
		t.Errorf("PermalinkURL should pass through full URLs")
	}

	// Markdown with no color returns input
	md := theme.Markdown("# hello")
	if md != "# hello" {
		t.Errorf("Markdown should return input when color disabled")
	}
}

func TestImageEncoders(t *testing.T) {
	data := []byte("hello image bytes")
	b64 := base64.StdEncoding.EncodeToString(data)

	// iTerm2: OSC 1337 inline protocol with base64 payload.
	var buf bytes.Buffer
	if err := renderITerm2(&buf, data, 140, 0); err != nil {
		t.Fatalf("renderITerm2: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "\x1b]1337;File=") || !strings.HasSuffix(out, b64+"\x07") {
		t.Errorf("iTerm2 encoding wrong: %q", out[:min(len(out), 60)])
	}

	// Kitty: transmit header, chunked data, then place+display.
	buf.Reset()
	// renderKitty expects (maxWidthPx, rows). 3 rows = typical thumbnail height.
	if err := renderKitty(&buf, data, 140, 3); err != nil {
		t.Fatalf("renderKitty: %v", err)
	}
	kitty := buf.String()
	if !strings.HasPrefix(kitty, "\x1b_Ga=T,f=100,i=1") {
		t.Errorf("Kitty should open with transmit header, got %q", kitty[:min(len(kitty), 50)])
	}
	if !strings.HasSuffix(kitty, "\x1b_Ga=p,i=1,d=1,r=3,q=2;\x1b\\") {
		t.Errorf("Kitty should end with place+display, got %q", kitty[len(kitty)-50:])
	}

	// Chunked data must reassemble to the full base64.
	chunks := strings.Split(kitty, "\x1b\\")
	assembled := ""
	for _, c := range chunks {
		if !strings.HasPrefix(c, "\x1b_Ga=T,m=") {
			continue
		}
		i := strings.Index(c, ";")
		assembled += c[i+1:]
	}
	if assembled != b64 {
		t.Errorf("Kitty chunks should reassemble to base64 payload")
	}
}

func TestMediaImageURL(t *testing.T) {
	// Preview resolutions: pick largest that fits maxWidth.
	p := &reddit.Post{
		Preview: &reddit.Preview{Images: []reddit.PreviewImage{{
			Resolutions: []reddit.ImageVariant{
				{URL: "//a/140.jpg", Width: 140},
				{URL: "//a/320.jpg", Width: 320},
				{URL: "//a/640.jpg", Width: 640},
			},
			Source: reddit.ImageVariant{URL: "//a/source.jpg", Width: 1280},
		}}},
	}
	if got := p.MediaImageURL(400); got != "https://a/320.jpg" {
		t.Errorf("MediaImageURL(400) = %q, want 320.jpg", got)
	}

	// Direct image URL when no preview.
	p2 := &reddit.Post{URL: "https://i.redd.it/abc.jpg"}
	if got := p2.MediaImageURL(100); got != "https://i.redd.it/abc.jpg" {
		t.Errorf("MediaImageURL direct = %q", got)
	}

	// Placeholder thumbnail -> no image.
	p3 := &reddit.Post{Thumbnail: "self"}
	if got := p3.MediaImageURL(100); got != "" {
		t.Errorf("MediaImageURL self = %q, want empty", got)
	}
}

func TestGalleryImageURLs(t *testing.T) {
	p := &reddit.Post{
		GalleryData: []byte(`{"items":[{"media_id":"a1"},{"media_id":"b2"}]}`),
		MediaMetadata: []byte(`{"a1":{"status":"valid","s":{"u":"//a/a.jpg","x":100,"y":50},"p":[{"u":"//a/a-140.jpg","width":140}]},"b2":{"status":"valid","s":{"u":"//a/b.jpg","x":100,"y":50}}}`),
	}
	urls := p.GalleryImageURLs(400)
	if len(urls) != 2 || urls[0] != "https://a/a-140.jpg" || urls[1] != "https://a/b.jpg" {
		t.Errorf("GalleryImageURLs = %v", urls)
	}
}

func TestParseBGReply(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\x1b]11;rgb:ffff/0000/0000\x1b\\", "#ff0000"},   // xterm/kitty 4-digit
		{"\x1b]11;rgb:ff/00/00\x1b\\", "#ff0000"},          // 2-digit
		{"\x1b]11;rgb:fff/000/000\x1b\\", "#ff0000"},       // 3-digit -> first two chars
		{"\x1b]1337;SetBackgroundColor=255,0,0\x07", "#ff0000"}, // iTerm2
		{"no reply", ""},
	}
	for _, c := range cases {
		if got := parseBGReply([]byte(c.in)); got != c.want {
			t.Errorf("parseBGReply(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}