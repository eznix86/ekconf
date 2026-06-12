package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var importForce bool

var importCmd = &cobra.Command{
	Use:   "import [--force]",
	Short: "Import ~/.kube/config into the encrypted store",
	Long: `Read the plaintext kubeconfig at ~/.kube/config and import all
contexts into the encrypted store. If --force is set, the source file
is removed after a successful import. Otherwise, a confirmation prompt
is shown.`,
	Example: `  ekconf import
  ekconf import --force`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}

		srcPath := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist", srcPath)
		} else if err != nil {
			return fmt.Errorf("stat %s: %w", srcPath, err)
		}

		password, err := resolvePassword(cmd.Context())
		if err != nil {
			return err
		}

		kubeconfig, err := clientcmd.LoadFromFile(srcPath)
		if err != nil {
			return fmt.Errorf("load %s: %w", srcPath, err)
		}

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

		allRenames, err := planAddRenames(cfg, kubeconfig, "")
		if err != nil {
			return err
		}

		mergeKubeconfigContexts(cfg, existingKubeconfig, kubeconfig, allRenames)

		if err := writeMergedKubeconfig(cfg, existingKubeconfig, password); err != nil {
			return err
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		for _, r := range allRenames {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Imported context '%s'\n", r.dst); err != nil {
				return err
			}
		}

		if importForce {
			if err := os.Remove(srcPath); err != nil {
				return fmt.Errorf("remove %s: %w", srcPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", srcPath)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "\nTo remove the plaintext source file, run: rm %s\n", srcPath)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().BoolVar(&importForce, "force", false, "Remove ~/.kube/config after import")
}
