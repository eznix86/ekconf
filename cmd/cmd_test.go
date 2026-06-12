package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestMain(m *testing.M) {
	oldOpenTTY := openTTY
	openTTY = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return nil, errors.New("tty disabled in test")
	}
	code := m.Run()
	openTTY = oldOpenTTY
	os.Exit(code)
}

func setupTestHome(t *testing.T) {
	t.Helper()
	resetCommandTestState(t)
	dir := t.TempDir()
	t.Setenv("HOME", dir)
}

func resetCommandTestState(t *testing.T) {
	t.Helper()
	passwordFlag = ""
	passwordStdin = false
	envPassword = ""
	addName = ""
	viewPlain = false
	ejectForce = false
	ejectMerge = false
	execNoShell = false
	resetBoolFlag(ejectCmd, "force")
	resetBoolFlag(ejectCmd, "merge")
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	t.Cleanup(func() {
		passwordFlag = ""
		passwordStdin = false
		envPassword = ""
		addName = ""
		viewPlain = false
		ejectForce = false
		ejectMerge = false
		execNoShell = false
		resetBoolFlag(ejectCmd, "force")
		resetBoolFlag(ejectCmd, "merge")
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
	})
}

func resetBoolFlag(cmd *cobra.Command, name string) {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return
	}
	_ = flag.Value.Set("false")
	flag.Changed = false
}

func writeTestConfigYAML(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(os.Getenv("HOME"), ".ekube", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func writeTestConfigEnc(t *testing.T) {
	t.Helper()
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "test-cluster"
	kc.Contexts["test-cluster"] = &clientcmdapi.Context{
		Cluster:  "test-cluster",
		AuthInfo: "test-user",
	}
	kc.Contexts["staging"] = &clientcmdapi.Context{
		Cluster:  "staging-cluster",
		AuthInfo: "staging-user",
	}
	kc.Clusters["test-cluster"] = &clientcmdapi.Cluster{
		Server:                   "https://example.com:6443",
		CertificateAuthorityData: []byte("test-ca-data"),
		CertificateAuthority:     "/tmp/test-ca.crt",
	}
	kc.Clusters["staging-cluster"] = &clientcmdapi.Cluster{
		Server:                   "https://staging.example.com:6443",
		CertificateAuthorityData: []byte("staging-ca-data"),
		CertificateAuthority:     "/tmp/staging-ca.crt",
	}
	kc.AuthInfos["test-user"] = &clientcmdapi.AuthInfo{
		Username: "admin",
		Token:    "test-token",
	}
	kc.AuthInfos["staging-user"] = &clientcmdapi.AuthInfo{
		Username: "deploy",
		Token:    "staging-token",
	}

	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)

	ef, err := crypto.Encrypt(data, "test-password")
	require.NoError(t, err)

	encPath := filepath.Join(os.Getenv("HOME"), ".ekube", "config.enc")
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(ef), 0o600))
}

func executeCommand(args ...string) error {
	_ = execCmd.Flags().Parse(nil)
	rootCmd.SetArgs(args)
	defer rootCmd.SetArgs(nil)
	return rootCmd.Execute()
}

func captureOutput(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	old := os.Stdout
	os.Stdout = w
	rootCmd.SetOut(w)

	return func() string {
		require.NoError(t, w.Close())
		os.Stdout = old
		rootCmd.SetOut(old)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		return string(data)
	}
}

func TestTempDir(t *testing.T) {
	dir := tempDir()
	assert.NotEmpty(t, dir)

	fi, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, fi.IsDir())
}

func TestTempDir_ShmPreferred(t *testing.T) {
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		assert.Equal(t, "/dev/shm", tempDir())
	}
}

func TestWipeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("sensitive-data-here"), 0o600))

	err := wipeFile(path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, b := range data {
		assert.Equal(t, byte(0), b, "file should be zeroed after wipe")
	}
}

func TestWipeFile_Nonexistent(t *testing.T) {
	err := wipeFile("/nonexistent/path")
	require.Error(t, err)
}

func TestWipeFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

	err := wipeFile(path)
	require.NoError(t, err)
}

func TestShouldUseKeychain_Enabled(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: true\ncurrent: \"\"\ncontexts: {}\n")

	assert.True(t, shouldUseKeychain())
}

func TestShouldUseKeychain_Disabled(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	assert.False(t, shouldUseKeychain())
}

func TestShouldUseKeychain_NoConfig(t *testing.T) {
	setupTestHome(t)
	assert.False(t, shouldUseKeychain())
}

func TestResolvePassword_PasswordFlag(t *testing.T) {
	passwordFlag = "test-pass"
	t.Cleanup(func() { passwordFlag = "" })

	pw, err := resolvePasswordWithKeychain(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, "test-pass", pw)
}

func TestLS_NoContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	output := captureOutput(t)

	err := executeCommand("ls")
	require.NoError(t, err)

	assert.Contains(t, output(), "No contexts found")
}

func TestLS_WithContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: prod\ncontexts:\n  prod:\n    namespace: production\n  staging:\n    namespace: staging\n")

	output := captureOutput(t)

	err := executeCommand("ls")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "prod")
	assert.Contains(t, got, "staging")
	assert.Contains(t, got, "* prod")
	// contexts should be in alphabetical order
	lines := strings.Split(strings.TrimSpace(got), "\n")
	assert.Equal(t, "* prod                          namespace: production", strings.TrimSpace(lines[0]))
	assert.Equal(t, "staging                       namespace: staging", strings.TrimSpace(lines[1]))
}

