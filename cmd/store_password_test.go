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

func writeSourceKubeconfig(t *testing.T, name string) string {
	t.Helper()

	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = name
	kc.Contexts[name] = &clientcmdapi.Context{Cluster: name + "-cluster", AuthInfo: name + "-user"}
	kc.Clusters[name+"-cluster"] = &clientcmdapi.Cluster{Server: "https://" + name + ".example.com"}
	kc.AuthInfos[name+"-user"] = &clientcmdapi.AuthInfo{Token: name + "-token"}

	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), name+".yaml")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestAdd_NewStoreRejectsAShortPassword(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	src := writeSourceKubeconfig(t, "prod")

	passwordFlag = "short"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("add", src)
	require.ErrorContains(t, err, "at least 12 characters")

	encPath, err := config.EncPath()
	require.NoError(t, err)
	_, statErr := os.Stat(encPath)
	assert.True(t, os.IsNotExist(statErr), "no store may be created with a weak password")
}

func TestAdd_NewStoreAcceptsALongEnoughPassword(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	src := writeSourceKubeconfig(t, "prod")

	passwordFlag = "a-long-enough-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("add", src))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "prod")
}

// The length rule guards store creation only. An existing store keeps working
// with whatever password already protects it.
func TestAdd_ExistingStoreKeepsAShortPassword(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("add", writeSourceKubeconfig(t, "extra")))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "extra")
}

func TestImport_NewStoreRejectsAShortPassword(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")
	writeTestKubeconfig(t, "prod")

	passwordFlag = "short"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("import")
	require.ErrorContains(t, err, "at least 12 characters")

	encPath, err := config.EncPath()
	require.NoError(t, err)
	_, statErr := os.Stat(encPath)
	assert.True(t, os.IsNotExist(statErr), "no store may be created with a weak password")
}
