package database

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/oklog/ulid/v2"
)

type MutationResult struct {
	ID       string
	Inserted int
	Updated  int
	Deleted  int
	Total    int
	Error    error
}

func (r *MutationResult) addInserted() {
	r.Inserted++
	r.Total++
}

func (r *MutationResult) addUpdated() {
	r.Updated++
	r.Total++
}

func (r *MutationResult) addDeleted() {
	r.Deleted++
	r.Total++
}

func mutationError(err error) MutationResult {
	return MutationResult{Error: err}
}

func sameRecord(
	old json.RawMessage,
	updated json.RawMessage,
) (bool, error) {
	var oldData map[string]any
	var updatedData map[string]any

	if err := json.Unmarshal(old, &oldData); err != nil {
		return false, err
	}

	if err := json.Unmarshal(updated, &updatedData); err != nil {
		return false, err
	}

	delete(oldData, "updated_at")
	delete(updatedData, "updated_at")

	oldJSON, err := json.Marshal(oldData)
	if err != nil {
		return false, err
	}

	updatedJSON, err := json.Marshal(updatedData)
	if err != nil {
		return false, err
	}

	return bytes.Equal(oldJSON, updatedJSON), nil
}

// Insert adds a new record to the database.
//
// The record ID is generated automatically using ULID.
// The value must be a non-nil pointer to a struct
// containing a settable string field named ID.
func (f *DatabaseFile) Insert(value any) MutationResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := validateValue(value); err != nil {
		return mutationError(err)
	}

	id, err := getID(value)
	if err != nil {
		return mutationError(err)
	}

	if id == "" {
		id = ulid.Make().String()

		if err := setID(value, id); err != nil {
			return mutationError(err)
		}
	}

	if err := setTimestamps(value, false); err != nil {
		return mutationError(err)
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return mutationError(err)
	}

	oldVersion := f.file.Version
	old, existed := f.data[id]

	result := MutationResult{ID: id}

	if existed {
		same, err := sameRecord(old, raw)
		if err != nil {
			return mutationError(err)
		}

		if same {
			return result
		}

		result.addUpdated()
	} else {
		result.addInserted()
	}

	f.data[id] = raw
	f.file.Version++

	if err := f.save(); err != nil {
		if existed {
			f.data[id] = old
		} else {
			delete(f.data, id)
		}

		f.file.Version = oldVersion
		return mutationError(err)
	}

	return result
}

// Update updates a single record by its ID.
//
// The value must be a non-nil pointer to a struct.
// The ID field of value is used as the record ID.
func (f *DatabaseFile) Update(value any) MutationResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := validateValue(value); err != nil {
		return mutationError(err)
	}

	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return mutationError(
			errors.New("value must be a non-nil pointer"),
		)
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return mutationError(
			errors.New("value must be a pointer to struct"),
		)
	}

	idField := v.FieldByName("ID")
	if !idField.IsValid() || idField.Kind() != reflect.String {
		return mutationError(
			errors.New("value must contain a string ID field"),
		)
	}

	id := idField.String()
	if id == "" {
		return mutationError(ErrNullID)
	}

	old, exists := f.data[id]
	if !exists {
		return mutationError(ErrRecordNotFound)
	}

	// Bandingkan sebelum mengubah timestamp.
	raw, err := json.Marshal(value)
	if err != nil {
		return mutationError(err)
	}

	same, err := sameRecord(old, raw)
	if err != nil {
		return mutationError(err)
	}

	if same {
		return MutationResult{ID: id}
	}

	if err := setTimestamps(value, true); err != nil {
		return mutationError(err)
	}

	raw, err = json.Marshal(value)
	if err != nil {
		return mutationError(err)
	}

	oldVersion := f.file.Version

	f.data[id] = raw
	f.file.Version++

	if err := f.save(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion
		return mutationError(err)
	}

	result := MutationResult{ID: id}
	result.addUpdated()

	return result
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
) MutationResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	if predicate == nil {
		return mutationError(errors.New("predicate cannot be nil"))
	}

	if updater == nil {
		return mutationError(errors.New("updater cannot be nil"))
	}

	original := make(map[string]json.RawMessage)
	result := MutationResult{}

	for id, raw := range f.data {
		var data map[string]any

		if err := json.Unmarshal(raw, &data); err != nil {
			restoreData(f.data, original)
			return mutationError(err)
		}

		if !predicate(data) {
			continue
		}

		if err := updater(data); err != nil {
			restoreData(f.data, original)
			return mutationError(err)
		}

		setMapTimestamps(data, true)

		updated, err := json.Marshal(data)
		if err != nil {
			restoreData(f.data, original)
			return mutationError(err)
		}

		same, err := sameRecord(raw, updated)
		if err != nil {
			restoreData(f.data, original)
			return mutationError(err)
		}

		if same {
			continue
		}

		original[id] = raw
		f.data[id] = updated
		result.addUpdated()
	}

	if result.Total == 0 {
		return result
	}

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		restoreData(f.data, original)
		f.file.Version = oldVersion
		return mutationError(err)
	}

	return result
}