func TestUse_Success(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: staging\ncontexts:\n  staging:\n    namespace: staging\n  prod:\n    namespace: production\n")

	output := captureOutput(t)

	err := executeCommand("use", "prod")
	require.NoError(t, err)

	assert.Contains(t, output(), "Switched to context 'prod'")
}

func TestUse_NotFound(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	err := executeCommand("use", "nonexistent")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestNS_Success(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: prod\ncontexts:\n  prod:\n    namespace: production\n")

	output := captureOutput(t)

	err := executeCommand("ns", "staging")
	require.NoError(t, err)

	assert.Contains(t, output(), "Set namespace 'staging' on context 'prod'")
}

func TestNS_NoActiveContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	err := executeCommand("ns", "default")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no active context set")
}

func TestMergeKubeconfigContexts_PrefixesClusterAndAuthInfoNames(t *testing.T) {
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{}}
	existing := clientcmdapi.NewConfig()
	initializeKubeconfigMaps(existing)

	src := clientcmdapi.NewConfig()
	src.Contexts["admin@prod"] = &clientcmdapi.Context{Cluster: "prod-cluster", AuthInfo: "user"}
	src.Clusters["prod-cluster"] = &clientcmdapi.Cluster{Server: "https://prod.example.com"}
	src.AuthInfos["user"] = &clientcmdapi.AuthInfo{Token: "prod-token"}

	mergeKubeconfigContexts(cfg, existing, src, []contextRename{{src: "admin@prod", dst: "admin@prod"}})

	assert.Equal(t, "admin@prod/prod-cluster", existing.Contexts["admin@prod"].Cluster)
	assert.Equal(t, "admin@prod/user", existing.Contexts["admin@prod"].AuthInfo)
	assert.Equal(t, "https://prod.example.com", existing.Clusters["admin@prod/prod-cluster"].Server)
	assert.Equal(t, "prod-token", existing.AuthInfos["admin@prod/user"].Token)
	assert.Contains(t, cfg.Contexts, "admin@prod")
}

func TestMergeKubeconfigContexts_PreventsAuthInfoCollision(t *testing.T) {
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{}}
	existing := clientcmdapi.NewConfig()
	initializeKubeconfigMaps(existing)

	// Add first context with auth info named "user"
	fileA := clientcmdapi.NewConfig()
	fileA.Contexts["admin@prod"] = &clientcmdapi.Context{Cluster: "prod-cluster", AuthInfo: "user"}
	fileA.Clusters["prod-cluster"] = &clientcmdapi.Cluster{Server: "https://prod.example.com"}
	fileA.AuthInfos["user"] = &clientcmdapi.AuthInfo{Token: "prod-token"}
	mergeKubeconfigContexts(cfg, existing, fileA, []contextRename{{src: "admin@prod", dst: "admin@prod"}})

	// Add second context with auth info also named "user" but different token
	fileB := clientcmdapi.NewConfig()
	fileB.Contexts["admin@preprod"] = &clientcmdapi.Context{Cluster: "preprod-cluster", AuthInfo: "user"}
	fileB.Clusters["preprod-cluster"] = &clientcmdapi.Cluster{Server: "https://preprod.example.com"}
	fileB.AuthInfos["user"] = &clientcmdapi.AuthInfo{Token: "preprod-token"}
	mergeKubeconfigContexts(cfg, existing, fileB, []contextRename{{src: "admin@preprod", dst: "admin@preprod"}})

	// Both auth infos should exist independently
	assert.Equal(t, "prod-token", existing.AuthInfos["admin@prod/user"].Token)
	assert.Equal(t, "preprod-token", existing.AuthInfos["admin@preprod/user"].Token)
	// Clusters should also be independent
	assert.Equal(t, "https://prod.example.com", existing.Clusters["admin@prod/prod-cluster"].Server)
	assert.Equal(t, "https://preprod.example.com", existing.Clusters["admin@preprod/preprod-cluster"].Server)
	assert.Len(t, existing.AuthInfos, 2)
	assert.Len(t, existing.Clusters, 2)
}

func TestMergeKubeconfigContexts_NestedContextClustersRenames(t *testing.T) {
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{}}
	existing := clientcmdapi.NewConfig()
	initializeKubeconfigMaps(existing)

	src := clientcmdapi.NewConfig()
	src.CurrentContext = "user@cluster"
	src.Contexts["user@cluster"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "user"}
	src.Clusters["cluster"] = &clientcmdapi.Cluster{Server: "https://example.com"}
	src.AuthInfos["user"] = &clientcmdapi.AuthInfo{Token: "token"}

	// With -n rename
	mergeKubeconfigContexts(cfg, existing, src, []contextRename{{src: "user@cluster", dst: "my-custom-name"}})

	assert.Equal(t, "my-custom-name/cluster", existing.Contexts["my-custom-name"].Cluster)
	assert.Equal(t, "my-custom-name/user", existing.Contexts["my-custom-name"].AuthInfo)
}

func TestMergeKubeconfigContexts_UnprefixedEntryIsNotOverwritten(t *testing.T) {
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{}}
	existing := clientcmdapi.NewConfig()
	initializeKubeconfigMaps(existing)

	// Simulate an existing (legacy) context with unprefixed names
	existing.Contexts["legacy"] = &clientcmdapi.Context{Cluster: "legacy-cluster", AuthInfo: "legacy-user"}
	existing.Clusters["legacy-cluster"] = &clientcmdapi.Cluster{Server: "https://legacy.example.com"}
	existing.AuthInfos["legacy-user"] = &clientcmdapi.AuthInfo{Token: "legacy-token"}

	// Add a new context that has no name collision — should not touch legacy entries
	src := clientcmdapi.NewConfig()
	src.Contexts["new"] = &clientcmdapi.Context{Cluster: "new-cluster", AuthInfo: "new-user"}
	src.Clusters["new-cluster"] = &clientcmdapi.Cluster{Server: "https://new.example.com"}
	src.AuthInfos["new-user"] = &clientcmdapi.AuthInfo{Token: "new-token"}
	mergeKubeconfigContexts(cfg, existing, src, []contextRename{{src: "new", dst: "new"}})

	// Legacy entries are untouched
	assert.Equal(t, "https://legacy.example.com", existing.Clusters["legacy-cluster"].Server)
	assert.Equal(t, "legacy-token", existing.AuthInfos["legacy-user"].Token)
}

