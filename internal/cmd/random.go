package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minoa/guddit/internal/output"
)

var randomCmd = &cobra.Command{
	Use:   "random [--nsfw]",
	Short: "Fetch a random post from a random subreddit",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		nsfw, _ := cmd.Flags().GetBool("nsfw")
		opt := listingOpts()
		posts, pager, err := c.Random(nsfw, opt)
		if err != nil {
			return err
		}
		scope := "random"
		if nsfw {
			scope = "randnsfw"
		}
		printBanner("r/" + scope)
		return emit(output.WithPaginator(posts, pager))
	},
}

func init() {
	randomCmd.Flags().Bool("nsfw", false, "use r/randnsfw instead of r/random")
	rootCmd.AddCommand(randomCmd)
}