package cmd

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/output"
	"github.com/minoa/guddit/internal/reddit"
	"github.com/minoa/guddit/internal/render"
)

// server holds the state shared by every /api handler: a reddit client built
// once at startup and an optional bearer password guarding the API.
type server struct {
	client   *reddit.Client
	password string
}

// serveCmd exposes the CLI as a local HTTP API. Endpoints mirror the
// subcommands; JSON is the default response, ?format=csv (or agent) works too.
var serveCmd = &cobra.Command{
	Use:     "serve",
	Aliases: []string{"web", "api"},
	Short:   "Expose guddit as a local HTTP API",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		port, _ := cmd.Flags().GetInt("port")
		password, _ := cmd.Flags().GetString("password")

		s := &server{client: c, password: password}
		mux := http.NewServeMux()
		mux.HandleFunc("/", s.handleDocs)
		mux.HandleFunc("/docs.json", s.handleSpec)
		mux.HandleFunc("/api/home", s.auth(s.handleHome))
		mux.HandleFunc("/api/sub/{sub}/about", s.auth(s.handleSubAbout))
		mux.HandleFunc("/api/sub/{sub}", s.auth(s.handleSub))
		mux.HandleFunc("/api/search", s.auth(s.handleSearch))
		mux.HandleFunc("/api/post/{ref}", s.auth(s.handlePost))
		mux.HandleFunc("/api/user/{name}", s.auth(s.handleUser))
		mux.HandleFunc("/api/user/{name}/{kind}", s.auth(s.handleUserKind))
		mux.HandleFunc("/api/me", s.auth(s.handleMe))
		mux.HandleFunc("/api/me/{kind}", s.auth(s.handleMeKind))
		mux.HandleFunc("/api/random", s.auth(s.handleRandom))
		mux.HandleFunc("/api/wiki/{sub}/{page}", s.auth(s.handleWiki))
		mux.HandleFunc("/api/wiki/{sub}", s.auth(s.handleWiki))

		addr := ":" + strconv.Itoa(port)
		auth := "no password"
		if password != "" {
			auth = "Authorization: Bearer <password>"
		}
		fmt.Fprintf(os.Stderr, "guddit serve: listening on http://127.0.0.1%s\n", addr)
		fmt.Fprintf(os.Stderr, "  docs:   http://127.0.0.1%s/\n", addr)
		fmt.Fprintf(os.Stderr, "  auth:   %s\n", auth)
		return http.ListenAndServe(addr, mux)
	},
}

func init() {
	serveCmd.Flags().Int("port", 8484, "port to listen on")
	serveCmd.Flags().String("password", "", "require Authorization: Bearer <password> on /api/*")
	rootCmd.AddCommand(serveCmd)
}

// --- auth ----------------------------------------------------------------

// auth wraps an API handler with bearer-token protection when a password is set.
func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.password != "" && !bearerOK(r, s.password) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="guddit"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// bearerOK does a constant-time compare of the Authorization header token.
func bearerOK(r *http.Request, pw string) bool {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(h, "Bearer ")), []byte(pw)) == 1
}

// --- request helpers -----------------------------------------------------

func qStr(r *http.Request, name, def string) string {
	if s := r.URL.Query().Get(name); s != "" {
		return s
	}
	return def
}

