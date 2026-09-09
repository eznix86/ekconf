package certinfo

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func certPEM(t *testing.T, notAfter time.Time) []byte {
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

	return pem.EncodeToMemory(&pem.Block{Type: certificateBlockType, Bytes: der})
}

func TestExpiry_FromCertificateData(t *testing.T) {
	notAfter := time.Now().Add(48 * time.Hour).Truncate(time.Second).UTC()

	expiry, ok := Expiry(&clientcmdapi.AuthInfo{ClientCertificateData: certPEM(t, notAfter)})

	require.True(t, ok)
	assert.True(t, expiry.Equal(notAfter), "got %s, want %s", expiry, notAfter)
}

func TestExpiry_FromCertificateFile(t *testing.T) {
	notAfter := time.Now().Add(-1 * time.Hour).Truncate(time.Second).UTC()
	path := filepath.Join(t.TempDir(), "client.crt")
	require.NoError(t, os.WriteFile(path, certPEM(t, notAfter), 0o600))

	expiry, ok := Expiry(&clientcmdapi.AuthInfo{ClientCertificate: path})

	require.True(t, ok)
	assert.True(t, expiry.Equal(notAfter), "got %s, want %s", expiry, notAfter)
}

func TestExpiry_CertificateDataWinsOverFile(t *testing.T) {
	inline := time.Now().Add(10 * time.Hour).Truncate(time.Second).UTC()
	onDisk := time.Now().Add(500 * time.Hour).Truncate(time.Second).UTC()
	path := filepath.Join(t.TempDir(), "client.crt")
	require.NoError(t, os.WriteFile(path, certPEM(t, onDisk), 0o600))

	expiry, ok := Expiry(&clientcmdapi.AuthInfo{
		ClientCertificateData: certPEM(t, inline),
		ClientCertificate:     path,
	})

	require.True(t, ok)
	assert.True(t, expiry.Equal(inline), "got %s, want %s", expiry, inline)
}

func TestExpiry_ChainUsesEarliestNotAfter(t *testing.T) {
	leaf := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	issuer := time.Now().Add(9000 * time.Hour).Truncate(time.Second).UTC()
	chain := append(certPEM(t, issuer), certPEM(t, leaf)...)

	expiry, ok := Expiry(&clientcmdapi.AuthInfo{ClientCertificateData: chain})

	require.True(t, ok)
	assert.True(t, expiry.Equal(leaf), "got %s, want %s", expiry, leaf)
}

func TestExpiry_NoCertificate(t *testing.T) {
	for name, authInfo := range map[string]*clientcmdapi.AuthInfo{
		"nil":            nil,
		"empty":          {},
		"token only":     {Token: "abc"},
		"exec plugin":    {Exec: &clientcmdapi.ExecConfig{Command: "aws"}},
		"missing file":   {ClientCertificate: "/nonexistent/client.crt"},
		"garbage pem":    {ClientCertificateData: []byte("not a certificate")},
		"non cert block": {ClientCertificateData: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})},
		"broken der":     {ClientCertificateData: pem.EncodeToMemory(&pem.Block{Type: certificateBlockType, Bytes: []byte("x")})},
	} {
		t.Run(name, func(t *testing.T) {
			expiry, ok := Expiry(authInfo)
			assert.False(t, ok)
			assert.True(t, expiry.IsZero())
		})
	}
}
