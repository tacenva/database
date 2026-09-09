package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/oklog/ulid/v2"
	"github.com/tacenva/database/internal/structure"
)

func (db *DB) NewRawFile(
	filename string,
) (*DatabaseFile, error) {
	if filename == "" {
		return nil, errors.New(
			"filename cannot be empty",
		)
	}

	blob, err := db.fileStore.Read(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}

		return nil, err
	}

	var file structure.File

	if err := json.Unmarshal(
		blob,
		&file,
	); err != nil {
		return nil, fmt.Errorf(
			"decode database: %w",
			err,
		)
	}

	if file.Version == 0 {
		file.Version = 1
	}

	data := make(
		map[string]json.RawMessage,
		len(file.Records),
	)

	for _, record := range file.Records {
		raw, err := json.Marshal(record.Data)
		if err != nil {
			return nil, fmt.Errorf(
				"encode record %q: %w",
				record.ID,
				err,
			)
		}

		data[record.ID] = raw
	}

	return &DatabaseFile{
		db:       db,
		filename: filename,
		file:     file,
		data:     data,
		raw:      true,
	}, nil
}

func (f *DatabaseFile) InsertRaw(
	data []byte,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.raw {
		return "", errors.New(
			"database file is not in raw mode",
		)
	}

	if len(data) == 0 {
		return "", errors.New(
			"data cannot be empty",
		)
	}

	id := ulid.Make().String()

	raw, err := json.Marshal(string(data))
	if err != nil {
		return "", fmt.Errorf(
			"encode raw data: %w",
			err,
		)
	}

	f.data[id] = raw

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.saveRaw(); err != nil {
		delete(f.data, id)
		f.file.Version = oldVersion

		return "", err
	}

	return id, nil
}

func (f *DatabaseFile) UpdateRaw(
	id string,
	data []byte,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.raw {
		return errors.New(
			"database file is not in raw mode",
		)
	}

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

	raw, err := json.Marshal(string(data))
	if err != nil {
		return fmt.Errorf(
			"encode raw data: %w",
			err,
		)
	}

	f.data[id] = raw

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.saveRaw(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion

		return err
	}

	return nil
}

func (f *DatabaseFile) FindRaw(
	id string,
) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if !f.raw {
		return nil, errors.New(
			"database file is not in raw mode",
		)
	}

	if id == "" {
		return nil, errors.New(
			"id cannot be empty",
		)
	}

	raw, exists := f.data[id]

	if !exists {
		return nil, fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	var data string

	if err := json.Unmarshal(
		raw,
		&data,
	); err != nil {
		return nil, fmt.Errorf(
			"decode raw record %q: %w",
			id,
			err,
		)
	}

	return []byte(data), nil
}

func (f *DatabaseFile) FindAllRaw() (
	map[string][]byte,
	error,
) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if !f.raw {
		return nil, errors.New(
			"database file is not in raw mode",
		)
	}

	result := make(
		map[string][]byte,
		len(f.data),
	)

	for id, raw := range f.data {
		var data string

		if err := json.Unmarshal(
			raw,
			&data,
		); err != nil {
			return nil, fmt.Errorf(
				"decode raw record %q: %w",
				id,
				err,
			)
		}

		result[id] = []byte(data)
	}

	return result, nil
}

func (f *DatabaseFile) DeleteRaw(
	id string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.raw {
		return errors.New(
			"database file is not in raw mode",
		)
	}

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

	if err := f.saveRaw(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion

		return err
	}

	return nil
}

func (f *DatabaseFile) saveRaw() error {
	if !f.raw {
		return errors.New(
			"database file is not in raw mode",
		)
	}

	records := make(
		[]structure.EncryptedRecord,
		0,
		len(f.data),
	)

	for id, raw := range f.data {
		var encrypted string

		if err := json.Unmarshal(
			raw,
			&encrypted,
		); err != nil {
			return fmt.Errorf(
				"decode raw record %q: %w",
				id,
				err,
			)
		}

		records = append(
			records,
			structure.EncryptedRecord{
				ID:   id,
				Data: encrypted,
			},
		)
	}

	f.file.Records = records

	blob, err := json.MarshalIndent(
		f.file,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"encode database: %w",
			err,
		)
	}

	if err := f.db.fileStore.Write(
		f.filename,
		blob,
	); err != nil {
		return fmt.Errorf(
			"save database: %w",
			err,
		)
	}

	return nil
}

func (f *DatabaseFile) RawVersion() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.file.Version
}

var _ sync.Locker
