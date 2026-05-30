package cmd

import (
	"fmt"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"
)

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a context from config.enc",
	Long:  `Remove a context and its unreferenced clusters and auth infos from the encrypted kubeconfig.`,
	Example: `  ekconf rm staging
  ekconf rm --password=secret prod`,
	Args:              cobra.ExactArgs(1),
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

		kubeconfig, err := loadDecryptedKubeconfig(password)
		if err != nil {
			return err
		}

		ctx, ok := kubeconfig.Contexts[contextName]
		if !ok {
			return fmt.Errorf("context '%s' not found", contextName)
		}

		delete(kubeconfig.Contexts, contextName)

		if !clusterInUse(kubeconfig.Contexts, ctx.Cluster) {
			delete(kubeconfig.Clusters, ctx.Cluster)
		}

		if !authInfoInUse(kubeconfig.Contexts, ctx.AuthInfo) {
			delete(kubeconfig.AuthInfos, ctx.AuthInfo)
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		delete(cfg.Contexts, contextName)
		if cfg.Current == contextName {
			cfg.Current = ""
		}

		mergedData, err := clientcmd.Write(*kubeconfig)
		if err != nil {
			return fmt.Errorf("marshal kubeconfig: %w", err)
		}

		ef2, err := crypto.Encrypt(mergedData, password)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}

		configPath, err := config.ConfigPath()
		if err != nil {
			return err
		}

		cfgData, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal config.yaml: %w", err)
		}

		if err := replaceFilesAtomically([]fileUpdate{
			{path: encPath, data: crypto.Marshal(ef2)},
			{path: configPath, data: cfgData},
		}); err != nil {
			return err
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed context '%s'\n", contextName)
		return err
	},
}

func clusterInUse(contexts map[string]*clientcmdapi.Context, clusterName string) bool {
	for _, ctx := range contexts {
		if ctx != nil && ctx.Cluster == clusterName {
			return true
		}
	}
	return false
}

func authInfoInUse(contexts map[string]*clientcmdapi.Context, authInfoName string) bool {
	for _, ctx := range contexts {
		if ctx != nil && ctx.AuthInfo == authInfoName {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(rmCmd)
}
