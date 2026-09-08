package database

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
)

func (f *DatabaseFile) Hash() (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	ids := make([]string, 0, len(f.data))

	for id := range f.data {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	hash := sha256.New()

	for _, id := range ids {
		_, _ = hash.Write([]byte(id))
		_, _ = hash.Write([]byte{0})

		_, _ = hash.Write(f.data[id])
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (f *DatabaseFile) RecordHash(id string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	raw, exists := f.data[id]
	if !exists {
		return "", errors.New("record not found")
	}

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:]), nil
}

func (f *DatabaseFile) CompareHash(hash string) (bool, error) {
	currentHash, err := f.Hash()
	if err != nil {
		return false, err
	}

	return currentHash == hash, nil
}

func (f *DatabaseFile) CompareRecordHash(id string, hash string) (bool, error) {
	currentHash, err := f.RecordHash(id)
	if err != nil {
		return false, err
	}

	return currentHash == hash, nil
}

func (f *RawDatabaseFile) Hash() (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	ids := make([]string, 0, len(f.data))

	for id := range f.data {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	hash := sha256.New()

	for _, id := range ids {
		_, _ = hash.Write([]byte(id))
		_, _ = hash.Write([]byte{0})

		_, _ = hash.Write(f.data[id])
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (f *RawDatabaseFile) RecordHash(id string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	raw, exists := f.data[id]
	if !exists {
		return "", errors.New("record not found")
	}

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:]), nil
}

func (f *RawDatabaseFile) CompareHash(hash string) (bool, error) {
	currentHash, err := f.Hash()
	if err != nil {
		return false, err
	}

	return currentHash == hash, nil
}

func (f *RawDatabaseFile) CompareRecordHash(id string, hash string) (bool, error) {
	currentHash, err := f.RecordHash(id)
	if err != nil {
		return false, err
	}

	return currentHash == hash, nil
}
