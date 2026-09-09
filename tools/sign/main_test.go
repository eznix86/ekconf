package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKeyPEM(t *testing.T) (string, ed25519.PublicKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)

	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), pub
}

func TestRun_SignatureVerifies(t *testing.T) {
	keyPEM, pub := testKeyPEM(t)
	t.Setenv(keyEnvVar, keyPEM)

	dir := t.TempDir()
	artifact := filepath.Join(dir, "checksums.txt")
	sigPath := filepath.Join(dir, "checksums.txt.sig")
	contents := []byte("abc123  ekconf_1.2.3_linux_amd64.tar.gz\n")
	require.NoError(t, os.WriteFile(artifact, contents, 0o600))

	require.NoError(t, run([]string{artifact, sigPath}))

	signature, err := os.ReadFile(sigPath)
	require.NoError(t, err)
	assert.Len(t, signature, ed25519.SignatureSize)
	assert.True(t, ed25519.Verify(pub, contents, signature))
	assert.False(t, ed25519.Verify(pub, append(contents, 'x'), signature))
}

func TestRun_Errors(t *testing.T) {
	keyPEM, _ := testKeyPEM(t)

	t.Run("wrong argument count", func(t *testing.T) {
		t.Setenv(keyEnvVar, keyPEM)
		assert.ErrorContains(t, run([]string{"only-one"}), "usage")
	})

	t.Run("missing key", func(t *testing.T) {
		t.Setenv(keyEnvVar, "")
		assert.ErrorContains(t, run([]string{"a", "b"}), "is not set")
	})

	t.Run("key is not PEM", func(t *testing.T) {
		t.Setenv(keyEnvVar, "not a pem block")
		assert.ErrorContains(t, run([]string{"a", "b"}), "is not a PEM block")
	})

	t.Run("key is not ed25519", func(t *testing.T) {
		t.Setenv(keyEnvVar, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("nope")})))
		assert.ErrorContains(t, run([]string{"a", "b"}), "parse signing key")
	})
}
