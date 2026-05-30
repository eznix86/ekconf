package password

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_PasswordFlag(t *testing.T) {
	pw, err := Resolve("my-password", false, false)
	require.NoError(t, err)
	assert.Equal(t, "my-password", pw)
}

func TestResolve_EmptyFlag(t *testing.T) {
	pw, err := Resolve("", false, false)
	assert.Error(t, err)
	assert.Empty(t, pw)
}

func TestResolve_EnvironmentVariable(t *testing.T) {
	os.Setenv("EKCONF_PASSWORD", "env-password")
	t.Cleanup(func() { os.Unsetenv("EKCONF_PASSWORD") })

	pw, err := Resolve("", false, false)
	require.NoError(t, err)
	assert.Equal(t, "env-password", pw)
}

func TestResolve_FlagOverridesEnv(t *testing.T) {
	os.Setenv("EKCONF_PASSWORD", "env-password")
	t.Cleanup(func() { os.Unsetenv("EKCONF_PASSWORD") })

	pw, err := Resolve("flag-password", false, false)
	require.NoError(t, err)
	assert.Equal(t, "flag-password", pw)
}

func TestResolve_EnvOverridesKeychain(t *testing.T) {
	os.Setenv("EKCONF_PASSWORD", "env-password")
	t.Cleanup(func() { os.Unsetenv("EKCONF_PASSWORD") })

	pw, err := Resolve("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "env-password", pw)
}

func TestResolve_NoSources(t *testing.T) {
	os.Unsetenv("EKCONF_PASSWORD")

	_, err := Resolve("", false, false)
	assert.Error(t, err)
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

	pw, err := Resolve("", true, false)
	require.NoError(t, err)
	assert.Equal(t, "stdin-password", pw)
}
