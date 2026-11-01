package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrTransactionClosed = errors.New("transaction already closed")
)

type DatabaseTransaction struct {
	parent      *DatabaseFile
	working     *DatabaseFile
	baseVersion uint64

	mu     sync.Mutex
	closed bool
}

// Begin starts a new transaction.
//
// The parent database is locked until Commit or Rollback.
// All transaction operations must use tx.DB(), not the parent file.
func (f *DatabaseFile) Begin() (*DatabaseTransaction, error) {
	f.mu.Lock()
	if f.txActive {
		f.mu.Unlock()
		return nil, errors.New(
			"nested transactions are not supported",
		)
	}

	working, err := cloneDatabaseFile(f)
	if err != nil {
		f.mu.Unlock()
		return nil, fmt.Errorf("clone database state: %w", err)
	}

	working.txActive = true

	return &DatabaseTransaction{
		parent:      f,
		working:     working,
		baseVersion: f.file.Version,
	}, nil
}

// DB returns the working database file.
//
// Use this database file for all operations inside the transaction.
// Do not use the original DatabaseFile until the transaction closes.
func (tx *DatabaseTransaction) DB() *DatabaseFile {
	return tx.working
}

// Commit persists all transaction changes in one file write.
func (tx *DatabaseTransaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.closed {
		return ErrTransactionClosed
	}

	tx.working.mu.Lock()

	// No changes were made.
	if tx.working.file.Version == tx.baseVersion {
		tx.closed = true
		tx.working.mu.Unlock()
		tx.parent.mu.Unlock()
		return nil
	}

	candidate, err := cloneDatabaseFile(tx.working)
	if err != nil {
		tx.closeLocked()
		return fmt.Errorf("clone transaction state: %w", err)
	}

	candidate.txActive = false
	candidate.file.Version = tx.baseVersion + 1

	// Persist according to the database file mode.
	if candidate.raw {
		err = candidate.saveRaw()
	} else {
		err = candidate.save()
	}

	if err != nil {
		tx.closeLocked()
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Replace the parent state only after persistence succeeds.
	tx.parent.data = candidate.data
	tx.parent.file = candidate.file
	tx.parent.key = candidate.key

	tx.closeLocked()

	return nil
}

// Rollback discards all transaction changes.
func (tx *DatabaseTransaction) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.closed {
		return ErrTransactionClosed
	}

	tx.working.mu.Lock()
	tx.closed = true
	tx.working.mu.Unlock()

	tx.parent.mu.Unlock()

	return nil
}

// closeLocked closes the transaction and releases both locks.
// tx.mu and working.mu must already be locked.
func (tx *DatabaseTransaction) closeLocked() {
	tx.closed = true
	tx.working.mu.Unlock()
	tx.parent.mu.Unlock()
}

func cloneDatabaseFile(src *DatabaseFile) (*DatabaseFile, error) {
	fileJSON, err := json.Marshal(src.file)
	if err != nil {
		return nil, err
	}

	var copiedFile = src.file

	if err := json.Unmarshal(fileJSON, &copiedFile); err != nil {
		return nil, err
	}

	copiedData := make(
		map[string]json.RawMessage,
		len(src.data),
	)

	for id, raw := range src.data {
		copiedData[id] = append(json.RawMessage(nil), raw...)
	}

	return &DatabaseFile{
		db:       src.db,
		filename: src.filename,
		file:     copiedFile,
		data:     copiedData,
		key:      append([]byte(nil), src.key...),
		raw:      src.raw,
	}, nil
}
