package password

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestKeychain_StoreAndResolve(t *testing.T) {
	keyring.MockInit()

	err := Store([]byte("mock-password"))
	require.NoError(t, err)

	pw, err := Resolve(t.Context(), "", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "mock-password", string(pw))
}

func TestKeychain_Delete(t *testing.T) {
	keyring.MockInit()

	err := Store([]byte("delete-me"))
	require.NoError(t, err)

	pw, err := Resolve(t.Context(), "", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "delete-me", string(pw))

	err = Delete()
	require.NoError(t, err)

	_, err = Resolve(t.Context(), "", false, true, "")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestKeychain_EmptyPassword(t *testing.T) {
	keyring.MockInit()

	err := Store([]byte(""))
	require.NoError(t, err)

	pw, err := Resolve(t.Context(), "", false, true, "")
	require.NoError(t, err)
	assert.Empty(t, pw)
}

func TestKeychain_StoreOverwrites(t *testing.T) {
	keyring.MockInit()

	require.NoError(t, Store([]byte("first-password")))
	require.NoError(t, Store([]byte("second-password")))

	pw, err := Resolve(t.Context(), "", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "second-password", string(pw))
}

func TestKeychain_ResolvePrecedence(t *testing.T) {
	keyring.MockInit()

	require.NoError(t, Store([]byte("keychain-password")))

	pw, err := Resolve(t.Context(), "flag-password", false, true, "")
	require.NoError(t, err)
	assert.Equal(t, "flag-password", string(pw), "flag should override keychain")

	pw, err = Resolve(t.Context(), "", false, true, "env-password")
	require.NoError(t, err)
	assert.Equal(t, "env-password", string(pw), "env should override keychain")
}
