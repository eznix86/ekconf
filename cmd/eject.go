package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var ejectForce bool

var ejectCmd = &cobra.Command{
	Use:   "eject [--force]",
	Short: "Decrypt and write config.enc to ~/.kube/config",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !ejectForce {
			fmt.Fprint(os.Stderr, "This will write an unencrypted kubeconfig to ~/.kube/config. Continue? [y/N]: ")

			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" && response != "yes" && response != "YES" {
				fmt.Println("Aborted")
				return nil
			}
		}

		password, err := resolvePassword()
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

		plaintext, err := crypto.Decrypt(ef, password)
		if err != nil {
			return fmt.Errorf("decrypt (wrong password?): %w", err)
		}

		kubeconfig, err := clientcmd.Load(plaintext)
		if err != nil {
			return fmt.Errorf("parse kubeconfig: %w", err)
		}

		cfg, err := config.Load()
		if err == nil {
			for name, entry := range cfg.Contexts {
				if ctx, ok := kubeconfig.Contexts[name]; ok && entry.Namespace != "" {
					ctx.Namespace = entry.Namespace
				}
			}
			if cfg.Current != "" {
				kubeconfig.CurrentContext = cfg.Current
			}
		}

		ejectedPlaintext, err := clientcmd.Write(*kubeconfig)
		if err != nil {
			return fmt.Errorf("marshal ejected kubeconfig: %w", err)
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}

		kubeDir := filepath.Join(home, ".kube")
		if err := os.MkdirAll(kubeDir, 0700); err != nil {
			return fmt.Errorf("create ~/.kube: %w", err)
		}

		kubeConfigPath := filepath.Join(kubeDir, "config")
		if err := os.WriteFile(kubeConfigPath, ejectedPlaintext, 0600); err != nil {
			return fmt.Errorf("write ~/.kube/config: %w", err)
		}

		storePasswordIfNeeded(password)

		fmt.Printf("Decrypted config written to %s\n", kubeConfigPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ejectCmd)
	ejectCmd.Flags().BoolVar(&ejectForce, "force", false, "Skip confirmation prompt")
}
