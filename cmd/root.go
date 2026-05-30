package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/password"
	"github.com/spf13/cobra"
)

var (
	passwordFlag  string
	passwordStdin bool
)

var rootCmd = &cobra.Command{
	Use:   "ekconf",
	Short: "Encrypted kubeconfig manager",
	Long: "\n\x1b[36m" +
		"           oooo                                         .o88o. \n" +
		"          `888                                         888 `\" \n" +
		" .ooooo.   888  oooo   .ooooo.   .ooooo.  ooo. .oo.   o888oo  \n" +
		"d88' `88b  888 .8P'   d88' `\"Y8 d88' `88b `888P\"Y88b   888    \n" +
		"888ooo888  888888.    888       888   888  888   888   888    \n" +
		"888    .o  888 `88b.  888   .o8 888   888  888   888   888    \n" +
		"`Y8bod8P' o888o o888o `Y8bod8P' `Y8bod8P' o888o o888o o888o \n" +
		"\n" +
		"\n" +
		"\n" +
		"\x1b[0m" +
		"Allows you to add and delete kubeconfigs by merging kubeconfig\n" +
		"files together and breaking them apart appropriately, with\n" +
		"encryption at rest and optional keychain integration.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.EnsureDir()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.InitDefaultCompletionCmd()
	rootCmd.PersistentFlags().StringVar(&passwordFlag, "password", "", "Password for decryption (inline)")
	rootCmd.PersistentFlags().BoolVar(&passwordStdin, "password-stdin", false, "Read password from stdin")
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
	return passwordResolve(passwordFlag, passwordStdin, useKeychain)
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

func storePasswordIfNeeded(pw string) {
	if !shouldUseKeychain() {
		return
	}
	if pw == "" || passwordFlag != "" {
		return
	}
	if err := password.Store(pw); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store password in keychain: %v\n", err)
	}
}
