package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/password"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	passwordFlag  string
	passwordStdin bool
	envPassword   string
	version       = "dev"
	commit        = "none"
	built         = "unknown"
)

//go:embed banner.txt
var banner string

var rootCmd = &cobra.Command{
	Use:           "ekconf",
	Short:         "Encrypted kubeconfig manager",
	Version:       versionString(),
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: color.New(color.FgCyan).Sprint(banner) + `
Allows you to add and delete kubeconfigs by merging kubeconfig
files together and breaking them apart appropriately, with
encryption at rest and optional keychain integration.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "version" || cmd.Name() == "update" || cmd.Name() == "completion" {
			return nil
		}
		if err := config.EnsureDir(); err != nil {
			return err
		}
		if shouldRecoverPendingFileTransaction(cmd) {
			return recoverPendingFileTransaction()
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cmd.Help(); err != nil {
			return err
		}
		noticeCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		MaybePrintUpdateNotice(noticeCtx, cmd.OutOrStdout())
		return nil
	},
}

func ExecuteContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

func SetEnvPassword(value string) {
	envPassword = value
}

func init() {
	rootCmd.InitDefaultCompletionCmd()
	rootCmd.InitDefaultVersionFlag()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.PersistentFlags().StringVar(&passwordFlag, "password", "", "Password for decryption (inline)")
	rootCmd.PersistentFlags().BoolVar(&passwordStdin, "password-stdin", false, "Read password from stdin")
	rootCmd.MarkFlagsMutuallyExclusive("password", "password-stdin")
}

func versionString() string {
	parts := []string{fmt.Sprintf("version: %s", version)}
	if commit != "none" {
		parts = append(parts, fmt.Sprintf("commit: %s", commit))
	}
	if built != "unknown" {
		parts = append(parts, fmt.Sprintf("built: %s", built))
	}
	return strings.Join(parts, "\n")
}

func shouldUseKeychain() bool {
	cfg, err := config.Load()
	if err != nil {
		return false
	}
	return cfg.Keychain
}

func resolvePassword(ctx context.Context) (string, error) {
	return resolvePasswordWithKeychain(ctx, shouldUseKeychain())
}

func resolvePasswordWithKeychain(ctx context.Context, useKeychain bool) (string, error) {
	return password.Resolve(ctx, passwordFlag, passwordStdin, useKeychain, envPassword)
}

func shouldRecoverPendingFileTransaction(cmd *cobra.Command) bool {
	if cmd.Flags().Changed("password") || passwordStdin {
		return true
	}

	switch cmd.Name() {
	case "add", "eject", "exec", "migrate", "rm", "rotate", "view":
		return true
	default:
		return false
	}
}

func completeContext(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return sortedContextNames(cfg), cobra.ShellCompDirectiveNoFileComp
}

func storePasswordIfNeeded(w io.Writer, pw string) {
	if !shouldUseKeychain() {
		return
	}
	if pw == "" || passwordFlag != "" {
		return
	}
	if err := password.Store(pw); err != nil {
		fmt.Fprintf(w, "Warning: failed to store password in keychain: %v\n", err)
	}
}
