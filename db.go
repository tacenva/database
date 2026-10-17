package database

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/tacenva/database/internal/crypto"
	"github.com/tacenva/database/internal/storage"
	"github.com/tacenva/database/internal/structure"
)

const passwordVerifier = "tacenva-password-verifier"

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrFileExists      = errors.New("database file already exists")
	ErrFileNotFound    = errors.New("database file not found")
	ErrInvalidFileMode = errors.New("invalid file mode")
)

type FileMode uint8

const (
	FileModeOpenOrCreate FileMode = iota + 1
	FileModeOpen
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

	raw bool
}

func New(baseDir string) *DB {
	return &DB{
		crypto:    crypto.NewCrypto(),
		fileStore: storage.NewFileStore(baseDir),
	}
}

func (db *DB) Destroy(filename string) error {
	err := db.fileStore.Delete(filename)
	if err != nil && os.IsNotExist(err) {
		return ErrFileNotFound
	}
	return err
}

func (db *DB) Rename(
	oldName string,
	newName string,
) error {
	return db.fileStore.Rename(
		oldName,
		newName,
	)
}

func (db *DB) Read(
	filename string,
) ([]byte, error) {
	return db.fileStore.Read(filename)
}

func (db *DB) ReadMetadataOnly(
	filename string,
) ([]byte, error) {
	blob, err := db.Read(filename)
	if err != nil {
		return nil, err
	}

	var file structure.File
	if err := json.Unmarshal(blob, &file); err != nil {
		return nil, err
	}

	file.Records = nil
	return json.Marshal(file)
}

func (db *DB) Write(
	filename string,
	blob []byte,
) error {
	if len(blob) == 0 {
		return errors.New("data cannot be empty")
	}

	return db.fileStore.Write(
		filename,
		blob,
	)
}

func (db *DB) GetVersion(
	filename string,
) (uint64, error) {
	blob, err := db.Read(filename)
	if err != nil {
		return 0, err
	}

	var file structure.File

	if err := json.Unmarshal(blob, &file); err != nil {
		return 0, err
	}

	return file.Version, nil
}

func (db *DB) File(
	filename string,
	password string,
	mode FileMode,
) (*DatabaseFile, error) {
	switch mode {
	case FileModeOpenOrCreate:
		return db.openOrCreateFile(
			filename,
			password,
		)

	case FileModeOpen:
		return db.openFile(
			filename,
			password,
		)

	default:
		return nil, ErrInvalidFileMode
	}
}

func (db *DB) openOrCreateFile(
	filename string,
	password string,
) (*DatabaseFile, error) {
	_, err := db.fileStore.Read(filename)

	if err == nil {
		return db.openFile(
			filename,
			password,
		)
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	salt := make([]byte, 16)

	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	file := structure.File{
		Version: 1,
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

	key := db.crypto.DeriveKey(
		password,
		salt,
	)

	verifier, err := db.crypto.Encrypt(
		[]byte(passwordVerifier),
		key,
	)
	if err != nil {
		return nil, err
	}

	file.Verifier = verifier

	dbFile := &DatabaseFile{
		db:       db,
		filename: filename,
		file:     file,
		data:     make(map[string]json.RawMessage),
		key:      key,
	}

	if err := dbFile.save(); err != nil {
		return nil, err
	}

	return dbFile, nil
}

func (db *DB) openFile(
	filename string,
	password string,
) (*DatabaseFile, error) {
	blob, err := db.fileStore.Read(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}

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

	// Verify password before opening records.
	verified, err := db.crypto.Decrypt(
		file.Verifier,
		key,
	)
	if err != nil {
		return nil, ErrInvalidPassword
	}

	if string(verified) != passwordVerifier {
		return nil, ErrInvalidPassword
	}

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
	records := make(
		[]structure.EncryptedRecord,
		0,
		len(f.data),
	)

	for id, raw := range f.data {
		encrypted, err := f.db.crypto.Encrypt(
			raw,
			f.key,
		)
		if err != nil {
			return err
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
		return err
	}

	return f.db.fileStore.Write(
		f.filename,
		blob,
	)
}

func (f *DatabaseFile) Count() int {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return len(f.data)
}

// SimulateEncryption mengenkripsi data menggunakan key
// milik DatabaseFile dan mengembalikan hasil encrypted
// yang siap disimpan.
func (f *DatabaseFile) SimulateEncryption(
	data any,
) (string, error) {
	if data == nil {
		return "", fmt.Errorf("data cannot be nil")
	}

	if len(f.key) == 0 {
		return "", fmt.Errorf("encryption key is not available")
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf(
			"failed to marshal data: %w",
			err,
		)
	}

	encrypted, err := f.db.crypto.Encrypt(
		raw,
		f.key,
	)
	if err != nil {
		return "", fmt.Errorf(
			"failed to encrypt data: %w",
			err,
		)
	}

	return encrypted, nil
}

func (f *DatabaseFile) ChangePassword(
	currentPassword string,
	newPassword string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if currentPassword == "" {
		return ErrInvalidPassword
	}

	if newPassword == "" {
		return errors.New("new password cannot be empty")
	}

	// Verify current password against the current key.
	currentKey := f.db.crypto.DeriveKey(
		currentPassword,
		f.file.KDF.Salt,
	)

	verified, err := f.db.crypto.Decrypt(
		f.file.Verifier,
		currentKey,
	)
	if err != nil {
		return ErrInvalidPassword
	}

	if string(verified) != passwordVerifier {
		return ErrInvalidPassword
	}

	// Generate a new salt for the new password.
	newSalt := make([]byte, 16)

	if _, err := rand.Read(newSalt); err != nil {
		return err
	}

	// Derive a completely new encryption key.
	newKey := f.db.crypto.DeriveKey(
		newPassword,
		newSalt,
	)

	// Re-encrypt every record using the new key.
	newRecords := make(
		[]structure.EncryptedRecord,
		0,
		len(f.data),
	)

	for id, raw := range f.data {
		encrypted, err := f.db.crypto.Encrypt(
			raw,
			newKey,
		)
		if err != nil {
			return err
		}

		newRecords = append(
			newRecords,
			structure.EncryptedRecord{
				ID:   id,
				Data: encrypted,
			},
		)
	}

	// Create a new verifier using the new key.
	newVerifier, err := f.db.crypto.Encrypt(
		[]byte(passwordVerifier),
		newKey,
	)
	if err != nil {
		return err
	}

	// Update file metadata.
	f.file.KDF.Salt = newSalt
	f.file.Verifier = newVerifier
	f.file.Records = newRecords

	// Persist using the new key.
	blob, err := json.MarshalIndent(
		f.file,
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	if err := f.db.fileStore.Write(
		f.filename,
		blob,
	); err != nil {
		return err
	}

	// Only update the in-memory key after the new
	// encrypted database has been successfully written.
	f.key = newKey

	return nil
}
