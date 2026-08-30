package auth

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ParseNetscape reads a Netscape-format cookies.txt file (the format curl,
// yt-dlp, and browser export extensions use) and returns http.Cookies.
//
// Format per line (tab-separated):
//
//	domain	flag	path	secure	expiration	name	value
//
// Lines starting with # are comments (including the "# Netscape HTTP Cookie
// File" header). Blank lines are skipped.
func ParseNetscape(r io.Reader) ([]*http.Cookie, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	var out []*http.Cookie
	for s.Scan() {
		line := strings.TrimRight(s.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue // not a cookie line
		}
		domain := strings.TrimPrefix(fields[0], ".") // leading dot = any subdomain
		path := fields[2]
		secure := strings.EqualFold(fields[3], "TRUE")
		exp, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		name := fields[5]
		val := fields[6]
		// Skip g_state cookie: its value contains JSON with quotes which breaks
		// Go's cookie parser (invalid byte '"' in Cookie.Value).
		if name == "g_state" {
			continue
		}
		c := &http.Cookie{
			Name:    name,
			Value:   val,
			Path:    path,
			Domain:  domain,
			Secure:  secure,
		}
		if exp > 0 {
			c.Expires = time.Unix(exp, 0)
		} else {
			c.Expires = time.Now().Add(365 * 24 * time.Hour)
		}
		out = append(out, c)
	}
	if err := s.Err(); err != nil {
		return out, err
	}
	if len(out) == 0 {
		return nil, errors.New("auth: cookies.txt had no cookies")
	}
	return out, nil
}

// ParseNetscapeFile is a convenience wrapper around ParseNetscape for a path.
func ParseNetscapeFile(path string) ([]*http.Cookie, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("auth: open %s: %w", path, err)
	}
	defer f.Close()
	return ParseNetscape(f)
}
