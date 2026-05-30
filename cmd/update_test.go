package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testReleaseSigningPrivateKeyPEM = `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIO94vupFWboXEe+uEztjVQ1s1i14qa7EiFuzxxKUvhOp
-----END PRIVATE KEY-----`

func TestNormalizePlatform(t *testing.T) {
	platform, err := normalizePlatform("darwin", "arm64")
	require.NoError(t, err)
	assert.Equal(t, updatePlatform{goos: "darwin", goarch: "arm64"}, platform)

	_, err = normalizePlatform("windows", "amd64")
	assert.ErrorContains(t, err, "unsupported operating system")
}

func TestUpdateAssetName(t *testing.T) {
	platform := updatePlatform{goos: "linux", goarch: "amd64"}
	assert.Equal(t, "ekconf_1.2.3_linux_amd64.tar.gz", updateAssetName("1.2.3", platform))
}

func TestIsNewerVersion(t *testing.T) {
	assert.True(t, isNewerVersion("1.2.4", "1.2.3"))
	assert.False(t, isNewerVersion("1.2.3", "1.2.3"))
	assert.False(t, isNewerVersion("1.2.2", "1.2.3"))
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	archive := tarGzWithBinary(t, "ekconf", []byte("new-binary"))

	binary, err := extractBinaryFromTarGz(archive)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-binary"), binary)
}

