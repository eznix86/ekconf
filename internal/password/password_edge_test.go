package password

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentUser_FromEnv(t *testing.T) {
	os.Setenv("USER", "test-user")
	os.Unsetenv("USERNAME")
	t.Cleanup(func() { os.Unsetenv("USER") })

	u := currentUser()
	assert.Equal(t, "test-user", u)
}

func TestCurrentUser_FromUsernameEnv(t *testing.T) {
	os.Unsetenv("USER")
	os.Setenv("USERNAME", "windows-user")
	t.Cleanup(func() { os.Unsetenv("USERNAME") })

	u := currentUser()
	assert.Equal(t, "windows-user", u)
}

func TestCurrentUser_Fallback(t *testing.T) {
	os.Unsetenv("USER")
	os.Unsetenv("USERNAME")

	u := currentUser()
	assert.Equal(t, "unknown", u)
}

func TestResolve_FailsWithNoSources(t *testing.T) {
	os.Unsetenv("EKCONF_PASSWORD")

	_, err := Resolve("", false, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestResolve_StdinPiped(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	w.WriteString("pipe-password")
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	pw, err := Resolve("", true, false)
	require.NoError(t, err)
	assert.Equal(t, "pipe-password", pw)
}

func TestResolve_KeychainFallback(t *testing.T) {
	os.Unsetenv("EKCONF_PASSWORD")

	_, err := Resolve("", false, true)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestResolve_KeychainFallbackSkippedWithFlag(t *testing.T) {
	pw, err := Resolve("my-pass", false, true)
	require.NoError(t, err)
	assert.Equal(t, "my-pass", pw)
}

func TestStoreDelete_NoPanic(t *testing.T) {
	err := Store("test-password")
	if err != nil {
		t.Logf("Store failed (expected on Linux without keyring): %v", err)
	} else {
		err = Delete()
		if err != nil {
			t.Logf("Delete failed: %v", err)
		}
	}
}
