package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"
)

var addName string

type contextRename struct{ src, dst string }

var addCmd = &cobra.Command{
	Use:   "add <path> [-n <name>]",
	Short: "Encrypt and merge a kubeconfig into config.enc",
	Long: `Add one or more contexts from a kubeconfig file into the encrypted store.

If the kubeconfig contains multiple contexts, all are merged. Use -n to
rename a single-context file on import. If a context name already exists,
the command prompts for confirmation.`,
	Example: `  ekconf add ~/.kube/config
  ekconf add staging.yaml -n staging
  ekconf add --password=secret prod.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		kubeconfig, err := clientcmd.LoadFromFile(path)
		if err != nil {
			return fmt.Errorf("load kubeconfig: %w", err)
		}

		password, err := resolvePassword(cmd.Context())
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

		renames, err := planAddRenames(cfg, kubeconfig, addName)
		if err != nil {
			return err
		}

		mergeKubeconfigContexts(cfg, existingKubeconfig, kubeconfig, renames)

		if err := writeMergedKubeconfig(cfg, existingKubeconfig, password); err != nil {
			return err
		}

		storePasswordIfNeeded(cmd.ErrOrStderr(), password)

		for _, r := range renames {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Added context '%s'\n", r.dst); err != nil {
				return err
			}
		}
		return nil
	},
}

func loadExistingKubeconfig(password []byte) (*clientcmdapi.Config, error) {
	encPath, err := config.EncPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(encPath)
	if err != nil {
		if os.IsNotExist(err) {
			return initializedKubeconfig(), nil
		}
		return nil, fmt.Errorf("read config.enc: %w", err)
	}

	plaintext, err := crypto.Open(data, password)
	if err != nil {
		return nil, fmt.Errorf("decrypt config.enc: %w (wrong password?)", err)
	}

	kubeconfig, err := clientcmd.Load(plaintext)
	if err != nil {
		return nil, fmt.Errorf("parse existing kubeconfig: %w", err)
	}
	initializeKubeconfigMaps(kubeconfig)
	return kubeconfig, nil
}

func initializedKubeconfig() *clientcmdapi.Config {
	kubeconfig := clientcmdapi.NewConfig()
	initializeKubeconfigMaps(kubeconfig)
	return kubeconfig
}

func initializeKubeconfigMaps(kubeconfig *clientcmdapi.Config) {
	if kubeconfig.Contexts == nil {
		kubeconfig.Contexts = make(map[string]*clientcmdapi.Context)
	}
	if kubeconfig.Clusters == nil {
		kubeconfig.Clusters = make(map[string]*clientcmdapi.Cluster)
	}
	if kubeconfig.AuthInfos == nil {
		kubeconfig.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)
	}
}

func planAddRenames(
	cfg *config.Config,
	kubeconfig *clientcmdapi.Config,
	nameOverride string,
) ([]contextRename, error) {
	usedNames := make(map[string]struct{}, len(cfg.Contexts)+len(kubeconfig.Contexts))
	for name := range cfg.Contexts {
		usedNames[name] = struct{}{}
	}

	var renames []contextRename
	if nameOverride != "" {
		if len(kubeconfig.Contexts) > 1 {
			return nil, fmt.Errorf("cannot rename: kubeconfig has %d contexts, -n requires a single-context file", len(kubeconfig.Contexts))
		}
		for name := range kubeconfig.Contexts {
			renames = append(renames, contextRename{src: name, dst: nameOverride})
		}
	} else {
		for name := range kubeconfig.Contexts {
			renames = append(renames, contextRename{src: name, dst: name})
		}
	}

	for _, r := range renames {
		if cfg.ContextExists(r.dst) || hasUsedName(usedNames, r.dst) {
			return nil, fmt.Errorf("context '%s' already exists, use -n to rename or choose a different name", r.dst)
		}
		usedNames[r.dst] = struct{}{}
	}

	return renames, nil
}

func mergeKubeconfigContexts(
	cfg *config.Config,
	existingKubeconfig *clientcmdapi.Config,
	kubeconfig *clientcmdapi.Config,
	renames []contextRename,
) {
	for _, r := range renames {
		ctx, ok := kubeconfig.Contexts[r.src]
		if !ok {
			continue
		}

		clusterName := r.dst + "/" + ctx.Cluster
		authInfoName := r.dst + "/" + ctx.AuthInfo

		if cluster, ok := kubeconfig.Clusters[ctx.Cluster]; ok {
			existingKubeconfig.Clusters[clusterName] = cluster
		}
		if authInfo, ok := kubeconfig.AuthInfos[ctx.AuthInfo]; ok {
			existingKubeconfig.AuthInfos[authInfoName] = authInfo
		}
		existingKubeconfig.Contexts[r.dst] = &clientcmdapi.Context{
			Cluster:   clusterName,
			AuthInfo:  authInfoName,
			Namespace: ctx.Namespace,
		}

		cfg.Contexts[r.dst] = config.ContextEntry{Namespace: ctx.Namespace}
	}
}

func writeMergedKubeconfig(cfg *config.Config, kubeconfig *clientcmdapi.Config, password []byte) error {
	mergedData, err := clientcmd.Write(*kubeconfig)
	if err != nil {
		return fmt.Errorf("marshal merged kubeconfig: %w", err)
	}

	encryptedData, err := crypto.Seal(mergedData, password)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	encPath, err := config.EncPath()
	if err != nil {
		return err
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		return err
	}

	cfgData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config.yaml: %w", err)
	}

	return replaceFilesAtomically([]fileUpdate{
		{path: encPath, data: encryptedData},
		{path: configPath, data: cfgData},
	})
}

func hasUsedName(used map[string]struct{}, name string) bool {
	_, ok := used[name]
	return ok
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&addName, "name", "n", "", "Name for the added context")
}
