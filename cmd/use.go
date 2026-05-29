package cmd

import (
	"fmt"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the active context",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if !cfg.ContextExists(contextName) {
			return fmt.Errorf("context '%s' not found", contextName)
		}

		if err := config.SaveCurrent(contextName); err != nil {
			return fmt.Errorf("save current context: %w", err)
		}

		fmt.Printf("Switched to context '%s'\n", contextName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}
