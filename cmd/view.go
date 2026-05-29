package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var viewCmd = &cobra.Command{
	Use:   "view <name>",
	Short: "Decrypt and print a single context's kubeconfig",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

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

		if _, ok := kubeconfig.Contexts[contextName]; !ok {
			return fmt.Errorf("context '%s' not found", contextName)
		}

		ctx := *kubeconfig
		for name := range ctx.Contexts {
			if name != contextName {
				delete(ctx.Contexts, name)
			}
		}

		out, err := clientcmd.Write(ctx)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}

		storePasswordIfNeeded(password)

		fmt.Print(string(out))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(viewCmd)
}
