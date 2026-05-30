package cmd

import (
	"fmt"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage ekconf configuration",
	Long: `View or set ekconf configuration options.

Use "config list" to print the current configuration.
Use "config keychain=true" to enable keychain integration.`,
	Example: `  ekconf config list
  ekconf config keychain=true
  ekconf config keychain=false`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		parts := strings.SplitN(args[0], "=", 2)
		if len(parts) != 2 {
			return usageErrorf("expected key=value, got: %s", args[0])
		}

		key := parts[0]
		value := parts[1]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		switch key {
		case "keychain":
			switch value {
			case "true":
				cfg.Keychain = true
			case "false":
				cfg.Keychain = false
			default:
				return fmt.Errorf("keychain must be 'true' or 'false', got: %s", value)
			}
		case "yaml.colorize":
			if cfg.YAML == nil {
				cfg.YAML = &config.YAMLConfig{}
			}
			switch value {
			case "true":
				cfg.YAML.Colorize = true
			case "false":
				cfg.YAML.Colorize = false
			default:
				return fmt.Errorf("yaml.colorize must be 'true' or 'false', got: %s", value)
			}
		default:
			return fmt.Errorf("unknown config key: %s (supported: keychain, yaml.colorize)", key)
		}

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Set %s=%s\n", key, value)
		return err
	},
}

var configListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Print current configuration",
	Long:    `Print the current ekconf configuration including keychain status, active context, and context count.`,
	Example: `  ekconf config list`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listConfig(cmd)
	},
}

func listConfig(cmd *cobra.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	yamlColorize := cfg.YAML != nil && cfg.YAML.Colorize
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "keychain: %t\nyaml.colorize: %t\ncurrent: %s\ncontexts: %d\n", cfg.Keychain, yamlColorize, cfg.Current, len(cfg.Contexts)); err != nil {
		return err
	}

	return nil
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)
}
