package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/output"
	"github.com/minoa/guddit/internal/reddit"
)

var meCmd = &cobra.Command{
	Use:   "me [saved|upvoted|hidden|overview|submitted|karma]",
	Short: "Authenticated user feeds (requires cookies/OAuth)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		if !c.HasCookies() && c.OAuthToken == "" {
			return errAuthRequired()
		}
		if len(args) == 0 {
			m, err := c.Me()
			if err != nil {
				return err
			}
			return emit(m)
		}
		kind := args[0]
		if kind == "karma" {
			k, err := c.Karma()
			if err != nil {
				return err
			}
			return emit(k)
		}
		mk, err := parseMyKind(kind)
		if err != nil {
			return err
		}
		posts, pager, err := c.MyListing(mk, listingOpts())
		if err != nil {
			return err
		}
		return emit(output.WithPaginator(posts, pager))
	},
}

func init() {
	rootCmd.AddCommand(meCmd)
}

func parseMyKind(s string) (reddit.MyListingKind, error) {
	switch s {
	case "saved":
		return reddit.MySaved, nil
	case "upvoted":
		return reddit.MyUpvoted, nil
	case "hidden":
		return reddit.MyHidden, nil
	case "overview":
		return reddit.MyOverview, nil
	case "submitted":
		return reddit.MySubmitted, nil
	}
	return "", errInvalid("me kind", s, "saved|upvoted|hidden|overview|submitted|karma")
}

func errAuthRequired() error {
	return errInvalid("auth", "none", "cookies (--cookie/--cookie-file) or OAuth (--client-id/--client-secret + stored password)")
}