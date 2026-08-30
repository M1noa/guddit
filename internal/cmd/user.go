package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/output"
	"github.com/minoa/guddit/internal/reddit"
)

var userCmd = &cobra.Command{
	Use:   "user <name> [overview|submitted|comments]",
	Short: "View a user's profile or activity",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		name := args[0]
		if len(args) == 1 {
			// about
			u, err := c.UserAbout(name)
			if err != nil {
				return err
			}
			return emit(u)
		}
		kind := reddit.UserOverview
		if k, err := parseUserKind(args[1]); err != nil {
			return err
		} else {
			kind = k
		}
		opt := listingOpts()
		posts, pager, err := c.UserListing(name, kind, opt)
		if err != nil {
			return err
		}
		printBanner("u/" + name + "  /  " + string(kind))
		return emit(output.WithPaginator(posts, pager))
	},
}

func init() {
	rootCmd.AddCommand(userCmd)
}

func parseUserKind(s string) (reddit.UserKind, error) {
	switch s {
	case "overview", "":
		return reddit.UserOverview, nil
	case "submitted", "posts":
		return reddit.UserSubmitted, nil
	case "comments":
		return reddit.UserComments, nil
	}
	return "", errInvalid("user kind", s, "overview|submitted|comments")
}