func TestAdd_DuplicateExistingContextRejected(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: alpha\ncontexts:\n  alpha:\n    namespace: default\n")

	src := filepath.Join(t.TempDir(), "source.yaml")
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "alpha"
	kc.Contexts["alpha"] = &clientcmdapi.Context{Cluster: "alpha-cluster", AuthInfo: "alpha-user"}
	kc.Clusters["alpha-cluster"] = &clientcmdapi.Cluster{Server: "https://alpha.example.com"}
	kc.AuthInfos["alpha-user"] = &clientcmdapi.AuthInfo{Username: "alpha"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(src, data, 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })
	err = executeCommand("add", src)
	require.Error(t, err)
	require.ErrorContains(t, err, "already exists")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "alpha")
	assert.Len(t, cfg.Contexts, 1)
}

func TestAdd_DuplicateExistingContextsInBatchRejected(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: ctx1\ncontexts:\n  ctx1:\n    namespace: default\n  ctx2:\n    namespace: default\n")

	src := filepath.Join(t.TempDir(), "source.yaml")
	kc := clientcmdapi.NewConfig()
	kc.Contexts["ctx1"] = &clientcmdapi.Context{Cluster: "cluster1", AuthInfo: "user1", Namespace: "ns1"}
	kc.Contexts["ctx2"] = &clientcmdapi.Context{Cluster: "cluster2", AuthInfo: "user2", Namespace: "ns2"}
	kc.Clusters["cluster1"] = &clientcmdapi.Cluster{Server: "https://one.example.com"}
	kc.Clusters["cluster2"] = &clientcmdapi.Cluster{Server: "https://two.example.com"}
	kc.AuthInfos["user1"] = &clientcmdapi.AuthInfo{Username: "one"}
	kc.AuthInfos["user2"] = &clientcmdapi.AuthInfo{Username: "two"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(src, data, 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })
	err = executeCommand("add", src)
	require.Error(t, err)
	require.ErrorContains(t, err, "already exists")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "ctx1")
	assert.Contains(t, cfg.Contexts, "ctx2")
	assert.Len(t, cfg.Contexts, 2)
}

func TestAdd_SameAuthInfoNameNoCollision(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	writeFile := func(name, clusterName, server, token string) string {
		p := filepath.Join(t.TempDir(), name+".yaml")
		kc := clientcmdapi.NewConfig()
		kc.CurrentContext = name
		kc.Contexts[name] = &clientcmdapi.Context{Cluster: clusterName, AuthInfo: "user"} // same auth info name
		kc.Clusters[clusterName] = &clientcmdapi.Cluster{Server: server}
		kc.AuthInfos["user"] = &clientcmdapi.AuthInfo{Token: token}
		data, err := clientcmd.Write(*kc)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(p, data, 0o600))
		return p
	}

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	// First file — context name is "admin@prod-cluster1"
	fileA := writeFile("admin@prod-cluster1", "prod-cluster", "https://prod.example.com", "prod-token")
	err := executeCommand("add", fileA)
	require.NoError(t, err)

	// Second file — context name is "admin@preprod-cluster1", same auth info name "user"
	fileB := writeFile("admin@preprod-cluster1", "preprod-cluster", "https://preprod.example.com", "preprod-token")
	err = executeCommand("add", fileB)
	require.NoError(t, err)

	// Both contexts should be in config.yaml
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "admin@prod-cluster1")
	assert.Contains(t, cfg.Contexts, "admin@preprod-cluster1")

	// Decrypt and verify both auth infos exist independently
	kubeconfig, err := loadDecryptedKubeconfig("test-password")
	require.NoError(t, err)

	// Auth infos are prefixed
	assert.Equal(t, "prod-token", kubeconfig.AuthInfos["admin@prod-cluster1/user"].Token)
	assert.Equal(t, "preprod-token", kubeconfig.AuthInfos["admin@preprod-cluster1/user"].Token)

	// Clusters are prefixed
	assert.Equal(t, "https://prod.example.com", kubeconfig.Clusters["admin@prod-cluster1/prod-cluster"].Server)
	assert.Equal(t, "https://preprod.example.com", kubeconfig.Clusters["admin@preprod-cluster1/preprod-cluster"].Server)

	// Contexts reference the prefixed names
	assert.Equal(t, "admin@prod-cluster1/prod-cluster", kubeconfig.Contexts["admin@prod-cluster1"].Cluster)
	assert.Equal(t, "admin@prod-cluster1/user", kubeconfig.Contexts["admin@prod-cluster1"].AuthInfo)
	assert.Equal(t, "admin@preprod-cluster1/preprod-cluster", kubeconfig.Contexts["admin@preprod-cluster1"].Cluster)
	assert.Equal(t, "admin@preprod-cluster1/user", kubeconfig.Contexts["admin@preprod-cluster1"].AuthInfo)
}

