# guddit

> THIS PROJECT WAS MADE PARTIALLY USING AGENTIC AI CODING TOOLS

> **WIP:** unfinished and not working in its current state. A lot is left to do.

Reddit from your terminal, no API key. Uses Reddit's public `.json` endpoints with `old.reddit.com` fallback, cookie auth, and OAuth script-app support. Search, read threads, browse subreddits, and run it as a local HTTP API.

> **2026 note**: Reddit now serves block pages to unauthenticated `.json` requests. Authenticate with cookies (exported from your browser) or OAuth script-app credentials. The tool works fully once authenticated.

## Install

```bash
go install github.com/minoa/guddit/cmd/guddit@latest

# or from source
git clone https://github.com/minoa/guddit
cd guddit && make build
```

## Auth

No paid API tier. Pick one:

| Method | Command |
|--------|---------|
| Cookie string | `--cookie "reddit_session=...; token_v2=..."` |
| Cookies file | `--cookie-file cookies.txt` (Netscape format) |
| Username + password | `guddit auth login --method password -u you -p pass` |
| OAuth script-app | `--client-id --client-secret` + stored password |
| Browser/Google | `guddit auth login --method browser\|google` |

Save cookies or credentials permanently:

```bash
guddit settings set cookie_string "reddit_session=...; token_v2=..."
guddit settings set user_agent "mybot/1.0 by u/me"
```

## Usage

```bash
guddit home --limit 10                       # frontpage
guddit sub golang top -t week                # subreddit listing
guddit search "golang cli" --sub golang      # search
guddit post https://redd.it/abc123           # thread + comments
guddit user spez overview                    # user activity
guddit me saved                              # authenticated feeds
guddit wiki golang faq                       # wiki page
guddit random                                # random post
```

### Serve as an API

```bash
guddit serve --port 8484 --password hunter2
```

Serves a minimal HTML docs page at `http://localhost:8484/`, a machine-readable spec at `/docs.json`, and mirrors every CLI command under `/api/*`. JSON by default; `?format=csv` (or `agent`) switches output. With `--password`, every `/api/*` request needs `Authorization: Bearer <password>`.

```bash
curl localhost:8484/api/sub/golang?limit=5&format=agent
curl -H "Authorization: Bearer hunter2" localhost:8484/api/me
```

### Output formats

| Format | Use |
|--------|-----|
| `pretty` | Human-readable, colored, OSC 8 links (default on TTY) |
| `json` | Raw normalized JSON (default piped/non-TTY) |
| `csv` | Flattened rows for spreadsheets |
| `agent` | Compact CSV, minimal tokens for LLM consumption |
| `table` | Aligned columns |
| `markdown` | Reddit-flavored markdown export |
| `txt` | Plain text, no ANSI |

`--agent` is a shorthand for `--format agent`. Pipe to a file with `-o out.json`.

## Commands

| Command | Description |
|---------|-------------|
| `home [kind]` | Frontpage (hot/new/top/rising/controversial/best) |
| `sub <name> [kind] [--time]` | Subreddit listing |
| `sub <name> about` | Subreddit sidebar |
| `search <query> [--sub] [--sort] [--time]` | Search Reddit |
| `post <url\|id> [--comments=false]` | Thread + comment tree |
| `user <name> [overview\|submitted\|comments]` | User profile |
| `me [saved\|upvoted\|hidden\|overview\|karma]` | Authenticated feeds |
| `random [--nsfw]` | Random post |
| `wiki <sub> [page]` | Wiki pages |
| `serve [--port] [--password]` | Local HTTP API |
| `auth login/logout/status` | Manage credentials |
| `settings [get\|set\|unset\|list]` | Persistent config |
| `completion [bash\|zsh\|fish\|powershell]` | Shell completions |

## Global flags

```
-f, --format string      pretty|json|csv|agent|table|markdown|txt
--agent                  shorthand for --format agent
-o, --output string      write to file instead of stdout
--cookie string          raw cookie header
--cookie-file string     path to Netscape cookies.txt
--user-agent string      override User-Agent (default guddit/<version>)
--host string            www|old|new|oauth
-L, --limit int          limit items (max 100)
--after/--before string  pagination cursors
--no-color               disable ANSI color
--no-links               disable OSC 8 hyperlinks
-v, --verbose            verbose output
```

Config persists to `~/.config/guddit/config.yaml`.

## Development

```bash
make build    # build ./guddit
make test     # go test ./...
make vet      # go vet ./...
make smoke    # end-to-end sanity checks
```

Releases are cut with goreleaser on `v*` tags: binaries for linux/darwin/windows (amd64 + arm64), checksums, and a Homebrew cask. See `.goreleaser.yaml`.

## Architecture

```
cmd/guddit/main.go       entrypoint
internal/cmd/            cobra commands, serve API
internal/reddit/         HTTP client, types, fetchers
internal/auth/           cookie parsing, OAuth, login flows
internal/config/         config file wrapper
internal/render/         terminal capabilities, pretty renderers
internal/output/         format dispatch (json/csv/agent/table/...)
```

## Limitations

- Unauthenticated `.json` endpoints are blocked in 2026. Auth required.
- Respects `Retry-After`; falls back `www → old` on rate limits.
- No cache, no saved searches (by design).

## License

MIT