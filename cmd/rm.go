package cmd

import (
	"fmt"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"
)

type removeResult struct {
	name  string
	found bool
}

var rmCmd = &cobra.Command{
	Use:   "rm <name> [<name>...]",
	Short: "Remove one or more contexts from config.enc",
	Long:  `Remove one or more contexts and their unreferenced clusters and auth infos from the encrypted kubeconfig.`,
	Example: `  ekconf rm staging
  ekconf rm staging prod
  ekconf rm --password=secret prod development`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		password, err := resolvePassword(cmd.Context())
		if err != nil {
			return err
		}
		defer clear(password)

		encPath, err := config.EncPath()
		if err != nil {
			return err
		}

		kubeconfig, err := loadDecryptedKubeconfig(password)
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		results := make([]removeResult, 0, len(args))
		for _, name := range args {
			result := removeContext(cfg, kubeconfig, name)
			results = append(results, result)
		}

		mergedData, err := clientcmd.Write(*kubeconfig)
		if err != nil {
			return fmt.Errorf("marshal kubeconfig: %w", err)
		}

		encryptedData, err := crypto.Seal(mergedData, password)
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
			{path: encPath, data: encryptedData},
			{path: configPath, data: cfgData},
		}); err != nil {
			return err
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		var notFound []string
		for _, r := range results {
			if r.found {
				fmt.Fprintf(cmd.OutOrStdout(), "Removed context '%s'\n", r.name)
			} else {
				notFound = append(notFound, r.name)
			}
		}
		if len(notFound) > 0 {
			return fmt.Errorf("context(s) not found: %s", strings.Join(notFound, ", "))
		}
		return nil
	},
}

func removeContext(cfg *config.Config, kubeconfig *clientcmdapi.Config, name string) removeResult {
	ctx, ok := kubeconfig.Contexts[name]
	if !ok {
		return removeResult{name: name, found: false}
	}

	delete(kubeconfig.Contexts, name)

	if !clusterInUse(kubeconfig.Contexts, ctx.Cluster) {
		delete(kubeconfig.Clusters, ctx.Cluster)
	}

	if !authInfoInUse(kubeconfig.Contexts, ctx.AuthInfo) {
		delete(kubeconfig.AuthInfos, ctx.AuthInfo)
	}

	delete(cfg.Contexts, name)
	if cfg.Current == name {
		cfg.Current = ""
	}

	return removeResult{name: name, found: true}
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
