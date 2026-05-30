package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	return dir
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.Keychain)
	assert.Empty(t, cfg.Current)
	assert.NotNil(t, cfg.Contexts)
	assert.Empty(t, cfg.Contexts)
}

func TestDir(t *testing.T) {
	setupTestDir(t)

	dir, err := Dir()
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, DirName), dir)
}

func TestConfigPath(t *testing.T) {
	setupTestDir(t)

	path, err := ConfigPath()
	require.NoError(t, err)

	dir, _ := Dir()
	assert.Equal(t, filepath.Join(dir, ConfigFileName), path)
}

func TestEncPath(t *testing.T) {
	setupTestDir(t)

	path, err := EncPath()
	require.NoError(t, err)

	dir, _ := Dir()
	assert.Equal(t, filepath.Join(dir, EncryptedConfigFileName), path)
}

func TestEnsureDir(t *testing.T) {
	setupTestDir(t)

	err := EnsureDir()
	require.NoError(t, err)

	dir, _ := Dir()
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_NoFileReturnsDefault(t *testing.T) {
	setupTestDir(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.Keychain)
	assert.Empty(t, cfg.Contexts)
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	setupTestDir(t)

	cfg := DefaultConfig()
	cfg.Keychain = true
	cfg.Current = "prod"
	cfg.Contexts["prod"] = ContextEntry{Namespace: "production"}
	cfg.Contexts["staging"] = ContextEntry{Namespace: "staging"}

	err := Save(cfg)
	require.NoError(t, err)

	loaded, err := Load()
	require.NoError(t, err)
	assert.True(t, loaded.Keychain)
	assert.Equal(t, "prod", loaded.Current)
	assert.Equal(t, "production", loaded.Contexts["prod"].Namespace)
	assert.Equal(t, "staging", loaded.Contexts["staging"].Namespace)
}

func TestLoad_EmptyFile(t *testing.T) {
	setupTestDir(t)

	path, _ := ConfigPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

	_, err := Load()
	assert.NoError(t, err)
}

func TestContextExists(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Contexts["prod"] = ContextEntry{Namespace: "default"}

	assert.True(t, cfg.ContextExists("prod"))
	assert.False(t, cfg.ContextExists("staging"))
	assert.False(t, cfg.ContextExists(""))
}

func TestGetNamespace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Contexts["prod"] = ContextEntry{Namespace: "production"}

	assert.Equal(t, "production", cfg.GetNamespace("prod"))
	assert.Equal(t, "default", cfg.GetNamespace("nonexistent"))
	assert.Equal(t, "default", cfg.GetNamespace(""))

	cfg.Contexts["empty-ns"] = ContextEntry{}
	assert.Equal(t, "default", cfg.GetNamespace("empty-ns"))
}

func TestAddContext(t *testing.T) {
	setupTestDir(t)

	err := AddContext("prod", "production")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.ContextExists("prod"))
	assert.Equal(t, "production", cfg.Contexts["prod"].Namespace)
}

func TestAddContext_DefaultNamespace(t *testing.T) {
	setupTestDir(t)

	err := AddContext("dev", "")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "default", cfg.Contexts["dev"].Namespace)
}

func TestRemoveContext(t *testing.T) {
	setupTestDir(t)

	require.NoError(t, AddContext("prod", "production"))
	require.NoError(t, AddContext("staging", "staging"))
	require.NoError(t, SaveCurrent("prod"))

	err := RemoveContext("prod")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.ContextExists("prod"))
	assert.True(t, cfg.ContextExists("staging"))
	assert.Empty(t, cfg.Current, "current should be cleared when removed context was active")
}

func TestSaveCurrent(t *testing.T) {
	setupTestDir(t)

	require.NoError(t, AddContext("prod", "production"))

	err := SaveCurrent("prod")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "prod", cfg.Current)
}

func TestSetNamespace(t *testing.T) {
	setupTestDir(t)

	require.NoError(t, AddContext("prod", "production"))

	err := SetNamespace("prod", "staging")
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "staging", cfg.Contexts["prod"].Namespace)
}

func TestAddContext_FilePermissions(t *testing.T) {
	setupTestDir(t)

	err := AddContext("test", "ns")
	require.NoError(t, err)

	path, _ := ConfigPath()
	info, err := os.Stat(path)
	require.NoError(t, err)

	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0o600), perm, "config.yaml should have 0600 permissions")
}

func TestEnsureDir_DirPermissions(t *testing.T) {
	setupTestDir(t)

	err := EnsureDir()
	require.NoError(t, err)

	dir, _ := Dir()
	info, err := os.Stat(dir)
	require.NoError(t, err)

	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0o700), perm, "~/.ekube should have 0700 permissions")
}