// Delete deletes a single record by its ID.
//
// The deleted record is returned after a successful delete.
func (f *DatabaseFile) Delete(id string) MutationResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	if id == "" {
		return mutationError(ErrNullID)
	}

	old, exists := f.data[id]
	if !exists {
		return MutationResult{}
	}

	oldVersion := f.file.Version

	delete(f.data, id)
	f.file.Version++

	if err := f.save(); err != nil {
		f.data[id] = old
		f.file.Version = oldVersion
		return mutationError(err)
	}

	result := MutationResult{ID: id}
	result.addDeleted()

	return result
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
func (f *DatabaseFile) UpdateOrCreateBulk(values any) MutationResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	if values == nil {
		return mutationError(errors.New("values cannot be nil"))
	}

	v := reflect.ValueOf(values)

	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return mutationError(
			errors.New("values must be a slice or array"),
		)
	}

	original := make(map[string]json.RawMessage)
	result := MutationResult{}

	for i := 0; i < v.Len(); i++ {
		value := v.Index(i)

		if value.Kind() != reflect.Ptr || value.IsNil() {
			restoreData(f.data, original)
			return mutationError(fmt.Errorf(
				"value at index %d must be a non-nil pointer", i,
			))
		}

		if err := validateValue(value.Interface()); err != nil {
			restoreData(f.data, original)
			return mutationError(fmt.Errorf(
				"value at index %d: %w", i, err,
			))
		}

		structValue := value.Elem()
		idField := structValue.FieldByName("ID")

		if !idField.IsValid() || idField.Kind() != reflect.String {
			restoreData(f.data, original)
			return mutationError(fmt.Errorf(
				"value at index %d: ID field must be a string", i,
			))
		}

		id := idField.String()

		if id == "" {
			id = ulid.Make().String()
			idField.SetString(id)
		}

		old, existed := f.data[id]

		if err := setTimestamps(value.Interface(), !existed); err != nil {
			restoreData(f.data, original)
			return mutationError(err)
		}

		raw, err := json.Marshal(value.Interface())
		if err != nil {
			restoreData(f.data, original)
			return mutationError(fmt.Errorf(
				"failed to marshal record %q: %w", id, err,
			))
		}

		if existed {
			same, err := sameRecord(old, raw)
			if err != nil {
				restoreData(f.data, original)
				return mutationError(err)
			}

			if same {
				continue
			}
		}

		if _, tracked := original[id]; !tracked {
			if existed {
				original[id] = old
			} else {
				original[id] = nil
			}
		}

		f.data[id] = raw

		if existed {
			result.Updated++
		} else {
			result.Inserted++
		}
		result.Total++
	}

	if result.Total == 0 {
		return result
	}

	oldVersion := f.file.Version
	f.file.Version++

	if err := f.save(); err != nil {
		restoreData(f.data, original)
		f.file.Version = oldVersion
		return mutationError(err)
	}

	return result
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

func setTimestamps(value any, updateExisting bool) error {
	v := reflect.ValueOf(value)

	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("value must be a non-nil pointer")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("value must be a pointer to struct")
	}

	now := time.Now()

	for _, name := range []string{"CreatedAt", "UpdatedAt"} {
		field := v.FieldByName(name)

		if !field.IsValid() || !field.CanSet() ||
			field.Type() != reflect.TypeOf(time.Time{}) {
			continue
		}

		if name == "UpdatedAt" && updateExisting {
			field.Set(reflect.ValueOf(now))
			continue
		}

		if field.IsZero() {
			field.Set(reflect.ValueOf(now))
		}
	}

	return nil
}

func setMapTimestamps(data map[string]any, updateExisting bool) {
	now := time.Now()

	if createdAt, exists := data["created_at"]; exists {
		if createdAt == nil || createdAt == "" {
			data["created_at"] = now
		}
	} else {
		data["created_at"] = now
	}

	if updateExisting {
		data["updated_at"] = now
	} else if updatedAt, exists := data["updated_at"]; !exists ||
		updatedAt == nil || updatedAt == "" {
		data["updated_at"] = now
	}
}
