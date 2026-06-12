package password

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	oldOpenTTY := openTTY
	openTTY = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return nil, errors.New("tty disabled in test")
	}
	code := m.Run()
	openTTY = oldOpenTTY
	os.Exit(code)
}

func TestResolve_PasswordFlag(t *testing.T) {
	pw, err := Resolve("my-password", false, false, "")
	require.NoError(t, err)
	assert.Equal(t, "my-password", pw)
}

func TestResolve_EmptyFlag(t *testing.T) {
	pw, err := Resolve("", false, false, "")
	require.Error(t, err)
	assert.Empty(t, pw)
}

func TestResolve_EnvironmentVariable(t *testing.T) {
	pw, err := Resolve("", false, false, "env-password")
	require.NoError(t, err)
	assert.Equal(t, "env-password", pw)
}

func TestResolve_FlagOverridesEnv(t *testing.T) {
	pw, err := Resolve("flag-password", false, false, "env-password")
	require.NoError(t, err)
	assert.Equal(t, "flag-password", pw)
}

func TestResolve_EnvOverridesKeychain(t *testing.T) {
	pw, err := Resolve("", false, true, "env-password")
	require.NoError(t, err)
	assert.Equal(t, "env-password", pw)
}

func TestResolve_NoSources(t *testing.T) {
	_, err := Resolve("", false, false, "")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not a terminal")
}

func TestResolve_PasswordStdin(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, err = w.WriteString("stdin-password\n")
	require.NoError(t, err)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	pw, err := Resolve("", true, false, "")
	require.NoError(t, err)
	assert.Equal(t, "stdin-password", pw)
}
