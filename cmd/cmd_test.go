package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func writeTestConfigYAML(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(os.Getenv("HOME"), ".ekube", "config.yaml")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte(content), 0600)
}

func writeTestConfigEnc(t *testing.T, password string) {
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
		CertificateAuthorityData:  []byte("test-ca-data"),
		CertificateAuthority:      "/tmp/test-ca.crt",
	}
	kc.Clusters["staging-cluster"] = &clientcmdapi.Cluster{
		Server:                   "https://staging.example.com:6443",
		CertificateAuthorityData:  []byte("staging-ca-data"),
		CertificateAuthority:      "/tmp/staging-ca.crt",
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

	ef, err := crypto.Encrypt(data, password)
	require.NoError(t, err)

	encPath := filepath.Join(os.Getenv("HOME"), ".ekube", "config.enc")
	os.MkdirAll(filepath.Dir(encPath), 0700)
	os.WriteFile(encPath, crypto.Marshal(ef), 0600)
}

func executeCommand(args ...string) error {
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func captureOutput(t *testing.T) (*os.File, func()) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	old := os.Stdout
	os.Stdout = w

	return r, func() {
		w.Close()
		os.Stdout = old
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
	os.WriteFile(path, []byte("sensitive-data-here"), 0600)

	wipeFile(path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, b := range data {
		assert.Equal(t, byte(0), b, "file should be zeroed after wipe")
	}
}

func TestWipeFile_Nonexistent(t *testing.T) {
	assert.NotPanics(t, func() {
		wipeFile("/nonexistent/path")
	})
}

func TestWipeFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte{}, 0600)

	assert.NotPanics(t, func() {
		wipeFile(path)
	})
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

	pw, err := resolvePasswordWithKeychain(false)
	require.NoError(t, err)
	assert.Equal(t, "test-pass", pw)
}

func TestLS_NoContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("ls")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Contains(t, string(buf[:n]), "No contexts found")
}

func TestLS_WithContexts(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: prod\ncontexts:\n  prod:\n    namespace: production\n  staging:\n    namespace: staging\n")

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("ls")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	assert.Contains(t, output, "prod")
	assert.Contains(t, output, "staging")
	assert.Contains(t, output, "* prod")
}

func TestUse_Success(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: staging\ncontexts:\n  staging:\n    namespace: staging\n  prod:\n    namespace: production\n")

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("use", "prod")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Contains(t, string(buf[:n]), "Switched to context 'prod'")
}

func TestUse_NotFound(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	err := executeCommand("use", "nonexistent")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestNS_Success(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: prod\ncontexts:\n  prod:\n    namespace: production\n")

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("ns", "staging")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Contains(t, string(buf[:n]), "Set namespace 'staging' on context 'prod'")
}

func TestNS_NoActiveContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	err := executeCommand("ns", "default")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "no active context set")
}

func TestConfig_List(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: prod\ncontexts:\n  prod:\n    namespace: production\n")

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("config", "list")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	assert.Contains(t, output, "keychain: false")
	assert.Contains(t, output, "current: prod")
}

func TestConfig_SetKeychain(t *testing.T) {
	setupTestHome(t)

	err := executeCommand("config", "keychain=true")
	require.NoError(t, err)

	r, cleanup := captureOutput(t)
	defer cleanup()

	err = executeCommand("config", "list")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Contains(t, string(buf[:n]), "keychain: true")
}

func TestConfig_InvalidKey(t *testing.T) {
	setupTestHome(t)
	err := executeCommand("config", "invalid=value")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unknown config key")
}

func TestConfig_InvalidFormat(t *testing.T) {
	setupTestHome(t)
	err := executeCommand("config", "badformat")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "expected key=value")
}

func TestConfig_InvalidKeychainValue(t *testing.T) {
	setupTestHome(t)
	err := executeCommand("config", "keychain=invalid")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "keychain must be 'true' or 'false'")
}

func TestView_WithPassword(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t, "test-password")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("view", "test-cluster")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	assert.Contains(t, output, "current-context: test-cluster")
	assert.Contains(t, output, "https://example.com:6443")
	assert.NotContains(t, output, "test-token")
	assert.NotContains(t, output, "certificate-authority-data:")
	assert.NotContains(t, output, "staging.example.com")
	assert.NotContains(t, output, "staging-token")
	assert.NotContains(t, output, "/tmp/staging-ca.crt")
}

func TestView_PlainIncludesSensitiveData(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t, "test-password")

	passwordFlag = "test-password"
	viewPlain = true
	t.Cleanup(func() {
		passwordFlag = ""
		viewPlain = false
	})

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("view", "test-cluster", "--plain")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	assert.Contains(t, output, "current-context: test-cluster")
	assert.Contains(t, output, "test-token")
	assert.Contains(t, output, "certificate-authority-data:")
	assert.NotContains(t, output, "staging.example.com")
	assert.NotContains(t, output, "staging-token")
	assert.NotContains(t, output, "/tmp/staging-ca.crt")
}

func TestView_NonexistentContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t, "test-password")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("view", "missing-cluster")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

func TestEject_Force(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t, "test-password")

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
	assert.Error(t, err)
	assert.ErrorContains(t, err, "read config.enc")
}

func TestExec_NoCommand(t *testing.T) {
	setupTestHome(t)

	err := executeCommand("exec")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "no command specified")
}

func TestExec_WithPasswordFlag(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n")
	writeTestConfigEnc(t, "test-password")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("exec", "echo", "hello-from-ekconf")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Contains(t, string(buf[:n]), "hello-from-ekconf")
}

func TestExec_SpecificContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: test-cluster\ncontexts:\n  test-cluster:\n    namespace: default\n  staging:\n    namespace: staging\n")
	writeTestConfigEnc(t, "test-password")

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("exec", "staging", "--", "echo", "staging-context")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	assert.Contains(t, string(buf[:n]), "staging-context")
}

func TestExec_NoActiveContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	passwordFlag = "test-pass"
	t.Cleanup(func() { passwordFlag = "" })

	err := executeCommand("exec", "echo", "hi")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "no active context set")
}

func TestRootHelp(t *testing.T) {
	r, cleanup := captureOutput(t)
	defer cleanup()

	err := executeCommand("--help")
	require.NoError(t, err)

	cleanup()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	assert.Contains(t, string(buf[:n]), "ekconf")
	assert.Contains(t, string(buf[:n]), "add")
	assert.Contains(t, string(buf[:n]), "rm")
	assert.Contains(t, string(buf[:n]), "ls")
	assert.Contains(t, string(buf[:n]), "view")
	assert.Contains(t, string(buf[:n]), "use")
	assert.Contains(t, string(buf[:n]), "ns")
	assert.Contains(t, string(buf[:n]), "exec")
	assert.Contains(t, string(buf[:n]), "eject")
	assert.Contains(t, string(buf[:n]), "config")
}
