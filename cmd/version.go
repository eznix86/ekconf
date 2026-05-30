package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the ekconf version",
	Example: `  ekconf version`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "ekconf %s\n", versionString())
		return err
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
