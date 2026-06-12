package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/eznix86/ekconf/internal/password"
	"github.com/spf13/cobra"
)

var rotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Re-encrypt config.enc with a new password",
	Long:  `Re-encrypt the encrypted kubeconfig with a new password. The old password is required. If keychain is enabled, the stored password is updated automatically.`,
	Example: `  ekconf rotate
  ekconf rotate --password=old-secret`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentPassword, err := resolvePassword(cmd.Context())
		if err != nil {
			return err
		}

		newPassword, err := password.PromptNewPassword(cmd.Context())
		if err != nil {
			return err
		}

		encPath, err := config.EncPath()
		if err != nil {
			return err
		}

		plaintext, err := readDecryptedConfigData(currentPassword)
		if err != nil {
			return err
		}

		encryptedData, err := crypto.Seal(plaintext, newPassword)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}

		tmpPath := encPath + ".tmp"
		if err := os.WriteFile(tmpPath, encryptedData, 0o600); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("write temp file: %w", err)
		}

		if err := os.Rename(tmpPath, encPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("atomic swap failed: %w", err)
		}

		cfg, err := config.Load()
		if err == nil && cfg.Keychain {
			if err := password.Store(newPassword); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update keychain: %v\n", err)
			}
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Password rotated successfully")
		return err
	},
}

func init() {
	rootCmd.AddCommand(rotateCmd)
}
