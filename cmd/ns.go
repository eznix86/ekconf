package cmd

import (
	"fmt"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

var nsCmd = &cobra.Command{
	Use:   "ns <namespace>",
	Short: "Set default namespace on the active context",
	Long:  `Set the default namespace for the active context. This is stored in config.yaml and used by ekconf exec.`,
	Example: `  ekconf ns default
  ekconf ns kube-system`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if cfg.Current == "" {
			return fmt.Errorf("no active context set, use 'ekconf use <name>' first")
		}

		if err := config.SetNamespace(cfg.Current, namespace); err != nil {
			return fmt.Errorf("set namespace: %w", err)
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Set namespace '%s' on context '%s'\n", namespace, cfg.Current)
		return err
	},
}

func init() {
	rootCmd.AddCommand(nsCmd)
}
