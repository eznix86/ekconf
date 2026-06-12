package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eznix86/ekconf/internal/config"
)

type fileUpdate struct {
	path     string
	data     []byte
	original []byte
	existed  bool
}

type journalUpdate struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

type fileTransaction struct {
	Updates []journalUpdate `json:"updates"`
}

var (
	createTempFile = os.CreateTemp
	renameFile     = os.Rename
)

func recoverPendingFileTransaction() error {
	journalPath, err := transactionJournalPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(journalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read transaction journal: %w", err)
	}

	var tx fileTransaction
	if err := json.Unmarshal(data, &tx); err != nil {
		return fmt.Errorf("parse transaction journal: %w", err)
	}

	for _, update := range tx.Updates {
		if err := writeFileAtomically(update.Path, update.Data); err != nil {
			return fmt.Errorf("recover %s: %w", update.Path, err)
		}
	}

	if err := os.Remove(journalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove transaction journal: %w", err)
	}

	return nil
}

func replaceFilesAtomically(updates []fileUpdate) (retErr error) {
	if len(updates) == 0 {
		return nil
	}

	for i := range updates {
		if data, err := os.ReadFile(updates[i].path); err == nil {
			updates[i].original = data
			updates[i].existed = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", updates[i].path, err)
		}
	}

	journalPath, err := transactionJournalPath()
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(journalPath); removeErr != nil && !os.IsNotExist(removeErr) && retErr == nil {
			retErr = fmt.Errorf("remove transaction journal: %w", removeErr)
		}
	}()

	tx := fileTransaction{Updates: make([]journalUpdate, 0, len(updates))}
	for _, update := range updates {
		tx.Updates = append(tx.Updates, journalUpdate{Path: update.path, Data: update.data})
	}
	journalData, err := json.Marshal(tx)
	if err != nil {
		return fmt.Errorf("marshal transaction journal: %w", err)
	}

	if err := writeFileAtomically(journalPath, journalData); err != nil {
		return fmt.Errorf("write transaction journal: %w", err)
	}

	tempPaths := make([]string, 0, len(updates))
	defer func() {
		for _, tempPath := range tempPaths {
			_ = os.Remove(tempPath)
		}
	}()

	for i := range updates {
		tmp, err := createTempFile(filepath.Dir(updates[i].path), ".ekconf-*")
		if err != nil {
			return fmt.Errorf("create temp file for %s: %w", updates[i].path, err)
		}
		if _, err := tmp.Write(updates[i].data); err != nil {
			cleanupErr := errors.Join(tmp.Close(), os.Remove(tmp.Name()))
			return fmt.Errorf("write temp file for %s: %w", updates[i].path, errors.Join(err, cleanupErr))
		}
		if err := tmp.Chmod(0o600); err != nil {
			cleanupErr := errors.Join(tmp.Close(), os.Remove(tmp.Name()))
			return fmt.Errorf("chmod temp file for %s: %w", updates[i].path, errors.Join(err, cleanupErr))
		}
		if err := tmp.Close(); err != nil {
			removeErr := os.Remove(tmp.Name())
			return fmt.Errorf("close temp file for %s: %w", updates[i].path, errors.Join(err, removeErr))
		}
		tempPaths = append(tempPaths, tmp.Name())
	}

	rollback := func(failedAt int) error {
		for i := failedAt - 1; i >= 0; i-- {
			if updates[i].existed {
				if err := restoreFile(updates[i].path, updates[i].original); err != nil {
					return err
				}
			} else if err := os.Remove(updates[i].path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}

	for i := range updates {
		if err := renameFile(tempPaths[i], updates[i].path); err != nil {
			if rollbackErr := rollback(i); rollbackErr != nil {
				return fmt.Errorf("replace %s: %w", updates[i].path, errors.Join(err, fmt.Errorf("rollback: %w", rollbackErr)))
			}
			return fmt.Errorf("replace %s: %w", updates[i].path, err)
		}
	}

	return retErr
}

func transactionJournalPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".ekconf-transaction.json"), nil
}

func writeFileAtomically(path string, data []byte) error {
	tmp, err := createTempFile(filepath.Dir(path), ".ekconf-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Chmod(0o600); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return renameFile(tmp.Name(), path)
}

func restoreFile(path string, data []byte) error {
	return writeFileAtomically(path, data)
}
