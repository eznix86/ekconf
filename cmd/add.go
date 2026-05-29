package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var addName string

var addCmd = &cobra.Command{
	Use:   "add <path> [-n <name>]",
	Short: "Encrypt and merge a kubeconfig into config.enc",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		kubeconfig, err := clientcmd.LoadFromFile(path)
		if err != nil {
			return fmt.Errorf("load kubeconfig: %w", err)
		}

		password, err := resolvePassword()
		if err != nil {
			return err
		}

		if err := config.EnsureDir(); err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		encPath, err := config.EncPath()
		if err != nil {
			return err
		}

		var existingData []byte
		if _, err := os.Stat(encPath); err == nil {
			existingData, err = os.ReadFile(encPath)
			if err != nil {
				return fmt.Errorf("read config.enc: %w", err)
			}
		}

		var existingKubeconfig *clientcmdapi.Config
		if len(existingData) > 0 {
			ef, err := crypto.Unmarshal(existingData)
			if err != nil {
				return fmt.Errorf("parse encrypted file: %w", err)
			}

			plaintext, err := crypto.Decrypt(ef, password)
			if err != nil {
				return fmt.Errorf("decrypt config.enc: %w (wrong password?)", err)
			}

			existingKubeconfig, err = clientcmd.Load(plaintext)
			if err != nil {
				return fmt.Errorf("parse existing kubeconfig: %w", err)
			}
		} else {
			existingKubeconfig = clientcmdapi.NewConfig()
		}

		if existingKubeconfig.Contexts == nil {
			existingKubeconfig.Contexts = make(map[string]*clientcmdapi.Context)
		}
		if existingKubeconfig.Clusters == nil {
			existingKubeconfig.Clusters = make(map[string]*clientcmdapi.Cluster)
		}
		if existingKubeconfig.AuthInfos == nil {
			existingKubeconfig.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)
		}

		type rename struct{ src, dst string }

		var renames []rename

		if addName != "" {
			if len(kubeconfig.Contexts) > 1 {
				return fmt.Errorf("cannot rename: kubeconfig has %d contexts, -n requires a single-context file", len(kubeconfig.Contexts))
			}
			for name := range kubeconfig.Contexts {
				renames = append(renames, rename{name, addName})
			}
		} else {
			for name := range kubeconfig.Contexts {
				renames = append(renames, rename{name, name})
			}
		}

		for i, r := range renames {
			for cfg.ContextExists(r.dst) {
				fmt.Fprintf(os.Stderr, "Context '%s' already exists, enter a new name: ", r.dst)
				reader := bufio.NewReader(os.Stdin)
				input, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("read input: %w", err)
				}
				r.dst = strings.TrimSpace(input)
			}
			renames[i] = r
		}

		for _, r := range renames {
			ctx, ok := kubeconfig.Contexts[r.src]
			if !ok {
				continue
			}

			existingKubeconfig.Contexts[r.dst] = ctx
			if cluster, ok := kubeconfig.Clusters[ctx.Cluster]; ok {
				existingKubeconfig.Clusters[ctx.Cluster] = cluster
			}
			if authInfo, ok := kubeconfig.AuthInfos[ctx.AuthInfo]; ok {
				existingKubeconfig.AuthInfos[ctx.AuthInfo] = authInfo
			}

			if err := config.AddContext(r.dst, ""); err != nil {
				return fmt.Errorf("update config.yaml: %w", err)
			}
		}

		mergedData, err := clientcmd.Write(*existingKubeconfig)
		if err != nil {
			return fmt.Errorf("marshal merged kubeconfig: %w", err)
		}

		ef, err := crypto.Encrypt(mergedData, password)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}

		if err := os.WriteFile(encPath, crypto.Marshal(ef), 0600); err != nil {
			return fmt.Errorf("write config.enc: %w", err)
		}

		storePasswordIfNeeded(password)

		for _, r := range renames {
			fmt.Printf("Added context '%s'\n", r.dst)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&addName, "name", "n", "", "Name for the added context")
}
