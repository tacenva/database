package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tacenva/database/internal/crypto"
	"github.com/tacenva/database/internal/structure"
)

type User struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Status string `json:"status"`
}

func createTestDatabase(
	t *testing.T,
	baseDir string,
	filename string,
	password string,
) {
	t.Helper()

	file := structure.File{
		Algorithm: structure.Algorithm{
			KDF:    "argon2id",
			Cipher: "aes-256-gcm",
		},
		KDF: structure.KDFParams{
			Salt:        []byte("test-salt-123456"),
			Memory:      64 * 1024,
			Iterations:  3,
			Parallelism: 2,
		},
		Records: []structure.EncryptedRecord{},
	}

	_ = password

	blob, err := json.MarshalIndent(
		file,
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(
		filename,
		".tacenva",
	) {
		filename += ".tacenva"
	}

	path := filepath.Join(
		baseDir,
		filename,
	)

	if err := os.MkdirAll(
		filepath.Dir(path),
		0700,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		path,
		blob,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	t.Logf(
		"created database: %s",
		path,
	)
}

func createTestDatabaseWithRecord(
	t *testing.T,
	baseDir string,
	filename string,
	password string,
) {
	t.Helper()

	cryptoService := crypto.NewCrypto()

	salt := []byte("test-salt-123456")

	key := cryptoService.DeriveKey(
		password,
		salt,
	)

	plain, err := json.Marshal(
		User{
			ID:     "test-record",
			Name:   "Test",
			Age:    20,
			Status: "active",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	encrypted, err := cryptoService.Encrypt(
		plain,
		key,
	)
	if err != nil {
		t.Fatal(err)
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
		Records: []structure.EncryptedRecord{
			{
				ID:   "test-record",
				Data: encrypted,
			},
		},
	}

	blob, err := json.MarshalIndent(
		file,
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(
		filename,
		".tacenva",
	) {
		filename += ".tacenva"
	}

	path := filepath.Join(
		baseDir,
		filename,
	)

	if err := os.MkdirAll(
		filepath.Dir(path),
		0700,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		path,
		blob,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	t.Logf(
		"created database with record: %s",
		path,
	)
}

func createAndOpenTestDatabase(
	t *testing.T,
	db *DB,
	baseDir string,
	filename string,
	password string,
) (*DatabaseFile, error) {
	t.Helper()

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	return db.File(
		filename,
		password,
	)
}

func TestDatabaseFileInsert(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users.tacenva",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	user := User{
		Name:   "Budi",
		Age:    20,
		Status: "active",
	}

	if err := dbFile.Insert(&user); err != nil {
		t.Fatal(err)
	}

	if user.ID == "" {
		t.Fatal("expected ID to be generated")
	}

	t.Logf(
		"inserted user: %+v",
		user,
	)

	var result User

	if err := dbFile.Find(
		user.ID,
		&result,
	); err != nil {
		t.Fatal(err)
	}

	if result.ID != user.ID {
		t.Fatalf(
			"expected ID %q, got %q",
			user.ID,
			result.ID,
		)
	}

	if result.Name != user.Name {
		t.Fatalf(
			"expected name %q, got %q",
			user.Name,
			result.Name,
		)
	}

	if result.Age != user.Age {
		t.Fatalf(
			"expected age %d, got %d",
			user.Age,
			result.Age,
		)
	}

	if result.Status != user.Status {
		t.Fatalf(
			"expected status %q, got %q",
			user.Status,
			result.Status,
		)
	}
}

func TestDatabaseFileUpdate(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users.tacenva",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	user := User{
		Name:   "Budi",
		Age:    20,
		Status: "active",
	}

	if err := dbFile.Insert(&user); err != nil {
		t.Fatal(err)
	}

	user.Name = "Budi Santoso"
	user.Age = 21

	updated, err := dbFile.Update(
		user.ID,
		&user,
	)
	if err != nil {
		t.Fatal(err)
	}

	if updated == nil {
		t.Fatal("expected updated record")
	}

	t.Logf(
		"updated record: %+v",
		updated,
	)

	var result User

	if err := dbFile.Find(
		user.ID,
		&result,
	); err != nil {
		t.Fatal(err)
	}

	if result.ID != user.ID {
		t.Fatalf(
			"expected ID %q, got %q",
			user.ID,
			result.ID,
		)
	}

	if result.Name != "Budi Santoso" {
		t.Fatalf(
			"expected name %q, got %q",
			"Budi Santoso",
			result.Name,
		)
	}

	if result.Age != 21 {
		t.Fatalf(
			"expected age %d, got %d",
			21,
			result.Age,
		)
	}

	if result.Status != "active" {
		t.Fatalf(
			"expected status %q, got %q",
			"active",
			result.Status,
		)
	}
}

func TestDatabaseFileUpdateWhere(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users.tacenva",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	users := []User{
		{
			Name:   "Budi",
			Age:    17,
			Status: "active",
		},
		{
			Name:   "Andi",
			Age:    20,
			Status: "active",
		},
		{
			Name:   "Siti",
			Age:    15,
			Status: "active",
		},
	}

	for i := range users {
		if err := dbFile.Insert(&users[i]); err != nil {
			t.Fatal(err)
		}
	}

	err = dbFile.UpdateWhere(
		func(data map[string]any) bool {
			age, ok := data["age"].(float64)

			return ok && age < 18
		},
		func(data map[string]any) error {
			data["status"] = "minor"

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	var result []User

	if err := dbFile.FindAll(&result); err != nil {
		t.Fatal(err)
	}

	if len(result) != 3 {
		t.Fatalf(
			"expected 3 records, got %d",
			len(result),
		)
	}

	for _, user := range result {
		t.Logf(
			"user: %+v",
			user,
		)

		if user.Age < 18 && user.Status != "minor" {
			t.Fatalf(
				"user %q should have status minor, got %q",
				user.Name,
				user.Status,
			)
		}

		if user.Age >= 18 && user.Status != "active" {
			t.Fatalf(
				"user %q should remain active, got %q",
				user.Name,
				user.Status,
			)
		}
	}
}

func TestDatabaseFileDelete(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users.tacenva",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	user := User{
		Name:   "Budi",
		Age:    20,
		Status: "active",
	}

	if err := dbFile.Insert(&user); err != nil {
		t.Fatal(err)
	}

	deleted, err := dbFile.Delete(user.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(deleted) == 0 {
		t.Fatal("expected deleted record data")
	}

	var deletedUser User

	if err := json.Unmarshal(
		deleted,
		&deletedUser,
	); err != nil {
		t.Fatal(err)
	}

	if deletedUser.ID != user.ID {
		t.Fatalf(
			"expected deleted ID %q, got %q",
			user.ID,
			deletedUser.ID,
		)
	}

	if deletedUser.Name != user.Name {
		t.Fatalf(
			"expected deleted name %q, got %q",
			user.Name,
			deletedUser.Name,
		)
	}

	t.Logf(
		"deleted user: %+v",
		deletedUser,
	)

	var result User

	err = dbFile.Find(
		user.ID,
		&result,
	)

	if err == nil {
		t.Fatal(
			"expected error when finding deleted record",
		)
	}
}

func TestDatabaseFileFindAll(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users.tacenva",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	input := []User{
		{
			Name:   "Budi",
			Age:    20,
			Status: "active",
		},
		{
			Name:   "Andi",
			Age:    25,
			Status: "active",
		},
		{
			Name:   "Siti",
			Age:    17,
			Status: "active",
		},
	}

	for i := range input {
		if err := dbFile.Insert(&input[i]); err != nil {
			t.Fatal(err)
		}
	}

	var result []User

	if err := dbFile.FindAll(&result); err != nil {
		t.Fatal(err)
	}

	if len(result) != len(input) {
		t.Fatalf(
			"expected %d records, got %d",
			len(input),
			len(result),
		)
	}

	t.Logf(
		"FindAll returned %d records",
		len(result),
	)

	for _, user := range result {
		t.Logf(
			"record: %+v",
			user,
		)
	}
}

func TestDatabaseFileFindWhere(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users.tacenva",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	input := []User{
		{
			Name:   "Budi",
			Age:    20,
			Status: "active",
		},
		{
			Name:   "Andi",
			Age:    25,
			Status: "active",
		},
		{
			Name:   "Siti",
			Age:    17,
			Status: "active",
		},
	}

	for i := range input {
		if err := dbFile.Insert(&input[i]); err != nil {
			t.Fatal(err)
		}
	}

	var result []User

	err = dbFile.FindWhere(
		&result,
		func(data map[string]any) bool {
			age, ok := data["age"].(float64)

			return ok && age >= 18
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Fatalf(
			"expected 2 records, got %d",
			len(result),
		)
	}

	t.Logf(
		"FindWhere returned %d records",
		len(result),
	)

	for _, user := range result {
		t.Logf(
			"matched record: %+v",
			user,
		)

		if user.Age < 18 {
			t.Fatalf(
				"user %q should not be included",
				user.Name,
			)
		}
	}
}

func TestDatabaseFileWrongPassword(t *testing.T) {
	baseDir := t.TempDir()

	filename := "users.tacenva"

	createTestDatabaseWithRecord(
		t,
		baseDir,
		filename,
		"correct-password",
	)

	db := New(baseDir)

	_, err := db.File(
		filename,
		"wrong-password",
	)

	if err == nil {
		t.Fatal(
			"expected error when using wrong password",
		)
	}

	t.Logf(
		"wrong password correctly rejected: %v",
		err,
	)
}

func TestDatabaseFileCorrectPassword(t *testing.T) {
	baseDir := t.TempDir()

	filename := "users.tacenva"

	createTestDatabaseWithRecord(
		t,
		baseDir,
		filename,
		"correct-password",
	)

	db := New(baseDir)

	dbFile, err := db.File(
		filename,
		"correct-password",
	)
	if err != nil {
		t.Fatal(err)
	}

	var user User

	if err := dbFile.Find(
		"test-record",
		&user,
	); err != nil {
		t.Fatal(err)
	}

	if user.ID != "test-record" {
		t.Fatalf(
			"expected ID %q, got %q",
			"test-record",
			user.ID,
		)
	}

	if user.Name != "Test" {
		t.Fatalf(
			"expected name %q, got %q",
			"Test",
			user.Name,
		)
	}

	if user.Age != 20 {
		t.Fatalf(
			"expected age %d, got %d",
			20,
			user.Age,
		)
	}

	if user.Status != "active" {
		t.Fatalf(
			"expected status %q, got %q",
			"active",
			user.Status,
		)
	}

	t.Logf(
		"correct password opened database: %+v",
		user,
	)
}

func TestDatabaseFileStorage(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users.tacenva",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	users := []User{
		{
			Name:   "Budi",
			Age:    20,
			Status: "active",
		},
		{
			Name:   "Andi",
			Age:    25,
			Status: "active",
		},
		{
			Name:   "Siti",
			Age:    17,
			Status: "active",
		},
	}

	for i := range users {
		if err := dbFile.Insert(&users[i]); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(
		baseDir,
		"users.tacenva",
	)

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var file structure.File

	if err := json.Unmarshal(
		blob,
		&file,
	); err != nil {
		t.Fatal(err)
	}

	if len(file.Records) != 3 {
		t.Fatalf(
			"expected 3 encrypted records, got %d",
			len(file.Records),
		)
	}

	t.Log("")
	t.Log("========== TACENVA FILE ==========")
	t.Logf("path: %s", path)
	t.Logf("kdf: %s", file.Algorithm.KDF)
	t.Logf("cipher: %s", file.Algorithm.Cipher)
	t.Logf(
		"kdf params: memory=%d, iterations=%d, parallelism=%d",
		file.KDF.Memory,
		file.KDF.Iterations,
		file.KDF.Parallelism,
	)
	t.Logf(
		"salt: %x",
		file.KDF.Salt,
	)
	t.Logf(
		"records: %d",
		len(file.Records),
	)

	for i, record := range file.Records {
		t.Logf(
			"record[%d].id: %s",
			i,
			record.ID,
		)
		t.Logf(
			"record[%d].data: %s",
			i,
			record.Data,
		)
	}

	t.Log("")
	t.Log("========== RAW FILE ==========")
	t.Log(string(blob))
	t.Log("================================")
	t.Log("")
}

func TestDatabaseFileExtension(t *testing.T) {
	baseDir := t.TempDir()

	db := New(baseDir)

	dbFile, err := createAndOpenTestDatabase(
		t,
		db,
		baseDir,
		"users",
		"secret",
	)
	if err != nil {
		t.Fatal(err)
	}

	user := User{
		Name:   "Budi",
		Age:    20,
		Status: "active",
	}

	if err := dbFile.Insert(&user); err != nil {
		t.Fatal(err)
	}

	expectedPath := filepath.Join(
		baseDir,
		"users.tacenva",
	)

	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf(
			"expected file %s to exist: %v",
			expectedPath,
			err,
		)
	}

	unexpectedPath := filepath.Join(
		baseDir,
		"users",
	)

	if _, err := os.Stat(unexpectedPath); err == nil {
		t.Fatalf(
			"unexpected file without extension exists: %s",
			unexpectedPath,
		)
	}

	t.Logf(
		"database stored at: %s",
		expectedPath,
	)
}
