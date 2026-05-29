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
	RunE: func(cmd *cobra.Command, args []string) error {
		currentPassword, err := resolvePassword()
		if err != nil {
			return err
		}

		newPassword, err := password.PromptNewPassword()
		if err != nil {
			return err
		}

		encPath, err := config.EncPath()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(encPath)
		if err != nil {
			return fmt.Errorf("read config.enc: %w", err)
		}

		ef, err := crypto.Unmarshal(data)
		if err != nil {
			return fmt.Errorf("parse encrypted file: %w", err)
		}

		plaintext, err := crypto.Decrypt(ef, currentPassword)
		if err != nil {
			return fmt.Errorf("decrypt (wrong password?): %w", err)
		}

		efNew, err := crypto.Encrypt(plaintext, newPassword)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}

		tmpPath := encPath + ".tmp"
		if err := os.WriteFile(tmpPath, crypto.Marshal(efNew), 0600); err != nil {
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
				fmt.Fprintf(os.Stderr, "Warning: failed to update keychain: %v\n", err)
			}
		}

		fmt.Println("Password rotated successfully")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rotateCmd)
}
