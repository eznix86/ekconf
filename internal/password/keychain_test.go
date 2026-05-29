package password

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestKeychain_StoreAndResolve(t *testing.T) {
	keyring.MockInit()

	err := Store("mock-password")
	require.NoError(t, err)

	os.Unsetenv("EKCONF_PASSWORD")

	pw, err := Resolve("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "mock-password", pw)
}

func TestKeychain_Delete(t *testing.T) {
	keyring.MockInit()

	err := Store("delete-me")
	require.NoError(t, err)

	pw, err := Resolve("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "delete-me", pw)

	err = Delete()
	require.NoError(t, err)

	os.Unsetenv("EKCONF_PASSWORD")

	_, err = Resolve("", false, true)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestKeychain_EmptyPassword(t *testing.T) {
	keyring.MockInit()

	err := Store("")
	require.NoError(t, err)

	os.Unsetenv("EKCONF_PASSWORD")

	pw, err := Resolve("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "", pw)
}

func TestKeychain_StoreOverwrites(t *testing.T) {
	keyring.MockInit()

	Store("first-password")
	Store("second-password")

	os.Unsetenv("EKCONF_PASSWORD")

	pw, err := Resolve("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "second-password", pw)
}

func TestKeychain_ResolvePrecedence(t *testing.T) {
	keyring.MockInit()

	Store("keychain-password")

	pw, err := Resolve("flag-password", false, true)
	require.NoError(t, err)
	assert.Equal(t, "flag-password", pw, "flag should override keychain")

	os.Setenv("EKCONF_PASSWORD", "env-password")
	t.Cleanup(func() { os.Unsetenv("EKCONF_PASSWORD") })

	pw, err = Resolve("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "env-password", pw, "env should override keychain")
}
