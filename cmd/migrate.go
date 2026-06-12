package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate config.enc to the current encrypted format",
	Long:  `Migrate the encrypted kubeconfig store from the legacy ekconf format to the current self-describing SecretBox format.`,
	Example: `  ekconf migrate
  ekconf migrate --password=secret`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		encPath, err := config.EncPath()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(encPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("encrypted config does not exist: %s", encPath)
			}
			return fmt.Errorf("read config.enc: %w", err)
		}

		if crypto.IsSecretBox(data) {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "config.enc already uses the current encrypted format")
			return err
		}

		password, err := resolvePassword(cmd.Context())
		if err != nil {
			return err
		}

		migratedData, migrated, err := crypto.Migrate(data, password)
		if err != nil {
			return fmt.Errorf("migrate config.enc: %w", err)
		}
		if !migrated {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "config.enc already uses the current encrypted format")
			return err
		}

		backupPath := encPath + ".v0.bak"
		updates := []fileUpdate{{path: encPath, data: migratedData}}
		if _, err := os.Stat(backupPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("stat backup file: %w", err)
			}
			updates = append([]fileUpdate{{path: backupPath, data: data}}, updates...)
		}

		if err := replaceFilesAtomically(updates); err != nil {
			return err
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		_, err = fmt.Fprintf(
			cmd.OutOrStdout(),
			"Migrated config.enc to the current encrypted format\nLegacy backup: %s\nUpdate all ekconf binaries before using aliases that call ekconf.\n",
			backupPath,
		)
		return err
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
