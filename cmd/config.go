package cmd

import (
	"fmt"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config <key=value>",
	Short: "Set a configuration option",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		arg := args[0]

		if arg == "list" {
			return listConfig()
		}

		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("expected key=value or 'list', got: %s", arg)
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
		default:
			return fmt.Errorf("unknown config key: %s (supported: keychain)", key)
		}

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Printf("Set %s=%s\n", key, value)
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "Print current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listConfig()
	},
}

func listConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("keychain: %t\n", cfg.Keychain)
	fmt.Printf("current: %s\n", cfg.Current)
	fmt.Printf("contexts: %d\n", len(cfg.Contexts))

	return nil
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)
}
