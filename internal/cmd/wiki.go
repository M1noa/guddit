package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/reddit"
)

var wikiCmd = &cobra.Command{
	Use:   "wiki <subreddit> [page]",
	Short: "Read a subreddit wiki page",
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
		page := "index"
		if len(args) > 1 {
			page = args[1]
		}
		w, err := c.Wiki(sub, page)
		if err != nil {
			return err
		}
		return emit(w)
	},
}

func init() {
	rootCmd.AddCommand(wikiCmd)
}