func TestAdd_SameClusterNameNoCollision(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	writeFile := func(name, server, token string) string {
		p := filepath.Join(t.TempDir(), name+".yaml")
		kc := clientcmdapi.NewConfig()
		kc.CurrentContext = name
		kc.Contexts[name] = &clientcmdapi.Context{Cluster: "production", AuthInfo: name + "-user"} // same cluster name
		kc.Clusters["production"] = &clientcmdapi.Cluster{Server: server}
		kc.AuthInfos[name+"-user"] = &clientcmdapi.AuthInfo{Token: token}
		data, err := clientcmd.Write(*kc)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(p, data, 0o600))
		return p
	}

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	// First file — cluster is named "production"
	fileA := writeFile("us-east", "https://us-east.example.com", "east-token")
	err := executeCommand("add", fileA)
	require.NoError(t, err)

	// Second file — same cluster name "production"
	fileB := writeFile("eu-west", "https://eu-west.example.com", "west-token")
	err = executeCommand("add", fileB)
	require.NoError(t, err)

	kubeconfig, err := loadDecryptedKubeconfig("test-password")
	require.NoError(t, err)

	// Both clusters exist independently
	assert.Equal(t, "https://us-east.example.com", kubeconfig.Clusters["us-east/production"].Server)
	assert.Equal(t, "https://eu-west.example.com", kubeconfig.Clusters["eu-west/production"].Server)
}

func TestConfig_List(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: prod\ncontexts:\n  prod:\n    namespace: production\n")

	output := captureOutput(t)

	err := executeCommand("config", "list")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "keychain: false")
	assert.Contains(t, got, "current: prod")
}

func TestConfig_SetKeychain(t *testing.T) {
	setupTestHome(t)

	err := executeCommand("config", "keychain=true")
	require.NoError(t, err)

	output := captureOutput(t)

	err = executeCommand("config", "list")
	require.NoError(t, err)

	assert.Contains(t, output(), "keychain: true")
}

func TestConfig_InvalidKey(t *testing.T) {
	setupTestHome(t)
	err := executeCommand("config", "invalid=value")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown config key")
}

func TestConfig_InvalidFormat(t *testing.T) {
	setupTestHome(t)
	err := executeCommand("config", "badformat")
	require.Error(t, err)
	assert.ErrorContains(t, err, "expected key=value")
}

func TestConfig_InvalidKeychainValue(t *testing.T) {
	setupTestHome(t)
	err := executeCommand("config", "keychain=invalid")
	require.Error(t, err)
	assert.ErrorContains(t, err, "keychain must be 'true' or 'false'")
}

func TestView_WithPassword(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err := executeCommand("view", "test-cluster")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "current-context: test-cluster")
	assert.Contains(t, got, "https://example.com:6443")
	assert.NotContains(t, got, "test-token")
	assert.NotContains(t, got, "certificate-authority-data:")
	assert.NotContains(t, got, "staging.example.com")
	assert.NotContains(t, got, "staging-token")
	assert.NotContains(t, got, "/tmp/staging-ca.crt")
}

func TestView_PlainIncludesSensitiveData(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	viewPlain = true
	t.Cleanup(func() {
		passwordFlag = ""
		viewPlain = false
	})

	output := captureOutput(t)

	err := executeCommand("view", "test-cluster", "--plain")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "current-context: test-cluster")
	assert.Contains(t, got, "test-token")
	assert.Contains(t, got, "certificate-authority-data:")
	assert.NotContains(t, got, "staging.example.com")
	assert.NotContains(t, got, "staging-token")
	assert.NotContains(t, got, "/tmp/staging-ca.crt")
}

func TestView_NonexistentContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("view", "missing-cluster")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestEject_Force(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	ejectForce = true
	t.Cleanup(func() {
		passwordFlag = ""
		ejectForce = false
	})

	err := executeCommand("eject", "--force")
	require.NoError(t, err)

	kubeConfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	_, err = os.Stat(kubeConfig)
	assert.NoError(t, err, "~/.kube/config should exist after eject")
}

func TestEject_NoEncryptedFile(t *testing.T) {
	setupTestHome(t)

	ejectForce = true
	passwordFlag = "test-pass"
	t.Cleanup(func() {
		passwordFlag = ""
		ejectForce = false
	})

	err := executeCommand("eject", "--force")
	require.Error(t, err)
	assert.ErrorContains(t, err, "read config.enc")
}

func TestPromptConfirmation_FromTTY(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString("yes\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	oldOpenTTY := openTTY
	openTTY = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return r, nil
	}
	t.Cleanup(func() { openTTY = oldOpenTTY })

	ok, err := promptConfirmation(t.Context(), io.Discard, "Continue? [y/N]: ")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestExec_NoCommand(t *testing.T) {
	setupTestHome(t)

	err := executeCommand("exec")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no command specified")
}

func TestExec_WithPasswordFlag(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err := executeCommand("exec", "echo", "hello-from-ekconf")
	require.NoError(t, err)

	assert.Contains(t, output(), "hello-from-ekconf")
}

func TestExec_SpecificContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n  staging:\n    namespace: staging\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err := executeCommand("exec", "staging", "--", "echo", "staging-context")
	require.NoError(t, err)

	assert.Contains(t, output(), "staging-context")
}

func TestExec_UnknownExplicitContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")

	err := executeCommand("exec", "missing", "--", "echo", "hi")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown context: missing")
}

func TestExec_NoActiveContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	passwordFlag = "test-pass"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("exec", "echo", "hi")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no active context set")
}

func TestExec_PreservesChildExitCode(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("exec", "--no-shell", "--", "sh", "-c", "exit 7")
	require.Error(t, err)
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		assert.Equal(t, 7, exitErr.ExitCode())
	} else {
		t.Fatalf("expected exit code error, got %T", err)
	}
}

