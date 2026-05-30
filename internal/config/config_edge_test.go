package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_InvalidYAML(t *testing.T) {
	setupTestDir(t)

	path, _ := ConfigPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("invalid: yaml: [bad"), 0o600))

	_, err := Load()
	require.Error(t, err)
	assert.ErrorContains(t, err, "parse config")
}

func TestSave_NoDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := DefaultConfig()
	cfg.Keychain = true

	err := Save(cfg)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, DirName, ConfigFileName))
	assert.NoError(t, err)
}

func TestSaveCurrent_ContextAdded(t *testing.T) {
	setupTestDir(t)

	err := AddContext("dev", "development")
	require.NoError(t, err)

	err = SaveCurrent("dev")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "dev", cfg.Current)
	assert.Equal(t, "development", cfg.Contexts["dev"].Namespace)
}

func TestSetNamespace_NewContext(t *testing.T) {
	setupTestDir(t)

	err := SetNamespace("new-ctx", "custom-ns")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "custom-ns", cfg.Contexts["new-ctx"].Namespace)
}

func TestRemoveContext_ClearsCurrent(t *testing.T) {
	setupTestDir(t)

	require.NoError(t, AddContext("primary", "default"))
	require.NoError(t, SaveCurrent("primary"))

	err := RemoveContext("primary")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.Current)
	assert.Empty(t, cfg.Contexts)
}

func TestRemoveContext_NonExistent(t *testing.T) {
	setupTestDir(t)

	err := RemoveContext("nonexistent")
	require.NoError(t, err)
}

func TestMultipleContextsSequential(t *testing.T) {
	setupTestDir(t)

	names := []string{"ctx-a", "ctx-b", "ctx-c", "ctx-d", "ctx-e"}
	for _, name := range names {
		err := AddContext(name, "ns-"+name)
		require.NoError(t, err)
	}

	cfg, err := Load()
	require.NoError(t, err)
	assert.Len(t, cfg.Contexts, len(names))

	for _, name := range names {
		assert.True(t, cfg.ContextExists(name))
		assert.Equal(t, "ns-"+name, cfg.Contexts[name].Namespace)
	}

	for _, name := range names[:3] {
		require.NoError(t, RemoveContext(name))
	}

	cfg, err = Load()
	require.NoError(t, err)
	assert.Len(t, cfg.Contexts, 2)
	assert.True(t, cfg.ContextExists("ctx-d"))
	assert.True(t, cfg.ContextExists("ctx-e"))
}

func TestConfigPath_RespectsHome(t *testing.T) {
	dir := setupTestDir(t)

	path, err := ConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, DirName, ConfigFileName), path)
}

func TestEncPath_RespectsHome(t *testing.T) {
	dir := setupTestDir(t)

	path, err := EncPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, DirName, EncryptedConfigFileName), path)
}
