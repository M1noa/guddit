package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/output"
	"github.com/minoa/guddit/internal/reddit"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search Reddit (site-wide or within a subreddit)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		q := joinArgs(args)
		sub, _ := cmd.Flags().GetString("sub")
		sortStr, _ := cmd.Flags().GetString("sort")
		timeStr, _ := cmd.Flags().GetString("time")

		sort := reddit.SortRelevance
		if sortStr != "" {
			s, err := parseSort(sortStr)
			if err != nil {
				return err
			}
			sort = s
		}
		opt := reddit.SearchOptions{
			Subreddit:      sub,
			Sort:           sort,
			ListingOptions: listingOpts(),
		}
		if timeStr != "" {
			opt.Time = parseTime(timeStr)
		}
		posts, pager, err := c.Search(q, opt)
		if err != nil {
			return err
		}
		scope := "all reddit"
		if sub != "" {
			scope = "r/" + reddit.SanitizeSub(sub)
		}
		printBanner("search  /  " + scope + "  /  " + string(sort))
		return emit(output.WithPaginator(posts, pager))
	},
}

func init() {
	searchCmd.Flags().String("sub", "", "restrict search to a subreddit (r/x or x)")
	searchCmd.Flags().String("sort", "relevance", "sort order (relevance|hot|new|top|comments)")
	searchCmd.Flags().String("time", "", "time window for sort=top (hour|day|week|month|year|all)")
	rootCmd.AddCommand(searchCmd)
}

func parseSort(s string) (reddit.SortKind, error) {
	switch s {
	case "relevance", "":
		return reddit.SortRelevance, nil
	case "hot":
		return reddit.SortHot, nil
	case "new":
		return reddit.SortNew, nil
	case "top":
		return reddit.SortTop, nil
	case "comments":
		return reddit.SortComments, nil
	}
	return "", errInvalid("sort", s, "relevance|hot|new|top|comments")
}

// joinArgs joins positional args into a single query string.
func joinArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = a
	}
	return joinSpace(out)
}
