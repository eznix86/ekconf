package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

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

	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.txt")
	secondPath := filepath.Join(dir, "second.txt")
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
