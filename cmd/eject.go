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
)

var ejectForce bool

var openTTY = os.OpenFile

var ejectCmd = &cobra.Command{
	Use:   "eject [--force]",
	Short: "Decrypt and write config.enc to ~/.kube/config",
	Long: `Decrypt the entire encrypted kubeconfig and write it to ~/.kube/config.

This creates a standard plaintext kubeconfig that tools like kubectl can
use directly. A confirmation prompt is shown unless --force is passed.`,
	Example: `  ekconf eject
  ekconf eject --force
  ekconf eject --password=secret`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !ejectForce {
			confirmed, err := promptConfirmation(cmd.ErrOrStderr(), "This will write an unencrypted kubeconfig to ~/.kube/config. Continue? [y/N]: ")
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
		// Eject still works if the plaintext index is missing or damaged; the encrypted kubeconfig is authoritative.

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

func init() {
	rootCmd.AddCommand(ejectCmd)
	ejectCmd.Flags().BoolVar(&ejectForce, "force", false, "Skip confirmation prompt")
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
