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
//
// Example:
//
//	user := User{
//		Name: "Budi",
//		Age: 20,
//	}
//
//	err := users.Insert(&user)
//	if err != nil {
//		return err
//	}
//
//	fmt.Println(user.ID)
func (f *DatabaseFile) Insert(value any) error {
	if err := validateValue(value); err != nil {
		return err
	}

	id := ulid.Make().String()

	if err := setID(value, id); err != nil {
		return err
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	f.data[id] = raw

	if err := f.save(); err != nil {
		delete(f.data, id)

		return err
	}

	return nil
}

// Update updates a single record by its ID.
//
// The value must be a non-nil pointer to a struct.
// The ID field of value will be replaced with the given ID.
//
// Update locks the database file while the record is being
// modified and persisted.
//
// The updated value is returned after a successful update.
//
// Example:
//
//	user.Name = "Budi Santoso"
//
//	updated, err := users.Update(
//		user.ID,
//		&user,
//	)
//	if err != nil {
//		return err
//	}
//
//	fmt.Println(updated)
func (f *DatabaseFile) Update(
	id string,
	value any,
) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if id == "" {
		return nil, errors.New("id cannot be empty")
	}

	if err := validateValue(value); err != nil {
		return nil, err
	}

	if _, exists := f.data[id]; !exists {
		return nil, fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	if err := setID(value, id); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	old := f.data[id]

	f.data[id] = raw

	if err := f.save(); err != nil {
		f.data[id] = old

		return nil, err
	}

	return value, nil
}

// UpdateWhere updates all records that match the predicate.
//
// The predicate receives each record as a map[string]any.
// The updater is called only for records where the predicate
// returns true.
//
// The updater can modify the record directly.
// Returning an error stops the operation and restores
// all records changed during the operation.
//
// Example:
//
//	err := users.UpdateWhere(
//		func(data map[string]any) bool {
//			age, ok := data["age"].(float64)
//
//			return ok && age < 18
//		},
//		func(data map[string]any) error {
//			data["status"] = "minor"
//			return nil
//		},
//	)
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

		// Save the original before calling updater.
		// This makes rollback reliable if updater or marshal fails.
		if _, exists := original[id]; !exists {
			original[id] = raw
		}

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

	if err := f.save(); err != nil {
		f.data = originalData(
			original,
			f.data,
		)

		return err
	}

	return nil
}

// Delete deletes a single record by its ID.
//
// Delete locks the database file while the record is being
// removed and persisted.
//
// The deleted record is returned after a successful delete.
//
// Example:
//
//	deleted, err := users.Delete(user.ID)
//	if err != nil {
//		return err
//	}
//
//	fmt.Println(deleted)
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

	delete(f.data, id)

	if err := f.save(); err != nil {
		f.data[id] = old

		return nil, err
	}

	return old, nil
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