func qInt(r *http.Request, name string, def int) int {
	if s := r.URL.Query().Get(name); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

func qBool(r *http.Request, name string) bool {
	if !r.URL.Query().Has(name) {
		return false
	}
	s := r.URL.Query().Get(name)
	return s == "" || s == "1" || strings.EqualFold(s, "true")
}

// serveListingOpts builds ListingOptions from config defaults overridden by
// query params.
func serveListingOpts(r *http.Request) reddit.ListingOptions {
	opt := listingOpts()
	if n := qInt(r, "limit", 0); n > 0 {
		opt.Limit = n
	}
	opt.After = qStr(r, "after", opt.After)
	opt.Before = qStr(r, "before", opt.Before)
	if t := parseTime(qStr(r, "time", "")); t != "" {
		opt.Time = t
	}
	if r.URL.Query().Has("nsfw") {
		opt.NSFW = qBool(r, "nsfw")
	}
	if r.URL.Query().Has("safe_search") {
		opt.SafeSearch = qBool(r, "safe_search")
	}
	return opt
}

// serveFormat resolves ?format, defaulting to json.
func serveFormat(r *http.Request) (output.Format, error) {
	f := output.Format(qStr(r, "format", "json"))
	switch f {
	case output.FormatPretty, output.FormatJSON, output.FormatCSV, output.FormatAgent,
		output.FormatTable, output.FormatMarkdown, output.FormatTxt:
		return f, nil
	}
	return "", fmt.Errorf("invalid format %q (want json, csv, agent, pretty, table, markdown, txt)", f)
}

// writeResult renders a result in the requested format and writes it out.
func (s *server) writeResult(w http.ResponseWriter, r *http.Request, result any) {
	format, err := serveFormat(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	var buf bytes.Buffer
	if err := output.Write(format, render.Capabilities{}, result, &buf, themeOptions(), linkOptions()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ct := "application/json; charset=utf-8"
	switch format {
	case output.FormatCSV, output.FormatAgent:
		ct = "text/csv; charset=utf-8"
	case output.FormatPretty, output.FormatMarkdown, output.FormatTxt:
		ct = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(buf.Bytes())
}

func httpErr(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}

// --- endpoints -----------------------------------------------------------

func (s *server) handleHome(w http.ResponseWriter, r *http.Request) {
	kind := reddit.ListingHot
	if k, err := parseListingKind(qStr(r, "kind", "")); err != nil {
		badRequest(w, err)
		return
	} else {
		kind = k
	}
	opt := serveListingOpts(r)
	posts, pager, err := s.client.Home(kind, opt)
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, output.WithPaginator(posts, pager))
}

func (s *server) handleSub(w http.ResponseWriter, r *http.Request) {
	sub := reddit.SanitizeSub(r.PathValue("sub"))
	kind := reddit.ListingHot
	if k, err := parseListingKind(qStr(r, "kind", "")); err != nil {
		badRequest(w, err)
		return
	} else {
		kind = k
	}
	opt := serveListingOpts(r)
	posts, pager, err := s.client.Subreddit(sub, kind, opt)
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, output.WithPaginator(posts, pager))
}

func (s *server) handleSubAbout(w http.ResponseWriter, r *http.Request) {
	sub := reddit.SanitizeSub(r.PathValue("sub"))
	info, err := s.client.SubredditAbout(sub)
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, info)
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := qStr(r, "q", "")
	if q == "" {
		badRequest(w, fmt.Errorf("missing query param: q"))
		return
	}
	sort := reddit.SortRelevance
	if s, err := parseSort(qStr(r, "sort", "")); err != nil {
		badRequest(w, err)
		return
	} else {
		sort = s
	}
	opt := reddit.SearchOptions{
		Subreddit:      qStr(r, "sub", ""),
		Sort:           sort,
		ListingOptions: serveListingOpts(r),
	}
	posts, pager, err := s.client.Search(q, opt)
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, output.WithPaginator(posts, pager))
}

func (s *server) handlePost(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	comments := !r.URL.Query().Has("comments") || qBool(r, "comments")
	thread, err := s.client.Post(ref, comments)
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, thread)
}

func (s *server) handleUser(w http.ResponseWriter, r *http.Request) {
	u, err := s.client.UserAbout(r.PathValue("name"))
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, u)
}

func (s *server) handleUserKind(w http.ResponseWriter, r *http.Request) {
	kind, err := parseUserKind(r.PathValue("kind"))
	if err != nil {
		badRequest(w, err)
		return
	}
	posts, pager, err := s.client.UserListing(r.PathValue("name"), kind, serveListingOpts(r))
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, output.WithPaginator(posts, pager))
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	m, err := s.client.Me()
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, m)
}

func (s *server) handleMeKind(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind == "karma" {
		k, err := s.client.Karma()
		if err != nil {
			httpErr(w, err)
			return
		}
		s.writeResult(w, r, k)
		return
	}
	mk, err := parseMyKind(kind)
	if err != nil {
		badRequest(w, err)
		return
	}
	posts, pager, err := s.client.MyListing(mk, serveListingOpts(r))
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, output.WithPaginator(posts, pager))
}

func (s *server) handleRandom(w http.ResponseWriter, r *http.Request) {
	posts, pager, err := s.client.Random(qBool(r, "nsfw"), serveListingOpts(r))
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, output.WithPaginator(posts, pager))
}

func (s *server) handleWiki(w http.ResponseWriter, r *http.Request) {
	sub := reddit.SanitizeSub(r.PathValue("sub"))
	page := r.PathValue("page")
	if page == "" {
		page = "index"
	}
	wpg, err := s.client.Wiki(sub, page)
	if err != nil {
		httpErr(w, err)
		return
	}
	s.writeResult(w, r, wpg)
}

// --- docs ----------------------------------------------------------------

type apiSpec struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Description string     `json:"description"`
	BaseURL     string     `json:"base_url"`
	Auth        string     `json:"auth"`
	Formats     []string   `json:"formats"`
	Endpoints   []endpoint `json:"endpoints"`
}

type endpoint struct {
	Path   string  `json:"path"`
	Method string  `json:"method"`
	Desc   string  `json:"description"`
	Params []param `json:"params"`
	Auth   bool    `json:"requires_auth"`
}

type param struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
	Desc    string `json:"description"`
}

