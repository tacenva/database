package storage

import (
	"os"
	"path/filepath"
	"strings"
)

const fileExtension = ".tacenva"

type FileStore struct {
	baseDir string
}

func NewFileStore(baseDir string) *FileStore {
	return &FileStore{
		baseDir: baseDir,
	}
}

func (fs *FileStore) Read(
	filename string,
) ([]byte, error) {
	path := filepath.Join(
		fs.baseDir,
		normalizeFilename(filename),
	)

	return os.ReadFile(path)
}

func (fs *FileStore) Write(
	filename string,
	data []byte,
) error {
	path := filepath.Join(
		fs.baseDir,
		normalizeFilename(filename),
	)

	if err := os.MkdirAll(
		filepath.Dir(path),
		0700,
	); err != nil {
		return err
	}

	return os.WriteFile(
		path,
		data,
		0600,
	)
}

func (fs *FileStore) Delete(
	filename string,
) error {
	path := filepath.Join(
		fs.baseDir,
		normalizeFilename(filename),
	)

	return os.Remove(path)
}

func (fs *FileStore) Rename(
	oldName string,
	newName string,
) error {
	oldPath := filepath.Join(
		fs.baseDir,
		normalizeFilename(oldName),
	)

	newPath := filepath.Join(
		fs.baseDir,
		normalizeFilename(newName),
	)

	return os.Rename(oldPath, newPath)
}

func (fs *FileStore) Exists(
	filename string,
) (bool, error) {
	path := filepath.Join(
		fs.baseDir,
		normalizeFilename(filename),
	)

	_, err := os.Stat(path)

	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func normalizeFilename(filename string) string {
	if strings.HasSuffix(
		filename,
		fileExtension,
	) {
		return filename
	}

	return filename + fileExtension
}
