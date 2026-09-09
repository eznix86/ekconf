package cmd

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func testCertPEM(t *testing.T, notAfter time.Time) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    notAfter.Add(-24 * time.Hour),
		NotAfter:     notAfter,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func kubeconfigWithCert(t *testing.T, contextName string, notAfter time.Time) *clientcmdapi.Config {
	t.Helper()

	kc := clientcmdapi.NewConfig()
	kc.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:  contextName + "/cluster",
		AuthInfo: contextName + "/user",
	}
	kc.Clusters[contextName+"/cluster"] = &clientcmdapi.Cluster{Server: "https://example.com:6443"}
	kc.AuthInfos[contextName+"/user"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: testCertPEM(t, notAfter),
	}
	return kc
}

func captureStderr(t *testing.T) func() string {
	t.Helper()

	var buf bytes.Buffer
	rootCmd.SetErr(&buf)
	return buf.String
}

func TestRecordContextExpiry_RecordsCertificateNotAfter(t *testing.T) {
	notAfter := time.Now().Add(72 * time.Hour).Truncate(time.Second).UTC()
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{"prod": {Namespace: "production"}}}

	changed := recordContextExpiry(cfg, kubeconfigWithCert(t, "prod", notAfter))

	assert.True(t, changed)
	require.NotNil(t, cfg.Contexts["prod"].Expires)
	assert.True(t, cfg.Contexts["prod"].Expires.Equal(notAfter))
	assert.Equal(t, "production", cfg.Contexts["prod"].Namespace)
}

func TestRecordContextExpiry_UnchangedWhenAlreadyRecorded(t *testing.T) {
	notAfter := time.Now().Add(72 * time.Hour).Truncate(time.Second).UTC()
	kubeconfig := kubeconfigWithCert(t, "prod", notAfter)
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{"prod": {}}}

	require.True(t, recordContextExpiry(cfg, kubeconfig))
	assert.False(t, recordContextExpiry(cfg, kubeconfig))
}

func TestRecordContextExpiry_ClearsExpiryWhenCertificateIsGone(t *testing.T) {
	stale := time.Now().Add(-1 * time.Hour)
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{"prod": {Expires: &stale}}}

	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.Contexts["prod"] = &clientcmdapi.Context{Cluster: "c", AuthInfo: "u"}
	kubeconfig.AuthInfos["u"] = &clientcmdapi.AuthInfo{Token: "abc"}

	changed := recordContextExpiry(cfg, kubeconfig)

	assert.True(t, changed)
	assert.Nil(t, cfg.Contexts["prod"].Expires)
}

func TestRecordContextExpiry_IgnoresContextsMissingFromKubeconfig(t *testing.T) {
	cfg := &config.Config{Contexts: map[string]config.ContextEntry{"ghost": {Namespace: "default"}}}

	changed := recordContextExpiry(cfg, clientcmdapi.NewConfig())

	assert.False(t, changed)
	assert.Nil(t, cfg.Contexts["ghost"].Expires)
}

func TestUse_WarnsWhenCertificateExpired(t *testing.T) {
	setupTestHome(t)
	expired := time.Now().Add(-36 * time.Hour).Truncate(time.Second).UTC()
	writeTestConfigYAML(t, fmt.Sprintf(
		"keychain: false\ncurrent: staging\ncontexts:\n  prod:\n    namespace: production\n    expires: %q\n",
		expired.Format(time.RFC3339),
	))

	stdout := captureOutput(t)
	stderr := captureStderr(t)

	require.NoError(t, executeCommand("use", "prod"))

	assert.Contains(t, stdout(), "Switched to context 'prod'")
	warning := stderr()
	assert.Contains(t, warning, "client certificate for 'prod' expired on")
	assert.Contains(t, warning, expired.Local().Format(expiryTimeFormat))
}

func TestUse_NoWarningWhenCertificateIsValid(t *testing.T) {
	setupTestHome(t)
	valid := time.Now().Add(36 * time.Hour).Truncate(time.Second).UTC()
	writeTestConfigYAML(t, fmt.Sprintf(
		"keychain: false\ncurrent: staging\ncontexts:\n  prod:\n    namespace: production\n    expires: %q\n",
		valid.Format(time.RFC3339),
	))

	stdout := captureOutput(t)
	stderr := captureStderr(t)

	require.NoError(t, executeCommand("use", "prod"))

	assert.Contains(t, stdout(), "Switched to context 'prod'")
	assert.Empty(t, stderr())
}

func TestUse_NoWarningWhenExpiryIsUnknown(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: staging\ncontexts:\n  prod:\n    namespace: production\n")

	stdout := captureOutput(t)
	stderr := captureStderr(t)

	require.NoError(t, executeCommand("use", "prod"))

	assert.Contains(t, stdout(), "Switched to context 'prod'")
	assert.Empty(t, stderr())
}

func TestAdd_RecordsExpiryThenUseWarns(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: \"\"\ncontexts: {}\n")

	expired := time.Now().Add(-90 * time.Hour).Truncate(time.Second).UTC()
	kc := clientcmdapi.NewConfig()
	kc.CurrentContext = "prod"
	kc.Contexts["prod"] = &clientcmdapi.Context{Cluster: "prod-cluster", AuthInfo: "prod-user"}
	kc.Clusters["prod-cluster"] = &clientcmdapi.Cluster{Server: "https://prod.example.com"}
	kc.AuthInfos["prod-user"] = &clientcmdapi.AuthInfo{ClientCertificateData: testCertPEM(t, expired)}

	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "prod.yaml")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("add", path))

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg.Contexts["prod"].Expires)
	assert.True(t, cfg.Contexts["prod"].Expires.Equal(expired))

	stderr := captureStderr(t)
	require.NoError(t, executeCommand("use", "prod"))
	assert.Contains(t, stderr(), "client certificate for 'prod' expired on")
}

