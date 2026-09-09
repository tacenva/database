package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/oklog/ulid/v2"
)

// Insert adds a new record to the database.
//
// The record ID is generated automatically using ULID.
// The value must be a non-nil pointer to a struct
// containing a settable string field named ID.
func (f *DatabaseFile) Insert(value any) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := validateValue(value); err != nil {
		return "", err
	}

	id, err := getID(value)
	if err != nil {
		return "", err
	}

	if id == "" {
		id = ulid.Make().String()

		if err := setID(value, id); err != nil {
			return "", err
		}
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	f.data[id] = raw

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		delete(f.data, id)
		f.file.Version = oldVersion

		return "", err
	}

	return id, nil
}

// Update updates a single record by its ID.
//
// The value must be a non-nil pointer to a struct.
// The ID field of value is used as the record ID.
func (f *DatabaseFile) Update(value any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := validateValue(value); err != nil {
		return err
	}

	v := reflect.ValueOf(value)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	id := v.FieldByName("ID").String()

	if id == "" {
		return errors.New("id cannot be empty")
	}

	if _, exists := f.data[id]; !exists {
		return fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	old := f.data[id]
	oldVersion := f.file.Version

	f.data[id] = raw
	f.file.Version++

	if err := f.save(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion

		return err
	}

	return nil
}

// UpdateWhere updates all records that match the predicate.
//
// The predicate receives each record as a map[string]any.
// The updater is called only for records where the predicate
// returns true.
//
// If no record is changed, the version is not incremented.
func (f *DatabaseFile) UpdateWhere(
	predicate func(map[string]any) bool,
	updater func(map[string]any) error,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if predicate == nil {
		return errors.New("predicate cannot be nil")
	}

	if updater == nil {
		return errors.New("updater cannot be nil")
	}

	original := make(map[string]json.RawMessage)

	for id, raw := range f.data {
		var data map[string]any

		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}

		if !predicate(data) {
			continue
		}

		original[id] = raw

		if err := updater(data); err != nil {
			f.data = originalData(
				original,
				f.data,
			)

			return err
		}

		updated, err := json.Marshal(data)
		if err != nil {
			f.data = originalData(
				original,
				f.data,
			)

			return err
		}

		f.data[id] = updated
	}

	if len(original) == 0 {
		return nil
	}

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		f.data = originalData(
			original,
			f.data,
		)

		f.file.Version = oldVersion

		return err
	}

	return nil
}

// Delete deletes a single record by its ID.
//
// The deleted record is returned after a successful delete.
func (f *DatabaseFile) Delete(
	id string,
) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if id == "" {
		return nil, errors.New("id cannot be empty")
	}

	old, exists := f.data[id]
	if !exists {
		return nil, fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	oldVersion := f.file.Version

	delete(f.data, id)
	f.file.Version++

	if err := f.save(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion

		return nil, err
	}

	return old, nil
}

// UpdateOrCreateBulk inserts or updates multiple records
// in a single operation.
//
// Records with an empty ID will receive a new ULID.
// Records with an existing ID will be updated if they already exist.
// Records with a non-empty ID that do not exist will be created
// using the provided ID.
//
// The version is incremented once for the entire operation.
func (f *DatabaseFile) UpdateOrCreateBulk(values any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if values == nil {
		return errors.New("values cannot be nil")
	}

	v := reflect.ValueOf(values)

	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return errors.New("values must be a slice or array")
	}

	original := make(map[string]json.RawMessage)

	for i := 0; i < v.Len(); i++ {
		value := v.Index(i)

		if value.Kind() != reflect.Pointer || value.IsNil() {
			return fmt.Errorf(
				"value at index %d must be a non-nil pointer",
				i,
			)
		}

		if err := validateValue(value.Interface()); err != nil {
			return fmt.Errorf(
				"value at index %d: %w",
				i,
				err,
			)
		}

		structValue := value.Elem()
		idField := structValue.FieldByName("ID")

		if !idField.IsValid() {
			return fmt.Errorf(
				"value at index %d: value must contain an ID field",
				i,
			)
		}

		if idField.Kind() != reflect.String {
			return fmt.Errorf(
				"value at index %d: ID field must be a string",
				i,
			)
		}

		id := idField.String()

		if id == "" {
			id = ulid.Make().String()
			idField.SetString(id)
		}

		if _, exists := original[id]; !exists {
			if old, exists := f.data[id]; exists {
				original[id] = old
			} else {
				original[id] = nil
			}
		}

		raw, err := json.Marshal(value.Interface())
		if err != nil {
			restoreData(f.data, original)

			return fmt.Errorf(
				"failed to marshal record %q: %w",
				id,
				err,
			)
		}

		f.data[id] = raw
	}

	if len(original) == 0 {
		return nil
	}

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		restoreData(f.data, original)
		f.file.Version = oldVersion

		return err
	}

	return nil
}

