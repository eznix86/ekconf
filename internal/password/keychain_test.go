package password

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestKeychain_StoreAndResolve(t *testing.T) {
	keyring.MockInit()

	err := Store("mock-password")
	require.NoError(t, err)

	pw, err := Resolve("", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "mock-password", pw)
}

func TestKeychain_Delete(t *testing.T) {
	keyring.MockInit()

	err := Store("delete-me")
	require.NoError(t, err)

	pw, err := Resolve("", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "delete-me", pw)

	err = Delete()
	require.NoError(t, err)

	_, err = Resolve("", false, true, "")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestKeychain_EmptyPassword(t *testing.T) {
	keyring.MockInit()

	err := Store("")
	require.NoError(t, err)

	pw, err := Resolve("", false, true, "")
	require.NoError(t, err)
	assert.Empty(t, pw)
}

func TestKeychain_StoreOverwrites(t *testing.T) {
	keyring.MockInit()

	require.NoError(t, Store("first-password"))
	require.NoError(t, Store("second-password"))

	pw, err := Resolve("", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "second-password", pw)
}

func TestKeychain_ResolvePrecedence(t *testing.T) {
	keyring.MockInit()

	require.NoError(t, Store("keychain-password"))

	pw, err := Resolve("flag-password", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "flag-password", pw, "flag should override keychain")

	pw, err = Resolve("", false, true, "env-password")
	require.NoError(t, err)
	assert.Equal(t, "env-password", pw, "env should override keychain")
}
