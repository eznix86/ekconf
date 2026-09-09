package cmd

import (
	"fmt"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the active context",
	Long: `Set the active context in config.yaml. This does not modify the encrypted kubeconfig.

If the context's client certificate has already expired, a warning with the
expiry date is printed to stderr. Expiry is read from config.yaml, so no
password is needed. It is recorded whenever ekconf decrypts the store.`,
	Example: `  ekconf use prod
  ekconf use staging`,
	Args:              cobra.ExactArgs(1),
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

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Switched to context '%s'\n", contextName); err != nil {
			return err
		}

		warnIfExpired(cmd.ErrOrStderr(), contextName, cfg.Contexts[contextName])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}
