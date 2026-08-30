package render

import (
	"strings"
)

// LinkFormat controls how links are displayed.
type LinkFormat int

const (
	LinkFormatShort LinkFormat = iota // just the ID (e.g., 1vdizu5)
	LinkFormatMedium                  // short permalink (e.g., redd.it/1vdizu5)
	LinkFormatLong                    // full permalink
)

// LinkDomain controls which domain to use for links.
type LinkDomain int

const (
	LinkDomainOld LinkDomain = iota    // old.reddit.com
	LinkDomainWWW                       // www.reddit.com
	LinkDomainCustom                    // custom domain from config
)

// LinkOptions holds link formatting options.
type LinkOptions struct {
	Format LinkFormat
	Domain LinkDomain
	CustomDomain string
}

// OSC 8 hyperlink escape sequences. The standard is:
//
//	\x1b]8;;<URL>\x1b\\<TEXT>\x1b]8;;\x1b\\
//
// Where \x1b\\ is the String Terminator (ST). Some terminals accept BEL (\x07)
// as ST; we use the canonical ST form.
const (
	osc8Start = "\x1b]8;;"
	osc8Sep   = "\x1b\\"
	osc8End   = "\x1b]8;;\x1b\\"
)

// Link wraps text in an OSC 8 hyperlink to url if the terminal supports them.
// Otherwise returns the text unchanged (callers may apply a URL style).
func (t Theme) Link(text, linkURL string) string {
	if !t.Caps.Hyperlinks || linkURL == "" {
		return text
	}
	return osc8Start + linkURL + osc8Sep + text + osc8End
}

// LinkWithOptions creates a link with custom formatting options.
func (t Theme) LinkWithOptions(text, linkURL string, opts LinkOptions) string {
	if linkURL == "" {
		return text
	}
	formattedURL := t.formatURL(linkURL, opts)
	if !t.Caps.Hyperlinks {
		return formattedURL
	}
	return osc8Start + formattedURL + osc8Sep + text + osc8End
}

// formatURL applies the link format and domain options.
func (t Theme) formatURL(rawURL string, opts LinkOptions) string {
	// Extract the post ID from the URL
	id := extractPostID(rawURL)
	if id == "" {
		return rawURL
	}

	domain := t.getDomain(opts.Domain)
	
	switch opts.Format {
	case LinkFormatShort:
		return id
	case LinkFormatMedium:
		return "https://redd.it/" + id
	case LinkFormatLong:
		return "https://" + domain + "/comments/" + id
	default:
		return rawURL
	}
}

func (t Theme) getDomain(domain LinkDomain) string {
	switch domain {
	case LinkDomainOld:
		return "old.reddit.com"
	case LinkDomainWWW:
		return "www.reddit.com"
	case LinkDomainCustom:
		if t.Opts.CustomDomain != "" {
			return t.Opts.CustomDomain
		}
		return "old.reddit.com"
	default:
		return "old.reddit.com"
	}
}

func extractPostID(url string) string {
	// Handle various URL formats:
	// https://www.reddit.com/r/sub/comments/abc123/title/
	// https://old.reddit.com/r/sub/comments/abc123/
	// https://redd.it/abc123
	// abc123 (just ID)
	
	// Try to extract from /comments/ID/ pattern
	if idx := strings.Index(url, "/comments/"); idx != -1 {
		start := idx + len("/comments/")
		end := start
		for end < len(url) && url[end] != '/' {
			end++
		}
		if end > start {
			return url[start:end]
		}
	}
	
	// Try redd.it/ID
	if idx := strings.Index(url, "redd.it/"); idx != -1 {
		start := idx + len("redd.it/")
		end := start
		for end < len(url) && url[end] != '/' && url[end] != '?' && url[end] != '#' {
			end++
		}
		if end > start {
			return url[start:end]
		}
	}
	
	// If it looks like a bare ID (alphanumeric, 5-8 chars)
	url = strings.TrimSpace(url)
	if len(url) >= 5 && len(url) <= 8 {
		isID := true
		for _, c := range url {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				isID = false
				break
			}
		}
		if isID {
			return url
		}
	}
	
	return ""
}

// PermalinkURL builds the full https reddit URL for a post permalink fragment.
func (t Theme) PermalinkURL(permalink string, opts LinkOptions) string {
	if permalink == "" {
		return ""
	}
	if strings.HasPrefix(permalink, "http") {
		return permalink
	}
	if !strings.HasPrefix(permalink, "/") {
		permalink = "/" + permalink
	}
	domain := t.getDomain(opts.Domain)
	return "https://" + domain + permalink
}

// AuthorURL builds a user profile URL.
func (t Theme) AuthorURL(author string, opts LinkOptions) string {
	if author == "" || author == "[deleted]" {
		return ""
	}
	domain := t.getDomain(opts.Domain)
	return "https://" + domain + "/u/" + strings.TrimPrefix(author, "/u/")
}

// SubredditURL builds a subreddit URL.
func (t Theme) SubredditURL(sub string, opts LinkOptions) string {
	sub = strings.TrimPrefix(strings.TrimPrefix(sub, "/r/"), "r/")
	domain := t.getDomain(opts.Domain)
	return "https://" + domain + "/r/" + sub
}

// ShortLink builds a redd.it shortlink for a post id.
func (t Theme) ShortLink(id string, opts LinkOptions) string {
	return "https://redd.it/" + id
}