// Sync synchronizes the database with the provided records.
//
// Records that exist in the source will be created or updated.
// Records that exist in the database but are missing from the source
// will be deleted.
//
// The version is incremented once for the entire operation.
func (f *DatabaseFile) Sync(values any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if values == nil {
		return errors.New("values cannot be nil")
	}

	v := reflect.ValueOf(values)

	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return errors.New("values must be a slice or array")
	}

	// Keep the complete original state for rollback.
	original := make(
		map[string]json.RawMessage,
		len(f.data),
	)

	for id, raw := range f.data {
		original[id] = raw
	}

	// Build the new database state separately.
	// This prevents the current database from being modified
	// until all input records have been validated and encoded.
	next := make(
		map[string]json.RawMessage,
		v.Len(),
	)

	for i := 0; i < v.Len(); i++ {
		value := v.Index(i)

		if value.Kind() != reflect.Pointer || value.IsNil() {
			return fmt.Errorf(
				"value at index %d must be a non-nil pointer",
				i,
			)
		}

		if err := validateValue(value.Interface()); err != nil {
			return fmt.Errorf(
				"value at index %d: %w",
				i,
				err,
			)
		}

		structValue := value.Elem()
		idField := structValue.FieldByName("ID")

		if !idField.IsValid() {
			return fmt.Errorf(
				"value at index %d: value must contain an ID field",
				i,
			)
		}

		if idField.Kind() != reflect.String {
			return fmt.Errorf(
				"value at index %d: ID field must be a string",
				i,
			)
		}

		id := idField.String()

		if id == "" {
			id = ulid.Make().String()
			idField.SetString(id)
		}

		if _, exists := next[id]; exists {
			return fmt.Errorf(
				"duplicate record ID %q",
				id,
			)
		}

		raw, err := json.Marshal(value.Interface())
		if err != nil {
			return fmt.Errorf(
				"failed to marshal record %q: %w",
				id,
				err,
			)
		}

		next[id] = raw
	}

	// Replace the current database with the synchronized state.
	f.data = next

	// Sync is one logical mutation.
	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		f.data = original
		f.file.Version = oldVersion

		return err
	}

	return nil
}

func originalData(
	original map[string]json.RawMessage,
	current map[string]json.RawMessage,
) map[string]json.RawMessage {
	for id, raw := range original {
		current[id] = raw
	}

	return current
}

func restoreData(
	data map[string]json.RawMessage,
	original map[string]json.RawMessage,
) {
	for id, raw := range original {
		if raw == nil {
			delete(data, id)
			continue
		}

		data[id] = raw
	}
}

func validateValue(value any) error {
	if value == nil {
		return errors.New("value cannot be nil")
	}

	v := reflect.ValueOf(value)

	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New(
			"value must be a non-nil pointer",
		)
	}

	v = v.Elem()

	if v.Kind() != reflect.Struct {
		return errors.New(
			"value must point to a struct",
		)
	}

	return nil
}

func getID(value any) (string, error) {
	v := reflect.ValueOf(value)

	if v.Kind() != reflect.Ptr {
		return "", fmt.Errorf(
			"value must be a pointer",
		)
	}

	if v.IsNil() {
		return "", fmt.Errorf(
			"value must not be nil",
		)
	}

	v = v.Elem()

	if v.Kind() != reflect.Struct {
		return "", fmt.Errorf(
			"value must be a struct pointer",
		)
	}

	field := v.FieldByName("ID")

	if !field.IsValid() {
		return "", fmt.Errorf(
			"value has no ID field",
		)
	}

	if field.Kind() != reflect.String {
		return "", fmt.Errorf(
			"ID field must be a string",
		)
	}

	return field.String(), nil
}

func setID(
	value any,
	id string,
) error {
	v := reflect.ValueOf(value)

	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New(
			"value must be a non-nil pointer",
		)
	}

	v = v.Elem()

	if v.Kind() != reflect.Struct {
		return errors.New(
			"value must point to a struct",
		)
	}

	field := v.FieldByName("ID")

	if !field.IsValid() {
		return errors.New(
			"value must contain an ID field",
		)
	}

	if !field.CanSet() {
		return errors.New(
			"ID field cannot be set",
		)
	}

	if field.Kind() != reflect.String {
		return errors.New(
			"ID field must be a string",
		)
	}

	field.SetString(id)

	return nil
}
