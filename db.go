package database

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"sync"

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

	mu sync.RWMutex
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
		if !os.IsNotExist(err) {
			return nil, err
		}

		salt := make([]byte, 16)

		if _, err := rand.Read(salt); err != nil {
			return nil, err
		}

		file := structure.File{
			Algorithm: structure.Algorithm{
				KDF:    "argon2id",
				Cipher: "aes-256-gcm",
			},
			KDF: structure.KDFParams{
				Salt:        salt,
				Memory:      64 * 1024,
				Iterations:  3,
				Parallelism: 2,
			},
			Records: []structure.EncryptedRecord{},
		}

		key := db.crypto.DeriveKey(password, salt)

		return &DatabaseFile{
			db:       db,
			filename: filename,
			file:     file,
			data:     make(map[string]json.RawMessage),
			key:      key,
		}, nil
	}

	var file structure.File

	if err := json.Unmarshal(blob, &file); err != nil {
		return nil, err
	}

	key := db.crypto.DeriveKey(
		password,
		file.KDF.Salt,
	)

	data := make(map[string]json.RawMessage)

	for _, record := range file.Records {
		decrypted, err := db.crypto.Decrypt(
			record.Data,
			key,
		)
		if err != nil {
			return nil, err
		}

		data[record.ID] = decrypted
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
	records := make([]structure.EncryptedRecord, 0, len(f.data))

	for id, raw := range f.data {
		encrypted, err := f.db.crypto.Encrypt(
			raw,
			f.key,
		)
		if err != nil {
			return err
		}

		records = append(records, structure.EncryptedRecord{
			ID:   id,
			Data: encrypted,
		})
	}

	f.file.Records = records

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
