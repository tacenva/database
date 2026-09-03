package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tacenva/database/internal/crypto"
	"github.com/tacenva/database/internal/structure"
)

type TestUser struct {
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

	cryptoService := crypto.NewCrypto()

	salt := []byte("test-salt-123456")

	key := cryptoService.DeriveKey(
		password,
		salt,
	)

	data := map[string]json.RawMessage{}

	plain, err := json.Marshal(data)
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
		Data: encrypted,
	}

	blob, err := json.MarshalIndent(
		file,
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(
		baseDir,
		filename,
	)

	if err := os.WriteFile(
		path,
		blob,
		0600,
	); err != nil {
		t.Fatal(err)
	}
}

func TestDBFile(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	if users == nil {
		t.Fatal("expected database file, got nil")
	}
}

func TestDatabaseFileInsert(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	user := TestUser{
		Name: "Budi",
		Age:  20,
	}

	if err := users.Insert(&user); err != nil {
		t.Fatal(err)
	}

	if user.ID == "" {
		t.Fatal("expected ID to be generated")
	}

	if _, exists := users.data[user.ID]; !exists {
		t.Fatal("expected user to be stored")
	}
}

func TestDatabaseFileFind(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	user := TestUser{
		Name: "Budi",
		Age:  20,
	}

	if err := users.Insert(&user); err != nil {
		t.Fatal(err)
	}

	var result TestUser

	if err := users.Find(
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

	if result.Name != "Budi" {
		t.Fatalf(
			"expected name %q, got %q",
			"Budi",
			result.Name,
		)
	}

	if result.Age != 20 {
		t.Fatalf(
			"expected age %d, got %d",
			20,
			result.Age,
		)
	}
}

func TestDatabaseFileFindAll(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	input := []TestUser{
		{
			Name: "Budi",
			Age:  20,
		},
		{
			Name: "Andi",
			Age:  25,
		},
		{
			Name: "Siti",
			Age:  30,
		},
	}

	for i := range input {
		if err := users.Insert(&input[i]); err != nil {
			t.Fatal(err)
		}
	}

	var result []TestUser

	if err := users.FindAll(&result); err != nil {
		t.Fatal(err)
	}

	if len(result) != len(input) {
		t.Fatalf(
			"expected %d users, got %d",
			len(input),
			len(result),
		)
	}
}

func TestDatabaseFileFindWhere(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	input := []TestUser{
		{
			Name: "Budi",
			Age:  17,
		},
		{
			Name: "Andi",
			Age:  20,
		},
		{
			Name: "Siti",
			Age:  25,
		},
	}

	for i := range input {
		if err := users.Insert(&input[i]); err != nil {
			t.Fatal(err)
		}
	}

	var result []TestUser

	if err := users.FindWhere(
		&result,
		func(data map[string]any) bool {
			age, ok := data["age"].(float64)

			return ok && age >= 18
		},
	); err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Fatalf(
			"expected 2 users, got %d",
			len(result),
		)
	}
}

func TestDatabaseFileUpdate(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	user := TestUser{
		Name: "Budi",
		Age:  20,
	}

	if err := users.Insert(&user); err != nil {
		t.Fatal(err)
	}

	user.Name = "Budi Santoso"
	user.Age = 21

	if err := users.Update(
		user.ID,
		&user,
	); err != nil {
		t.Fatal(err)
	}

	var result TestUser

	if err := users.Find(
		user.ID,
		&result,
	); err != nil {
		t.Fatal(err)
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
}

func TestDatabaseFileUpdateWhere(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	input := []TestUser{
		{
			Name: "Budi",
			Age:  17,
		},
		{
			Name: "Andi",
			Age:  20,
		},
		{
			Name: "Siti",
			Age:  15,
		},
	}

	for i := range input {
		if err := users.Insert(&input[i]); err != nil {
			t.Fatal(err)
		}
	}

	if err := users.UpdateWhere(
		func(data map[string]any) bool {
			age, ok := data["age"].(float64)

			return ok && age < 18
		},
		func(data map[string]any) error {
			data["status"] = "minor"
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}

	var result []TestUser

	if err := users.FindWhere(
		&result,
		func(data map[string]any) bool {
			status, ok := data["status"].(string)

			return ok && status == "minor"
		},
	); err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Fatalf(
			"expected 2 minor users, got %d",
			len(result),
		)
	}
}

func TestDatabaseFileDelete(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	user := TestUser{
		Name: "Budi",
		Age:  20,
	}

	if err := users.Insert(&user); err != nil {
		t.Fatal(err)
	}

	if err := users.Delete(user.ID); err != nil {
		t.Fatal(err)
	}

	if _, exists := users.data[user.ID]; exists {
		t.Fatal("expected user to be deleted")
	}

	var result TestUser

	if err := users.Find(
		user.ID,
		&result,
	); err == nil {
		t.Fatal("expected find to fail after delete")
	}
}

func TestDatabaseFilePersistence(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"
	password := "password"

	createTestDatabase(
		t,
		baseDir,
		filename,
		password,
	)

	db := New(baseDir)

	users, err := db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	user := TestUser{
		Name: "Budi",
		Age:  20,
	}

	if err := users.Insert(&user); err != nil {
		t.Fatal(err)
	}

	// Open the same database again to verify
	// that the record was persisted to disk.
	users, err = db.File(
		filename,
		password,
	)
	if err != nil {
		t.Fatal(err)
	}

	var result TestUser

	if err := users.Find(
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

	if result.Name != "Budi" {
		t.Fatalf(
			"expected name %q, got %q",
			"Budi",
			result.Name,
		)
	}

	if result.Age != 20 {
		t.Fatalf(
			"expected age %d, got %d",
			20,
			result.Age,
		)
	}
}

func TestDatabaseFileWrongPassword(t *testing.T) {
	baseDir := t.TempDir()
	filename := "users.json"

	createTestDatabase(
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
		t.Fatal("expected error when using wrong password")
	}
}
