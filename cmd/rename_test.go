package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func writeRenameFixture(t *testing.T, kc *clientcmdapi.Config) {
	t.Helper()
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	sealed, err := crypto.Seal(data, []byte("test-password"))
	require.NoError(t, err)
	encPath := filepath.Join(os.Getenv("HOME"), ".ekube", "config.enc")
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, sealed, 0o600))
}

func TestRename_OwnedClusterAndAuthInfoFollow(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: staging\ncontexts:\n  staging:\n    namespace: apps\n")

	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "staging"
	kc.Contexts["staging"] = &clientcmdapi.Context{
		Cluster:   "staging/cluster",
		AuthInfo:  "staging/user",
		Namespace: "apps",
	}
	kc.Clusters["staging/cluster"] = &clientcmdapi.Cluster{Server: "https://staging.example.com"}
	kc.AuthInfos["staging/user"] = &clientcmdapi.AuthInfo{Token: "staging-token"}
	writeRenameFixture(t, kc)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("rename", "staging", "preprod"))

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.NotContains(t, cfg.Contexts, "staging")
	assert.Equal(t, "apps", cfg.Contexts["preprod"].Namespace)
	assert.Equal(t, "preprod", cfg.Current)

	kubeconfig, err := loadDecryptedKubeconfig([]byte("test-password"))
	require.NoError(t, err)
	assert.NotContains(t, kubeconfig.Contexts, "staging")
	assert.NotContains(t, kubeconfig.Clusters, "staging/cluster")
	assert.NotContains(t, kubeconfig.AuthInfos, "staging/user")
	assert.Equal(t, "preprod/cluster", kubeconfig.Contexts["preprod"].Cluster)
	assert.Equal(t, "preprod/user", kubeconfig.Contexts["preprod"].AuthInfo)
	assert.Equal(t, "preprod", kubeconfig.CurrentContext)
	assert.Equal(t, "https://staging.example.com", kubeconfig.Clusters["preprod/cluster"].Server)
	assert.Equal(t, "staging-token", kubeconfig.AuthInfos["preprod/user"].Token)
}

func TestRename_SharedClusterAuthInfoUntouched(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts:\n  ctx1: {}\n  ctx2: {}\n")

	kc := clientcmdapi.NewConfig()
	kc.Contexts["ctx1"] = &clientcmdapi.Context{Cluster: "ctx1/shared", AuthInfo: "ctx1/shared"}
	kc.Contexts["ctx2"] = &clientcmdapi.Context{Cluster: "ctx1/shared", AuthInfo: "ctx1/shared"}
	kc.Clusters["ctx1/shared"] = &clientcmdapi.Cluster{Server: "https://shared.example.com"}
	kc.AuthInfos["ctx1/shared"] = &clientcmdapi.AuthInfo{Token: "shared-token"}
	writeRenameFixture(t, kc)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("rename", "ctx1", "renamed"))

	kubeconfig, err := loadDecryptedKubeconfig([]byte("test-password"))
	require.NoError(t, err)
	assert.Contains(t, kubeconfig.Clusters, "ctx1/shared")
	assert.NotContains(t, kubeconfig.Clusters, "renamed/shared")
	assert.Equal(t, "ctx1/shared", kubeconfig.Contexts["renamed"].Cluster)
	assert.Equal(t, "ctx1/shared", kubeconfig.Contexts["ctx2"].Cluster)
}

func TestRename_UnprefixedClusterKeptAsIs(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts:\n  legacy: {}\n")

	kc := clientcmdapi.NewConfig()
	kc.Contexts["legacy"] = &clientcmdapi.Context{Cluster: "some-cluster", AuthInfo: "some-user"}
	kc.Clusters["some-cluster"] = &clientcmdapi.Cluster{Server: "https://legacy.example.com"}
	kc.AuthInfos["some-user"] = &clientcmdapi.AuthInfo{Token: "legacy-token"}
	writeRenameFixture(t, kc)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("rename", "legacy", "modern"))

	kubeconfig, err := loadDecryptedKubeconfig([]byte("test-password"))
	require.NoError(t, err)
	assert.Equal(t, "some-cluster", kubeconfig.Contexts["modern"].Cluster)
	assert.Equal(t, "some-user", kubeconfig.Contexts["modern"].AuthInfo)
	assert.Contains(t, kubeconfig.Clusters, "some-cluster")
}

func TestRename_Errors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing source", []string{"rename", "nope", "other"}, "context 'nope' not found"},
		{"target exists", []string{"rename", "staging", "test-cluster"}, "context 'test-cluster' already exists"},
		{"same name", []string{"rename", "staging", "staging"}, "already has that name"},
		{"slash in name", []string{"rename", "staging", "a/b"}, "must not contain '/'"},
		{"empty name", []string{"rename", "staging", "  "}, "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestHome(t)
			writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts:\n  test-cluster: {}\n  staging: {}\n")
			writeTestConfigEnc(t)

			passwordFlag = "test-password"
			t.Cleanup(func() { passwordFlag = "" })

			err := executeCommand(tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
