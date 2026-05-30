package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var ejectForce bool

var openTTY = os.OpenFile

var ejectCmd = &cobra.Command{
	Use:   "eject [<name>...] [--force]",
	Short: "Decrypt and write contexts to ~/.kube/config",
	Long: `Decrypt one or more contexts and write them to ~/.kube/config.
If no context names are given, all contexts are written.

This creates a standard plaintext kubeconfig that tools like kubectl can
use directly. A confirmation prompt is shown unless --force is passed.`,
	Example: `  ekconf eject
  ekconf eject prod
  ekconf eject staging prod
  ekconf eject --force
  ekconf eject staging prod --force`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		ejectAll := len(args) == 0

		if !ejectForce {
			var msg string
			if ejectAll {
				msg = "This will write an unencrypted kubeconfig to ~/.kube/config. Continue? [y/N]: "
			} else {
				msg = fmt.Sprintf("This will write %d context(s) to ~/.kube/config. Continue? [y/N]: ", len(args))
			}
			confirmed, err := promptConfirmation(cmd.ErrOrStderr(), msg)
			if err != nil {
				return err
			}
			if !confirmed {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Aborted")
				return err
			}
		}

		password, err := resolvePassword()
		if err != nil {
			return err
		}

		kubeconfig, err := loadDecryptedKubeconfig(password)
		if err != nil {
			return err
		}

		if ejectAll {
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
		} else {
			filtered, err := filterKubeconfig(kubeconfig, args)
			if err != nil {
				return err
			}
			kubeconfig = filtered
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
		if err := os.MkdirAll(kubeDir, 0o700); err != nil {
			return fmt.Errorf("create ~/.kube: %w", err)
		}

		kubeConfigPath := filepath.Join(kubeDir, "config")
		if err := os.WriteFile(kubeConfigPath, ejectedPlaintext, 0o600); err != nil {
			return fmt.Errorf("write ~/.kube/config: %w", err)
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Decrypted config written to %s\n", kubeConfigPath)
		return err
	},
}

func filterKubeconfig(kubeconfig *clientcmdapi.Config, names []string) (*clientcmdapi.Config, error) {
	filtered := clientcmdapi.NewConfig()

	for _, name := range names {
		ctx, ok := kubeconfig.Contexts[name]
		if !ok {
			return nil, fmt.Errorf("context '%s' not found", name)
		}

		filtered.Contexts[name] = ctx
		filtered.CurrentContext = name

		if cluster, ok := kubeconfig.Clusters[ctx.Cluster]; ok {
			filtered.Clusters[ctx.Cluster] = cluster
		}
		if ctx.AuthInfo != "" {
			if authInfo, ok := kubeconfig.AuthInfos[ctx.AuthInfo]; ok {
				filtered.AuthInfos[ctx.AuthInfo] = authInfo
			}
		}
	}

	return filtered, nil
}

func promptConfirmation(w io.Writer, message string) (bool, error) {
	tty, err := openTTY("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return false, fmt.Errorf("prompt confirmation: %w", err)
	}
	defer tty.Close()

	fmt.Fprint(w, message)
	response, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(response)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func init() {
	rootCmd.AddCommand(ejectCmd)
	ejectCmd.Flags().BoolVar(&ejectForce, "force", false, "Skip confirmation prompt")
}
