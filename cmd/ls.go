package cmd

import (
	"fmt"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all contexts",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if len(cfg.Contexts) == 0 {
			fmt.Println("No contexts found")
			return nil
		}

		for name, entry := range cfg.Contexts {
			mark := " "
			if name == cfg.Current {
				mark = "*"
			}

			if entry.Namespace != "" && entry.Namespace != "default" {
				fmt.Printf("%s %-29s namespace: %s\n", mark, name, entry.Namespace)
			} else {
				fmt.Printf("%s %s\n", mark, name)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
