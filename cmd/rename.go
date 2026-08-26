package cmd

import (
	"fmt"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var renameCmd = &cobra.Command{
	Use:     "rename <old> <new>",
	Aliases: []string{"mv"},
	Short:   "Rename a context in config.enc",
	Long: `Rename a context in the encrypted kubeconfig and in config.yaml.

Clusters and auth infos owned by the context are renamed alongside it.
Entries shared with another context are left untouched. If the renamed
context is active, it stays active under its new name.`,
	Example: `  ekconf rename staging preprod
  ekconf rename --password=secret prod production`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName, newName := args[0], args[1]
		if err := validateRenameTarget(oldName, newName); err != nil {
			return err
		}

		password, err := resolvePassword(cmd.Context())
		if err != nil {
			return err
		}
		defer clear(password)

		kubeconfig, err := loadDecryptedKubeconfig(password)
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if err := renameContext(cfg, kubeconfig, oldName, newName); err != nil {
			return err
		}

		if err := writeMergedKubeconfig(cfg, kubeconfig, password); err != nil {
			return err
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Renamed context '%s' to '%s'\n", oldName, newName)
		return err
	},
}

func validateRenameTarget(oldName, newName string) error {
	if oldName == newName {
		return fmt.Errorf("context '%s' already has that name", oldName)
	}
	if strings.TrimSpace(newName) == "" {
		return fmt.Errorf("new context name must not be empty")
	}
	if strings.Contains(newName, "/") {
		return fmt.Errorf("new context name must not contain '/'")
	}
	return nil
}

func renameContext(cfg *config.Config, kubeconfig *clientcmdapi.Config, oldName, newName string) error {
	ctx, ok := kubeconfig.Contexts[oldName]
	if !ok || ctx == nil {
		return fmt.Errorf("context '%s' not found", oldName)
	}
	if _, exists := kubeconfig.Contexts[newName]; exists || cfg.ContextExists(newName) {
		return fmt.Errorf("context '%s' already exists", newName)
	}

	delete(kubeconfig.Contexts, oldName)

	ctx.Cluster = renameOwnedEntry(kubeconfig.Clusters, ctx.Cluster, oldName, newName, func(name string) bool {
		return clusterInUse(kubeconfig.Contexts, name)
	})
	ctx.AuthInfo = renameOwnedEntry(kubeconfig.AuthInfos, ctx.AuthInfo, oldName, newName, func(name string) bool {
		return authInfoInUse(kubeconfig.Contexts, name)
	})

	kubeconfig.Contexts[newName] = ctx
	if kubeconfig.CurrentContext == oldName {
		kubeconfig.CurrentContext = newName
	}

	entry := cfg.Contexts[oldName]
	delete(cfg.Contexts, oldName)
	cfg.Contexts[newName] = entry
	if cfg.Current == oldName {
		cfg.Current = newName
	}

	return nil
}

func renameOwnedEntry[T any](
	entries map[string]*T,
	current, oldName, newName string,
	stillReferenced func(string) bool,
) string {
	suffix, owned := strings.CutPrefix(current, oldName+"/")
	if !owned {
		return current
	}

	entry, ok := entries[current]
	if !ok || stillReferenced(current) {
		return current
	}

	renamed := newName + "/" + suffix
	if _, taken := entries[renamed]; taken {
		return current
	}

	delete(entries, current)
	entries[renamed] = entry
	return renamed
}

func init() {
	rootCmd.AddCommand(renameCmd)
}
