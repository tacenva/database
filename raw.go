package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/oklog/ulid/v2"
)

type RawDatabaseFile struct {
	db       *DB
	filename string
	file     RawFile
	data     map[string][]byte

	mu sync.RWMutex
}

type RawFile struct {
	Version uint64      `json:"version"`
	Records []RawRecord `json:"records"`
}

type RawRecord struct {
	ID   string `json:"id"`
	Data []byte `json:"data"`
}

func (db *DB) RawFile(
	filename string,
) (*RawDatabaseFile, error) {
	if filename == "" {
		return nil, errors.New(
			"filename cannot be empty",
		)
	}

	blob, err := db.fileStore.Read(filename)

	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		return &RawDatabaseFile{
			db:       db,
			filename: filename,
			file: RawFile{
				Version: 1,
			},
			data: make(map[string][]byte),
		}, nil
	}

	var file RawFile

	if err := json.Unmarshal(
		blob,
		&file,
	); err != nil {
		return nil, fmt.Errorf(
			"decode raw database: %w",
			err,
		)
	}

	// Legacy file without version.
	if file.Version == 0 {
		file.Version = 1
	}

	data := make(
		map[string][]byte,
		len(file.Records),
	)

	for _, record := range file.Records {
		data[record.ID] = record.Data
	}

	return &RawDatabaseFile{
		db:       db,
		filename: filename,
		file:     file,
		data:     data,
	}, nil
}

func (f *RawDatabaseFile) Insert(
	data []byte,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(data) == 0 {
		return "", errors.New(
			"data cannot be empty",
		)
	}

	id := ulid.Make().String()

	f.data[id] = data

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		delete(f.data, id)
		f.file.Version = oldVersion

		return "", err
	}

	return id, nil
}

func (f *RawDatabaseFile) Update(
	id string,
	data []byte,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if id == "" {
		return errors.New(
			"id cannot be empty",
		)
	}

	if len(data) == 0 {
		return errors.New(
			"data cannot be empty",
		)
	}

	old, exists := f.data[id]

	if !exists {
		return fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	f.data[id] = data

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion

		return err
	}

	return nil
}

func (f *RawDatabaseFile) Find(
	id string,
) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if id == "" {
		return nil, errors.New(
			"id cannot be empty",
		)
	}

	data, exists := f.data[id]

	if !exists {
		return nil, fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	return data, nil
}

func (f *RawDatabaseFile) FindAll() (
	map[string][]byte,
	error,
) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(
		map[string][]byte,
		len(f.data),
	)

	for id, data := range f.data {
		result[id] = data
	}

	return result, nil
}

func (f *RawDatabaseFile) Version() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.file.Version
}

func (f *RawDatabaseFile) Delete(
	id string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if id == "" {
		return errors.New(
			"id cannot be empty",
		)
	}

	old, exists := f.data[id]

	if !exists {
		return fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	delete(f.data, id)

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion

		return err
	}

	return nil
}

func (f *RawDatabaseFile) save() error {
	records := make(
		[]RawRecord,
		0,
		len(f.data),
	)

	for id, data := range f.data {
		records = append(
			records,
			RawRecord{
				ID:   id,
				Data: data,
			},
		)
	}

	file := RawFile{
		Version: f.file.Version,
		Records: records,
	}

	blob, err := json.MarshalIndent(
		file,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"encode raw database: %w",
			err,
		)
	}

	return f.db.fileStore.Write(
		f.filename,
		blob,
	)
}

// Sync synchronizes the raw database with the provided records.
//
// Records that exist in the source will be created or updated.
// Records that exist in the database but are missing from the source
// will be deleted.
//
// All records are persisted in a single save operation.
// If saving fails, the entire database is rolled back.
func (f *RawDatabaseFile) Sync(
	values map[string][]byte,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if values == nil {
		return errors.New(
			"values cannot be nil",
		)
	}

	// Keep the complete original state for rollback.
	original := make(
		map[string][]byte,
		len(f.data),
	)

	for id, data := range f.data {
		original[id] = data
	}

	// Build the new database state separately.
	next := make(
		map[string][]byte,
		len(values),
	)

	for id, data := range values {
		if id == "" {
			return errors.New(
				"id cannot be empty",
			)
		}

		if len(data) == 0 {
			return fmt.Errorf(
				"data for record %q cannot be empty",
				id,
			)
		}

		next[id] = data
	}

	// No changes.
	if rawDataEqual(f.data, next) {
		return nil
	}

	// Replace the current database with the synchronized state.
	f.data = next

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		f.data = original
		f.file.Version = oldVersion

		return err
	}

	return nil
}

func rawDataEqual(
	a map[string][]byte,
	b map[string][]byte,
) bool {
	if len(a) != len(b) {
		return false
	}

	for id, dataA := range a {
		dataB, exists := b[id]

		if !exists {
			return false
		}

		if string(dataA) != string(dataB) {
			return false
		}
	}

	return true
}
