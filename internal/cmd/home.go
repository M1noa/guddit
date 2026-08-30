package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/output"
	"github.com/minoa/guddit/internal/reddit"
)

// homeCmd fetches a frontpage listing.
var homeCmd = &cobra.Command{
	Use:   "home [hot|new|top|rising|controversial|best]",
	Short: "Browse the Reddit frontpage",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		kind := reddit.ListingHot
		if len(args) > 0 {
			k, err := parseListingKind(args[0])
			if err != nil {
				return err
			}
			kind = k
		}
		opt := listingOpts()
		if kind == reddit.ListingTop || kind == reddit.ListingControversial {
			opt.Time = parseTime(flagStr("time"))
		}
		posts, pager, err := c.Home(kind, opt)
		if err != nil {
			return err
		}
		printBanner("r/  home  /  " + string(kind))
		return emit(output.WithPaginator(posts, pager))
	},
}

func init() {
	homeCmd.Flags().StringP("time", "t", "", "time window for top/controversial (hour|day|week|month|year|all)")
	rootCmd.AddCommand(homeCmd)
}

// parseListingKind maps a string arg to a reddit.ListingKind.
func parseListingKind(s string) (reddit.ListingKind, error) {
	switch s {
	case "hot", "":
		return reddit.ListingHot, nil
	case "new":
		return reddit.ListingNew, nil
	case "top":
		return reddit.ListingTop, nil
	case "rising":
		return reddit.ListingRising, nil
	case "controversial":
		return reddit.ListingControversial, nil
	case "best":
		return reddit.ListingBest, nil
	}
	return "", errInvalid("listing kind", s, "hot|new|top|rising|controversial|best")
}

// parseTime maps a string to a reddit.TimeWindow.
func parseTime(s string) reddit.TimeWindow {
	switch s {
	case "hour":
		return reddit.TimeHour
	case "day":
		return reddit.TimeDay
	case "week":
		return reddit.TimeWeek
	case "month":
		return reddit.TimeMonth
	case "year":
		return reddit.TimeYear
	case "all":
		return reddit.TimeAll
	}
	return ""
}