// serveSpec is the machine-readable API description served at /docs.json.
var serveSpec = apiSpec{
	Name:        "guddit",
	Version:     version,
	Description: "Local HTTP API mirroring the guddit CLI. JSON by default; add ?format=csv (or agent) for tabular output.",
	BaseURL:     "/api",
	Auth:        "Optional. When serve runs with --password, every /api/* request needs Authorization: Bearer <password>.",
	Formats:     []string{"json", "csv", "agent", "pretty", "table", "markdown", "txt"},
	Endpoints: []endpoint{
		{Path: "/api/home", Method: "GET", Desc: "Frontpage listing.", Params: []param{
			{Name: "kind", Type: "string", Default: "hot", Desc: "hot|new|top|rising|controversial|best"},
			{Name: "time", Type: "string", Default: "", Desc: "top/controversial window: hour|day|week|month|year|all"},
			{Name: "limit", Type: "int", Default: "", Desc: "max items (1-100)"},
			{Name: "after", Type: "string", Desc: "pagination cursor"},
			{Name: "before", Type: "string", Desc: "pagination cursor"},
		}},
		{Path: "/api/sub/{sub}", Method: "GET", Desc: "Subreddit listing.", Params: []param{
			{Name: "kind", Type: "string", Default: "hot", Desc: "hot|new|top|rising|controversial"},
			{Name: "time", Type: "string", Desc: "top/controversial window"},
			{Name: "limit", Type: "int", Desc: "max items (1-100)"},
			{Name: "after", Type: "string", Desc: "pagination cursor"},
			{Name: "before", Type: "string", Desc: "pagination cursor"},
		}},
		{Path: "/api/sub/{sub}/about", Method: "GET", Desc: "Subreddit sidebar/about info."},
		{Path: "/api/search", Method: "GET", Desc: "Search Reddit or a subreddit.", Params: []param{
			{Name: "q", Type: "string", Desc: "search query (required)"},
			{Name: "sub", Type: "string", Desc: "restrict search to a subreddit"},
			{Name: "sort", Type: "string", Default: "relevance", Desc: "relevance|hot|new|top|comments"},
			{Name: "time", Type: "string", Desc: "top window"},
			{Name: "limit", Type: "int", Desc: "max items"},
		}},
		{Path: "/api/post/{ref}", Method: "GET", Desc: "Post and optional comment tree.", Params: []param{
			{Name: "comments", Type: "bool", Default: "true", Desc: "set comments=false to skip the comment tree"},
		}},
		{Path: "/api/user/{name}", Method: "GET", Desc: "User profile."},
		{Path: "/api/user/{name}/{kind}", Method: "GET", Desc: "User activity feed.", Params: []param{
			{Name: "kind", Type: "string", Desc: "overview|submitted|comments (path segment)"},
			{Name: "limit", Type: "int", Desc: "max items"},
		}},
		{Path: "/api/me", Method: "GET", Desc: "Authenticated user (requires cookies/OAuth config)."},
		{Path: "/api/me/{kind}", Method: "GET", Desc: "Authenticated feed or karma.", Params: []param{
			{Name: "kind", Type: "string", Desc: "saved|upvoted|hidden|overview|submitted|karma (path segment)"},
		}},
		{Path: "/api/random", Method: "GET", Desc: "Random post.", Params: []param{
			{Name: "nsfw", Type: "bool", Default: "false", Desc: "use r/randnsfw"},
		}},
		{Path: "/api/wiki/{sub}/{page}", Method: "GET", Desc: "Subreddit wiki page (page defaults to index)."},
	},
}

func (s *server) handleSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(serveSpec)
}

// handleDocs serves the minimal HTML docs page. It fetches /docs.json and
// renders the endpoint list client-side, with a link to the raw spec.
func (s *server) handleDocs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsHTML))
}

const docsHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>guddit API</title>
<style>
  body { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; max-width: 44rem; margin: 2rem auto; padding: 0 1rem; color: #222; line-height: 1.5; }
  h1 { font-size: 1.3rem; }
  code { background: #f2f2f2; padding: 0 0.25rem; border-radius: 3px; }
  table { width: 100%; border-collapse: collapse; }
  th, td { text-align: left; padding: 0.4rem 0.5rem; border-bottom: 1px solid #e5e5e5; vertical-align: top; }
  th { border-bottom: 2px solid #ccc; }
  .m { color: #7f8c8d; }
</style>
</head>
<body>
<header>
  <h1>guddit <span class="m">/ local reddit api</span></h1>
  <p><a href="/docs.json">view raw /docs.json</a></p>
</header>
<main id="app"><p>loading spec…</p></main>
<script>
fetch('/docs.json').then(r => r.json()).then(spec => {
  const el = document.getElementById('app');
  const rows = spec.endpoints.map(e =>
    '<tr><td><code>' + e.method + ' ' + e.path + '</code></td><td>' + e.description + '</td></tr>'
  ).join('');
  el.innerHTML =
    '<p>Base URL <code>' + spec.base_url + '</code>. ' +
    '<code>?format=csv</code> (or <code>agent</code>) switches from JSON to tabular output.</p>' +
    '<table><thead><tr><th>endpoint</th><th>description</th></tr></thead><tbody>' + rows + '</tbody></table>' +
    '<p class="m">' + spec.auth + '</p>';
}).catch(e => {
  document.getElementById('app').textContent = 'failed to load /docs.json: ' + e;
});
</script>
</body>
</html>
`