func TestUpdate_Success(t *testing.T) {
	setupTestHome(t)

	exePath := filepath.Join(t.TempDir(), "ekconf")
	require.NoError(t, os.WriteFile(exePath, []byte("old-binary"), 0o755))
	platform, err := normalizePlatform(runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err)
	assetName := updateAssetName("1.2.3", platform)
	archive := tarGzWithBinary(t, "ekconf", []byte("new-binary"))
	checksum := checksumLine(assetName, archive)
	signature := signChecksums(t, []byte(checksum))

	release := githubRelease{
		TagName: "v1.2.3",
		Assets: []githubAsset{{
			Name:               assetName,
			BrowserDownloadURL: "",
		}, {
			Name:               "checksums.txt",
			BrowserDownloadURL: "",
		}, {
			Name:               "checksums.txt.sig",
			BrowserDownloadURL: "",
		}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/eznix86/ekconf/releases/latest":
			release.Assets[0].BrowserDownloadURL = serverAssetURL(t, r, "/download/"+assetName)
			release.Assets[1].BrowserDownloadURL = serverAssetURL(t, r, "/download/checksums.txt")
			release.Assets[2].BrowserDownloadURL = serverAssetURL(t, r, "/download/checksums.txt.sig")
			_ = json.NewEncoder(w).Encode(release)
		case "/download/" + assetName:
			_, _ = w.Write(archive)
		case "/download/checksums.txt":
			_, _ = w.Write([]byte(checksum))
		case "/download/checksums.txt.sig":
			_, _ = w.Write(signature)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldAPIBaseURL := updateAPIBaseURL
	oldHTTPClient := updateHTTPClient
	oldExecutable := currentExecutable
	t.Cleanup(func() {
		updateAPIBaseURL = oldAPIBaseURL
		updateHTTPClient = oldHTTPClient
		currentExecutable = oldExecutable
	})

	updateAPIBaseURL = server.URL
	updateHTTPClient = server.Client()
	currentExecutable = func() (string, error) { return exePath, nil }

	output := captureOutput(t)

	err = executeCommand("update")
	require.NoError(t, err)

	assert.Contains(t, output(), "Updated ekconf to v1.2.3")

	data, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-binary"), data)
}

func TestUpdate_RejectsChecksumMismatch(t *testing.T) {
	setupTestHome(t)

	platform, err := normalizePlatform(runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err)
	assetName := updateAssetName("1.2.3", platform)
	archive := tarGzWithBinary(t, "ekconf", []byte("new-binary"))
	checksum := "deadbeef  " + assetName + "\n"
	signature := signChecksums(t, []byte(checksum))
	release := githubRelease{
		TagName: "v1.2.3",
		Assets:  []githubAsset{{Name: assetName, BrowserDownloadURL: ""}, {Name: "checksums.txt", BrowserDownloadURL: ""}, {Name: "checksums.txt.sig", BrowserDownloadURL: ""}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/eznix86/ekconf/releases/latest":
			release.Assets[0].BrowserDownloadURL = serverAssetURL(t, r, "/download/"+assetName)
			release.Assets[1].BrowserDownloadURL = serverAssetURL(t, r, "/download/checksums.txt")
			release.Assets[2].BrowserDownloadURL = serverAssetURL(t, r, "/download/checksums.txt.sig")
			_ = json.NewEncoder(w).Encode(release)
		case "/download/" + assetName:
			_, _ = w.Write(archive)
		case "/download/checksums.txt":
			_, _ = w.Write([]byte(checksum))
		case "/download/checksums.txt.sig":
			_, _ = w.Write(signature)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldAPIBaseURL := updateAPIBaseURL
	oldHTTPClient := updateHTTPClient
	oldExecutable := currentExecutable
	exePath := filepath.Join(t.TempDir(), "ekconf")
	require.NoError(t, os.WriteFile(exePath, []byte("old-binary"), 0o755))
	t.Cleanup(func() {
		updateAPIBaseURL = oldAPIBaseURL
		updateHTTPClient = oldHTTPClient
		currentExecutable = oldExecutable
	})

	updateAPIBaseURL = server.URL
	updateHTTPClient = server.Client()
	currentExecutable = func() (string, error) { return exePath, nil }

	err = executeCommand("update")
	assert.ErrorContains(t, err, "checksum mismatch")
}

func TestUpdateCheckOnly_ShowsNotice(t *testing.T) {
	setupTestHome(t)

	oldVersion := version
	oldAPIBaseURL := updateAPIBaseURL
	oldHTTPClient := updateHTTPClient
	t.Cleanup(func() {
		version = oldVersion
		updateAPIBaseURL = oldAPIBaseURL
		updateHTTPClient = oldHTTPClient
	})

	version = "1.2.3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/eznix86/ekconf/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v1.2.4",
				Assets:  []githubAsset{{Name: "ekconf_1.2.4_linux_amd64.tar.gz", BrowserDownloadURL: "http://example.com/asset"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	updateAPIBaseURL = server.URL
	updateHTTPClient = server.Client()

	errBuf := &bytes.Buffer{}
	oldErr := rootCmd.ErrOrStderr()
	rootCmd.SetErr(errBuf)
	t.Cleanup(func() { rootCmd.SetErr(oldErr) })

	rootCmd.SetArgs([]string{"update", "--check"})
	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, errBuf.String(), "\x1b[33mekconf v1.2.4 is available. Run 'ekconf update' to install it.\x1b[0m")
}

func TestRootShowsUpdateNotice(t *testing.T) {
	setupTestHome(t)

	oldVersion := version
	oldAPIBaseURL := updateAPIBaseURL
	oldHTTPClient := updateHTTPClient
	t.Cleanup(func() {
		version = oldVersion
		updateAPIBaseURL = oldAPIBaseURL
		updateHTTPClient = oldHTTPClient
	})

	version = "1.2.3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/eznix86/ekconf/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v1.2.4",
				Assets:  []githubAsset{{Name: "ekconf_1.2.4_linux_amd64.tar.gz", BrowserDownloadURL: "http://example.com/asset"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	updateAPIBaseURL = server.URL
	updateHTTPClient = server.Client()

	buf := &bytes.Buffer{}
	printed := MaybePrintUpdateNotice(context.Background(), buf)
	require.True(t, printed)
	assert.Contains(t, buf.String(), "\x1b[33mekconf v1.2.4 is available. Run 'ekconf update' to install it.\x1b[0m")
}

func TestReplaceCurrentExecutable_NotWritable(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "ekconf")
	require.NoError(t, os.WriteFile(exePath, []byte("old-binary"), 0o755))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})

	oldExecutable := currentExecutable
	t.Cleanup(func() {
		currentExecutable = oldExecutable
	})
	currentExecutable = func() (string, error) { return exePath, nil }

	err := replaceCurrentExecutable([]byte("new-binary"))
	assert.ErrorContains(t, err, "current executable is not writable")
}

func tarGzWithBinary(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(data)),
	}))
	_, err := tw.Write(data)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}

func checksumLine(assetName string, data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + "  " + assetName + "\n"
}

func signChecksums(t *testing.T, message []byte) []byte {
	t.Helper()

	block, _ := pem.Decode([]byte(testReleaseSigningPrivateKeyPEM))
	require.NotNil(t, block)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	require.NoError(t, err)
	priv, ok := key.(ed25519.PrivateKey)
	require.True(t, ok)

	return ed25519.Sign(priv, message)
}

func serverAssetURL(t *testing.T, r *http.Request, path string) string {
	t.Helper()
	return "http://" + r.Host + path
}
