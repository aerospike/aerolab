package inslice

import (
	"errors"
	"reflect"
)

// Reflect performs a comparison on interface type (interface must be of type slice - i.e. slice of anything).
// The item and/or slice may be pointers to values (e.g. []*int and *int) or any mix of those. Only actual values are evaluated.
// May be slow compared to other functions.
func Reflect(slice interface{}, item interface{}, count int) (index []int, err error) {
	if count == 0 {
		return
	}
	if reflect.TypeOf(item).Kind() == reflect.Ptr {
		item = reflect.Indirect(reflect.ValueOf(item)).Interface()
	}
	switch reflect.TypeOf(slice).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(slice)

		for i := 0; i < s.Len(); i++ {
			value := s.Index(i)
			valueInterface := value.Interface()
			if reflect.TypeOf(valueInterface).Kind() == reflect.Ptr {
				valueInterface = reflect.Indirect(reflect.ValueOf(valueInterface)).Interface()
			}
			if reflect.DeepEqual(valueInterface, item) {
				index = append(index, i)
			}
			if count > 0 && len(index) == count {
				return
			}
		}
	default:
		err = errors.New("slice interface is not a slice")
	}
	return
}

// ReflectAll works like Reflect, but returns all items.
func ReflectAll(slice interface{}, item interface{}) (index []int, err error) {
	return Reflect(slice, item, -1)
}
