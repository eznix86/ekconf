package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var importForce bool

var importCmd = &cobra.Command{
	Use:   "import [<name>...] [--force]",
	Short: "Import contexts from ~/.kube/config into the encrypted store",
	Long: `Read the plaintext kubeconfig at ~/.kube/config and import contexts into
the encrypted store. If no names are given, all contexts are imported.

--force removes ~/.kube/config after a successful import. It cannot be combined
with named contexts, because that would delete the contexts you did not import.`,
	Example: `  ekconf import
  ekconf import prod
  ekconf import prod staging
  ekconf import --force`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeKubeconfigContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		if importForce && len(args) > 0 {
			return fmt.Errorf("--force removes ~/.kube/config entirely, so it cannot be combined with named contexts")
		}

		srcPath, err := kubeconfigPath()
		if err != nil {
			return err
		}
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist", srcPath)
		} else if err != nil {
			return fmt.Errorf("stat %s: %w", srcPath, err)
		}

		kubeconfig, err := clientcmd.LoadFromFile(srcPath)
		if err != nil {
			return fmt.Errorf("load %s: %w", srcPath, err)
		}

		selected, err := selectKubeconfigContexts(kubeconfig, args)
		if err != nil {
			return err
		}

		password, err := resolvePasswordForStore(cmd.Context())
		if err != nil {
			return err
		}
		defer clear(password)

		if err := config.EnsureDir(); err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		existingKubeconfig, err := loadExistingKubeconfig(password)
		if err != nil {
			return err
		}

		renames, err := planAddRenames(cfg, selected, "")
		if err != nil {
			return err
		}

		mergeKubeconfigContexts(cfg, existingKubeconfig, selected, renames)

		if err := writeMergedKubeconfig(cfg, existingKubeconfig, password); err != nil {
			return err
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		for _, r := range renames {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Imported context '%s'\n", r.dst); err != nil {
				return err
			}
		}

		if importForce {
			if err := os.Remove(srcPath); err != nil {
				return fmt.Errorf("remove %s: %w", srcPath, err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", srcPath)
			return err
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nTo remove the plaintext source file, run: rm %s\n", srcPath)
		return err
	},
}

func kubeconfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".kube", "config"), nil
}

func selectKubeconfigContexts(kubeconfig *clientcmdapi.Config, names []string) (*clientcmdapi.Config, error) {
	if len(names) == 0 {
		return kubeconfig, nil
	}

	selected := initializedKubeconfig()
	for _, name := range names {
		ctx, ok := kubeconfig.Contexts[name]
		if !ok || ctx == nil {
			return nil, fmt.Errorf(
				"context '%s' not found in kubeconfig (available: %s)",
				name, strings.Join(sortedKubeconfigContextNames(kubeconfig), ", "),
			)
		}

		selected.Contexts[name] = ctx
		if cluster, ok := kubeconfig.Clusters[ctx.Cluster]; ok {
			selected.Clusters[ctx.Cluster] = cluster
		}
		if authInfo, ok := kubeconfig.AuthInfos[ctx.AuthInfo]; ok {
			selected.AuthInfos[ctx.AuthInfo] = authInfo
		}
	}

	return selected, nil
}

func sortedKubeconfigContextNames(kubeconfig *clientcmdapi.Config) []string {
	names := make([]string, 0, len(kubeconfig.Contexts))
	for name := range kubeconfig.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func completeKubeconfigContext(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	srcPath, err := kubeconfigPath()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	kubeconfig, err := clientcmd.LoadFromFile(srcPath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return sortedKubeconfigContextNames(kubeconfig), cobra.ShellCompDirectiveNoFileComp
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().BoolVar(&importForce, "force", false, "Remove ~/.kube/config after importing every context")
}
