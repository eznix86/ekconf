package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChildEnvironment_DropsThePasswordVariable(t *testing.T) {
	t.Setenv(PasswordEnvVar, "the-master-password")
	t.Setenv("EKCONF_UNRELATED", "keep-me")

	env := childEnvironment()

	for _, entry := range env {
		assert.NotContains(t, entry, "the-master-password")
	}
	assert.NotContains(t, env, PasswordEnvVar+"=the-master-password")
	assert.Contains(t, env, "EKCONF_UNRELATED=keep-me")
}

func TestChildEnvironment_KeepsEverythingElse(t *testing.T) {
	t.Setenv("EKCONF_UNRELATED", "keep-me")
	require.NoError(t, os.Unsetenv(PasswordEnvVar))

	assert.ElementsMatch(t, os.Environ(), childEnvironment())
}

func TestBuildExecCommand_ChildNeverSeesThePassword(t *testing.T) {
	t.Setenv(PasswordEnvVar, "the-master-password")

	c := buildExecCommand(t.Context(), []string{"true"}, "/tmp/kubeconfig")

	for _, entry := range c.Env {
		assert.NotContains(t, entry, "the-master-password")
	}
	assert.Contains(t, c.Env, "KUBECONFIG=/tmp/kubeconfig")
}
