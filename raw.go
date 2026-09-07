package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/oklog/ulid/v2"

	"github.com/tacenva/database/internal/storage"
)

type RawDB struct {
	fileStore *storage.FileStore
}

type RawDatabaseFile struct {
	db       *RawDB
	filename string
	data     map[string][]byte

	mu sync.RWMutex
}

type RawFile struct {
	Records []RawRecord `json:"records"`
}

type RawRecord struct {
	ID   string `json:"id"`
	Data []byte `json:"data"`
}

func NewRaw(
	baseDir string,
) *RawDB {
	return &RawDB{
		fileStore: storage.NewFileStore(baseDir),
	}
}

func (db *RawDB) File(
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
			data:     make(map[string][]byte),
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

	if err := f.save(); err != nil {
		delete(f.data, id)

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

	if err := f.save(); err != nil {
		f.data[id] = old

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

	if err := f.save(); err != nil {
		f.data[id] = old

		return err
	}

	return nil
}
