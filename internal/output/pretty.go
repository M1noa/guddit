package output

import (
	"fmt"
	"io"

	"github.com/minoa/guddit/internal/render"
	"github.com/minoa/guddit/internal/reddit"
)

// writePretty routes the result to the appropriate render.Theme method.
func writePretty(caps render.Capabilities, result any, w io.Writer, themeOpts *render.ThemeOptions, linkOpts render.LinkOptions) error {
	theme := render.NewTheme(caps, themeOpts)
	// Store link options in theme for use in rendering
	theme.LinkOpts = linkOpts
	switch v := result.(type) {
	case withPaginator:
		fmt.Fprint(w, theme.Posts(v.posts))
		if v.paginator != nil && v.paginator.After != "" {
			fmt.Fprintf(w, "\n%s\n", theme.Subtitle.Render("-- next page: guddit ... --after "+v.paginator.After))
		}
		return nil
	case []reddit.Post:
		fmt.Fprint(w, theme.Posts(v))
		return nil
	case *reddit.PostWithComments:
		fmt.Fprint(w, theme.PostFull(v, 6))
		return nil
	case *reddit.Subreddit:
		fmt.Fprint(w, theme.SubredditAbout(v))
		return nil
	case *reddit.User:
		fmt.Fprint(w, theme.User(v))
		return nil
	case *reddit.Me:
		fmt.Fprint(w, theme.Me(v))
		return nil
	case *reddit.WikiPage:
		fmt.Fprint(w, theme.Wiki(v))
		return nil
	case *reddit.KarmaBreakdown:
		fmt.Fprintf(w, "karma breakdown\n")
		for _, sr := range v.Data.Subreddits {
			fmt.Fprintf(w, "  r/%s: %d (comment %d, link %d)\n", sr.SR, sr.Karma, sr.CommentKarma, sr.LinkKarma)
		}
		return nil
	case error:
		fmt.Fprintln(w, theme.Error.Render("error: "+v.Error()))
		return nil
	case nil:
		return nil
	default:
		return writeJSON(result, w)
	}
}
