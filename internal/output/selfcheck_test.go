package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/minoa/guddit/internal/reddit"
	"github.com/minoa/guddit/internal/render"
)

func TestSelfCheck(t *testing.T) {
	// Format parsing
	if Parse("json", true) != FormatJSON {
		t.Errorf("Parse json TTY")
	}
	if Parse("", false) != FormatJSON {
		t.Errorf("Parse empty non-TTY = json")
	}
	if Parse("", true) != FormatPretty {
		t.Errorf("Parse empty TTY = pretty")
	}

	caps := render.Capabilities{}
	themeOpts := &render.ThemeOptions{}

	// JSON output
	var buf bytes.Buffer
	err := Write(FormatJSON, caps, []reddit.Post{{ID: "1", Title: "test"}}, &buf, themeOpts, render.LinkOptions{})
	if err != nil {
		t.Fatalf("Write JSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"title": "test"`) {
		t.Errorf("JSON output missing title: %s", buf.String())
	}

	// CSV output
	buf.Reset()
	err = Write(FormatCSV, caps, []reddit.Post{{ID: "1", Title: "test", Author: "u", Subreddit: "r", Score: 5}}, &buf, themeOpts, render.LinkOptions{})
	if err != nil {
		t.Fatalf("Write CSV: %v", err)
	}
	if !strings.Contains(buf.String(), "test") {
		t.Errorf("CSV output missing title: %s", buf.String())
	}

	// Table output
	buf.Reset()
	err = Write(FormatTable, caps, []reddit.Post{{ID: "1", Title: "test", Author: "u", Subreddit: "r", Score: 5}}, &buf, themeOpts, render.LinkOptions{})
	if err != nil {
		t.Fatalf("Write Table: %v", err)
	}
	if !strings.Contains(buf.String(), "test") {
		t.Errorf("Table output missing title: %s", buf.String())
	}

	// Markdown output
	buf.Reset()
	err = Write(FormatMarkdown, caps, []reddit.Post{{ID: "1", Title: "test", Author: "u", Subreddit: "r", Score: 5}}, &buf, themeOpts, render.LinkOptions{})
	if err != nil {
		t.Fatalf("Write Markdown: %v", err)
	}
	if !strings.Contains(buf.String(), "# test") {
		t.Errorf("Markdown output missing title: %s", buf.String())
	}

	// Txt output
	buf.Reset()
	err = Write(FormatTxt, caps, []reddit.Post{{ID: "1", Title: "test", Author: "u", Subreddit: "r", Score: 5}}, &buf, themeOpts, render.LinkOptions{})
	if err != nil {
		t.Fatalf("Write Txt: %v", err)
	}
	if !strings.Contains(buf.String(), "test") {
		t.Errorf("Txt output missing title: %s", buf.String())
	}

	// WithPaginator wrapper
	wrapped := WithPaginator([]reddit.Post{{ID: "1"}}, &reddit.Paginator{After: "t3_abc"})
	if _, ok := wrapped.(withPaginator); !ok {
		t.Errorf("WithPaginator type")
	}
}