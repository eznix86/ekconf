package password

import (
	"fmt"
	"os"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestCurrentUser_FromEnv(t *testing.T) {
	oldLookup := lookupCurrentUser
	lookupCurrentUser = func() (*user.User, error) { return nil, fmt.Errorf("boom") }
	t.Cleanup(func() { lookupCurrentUser = oldLookup })

	t.Setenv("USER", "test-user")
	t.Setenv("USERNAME", "")

	u := currentUser()
	assert.Equal(t, "test-user", u)
}

func TestCurrentUser_FromUsernameEnv(t *testing.T) {
	oldLookup := lookupCurrentUser
	lookupCurrentUser = func() (*user.User, error) { return nil, fmt.Errorf("boom") }
	t.Cleanup(func() { lookupCurrentUser = oldLookup })

	t.Setenv("USER", "")
	t.Setenv("USERNAME", "windows-user")

	u := currentUser()
	assert.Equal(t, "windows-user", u)
}

func TestCurrentUser_Fallback(t *testing.T) {
	oldLookup := lookupCurrentUser
	lookupCurrentUser = func() (*user.User, error) { return nil, fmt.Errorf("boom") }
	t.Cleanup(func() { lookupCurrentUser = oldLookup })

	t.Setenv("USER", "")
	t.Setenv("USERNAME", "")

	u := currentUser()
	assert.Equal(t, "uid-unknown", u)
}

func TestResolve_FailsWithNoSources(t *testing.T) {
	_, err := Resolve("", false, false, "")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestResolve_StdinPiped(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, err = w.WriteString("pipe-password")
	require.NoError(t, err)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	pw, err := Resolve("", true, false, "")
	require.NoError(t, err)
	assert.Equal(t, "pipe-password", pw)
}

func TestResolve_KeychainFallback(t *testing.T) {
	keyring.MockInit()
	oldLookup := lookupCurrentUser
	lookupCurrentUser = func() (*user.User, error) { return nil, fmt.Errorf("boom") }
	t.Cleanup(func() { lookupCurrentUser = oldLookup })

	_, err := Resolve("", false, true, "")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestResolve_KeychainFallbackSkippedWithFlag(t *testing.T) {
	pw, err := Resolve("my-pass", false, true, "")
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
