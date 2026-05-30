package cmd

import (
	"fmt"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:     "ls",
	Short:   "List all contexts",
	Long:    `List all contexts stored in the encrypted kubeconfig. The active context is marked with *.`,
	Example: `  ekconf ls`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if len(cfg.Contexts) == 0 {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "No contexts found")
			return err
		}

		for name, entry := range cfg.Contexts {
			mark := " "
			if name == cfg.Current {
				mark = "*"
			}

			if entry.Namespace != "" && entry.Namespace != "default" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %-29s namespace: %s\n", mark, name, entry.Namespace)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", mark, name)
			}
			if err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
