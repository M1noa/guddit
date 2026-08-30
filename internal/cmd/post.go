package cmd

import (
	"github.com/spf13/cobra"
)

var postCmd = &cobra.Command{
	Use:   "post <url|id|permalink>",
	Short: "Fetch a post and optionally its comment tree",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}
		c, err := buildClient()
		if err != nil {
			return err
		}
		ref := args[0]
		comments, _ := cmd.Flags().GetBool("comments")
		depth, _ := cmd.Flags().GetInt("depth")
		if depth <= 0 {
			depth = 6
		}
		thread, err := c.Post(ref, comments)
		if err != nil {
			return err
		}
		printBanner("post  /  " + thread.Post.Name)
		return emit(thread)
	},
}

func init() {
	postCmd.Flags().BoolP("comments", "c", true, "include comment tree")
	postCmd.Flags().Int("depth", 6, "max comment depth to render")
	rootCmd.AddCommand(postCmd)
}