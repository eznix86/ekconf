package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a context from config.enc",
	Args:  cobra.ExactArgs(1),
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

		ctx, ok := kubeconfig.Contexts[contextName]
		if !ok {
			return fmt.Errorf("context '%s' not found", contextName)
		}

		delete(kubeconfig.Contexts, contextName)

		clusterStillUsed := false
		for _, c := range kubeconfig.Contexts {
			if c.Cluster == ctx.Cluster {
				clusterStillUsed = true
				break
			}
		}
		if !clusterStillUsed {
			delete(kubeconfig.Clusters, ctx.Cluster)
		}

		authStillUsed := false
		for _, c := range kubeconfig.Contexts {
			if c.AuthInfo == ctx.AuthInfo {
				authStillUsed = true
				break
			}
		}
		if !authStillUsed {
			delete(kubeconfig.AuthInfos, ctx.AuthInfo)
		}

		mergedData, err := clientcmd.Write(*kubeconfig)
		if err != nil {
			return fmt.Errorf("marshal kubeconfig: %w", err)
		}

		ef2, err := crypto.Encrypt(mergedData, password)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}

		if err := os.WriteFile(encPath, crypto.Marshal(ef2), 0600); err != nil {
			return fmt.Errorf("write config.enc: %w", err)
		}

		if err := config.RemoveContext(contextName); err != nil {
			return fmt.Errorf("update config.yaml: %w", err)
		}

		storePasswordIfNeeded(password)

		fmt.Printf("Removed context '%s'\n", contextName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rmCmd)
}
