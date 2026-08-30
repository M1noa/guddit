package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/output"
	"github.com/minoa/guddit/internal/reddit"
)

var subCmd = &cobra.Command{
	Use:   "sub <name> [hot|new|top|rising|controversial]",
	Short: "Browse a subreddit or show its sidebar",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		sub := reddit.SanitizeSub(args[0])
		
		// Check if user explicitly wants the about page
		about, _ := cmd.Flags().GetBool("about")
		if about || (len(args) == 2 && args[1] == "about") {
			aboutInfo, err := c.SubredditAbout(sub)
			if err != nil {
				return err
			}
			return emit(aboutInfo)
		}
		
		kind := reddit.ListingHot
		if len(args) == 2 {
			k, err := parseListingKind(args[1])
			if err != nil {
				return err
			}
			kind = k
		}
		opt := listingOpts()
		if kind == reddit.ListingTop || kind == reddit.ListingControversial {
			opt.Time = parseTime(flagStr("time"))
		}
		posts, pager, err := c.Subreddit(sub, kind, opt)
		if err != nil {
			return err
		}
		printBanner("r/" + sub + "  /  " + string(kind))
		return emit(output.WithPaginator(posts, pager))
	},
}

func init() {
	subCmd.Flags().StringP("time", "t", "", "time window for top/controversial (hour|day|week|month|year|all)")
	subCmd.Flags().Bool("about", false, "show subreddit sidebar/about info")
	rootCmd.AddCommand(subCmd)
}