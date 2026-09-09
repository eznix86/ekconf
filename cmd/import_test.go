package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func writeTestKubeconfig(t *testing.T, names ...string) string {
	t.Helper()

	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = names[0]
	for _, name := range names {
		kc.Contexts[name] = &clientcmdapi.Context{Cluster: name + "-cluster", AuthInfo: name + "-user"}
		kc.Clusters[name+"-cluster"] = &clientcmdapi.Cluster{Server: "https://" + name + ".example.com"}
		kc.AuthInfos[name+"-user"] = &clientcmdapi.AuthInfo{Token: name + "-token"}
	}

	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)

	srcPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0o700))
	require.NoError(t, os.WriteFile(srcPath, data, 0o600))
	return srcPath
}

func TestImport_NamedContextImportsOnlyThatOne(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	srcPath := writeTestKubeconfig(t, "prod", "staging", "dev")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)
	require.NoError(t, executeCommand("import", "prod"))

	got := output()
	assert.Contains(t, got, "Imported context 'prod'")
	assert.NotContains(t, got, "staging")
	assert.NotContains(t, got, "dev")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"prod"}, sortedContextNames(cfg))

	decrypted, err := loadDecryptedKubeconfig([]byte("test-password"))
	require.NoError(t, err)
	assert.Equal(t, "prod-token", decrypted.AuthInfos["prod/prod-user"].Token)
	assert.NotContains(t, decrypted.Contexts, "staging")

	_, err = os.Stat(srcPath)
	require.NoError(t, err, "source file must survive a partial import")
}

func TestImport_SeveralNamedContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	writeTestKubeconfig(t, "prod", "staging", "dev")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("import", "prod", "dev"))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "prod"}, sortedContextNames(cfg))
}

func TestImport_UnknownContextListsAvailable(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	writeTestKubeconfig(t, "prod", "staging")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("import", "nope")
	require.ErrorContains(t, err, "context 'nope' not found in kubeconfig")
	require.ErrorContains(t, err, "prod, staging")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.Contexts, "nothing may be imported when a name is unknown")
}

func TestImport_ForceRejectedWithNamedContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	srcPath := writeTestKubeconfig(t, "prod", "staging")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("import", "--force", "prod")
	require.ErrorContains(t, err, "cannot be combined with named contexts")

	_, statErr := os.Stat(srcPath)
	require.NoError(t, statErr, "source file must not be removed")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.Contexts, "nothing may be imported when the flags conflict")
}

func TestImport_SecondImportOfAnotherContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	writeTestKubeconfig(t, "prod", "staging")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("import", "prod"))
	require.NoError(t, executeCommand("import", "staging"))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"prod", "staging"}, sortedContextNames(cfg))

	decrypted, err := loadDecryptedKubeconfig([]byte("test-password"))
	require.NoError(t, err)
	assert.Equal(t, "prod-token", decrypted.AuthInfos["prod/prod-user"].Token)
	assert.Equal(t, "staging-token", decrypted.AuthInfos["staging/staging-user"].Token)
}

func TestImport_RepeatedContextIsRejected(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	writeTestKubeconfig(t, "prod", "staging")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("import", "prod"))

	err := executeCommand("import", "prod")
	require.ErrorContains(t, err, "already exists")
}
