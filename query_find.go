package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

var (
	ErrRecordNotFound = errors.New("Record Not Found")
	ErrNullID         = errors.New("id cannot be empty")
)

type Query struct {
	db     *DatabaseFile
	dest   any
	orders []OrderClause
	Err    error
}

type OrderClause struct {
	Field string
	Desc  bool
}

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
		return ErrNullID
	}

	raw, exists := f.data[id]
	if !exists {
		return ErrRecordNotFound
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
func (f *DatabaseFile) FindAll(dest any) *Query {
	q := &Query{
		db:   f,
		dest: dest,
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	v, err := validateSliceDestination(dest)
	if err != nil {
		q.Err = err
		return q
	}

	slice := v.Elem()
	slice.Set(reflect.MakeSlice(slice.Type(), 0, len(f.data)))

	for id, raw := range f.data {
		item := reflect.New(slice.Type().Elem())

		if err := json.Unmarshal(raw, item.Interface()); err != nil {
			q.Err = err
			return q
		}

		if err := setDestinationID(item.Interface(), id); err != nil {
			q.Err = err
			return q
		}

		slice.Set(reflect.Append(slice, item.Elem()))
	}

	return q
}

func (q *Query) Order(clause string) *Query {
	if q.Err != nil {
		return q
	}

	for _, part := range strings.Split(clause, ",") {
		fields := strings.Fields(strings.TrimSpace(part))

		if len(fields) < 1 || len(fields) > 2 {
			q.Err = fmt.Errorf("invalid order clause: %q", part)
			return q
		}

		order := OrderClause{Field: fields[0]}

		if len(fields) == 2 {
			switch strings.ToUpper(fields[1]) {
			case "ASC":
			case "DESC":
				order.Desc = true
			default:
				q.Err = fmt.Errorf(
					"invalid order direction: %q", fields[1],
				)
				return q
			}
		}

		q.orders = append(q.orders, order)
	}

	if err := q.applyOrders(); err != nil {
		q.Err = err
	}

	return q
}

func (q *Query) applyOrders() error {
	v := reflect.ValueOf(q.dest)

	if v.Kind() != reflect.Pointer ||
		v.IsNil() ||
		v.Elem().Kind() != reflect.Slice {
		return errors.New("destination must be a pointer to a slice")
	}

	slice := v.Elem()
	timeType := reflect.TypeOf(time.Time{})

	for _, order := range q.orders {
		if order.Field == "id" || order.Field == "ID" {
			continue // ID ditangani sebagai field struct oleh setDestinationID.
		}

		for i := 0; i < slice.Len(); i++ {
			if !slice.Index(i).FieldByName(order.Field).IsValid() {
				return fmt.Errorf("field %q not found", order.Field)
			}
		}
	}

	sort.SliceStable(slice.Interface(), func(i, j int) bool {
		a := slice.Index(i)
		b := slice.Index(j)

		for _, order := range q.orders {
			var cmp int

			if order.Field == "id" || order.Field == "ID" {
				cmp = strings.Compare(
					a.FieldByName("ID").String(),
					b.FieldByName("ID").String(),
				)
			} else {
				fa := a.FieldByName(order.Field)
				fb := b.FieldByName(order.Field)

				cmp = compareOrderValues(fa, fb, timeType)
			}

			if cmp == 0 {
				continue
			}

			if order.Desc {
				return cmp > 0
			}

			return cmp < 0
		}

		return false
	})

	return nil
}

func compareOrderValues(
	a, b reflect.Value,
	timeType reflect.Type,
) int {
	if !a.IsValid() || !b.IsValid() || a.Type() != b.Type() {
		return 0
	}

	switch a.Kind() {
	case reflect.String:
		return strings.Compare(a.String(), b.String())

	case reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64:
		if a.Int() < b.Int() {
			return -1
		}
		if a.Int() > b.Int() {
			return 1
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64:
		if a.Uint() < b.Uint() {
			return -1
		}
		if a.Uint() > b.Uint() {
			return 1
		}

	case reflect.Float32, reflect.Float64:
		if a.Float() < b.Float() {
			return -1
		}
		if a.Float() > b.Float() {
			return 1
		}

	case reflect.Struct:
		if a.Type() == timeType {
			ta := a.Interface().(time.Time)
			tb := b.Interface().(time.Time)

			if ta.Before(tb) {
				return -1
			}
			if ta.After(tb) {
				return 1
			}
		}
	}

	return 0
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
) *Query {
	q := &Query{
		db:   f,
		dest: dest,
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if predicate == nil {
		q.Err = errors.New("predicate cannot be nil")
		return q
	}

	v, err := validateSliceDestination(dest)
	if err != nil {
		q.Err = err
		return q
	}

	slice := v.Elem()
	slice.Set(reflect.MakeSlice(slice.Type(), 0, 0))

	for id, raw := range f.data {
		var data map[string]any

		if err := json.Unmarshal(raw, &data); err != nil {
			q.Err = err
			return q
		}

		if !predicate(data) {
			continue
		}

		item := reflect.New(slice.Type().Elem())

		if err := json.Unmarshal(raw, item.Interface()); err != nil {
			q.Err = err
			return q
		}

		if err := setDestinationID(item.Interface(), id); err != nil {
			q.Err = err
			return q
		}

		slice.Set(reflect.Append(slice, item.Elem()))
	}

	return q
}

func (f *DatabaseFile) FindFirstWhere(
	dest any,
	predicate func(map[string]any) bool,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if predicate == nil {
		return errors.New("predicate cannot be nil")
	}

	for id, raw := range f.data {
		var data map[string]any

		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}

		if !predicate(data) {
			continue
		}

		if err := json.Unmarshal(raw, dest); err != nil {
			return err
		}

		if err := setDestinationID(dest, id); err != nil {
			return err
		}
		return nil
	}
	return ErrRecordNotFound
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
		return ErrNullID
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
