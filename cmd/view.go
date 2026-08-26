package cmd

import (
	"fmt"

	"gabe565.com/utils/coloryaml"
	"github.com/eznix86/ekconf/internal/config"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var viewPlain bool

var viewCmd = &cobra.Command{
	Use:   "view <name>",
	Short: "Decrypt and print a single context's kubeconfig (redacted by default)",
	Long: `Decrypt and print a single context's kubeconfig to stdout. Output is
always colorized YAML. Sensitive fields (tokens, certificates) are redacted
by default. Use --plain to include all fields.`,
	Example: `  ekconf view prod
  ekconf view --plain staging
  ekconf view --password=secret dev`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
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

		if _, ok := kubeconfig.Contexts[contextName]; !ok {
			return fmt.Errorf("context '%s' not found", contextName)
		}

		selectedContext := kubeconfig.Contexts[contextName]
		ctx := clientcmdapi.NewConfig()
		ctx.CurrentContext = contextName
		ctx.Contexts[contextName] = selectedContext

		if entry, ok := cfg.Contexts[contextName]; ok && entry.Namespace != "" {
			ctxCopy := *ctx.Contexts[contextName]
			ctxCopy.Namespace = entry.Namespace
			ctx.Contexts[contextName] = &ctxCopy
		}

		if clusterName := selectedContext.Cluster; clusterName != "" {
			cluster := kubeconfig.Clusters[clusterName]
			if !viewPlain {
				cluster = redactCluster(cluster)
			}
			ctx.Clusters[clusterName] = cluster
		}

		if authInfoName := selectedContext.AuthInfo; authInfoName != "" {
			authInfo := kubeconfig.AuthInfos[authInfoName]
			if !viewPlain {
				authInfo = redactAuthInfo(authInfo)
			}
			ctx.AuthInfos[authInfoName] = authInfo
		}

		out, err := clientcmd.Write(*ctx)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		if cfg.YAML != nil && cfg.YAML.Colorize {
			_, err = coloryaml.WriteString(cmd.OutOrStdout(), string(out))
			return err
		}
		_, err = cmd.OutOrStdout().Write(out)
		return err
	},
}

func init() {
	rootCmd.AddCommand(viewCmd)
	viewCmd.Flags().BoolVar(&viewPlain, "plain", false, "Include sensitive kubeconfig data")
}

func redactAuthInfo(authInfo *clientcmdapi.AuthInfo) *clientcmdapi.AuthInfo {
	if authInfo == nil {
		return nil
	}

	redacted := *authInfo
	redacted.ClientCertificate = ""
	redacted.ClientCertificateData = nil
	redacted.ClientKey = ""
	redacted.ClientKeyData = nil
	redacted.Token = ""
	redacted.TokenFile = ""
	redacted.Password = ""
	redacted.AuthProvider = nil
	redacted.Exec = nil
	redacted.Impersonate = ""
	redacted.ImpersonateGroups = nil
	redacted.ImpersonateUserExtra = nil
	return &redacted
}

func redactCluster(cluster *clientcmdapi.Cluster) *clientcmdapi.Cluster {
	if cluster == nil {
		return nil
	}

	redacted := *cluster
	redacted.CertificateAuthority = ""
	redacted.CertificateAuthorityData = nil
	return &redacted
}
