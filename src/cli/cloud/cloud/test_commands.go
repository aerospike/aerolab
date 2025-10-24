package cloud

import (
	"fmt"
	"reflect"
)

// Test function to debug command discovery
func TestCommandDiscovery() {
	opts := &Options{}
	optsValue := reflect.ValueOf(opts).Elem()
	optsType := optsValue.Type()

	fmt.Printf("Options struct has %d fields\n", optsType.NumField())

	for i := 0; i < optsType.NumField(); i++ {
		field := optsType.Field(i)
		fieldValue := optsValue.Field(i)

		fmt.Printf("Field %d: %s (kind: %s)\n", i, field.Name, fieldValue.Kind())
		fmt.Printf("  Command tag: '%s'\n", field.Tag.Get("command"))
		fmt.Printf("  Description tag: '%s'\n", field.Tag.Get("description"))

		if field.Tag.Get("command") != "" {
			commandName := field.Tag.Get("command")
			description := field.Tag.Get("description")
			fmt.Printf("  -> Found command: %s - %s\n", commandName, description)

			if fieldValue.Kind() == reflect.Struct {
				fmt.Printf("  -> This is a struct, processing subcommands...\n")
				processStruct(fieldValue, commandName, 1)
			}
		}
		fmt.Println()
	}
}

func processStruct(structValue reflect.Value, basePath string, depth int) {
	structType := structValue.Type()
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	fmt.Printf("%sStruct %s has %d fields\n", indent, basePath, structType.NumField())

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)

		commandTag := field.Tag.Get("command")
		if commandTag != "" {
			description := field.Tag.Get("description")
			commandPath := basePath + " " + commandTag
			fmt.Printf("%s  -> Found subcommand: %s - %s\n", indent, commandPath, description)

			if fieldValue.Kind() == reflect.Struct {
				processStruct(fieldValue, commandPath, depth+1)
			}
		}
	}
}