func TestExec_BackfillsExpiryForExistingContext(t *testing.T) {
	setupTestHome(t)
	writeTestConfigYAML(t, "keychain: false\ncurrent: prod\ncontexts:\n  prod:\n    namespace: production\n")

	expired := time.Now().Add(-5 * time.Hour).Truncate(time.Second).UTC()
	kc := kubeconfigWithCert(t, "prod", expired)
	data, err := clientcmd.Write(*kc)
	require.NoError(t, err)

	encrypted, err := crypto.Encrypt(data, []byte("test-password"))
	require.NoError(t, err)
	encPath := filepath.Join(os.Getenv("HOME"), ".ekube", "config.enc")
	require.NoError(t, os.MkdirAll(filepath.Dir(encPath), 0o700))
	require.NoError(t, os.WriteFile(encPath, crypto.Marshal(encrypted), 0o600))

	passwordFlag = "test-password"
	t.Cleanup(func() { passwordFlag = "" })

	require.NoError(t, executeCommand("exec", "--no-shell", "--", "true"))

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg.Contexts["prod"].Expires)
	assert.True(t, cfg.Contexts["prod"].Expires.Equal(expired))
	assert.Equal(t, "production", cfg.Contexts["prod"].Namespace)
}

func TestLS_ShowsExpiryColumn(t *testing.T) {
	setupTestHome(t)
	expired := time.Now().Add(-48 * time.Hour)
	valid := time.Now().Add(29*24*time.Hour + 12*time.Hour)
	writeTestConfigYAML(t, fmt.Sprintf(
		"keychain: false\ncurrent: prod\ncontexts:\n"+
			"  prod:\n    namespace: production\n    expires: %q\n"+
			"  staging:\n    namespace: staging\n    expires: %q\n"+
			"  token-only:\n    namespace: default\n",
		expired.Format(time.RFC3339), valid.Format(time.RFC3339),
	))

	output := captureOutput(t)
	require.NoError(t, executeCommand("ls"))

	lines := strings.Split(strings.TrimRight(output(), "\n"), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, "  NAME        NAMESPACE   EXPIRES", lines[0])
	assert.Equal(t, "* prod        production  "+expired.Local().Format(expiryDateFormat)+" (expired)", lines[1])
	assert.Equal(t, "  staging     staging     "+valid.Local().Format(expiryDateFormat)+" (in 29d)", lines[2])
	assert.Equal(t, "  token-only", lines[3])
}

func TestRenderContextTable_DropsEmptyColumns(t *testing.T) {
	tests := map[string]struct {
		rows []contextRow
		want []string
	}{
		"names only": {
			rows: []contextRow{{mark: "*", name: "prod"}, {mark: " ", name: "dev"}},
			want: []string{"  NAME", "* prod", "  dev"},
		},
		"no namespaces": {
			rows: []contextRow{{mark: " ", name: "prod", expiry: "2027-01-01 (in 1y)"}},
			want: []string{"  NAME  EXPIRES", "  prod  2027-01-01 (in 1y)"},
		},
		"no expiries": {
			rows: []contextRow{{mark: " ", name: "prod", namespace: "production"}},
			want: []string{"  NAME  NAMESPACE", "  prod  production"},
		},
		"gaps within a column": {
			rows: []contextRow{
				{mark: "*", name: "prod", namespace: "production", expiry: "2025-03-01 (expired)", expired: true},
				{mark: " ", name: "token-only"},
			},
			want: []string{
				"  NAME        NAMESPACE   EXPIRES",
				"* prod        production  2025-03-01 (expired)",
				"  token-only",
			},
		},
		"name longer than header": {
			rows: []contextRow{{mark: " ", name: "a-very-long-context-name", namespace: "ops"}},
			want: []string{"  NAME                      NAMESPACE", "  a-very-long-context-name  ops"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, renderContextTable(tc.rows))
		})
	}
}

func TestExpiryCell(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		expires     *time.Time
		want        string
		wantExpired bool
	}{
		"unknown":     {nil, "", false},
		"expired":     {new(now.Add(-1 * time.Hour)), "2026-01-01 (expired)", true},
		"exactly now": {new(now), "2026-01-01 (expired)", true},
		"minutes":     {new(now.Add(42 * time.Minute)), "2026-01-01 (in 42m)", false},
		"hours":       {new(now.Add(5 * time.Hour)), "2026-01-01 (in 5h)", false},
		"days":        {new(now.Add(9 * 24 * time.Hour)), "2026-01-10 (in 9d)", false},
		"months":      {new(now.Add(90 * 24 * time.Hour)), "2026-04-01 (in 3mo)", false},
		"years":       {new(now.Add(800 * 24 * time.Hour)), "2028-03-11 (in 2y)", false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cell, expired := expiryCell(config.ContextEntry{Expires: tc.expires}, now)
			assert.Equal(t, tc.want, cell)
			assert.Equal(t, tc.wantExpired, expired)
		})
	}
}
