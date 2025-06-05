package structtags

import (
	"fmt"
	"reflect"
	"time"
)

// recursively goes through the struct s and check if all fields that have the `required` tag set are not empty/nil
// for pointers, the item is considered empty if the pointer is nil
// for slices, it is considered to be empty if slice is nil (len=0)
// for maps, it is considered to be empty if map is nil (len=0)
// for strings (not pointers), it is considered to be empty if string is empty (len=0)
// for ints and floats (not pointers), it is considered to be empty if number is 0
// for time.Time (not pointers), it is considered to be empty if time is zero value (time.IsZero() check)
// for other types, it is considered to be empty if the value is nil
// checks underlying type, just just main type, for exmple, if we have `type bob int`, it will perform int-check for if it's empty
func CheckRequired(s any) error {
	val := reflect.ValueOf(s)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("input must be a struct")
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		if fieldType.Tag.Get("required") != "" {
			if err := checkEmpty(field); err != nil {
				return fmt.Errorf("field %s is required but empty: %v", fieldType.Name, err)
			}
		}

		// recursively check nested structs
		if field.Type() != reflect.TypeOf(time.Time{}) {
			if field.Kind() == reflect.Struct {
				if err := CheckRequired(field.Interface()); err != nil {
					return err
				}
			} else if field.Kind() == reflect.Ptr && !field.IsNil() && field.Elem().Kind() == reflect.Struct {
				if err := CheckRequired(field.Interface()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkEmpty(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return fmt.Errorf("nil value")
		}
	case reflect.Slice, reflect.Map:
		if v.IsNil() || v.Len() == 0 {
			return fmt.Errorf("empty collection")
		}
	case reflect.String:
		if v.String() == "" {
			return fmt.Errorf("empty string")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() == 0 {
			return fmt.Errorf("zero integer")
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Uint() == 0 {
			return fmt.Errorf("zero unsigned integer")
		}
	case reflect.Float32, reflect.Float64:
		if v.Float() == 0 {
			return fmt.Errorf("zero float")
		}
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			if v.Interface().(time.Time).IsZero() {
				return fmt.Errorf("zero time")
			}
		}
	}
	return nil
}
