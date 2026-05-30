package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"

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
		if cmd.Flags().Changed("password") || passwordStdin || cmd.Name() == "exec" || cmd.Name() == "add" || cmd.Name() == "rm" || cmd.Name() == "eject" || cmd.Name() == "rotate" || cmd.Name() == "view" {
			return recoverPendingFileTransaction()
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
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

func resolvePassword() (string, error) {
	return resolvePasswordWithKeychain(shouldUseKeychain())
}

func resolvePasswordWithKeychain(useKeychain bool) (string, error) {
	return password.Resolve(passwordFlag, passwordStdin, useKeychain, envPassword)
}

func completeContext(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
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
