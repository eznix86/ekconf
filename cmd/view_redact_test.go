package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const secretMarker = "SHOULD-NOT-APPEAR"

func fullyPopulatedAuthInfo() *clientcmdapi.AuthInfo {
	return &clientcmdapi.AuthInfo{
		ClientCertificate:     secretMarker + "-cert-path",
		ClientCertificateData: []byte(secretMarker + "-cert-data"),
		ClientKey:             secretMarker + "-key-path",
		ClientKeyData:         []byte(secretMarker + "-key-data"),
		Token:                 secretMarker + "-token",
		TokenFile:             secretMarker + "-token-file",
		Impersonate:           secretMarker + "-impersonate",
		ImpersonateUID:        secretMarker + "-uid",
		ImpersonateGroups:     []string{secretMarker + "-group"},
		ImpersonateUserExtra:  map[string][]string{"k": {secretMarker + "-extra"}},
		Username:              "admin",
		Password:              secretMarker + "-password",
		AuthProvider:          &clientcmdapi.AuthProviderConfig{Name: secretMarker + "-provider"},
		Exec:                  &clientcmdapi.ExecConfig{Command: secretMarker + "-exec"},
		Extensions: map[string]runtime.Object{
			"cache": &runtime.Unknown{Raw: []byte(secretMarker + "-extension")},
		},
	}
}

// Every field the upstream struct carries is populated with a marker. Anything
// the allowlist does not name must not survive redaction, so a field added
// upstream is hidden by default instead of leaking until someone notices.
func TestRedactAuthInfo_LeaksNothingOutsideTheAllowlist(t *testing.T) {
	redacted := redactAuthInfo(fullyPopulatedAuthInfo())

	require.NotNil(t, redacted)
	assert.Equal(t, "admin", redacted.Username, "username is deliberately kept")

	rendered := renderAuthInfo(t, redacted)
	assert.NotContains(t, rendered, secretMarker)
}

func TestRedactCluster_RedactsProxyCredentials(t *testing.T) {
	tests := map[string]struct {
		proxyURL string
		want     string
	}{
		"credentials stripped": {"socks5://svc:S3cr3t@bastion:1080", "socks5://svc@bastion:1080"},
		"no credentials kept":  {"http://proxy.internal:3128", "http://proxy.internal:3128"},
		"unparseable dropped":  {"://not a url", ""},
		"empty stays empty":    {"", ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			redacted := redactCluster(&clientcmdapi.Cluster{
				Server:   "https://example.com:6443",
				ProxyURL: tc.proxyURL,
			})
			require.NotNil(t, redacted)
			assert.Equal(t, tc.want, redacted.ProxyURL)
			assert.NotContains(t, redacted.ProxyURL, "S3cr3t")
		})
	}
}

func TestRedactCluster_LeaksNothingOutsideTheAllowlist(t *testing.T) {
	redacted := redactCluster(&clientcmdapi.Cluster{
		Server:                   "https://example.com:6443",
		TLSServerName:            "example.com",
		InsecureSkipTLSVerify:    true,
		DisableCompression:       true,
		CertificateAuthority:     secretMarker + "-ca-path",
		CertificateAuthorityData: []byte(secretMarker + "-ca-data"),
		ProxyURL:                 "socks5://svc:" + secretMarker + "@bastion:1080",
	})

	require.NotNil(t, redacted)
	assert.Equal(t, "https://example.com:6443", redacted.Server)
	assert.Equal(t, "example.com", redacted.TLSServerName)
	assert.True(t, redacted.InsecureSkipTLSVerify)
	assert.True(t, redacted.DisableCompression)

	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.Clusters["c"] = redacted
	out, err := clientcmd.Write(*kubeconfig)
	require.NoError(t, err)
	assert.NotContains(t, string(out), secretMarker)
}

// A field added to the upstream struct must not silently start flowing through
// the allowlist. This fails when client-go grows a field, which is the moment
// to decide whether it is safe to show.
func TestRedactionAllowlist_MatchesTheUpstreamStructs(t *testing.T) {
	assert.Equal(t, 16, reflect.TypeFor[clientcmdapi.AuthInfo]().NumField(),
		"clientcmdapi.AuthInfo changed, review redactAuthInfo before updating this count")
	assert.Equal(t, 9, reflect.TypeFor[clientcmdapi.Cluster]().NumField(),
		"clientcmdapi.Cluster changed, review redactCluster before updating this count")
}

func renderAuthInfo(t *testing.T, authInfo *clientcmdapi.AuthInfo) string {
	t.Helper()

	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.AuthInfos["u"] = authInfo
	out, err := clientcmd.Write(*kubeconfig)
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