func TestExec_CleanupTempRemovesFile(t *testing.T) {
	tmpPath, cleanup, err := writeTempKubeconfig([]byte("test-data"))
	require.NoError(t, err)
	tmpDir := filepath.Dir(tmpPath)

	_, err = os.Stat(tmpPath)
	require.NoError(t, err, "temp file should exist before cleanup")

	err = cleanup()
	require.NoError(t, err)

	_, err = os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(err), "temp file should be removed after cleanup")
	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err), "temp dir should be removed after cleanup")
}

func TestExec_TempKubeconfigPrivatePath(t *testing.T) {
	tmpPath, cleanup, err := writeTempKubeconfig([]byte("test-data"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })

	fileInfo, err := os.Stat(tmpPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	dirInfo, err := os.Stat(filepath.Dir(tmpPath))
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestExec_DoubleCleanupOnceNoPanic(t *testing.T) {
	tmpPath, cleanup, err := writeTempKubeconfig([]byte("test-data"))
	require.NoError(t, err)
	require.FileExists(t, tmpPath)

	var once sync.Once
	once.Do(func() {
		require.NoError(t, cleanup())
	})

	// Second call via sync.Once should be a no-op — no error, no panic
	assert.NotPanics(t, func() {
		once.Do(func() {
			t.Error("sync.Once allowed second execution")
		})
	})

	_, err = os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(err), "file should be gone after first cleanup")
}

func TestMigrate_LegacyConfigEnc(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	encPath := filepath.Join(os.Getenv("HOME"), ".ekube", "config.enc")
	before, err := os.ReadFile(encPath)
	require.NoError(t, err)
	require.False(t, crypto.IsSecretBox(before))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err = executeCommand("migrate")
	require.NoError(t, err)
	gotOutput := output()
	assert.Contains(t, gotOutput, "Migrated config.enc")
	assert.Contains(t, gotOutput, "Legacy backup:")

	after, err := os.ReadFile(encPath)
	require.NoError(t, err)
	require.True(t, crypto.IsSecretBox(after))
	assert.NotEqual(t, before, after)

	backup, err := os.ReadFile(encPath + ".v0.bak")
	require.NoError(t, err)
	assert.Equal(t, before, backup)

	kubeconfig, err := loadDecryptedKubeconfig("test-password")
	require.NoError(t, err)
	assert.Contains(t, kubeconfig.Contexts, "test-cluster")
}

func TestMigrate_CurrentFormatNoPasswordNeeded(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")

	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "test-cluster"
	kc.Contexts["test-cluster"] = &clientcmdapi.Context{Cluster: "test-cluster", AuthInfo: "test-user"}
	kc.Clusters["test-cluster"] = &clientcmdapi.Cluster{Server: "https://example.com:6443"}
	kc.AuthInfos["test-user"] = &clientcmdapi.AuthInfo{Token: "test-token"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)

	encryptedData, err := crypto.Seal(data, "test-password")
	require.NoError(t, err)
	encPath := filepath.Join(os.Getenv("HOME"), ".ekube", "config.enc")
	require.NoError(t, os.WriteFile(encPath, encryptedData, 0o600))

	output := captureOutput(t)

	err = executeCommand("migrate")
	require.NoError(t, err)
	assert.Contains(t, output(), "already uses the current encrypted format")

	after, err := os.ReadFile(encPath)
	require.NoError(t, err)
	assert.Equal(t, encryptedData, after)
}

func TestRm_NotFound(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	encPath, err := config.EncPath()
	require.NoError(t, err)
	kc := clientcmdapi.NewConfig()
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	ef, err := crypto.Encrypt(data, "test-password")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(ef), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err = executeCommand("rm", "nonexistent")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestRm_Single(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: staging\ncontexts:\n  staging:\n    namespace: staging\n  prod:\n    namespace: production\n")

	// Write config.enc with staging + prod
	encPath, err := config.EncPath()
	require.NoError(t, err)
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "staging"
	kc.Contexts["staging"] = &clientcmdapi.Context{Cluster: "staging-cluster", AuthInfo: "staging-user"}
	kc.Contexts["prod"] = &clientcmdapi.Context{Cluster: "prod-cluster", AuthInfo: "prod-user"}
	kc.Clusters["staging-cluster"] = &clientcmdapi.Cluster{Server: "https://staging.example.com"}
	kc.Clusters["prod-cluster"] = &clientcmdapi.Cluster{Server: "https://prod.example.com"}
	kc.AuthInfos["staging-user"] = &clientcmdapi.AuthInfo{Username: "deploy"}
	kc.AuthInfos["prod-user"] = &clientcmdapi.AuthInfo{Username: "admin"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	ef, err := crypto.Encrypt(data, "test-password")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(ef), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err = executeCommand("rm", "staging")
	require.NoError(t, err)

	assert.Contains(t, output(), "Removed context 'staging'")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.NotContains(t, cfg.Contexts, "staging")
	assert.Contains(t, cfg.Contexts, "prod")
	// current was reset since staging was active
	assert.Empty(t, cfg.Current)

	// Verify cluster and auth info removed
	kubeconfig, err := loadDecryptedKubeconfig("test-password")
	require.NoError(t, err)
	assert.NotContains(t, kubeconfig.Contexts, "staging")
	assert.NotContains(t, kubeconfig.Clusters, "staging-cluster")
	assert.NotContains(t, kubeconfig.AuthInfos, "staging-user")
	// Unrelated entries survive
	assert.Contains(t, kubeconfig.Contexts, "prod")
}

func TestRm_Multiple(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: staging\ncontexts:\n  staging:\n    namespace: staging\n  prod:\n    namespace: production\n  dev:\n    namespace: dev\n")

	// Write config.enc with staging + prod + dev
	encPath, err := config.EncPath()
	require.NoError(t, err)
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "staging"
	kc.Contexts["staging"] = &clientcmdapi.Context{Cluster: "staging-cluster", AuthInfo: "staging-user"}
	kc.Contexts["prod"] = &clientcmdapi.Context{Cluster: "prod-cluster", AuthInfo: "prod-user"}
	kc.Contexts["dev"] = &clientcmdapi.Context{Cluster: "dev-cluster", AuthInfo: "dev-user"}
	kc.Clusters["staging-cluster"] = &clientcmdapi.Cluster{Server: "https://staging.example.com"}
	kc.Clusters["prod-cluster"] = &clientcmdapi.Cluster{Server: "https://prod.example.com"}
	kc.Clusters["dev-cluster"] = &clientcmdapi.Cluster{Server: "https://dev.example.com"}
	kc.AuthInfos["staging-user"] = &clientcmdapi.AuthInfo{Username: "deploy"}
	kc.AuthInfos["prod-user"] = &clientcmdapi.AuthInfo{Username: "admin"}
	kc.AuthInfos["dev-user"] = &clientcmdapi.AuthInfo{Username: "dev"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	ef, err := crypto.Encrypt(data, "test-password")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(ef), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err = executeCommand("rm", "staging", "dev")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "Removed context 'staging'")
	assert.Contains(t, got, "Removed context 'dev'")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.NotContains(t, cfg.Contexts, "staging")
	assert.NotContains(t, cfg.Contexts, "dev")
	assert.Contains(t, cfg.Contexts, "prod")
}

func TestRm_PartialNotFound(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts:\n  staging:\n    namespace: staging\n")

	encPath, err := config.EncPath()
	require.NoError(t, err)
	kc := clientcmdapi.NewConfig()
	kc.Contexts["staging"] = &clientcmdapi.Context{Cluster: "staging-cluster", AuthInfo: "staging-user"}
	kc.Clusters["staging-cluster"] = &clientcmdapi.Cluster{Server: "https://staging.example.com"}
	kc.AuthInfos["staging-user"] = &clientcmdapi.AuthInfo{Username: "deploy"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	ef, err := crypto.Encrypt(data, "test-password")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(ef), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err = executeCommand("rm", "staging", "missing")
	// The one that was found should be removed, but error for the missing one
	require.Error(t, err)
	require.ErrorContains(t, err, "context(s) not found: missing")
	assert.Contains(t, output(), "Removed context 'staging'")

	// staging should still be removed despite the error
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.NotContains(t, cfg.Contexts, "staging")
}

func TestRm_NoContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	encPath, err := config.EncPath()
	require.NoError(t, err)
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = ""
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	ef, err := crypto.Encrypt(data, "test-password")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(ef), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err = executeCommand("rm", "nonexistent")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestRm_SharedClusterAuthInfoPreserved(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts:\n  ctx1:\n    namespace: default\n  ctx2:\n    namespace: default\n")

	encPath, err := config.EncPath()
	require.NoError(t, err)
	kc := clientcmdapi.NewConfig()
	kc.Contexts["ctx1"] = &clientcmdapi.Context{Cluster: "shared-cluster", AuthInfo: "shared-user"}
	kc.Contexts["ctx2"] = &clientcmdapi.Context{Cluster: "shared-cluster", AuthInfo: "shared-user"}
	kc.Clusters["shared-cluster"] = &clientcmdapi.Cluster{Server: "https://shared.example.com"}
	kc.AuthInfos["shared-user"] = &clientcmdapi.AuthInfo{Token: "shared-token"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	ef, err := crypto.Encrypt(data, "test-password")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(ef), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err = executeCommand("rm", "ctx1")
	require.NoError(t, err)

	kubeconfig, err := loadDecryptedKubeconfig("test-password")
	require.NoError(t, err)
	// shared cluster and auth info survive because ctx2 still references them
	assert.Contains(t, kubeconfig.Clusters, "shared-cluster")
	assert.Contains(t, kubeconfig.AuthInfos, "shared-user")
}

func TestEject_NamedContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n  staging:\n    namespace: staging\n")
	writeTestConfigEnc(t)

	ejectForce = true
	passwordFlag = "test-password"
	t.Cleanup(func() {
		passwordFlag = ""
		ejectForce = false
	})

	output := captureOutput(t)

	err := executeCommand("eject", "staging", "--force")
	require.NoError(t, err)
	assert.Contains(t, output(), "Decrypted config written")

	// Verify only staging context was written
	kubeConfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	_, err = os.Stat(kubeConfig)
	require.NoError(t, err)

	written, err := clientcmd.LoadFromFile(kubeConfig)
	require.NoError(t, err)
	assert.Contains(t, written.Contexts, "staging")
	assert.NotContains(t, written.Contexts, "test-cluster")
	// current-context should be the ejected one
	assert.Equal(t, "staging", written.CurrentContext)
}

func TestEject_NamedContextNotFound(t *testing.T) {
	setupTestHome(t)
	writeTestConfigEnc(t)

	ejectForce = true
	passwordFlag = "test-password"
	t.Cleanup(func() {
		passwordFlag = ""
		ejectForce = false
	})

	err := executeCommand("eject", "nonexistent", "--force")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestEject_MultipleNamedContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n  staging:\n    namespace: staging\n")
	writeTestConfigEnc(t)

	ejectForce = true
	passwordFlag = "test-password"
	t.Cleanup(func() {
		passwordFlag = ""
		ejectForce = false
	})

	err := executeCommand("eject", "test-cluster", "staging", "--force")
	require.NoError(t, err)

	written, err := clientcmd.LoadFromFile(filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	require.NoError(t, err)
	assert.Contains(t, written.Contexts, "test-cluster")
	assert.Contains(t, written.Contexts, "staging")
	assert.Len(t, written.Contexts, 2)
}

func TestEject_MultipleWithOneMissing(t *testing.T) {
	setupTestHome(t)
	writeTestConfigEnc(t)

	ejectForce = true
	passwordFlag = "test-password"
	t.Cleanup(func() {
		passwordFlag = ""
		ejectForce = false
	})

	err := executeCommand("eject", "test-cluster", "missing", "--force")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestEject_WithNamesAndNoForceRequiresConfirmation(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	ejectForce = false
	passwordFlag = "test-password"
	t.Cleanup(func() {
		passwordFlag = ""
		ejectForce = false
	})

	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString("no\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	oldOpenTTY := openTTY
	openTTY = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return r, nil
	}
	t.Cleanup(func() { openTTY = oldOpenTTY })

	output := captureOutput(t)

	err = executeCommand("eject", "test-cluster")
	require.NoError(t, err)
	assert.Contains(t, output(), "Aborted")
}

func TestEject_ExistingConfigWithoutForceWarnsAndAborts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	require.NoError(t, os.MkdirAll(kubeDir, 0o700))
	kubeConfigPath := filepath.Join(kubeDir, "config")
	require.NoError(t, os.WriteFile(kubeConfigPath, []byte("existing"), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString("no\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	oldOpenTTY := openTTY
	openTTY = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return r, nil
	}
	t.Cleanup(func() { openTTY = oldOpenTTY })

	output := captureOutput(t)

	err = executeCommand("eject", "test-cluster")
	require.NoError(t, err)
	assert.Contains(t, output(), "Aborted")

	data, err := os.ReadFile(kubeConfigPath)
	require.NoError(t, err)
	assert.Equal(t, "existing", string(data))
}

func TestEject_MergeAddsContextToExistingConfig(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	require.NoError(t, os.MkdirAll(kubeDir, 0o700))
	kubeConfigPath := filepath.Join(kubeDir, "config")
	existing := clientcmdapi.NewConfig()
	existing.Contexts["existing"] = &clientcmdapi.Context{Cluster: "existing-cluster", AuthInfo: "existing-user"}
	existing.Clusters["existing-cluster"] = &clientcmdapi.Cluster{Server: "https://existing.example.com"}
	existing.AuthInfos["existing-user"] = &clientcmdapi.AuthInfo{Token: "existing-token"}
	data, err := clientcmd.Write(*existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(kubeConfigPath, data, 0o600))

	ejectForce = true
	ejectMerge = true
	passwordFlag = "test-password"
	t.Cleanup(func() {
		ejectForce = false
		ejectMerge = false
		passwordFlag = ""
	})

	err = executeCommand("eject", "test-cluster", "--merge", "--force")
	require.NoError(t, err)

	written, err := clientcmd.LoadFromFile(kubeConfigPath)
	require.NoError(t, err)
	assert.Contains(t, written.Contexts, "existing")
	assert.Contains(t, written.Contexts, "test-cluster")
	assert.Equal(t, "https://existing.example.com", written.Clusters["existing-cluster"].Server)
	assert.Equal(t, "https://example.com:6443", written.Clusters["test-cluster"].Server)
}

func TestEject_MergeConflictSkippedWithoutConfirmation(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	require.NoError(t, os.MkdirAll(kubeDir, 0o700))
	kubeConfigPath := filepath.Join(kubeDir, "config")
	existing := clientcmdapi.NewConfig()
	existing.Contexts["test-cluster"] = &clientcmdapi.Context{Cluster: "old-cluster", AuthInfo: "old-user"}
	existing.Clusters["old-cluster"] = &clientcmdapi.Cluster{Server: "https://old.example.com"}
	existing.AuthInfos["old-user"] = &clientcmdapi.AuthInfo{Token: "old-token"}
	data, err := clientcmd.Write(*existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(kubeConfigPath, data, 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	responses := []string{"yes\n", "no\n"}
	oldOpenTTY := openTTY
	openTTY = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		if len(responses) == 0 {
			return nil, io.EOF
		}
		r, w, err := os.Pipe()
		require.NoError(t, err)
		_, err = w.WriteString(responses[0])
		require.NoError(t, err)
		require.NoError(t, w.Close())
		responses = responses[1:]
		return r, nil
	}
	t.Cleanup(func() { openTTY = oldOpenTTY })

	output := captureOutput(t)

	err = executeCommand("eject", "test-cluster", "--merge")
	require.NoError(t, err)
	assert.Contains(t, output(), "Skipped context 'test-cluster'")

	written, err := clientcmd.LoadFromFile(kubeConfigPath)
	require.NoError(t, err)
	assert.Equal(t, "old-cluster", written.Contexts["test-cluster"].Cluster)
	assert.Equal(t, "https://old.example.com", written.Clusters["old-cluster"].Server)
}

func TestEject_MergeConflictForceReplacesContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t)

	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	require.NoError(t, os.MkdirAll(kubeDir, 0o700))
	kubeConfigPath := filepath.Join(kubeDir, "config")
	existing := clientcmdapi.NewConfig()
	existing.Contexts["test-cluster"] = &clientcmdapi.Context{Cluster: "old-cluster", AuthInfo: "old-user"}
	existing.Clusters["old-cluster"] = &clientcmdapi.Cluster{Server: "https://old.example.com"}
	existing.AuthInfos["old-user"] = &clientcmdapi.AuthInfo{Token: "old-token"}
	data, err := clientcmd.Write(*existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(kubeConfigPath, data, 0o600))

	ejectForce = true
	ejectMerge = true
	passwordFlag = "test-password"
	t.Cleanup(func() {
		ejectForce = false
		ejectMerge = false
		passwordFlag = ""
	})

	err = executeCommand("eject", "test-cluster", "--merge", "--force")
	require.NoError(t, err)

	written, err := clientcmd.LoadFromFile(kubeConfigPath)
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", written.Contexts["test-cluster"].Cluster)
	assert.Equal(t, "https://example.com:6443", written.Clusters["test-cluster"].Server)
}

func TestImport_NoSourceFile(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("import")
	require.Error(t, err)
	assert.ErrorContains(t, err, "does not exist")
}

func TestImport_Success(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	// Write a plaintext kubeconfig
	home := os.Getenv("HOME")
	kubeDir := filepath.Join(home, ".kube")
	require.NoError(t, os.MkdirAll(kubeDir, 0o700))
	srcPath := filepath.Join(kubeDir, "config")
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "imported-ctx"
	kc.Contexts["imported-ctx"] = &clientcmdapi.Context{Cluster: "imported-cluster", AuthInfo: "imported-user"}
	kc.Clusters["imported-cluster"] = &clientcmdapi.Cluster{Server: "https://imported.example.com"}
	kc.AuthInfos["imported-user"] = &clientcmdapi.AuthInfo{Token: "imported-token"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(srcPath, data, 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err = executeCommand("import")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "Imported context 'imported-ctx'")
	// Source file should still exist without --force
	_, err = os.Stat(srcPath)
	require.NoError(t, err, "source file should not be removed without --force")

	// Verify context was added
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "imported-ctx")

	// Verify auth info is preserved in encrypted config
	decrypted, err := loadDecryptedKubeconfig("test-password")
	require.NoError(t, err)
	assert.Equal(t, "imported-token", decrypted.AuthInfos["imported-ctx/imported-user"].Token)
}

func TestImport_WithForceRemovesSource(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	home := os.Getenv("HOME")
	kubeDir := filepath.Join(home, ".kube")
	require.NoError(t, os.MkdirAll(kubeDir, 0o700))
	srcPath := filepath.Join(kubeDir, "config")
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "imported-ctx"
	kc.Contexts["imported-ctx"] = &clientcmdapi.Context{Cluster: "imported-cluster", AuthInfo: "imported-user"}
	kc.Clusters["imported-cluster"] = &clientcmdapi.Cluster{Server: "https://imported.example.com"}
	kc.AuthInfos["imported-user"] = &clientcmdapi.AuthInfo{Token: "imported-token"}
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(srcPath, data, 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	output := captureOutput(t)

	err = executeCommand("import", "--force")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "Imported context 'imported-ctx'")
	assert.Contains(t, got, "Removed")

	// Source file should be removed
	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err), "source file should be removed with --force")
}

func TestFilterKubeconfig(t *testing.T) {
	kc := clientcmdapi.NewConfig()
	kc.Contexts["ctx1"] = &clientcmdapi.Context{Cluster: "cluster1", AuthInfo: "user1"}
	kc.Contexts["ctx2"] = &clientcmdapi.Context{Cluster: "cluster2", AuthInfo: "user2"}
	kc.Clusters["cluster1"] = &clientcmdapi.Cluster{Server: "https://one.example.com"}
	kc.Clusters["cluster2"] = &clientcmdapi.Cluster{Server: "https://two.example.com"}
	kc.AuthInfos["user1"] = &clientcmdapi.AuthInfo{Token: "token1"}
	kc.AuthInfos["user2"] = &clientcmdapi.AuthInfo{Token: "token2"}

	filtered, err := filterKubeconfig(kc, []string{"ctx1"})
	require.NoError(t, err)

	assert.Contains(t, filtered.Contexts, "ctx1")
	assert.NotContains(t, filtered.Contexts, "ctx2")
	assert.Contains(t, filtered.Clusters, "cluster1")
	assert.NotContains(t, filtered.Clusters, "cluster2")
	assert.Contains(t, filtered.AuthInfos, "user1")
	assert.NotContains(t, filtered.AuthInfos, "user2")
	assert.Equal(t, "ctx1", filtered.CurrentContext)
}

func TestFilterKubeconfig_MissingContext(t *testing.T) {
	kc := clientcmdapi.NewConfig()
	kc.Contexts["ctx1"] = &clientcmdapi.Context{Cluster: "cluster1", AuthInfo: "user1"}
	kc.Clusters["cluster1"] = &clientcmdapi.Cluster{Server: "https://one.example.com"}
	kc.AuthInfos["user1"] = &clientcmdapi.AuthInfo{Token: "token1"}

	_, err := filterKubeconfig(kc, []string{"nonexistent"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestRootHelp(t *testing.T) {
	resetCommandTestState(t)
	output := captureOutput(t)

	err := executeCommand("--help")
	require.NoError(t, err)

	got := output()
	assert.Contains(t, got, "ekconf")
	assert.Contains(t, got, "add")
	assert.Contains(t, got, "rm")
	assert.Contains(t, got, "ls")
	assert.Contains(t, got, "view")
	assert.Contains(t, got, "use")
	assert.Contains(t, got, "ns")
	assert.Contains(t, got, "exec")
	assert.Contains(t, got, "eject")
	assert.Contains(t, got, "config")
	assert.Contains(t, got, "import")
}
