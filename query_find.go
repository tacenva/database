package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// Find finds a single record by its ID and decodes it into dest.
//
// dest must be a pointer to a struct.
//
// Example:
//
//	var user User
//
//	err := users.Find(userID, &user)
//	if err != nil {
//		return err
//	}
//
//	fmt.Println(user.Name)
func (f *DatabaseFile) Find(
	id string,
	dest any,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if id == "" {
		return errors.New("id cannot be empty")
	}

	raw, exists := f.data[id]
	if !exists {
		return fmt.Errorf(
			"record %q not found",
			id,
		)
	}

	if err := validateDestination(dest); err != nil {
		return err
	}

	if err := json.Unmarshal(raw, dest); err != nil {
		return err
	}

	if err := setDestinationID(dest, id); err != nil {
		return err
	}

	return nil
}

// FindAll finds all records and decodes them into dest.
//
// dest must be a pointer to a slice of structs.
//
// Example:
//
//	var users []User
//
//	err := usersFile.FindAll(&users)
//	if err != nil {
//		return err
//	}
//
//	for _, user := range users {
//		fmt.Println(user.Name)
//	}
func (f *DatabaseFile) FindAll(
	dest any,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	v, err := validateSliceDestination(dest)
	if err != nil {
		return err
	}

	slice := v.Elem()

	for id, raw := range f.data {
		item := reflect.New(slice.Type().Elem())

		if err := json.Unmarshal(raw, item.Interface()); err != nil {
			return err
		}

		if err := setDestinationID(item.Interface(), id); err != nil {
			return err
		}

		slice.Set(
			reflect.Append(
				slice,
				item.Elem(),
			),
		)
	}

	return nil
}

// FindWhere finds all records that match the given predicate
// and decodes them into dest.
//
// The predicate receives each record as a map[string]any.
// It must return true for records that should be included.
//
// dest must be a pointer to a slice of structs.
//
// Example:
//
//	var users []User
//
//	err := usersFile.FindWhere(
//		&users,
//		func(data map[string]any) bool {
//			age, ok := data["age"].(float64)
//
//			return ok && age >= 18
//		},
//	)
//	if err != nil {
//		return err
//	}
//
//	for _, user := range users {
//		fmt.Println(user.Name)
//	}
func (f *DatabaseFile) FindWhere(
	dest any,
	predicate func(map[string]any) bool,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if predicate == nil {
		return errors.New("predicate cannot be nil")
	}

	v, err := validateSliceDestination(dest)
	if err != nil {
		return err
	}

	slice := v.Elem()

	for id, raw := range f.data {
		var data map[string]any

		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}

		if !predicate(data) {
			continue
		}

		item := reflect.New(slice.Type().Elem())

		if err := json.Unmarshal(raw, item.Interface()); err != nil {
			return err
		}

		if err := setDestinationID(item.Interface(), id); err != nil {
			return err
		}

		slice.Set(
			reflect.Append(
				slice,
				item.Elem(),
			),
		)
	}

	return nil
}

// validateDestination validates that dest is a non-nil pointer
// to a struct.
func validateDestination(dest any) error {
	if dest == nil {
		return errors.New("destination cannot be nil")
	}

	v := reflect.ValueOf(dest)

	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New(
			"destination must be a non-nil pointer",
		)
	}

	if v.Elem().Kind() != reflect.Struct {
		return errors.New(
			"destination must point to a struct",
		)
	}

	return nil
}

// validateSliceDestination validates that dest is a non-nil pointer
// to a slice containing structs.
func validateSliceDestination(dest any) (reflect.Value, error) {
	if dest == nil {
		return reflect.Value{}, errors.New(
			"destination cannot be nil",
		)
	}

	v := reflect.ValueOf(dest)

	if v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}, errors.New(
			"destination must be a non-nil pointer",
		)
	}

	if v.Elem().Kind() != reflect.Slice {
		return reflect.Value{}, errors.New(
			"destination must point to a slice",
		)
	}

	slice := v.Elem()

	if slice.Type().Elem().Kind() != reflect.Struct {
		return reflect.Value{}, errors.New(
			"destination slice must contain structs",
		)
	}

	return v, nil
}

// setDestinationID sets the record ID from the database map key.
//
// The ID is stored separately from the JSON data, so this restores
// the ID after the record has been decoded from JSON.
//
// The destination must be a non-nil pointer to a struct
// containing a settable string field named ID.
func setDestinationID(
	dest any,
	id string,
) error {
	if id == "" {
		return errors.New("id cannot be empty")
	}

	v := reflect.ValueOf(dest)

	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New(
			"destination must be a non-nil pointer",
		)
	}

	v = v.Elem()

	if v.Kind() != reflect.Struct {
		return errors.New(
			"destination must point to a struct",
		)
	}

	field := v.FieldByName("ID")

	if !field.IsValid() {
		return errors.New(
			"destination must contain an ID field",
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
