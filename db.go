package database

import (
	"encoding/json"

	"github.com/tacenva/database/internal/crypto"
	"github.com/tacenva/database/internal/storage"
	"github.com/tacenva/database/internal/structure"
)

type DB struct {
	crypto    *crypto.Crypto
	fileStore *storage.FileStore
}

type DatabaseFile struct {
	db       *DB
	filename string
	file     structure.File
	data     map[string]json.RawMessage
	key      []byte
}

func New(baseDir string) *DB {
	return &DB{
		crypto:    crypto.NewCrypto(),
		fileStore: storage.NewFileStore(baseDir),
	}
}

func (db *DB) File(
	filename string,
	password string,
) (*DatabaseFile, error) {
	blob, err := db.fileStore.Read(filename)
	if err != nil {
		return nil, err
	}

	var file structure.File

	if err := json.Unmarshal(blob, &file); err != nil {
		return nil, err
	}

	key := db.crypto.DeriveKey(
		password,
		file.KDF.Salt,
	)

	decrypted, err := db.crypto.Decrypt(
		file.Data,
		key,
	)
	if err != nil {
		return nil, err
	}

	var data map[string]json.RawMessage

	if err := json.Unmarshal(decrypted, &data); err != nil {
		return nil, err
	}

	if data == nil {
		data = make(map[string]json.RawMessage)
	}

	return &DatabaseFile{
		db:       db,
		filename: filename,
		file:     file,
		data:     data,
		key:      key,
	}, nil
}

func (f *DatabaseFile) save() error {
	plain, err := json.Marshal(f.data)
	if err != nil {
		return err
	}

	encrypted, err := f.db.crypto.Encrypt(
		plain,
		f.key,
	)
	if err != nil {
		return err
	}

	f.file.Data = encrypted

	blob, err := json.MarshalIndent(
		f.file,
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	return f.db.fileStore.Write(
		f.filename,
		blob,
	)
}
