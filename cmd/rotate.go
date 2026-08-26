package cmd

import (
	"fmt"

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
		defer clear(currentPassword)

		newPassword, err := password.PromptNewPassword(cmd.Context())
		if err != nil {
			return err
		}
		defer clear(newPassword)

		encPath, err := config.EncPath()
		if err != nil {
			return err
		}

		plaintext, err := readDecryptedConfigData(currentPassword)
		if err != nil {
			return err
		}
		defer clear(plaintext)

		encryptedData, err := crypto.Seal(plaintext, newPassword)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}

		if err := writeFileAtomically(encPath, encryptedData); err != nil {
			return fmt.Errorf("write encrypted config: %w", err)
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
