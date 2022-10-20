package main

import (
	"reflect"
)

func (c *restCmd) resetBools() {
	keyField := reflect.ValueOf(a.opts).Elem()
	c.resetBoolsDo(keyField, "", "")
}

func (c *restCmd) resetBoolsDo(keyField reflect.Value, start string, tags reflect.StructTag) {
	switch keyField.Type().Kind() {
	case reflect.Bool:
		keyField.SetBool(false)
	case reflect.Struct:
		for i := 0; i < keyField.NumField(); i++ {
			fieldName := keyField.Type().Field(i).Name
			fieldTag := keyField.Type().Field(i).Tag
			if len(fieldName) > 0 && fieldName[0] >= 97 && fieldName[0] <= 122 {
				if keyField.Field(i).Type().Kind() != reflect.Struct {
					continue
				}
				c.resetBoolsDo(keyField.Field(i), start, fieldTag)
			}
			if len(fieldName) == 0 || fieldName[0] < 65 || fieldName[0] > 90 {
				continue
			}
			if start != "" {
				fieldName = start + "." + fieldName
			}
			c.resetBoolsDo(keyField.Field(i), fieldName, fieldTag)
		}
	}
}
