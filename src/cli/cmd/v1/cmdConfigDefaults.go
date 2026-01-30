package cmd

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	flags "github.com/rglonek/go-flags"
)

type ConfigDefaultsCmd struct {
	Key         string         `short:"k" long:"key" description:"Key to modify or show, character '*' expansion is supported" default:"" no-default:"true"`
	OnlyChanged bool           `short:"o" long:"only-changed" description:"Set to only display values different from application default" no-default:"true"`
	Value       flags.Filename `short:"v" long:"value" description:"Value to set" default:"" no-default:"true"`
	Reset       bool           `short:"r" long:"reset" description:"Reset to default value. Use instead of --value" no-default:"true"`
	Help        HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
	saveFile    bool
}

func (c *ConfigDefaultsCmd) handleRecursive(system *System, args []string) error {
	ret := make(chan ConfigValueCmd, 1)
	value := c.Value
	reset := c.Reset
	keyField := reflect.ValueOf(system.Opts).Elem()
	findKey := c.Key
	findKeyR := strings.ReplaceAll(strings.ReplaceAll(findKey, ".", "\\."), "*", ".*")
	if strings.HasSuffix(findKeyR, ".*") && c.Value != "" {
		return errors.New("when specifying a value, key cannot end with .*")
	}
	findKeyRegex, err := regexp.Compile("^" + findKeyR + "$")
	if err != nil {
		return err
	}
	go c.getValues(keyField, "", ret, "", c.OnlyChanged)
	for {
		val, ok := <-ret
		if !ok {
			return nil
		}
		if !findKeyRegex.MatchString(val.Key) {
			continue
		}
		c.Key = val.Key
		c.Value = value
		c.Reset = reset
		if !c.Reset && c.Value == "" {
			fmt.Println(val.Key + " = " + val.Value)
		} else {
			err = c.configDefaults(system, args)
			if err != nil {
				return err
			}
		}
	}
}

func (c *ConfigDefaultsCmd) get(system *System, onlyChanged bool) map[string]string {
	keyField := reflect.ValueOf(system.Opts).Elem()
	var tags reflect.StructTag
	ret := make(chan ConfigValueCmd, 1)
	vals := make(map[string]string)
	go c.getValues(keyField, "", ret, tags, onlyChanged)
	for {
		val, ok := <-ret
		if !ok {
			break
		}
		vals[val.Key] = val.Value
	}
	return vals
}

func (c *ConfigDefaultsCmd) Execute(args []string) error {
	system, err := Initialize(&Init{InitBackend: false}, []string{"config", "defaults"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"config", "defaults"}, c, args)
	}
	return Error(c.ConfigDefaults(system, args), system, []string{"config", "defaults"}, c, args)
}

func (c *ConfigDefaultsCmd) ConfigDefaults(system *System, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: false}, []string{"config", "defaults"}, c, args...)
		if err != nil {
			return err
		}
	}
	err := c.configDefaults(system, args)
	if err != nil {
		return err
	}
	if c.saveFile {
		err := system.WriteConfigFile()
		if err != nil {
			fmt.Printf("ERROR writing configuration file: %s\n", err)
			return errors.New("ERROR writing configuration file")
		}
	}
	return nil
}

func (c *ConfigDefaultsCmd) configDefaults(system *System, args []string) error {
	if strings.Contains(c.Key, "*") {
		return c.handleRecursive(system, args)
	}
	var keys []string
	if c.Key == "" {
		keys = []string{}
	} else {
		keys = strings.Split(c.Key, ".")
	}
	system.Opts.Config.Defaults.Key = ""
	c.Key = ""

	value := string(c.Value)
	system.Opts.Config.Defaults.Value = ""
	c.Value = ""

	reset := c.Reset
	system.Opts.Config.Defaults.Reset = false
	c.Reset = false

	onlyChanged := c.OnlyChanged
	system.Opts.Config.Defaults.OnlyChanged = false
	c.OnlyChanged = false

	keyField := reflect.ValueOf(system.Opts).Elem()
	var tags reflect.StructTag
	for i, key := range keys {
		fieldType, ok := keyField.Type().FieldByName(key)
		if ok {
			tags = fieldType.Tag
		}
		keyField = keyField.FieldByName(key)
		if !keyField.IsValid() {
			fmt.Printf("Key not found: %s. If using *, try enclosing key name in single quotes ''\n", strings.Join(keys[0:i+1], "."))
			return nil
		}
	}

	// display values if called for
	if value == "" && !reset {
		c.displayValues(keyField, strings.Join(keys, "."), tags, onlyChanged)
		return nil
	}

	// reset value if called for - set value parameter
	if reset && value == "" {
		def := tags.Get("default")
		value = def
		switch keyField.Type().Kind() {
		case reflect.Bool:
			if def == "" {
				value = "false"
			}
		case reflect.Int, reflect.String, reflect.Float64, reflect.Ptr, reflect.Int64:
			if keyField.Type().String() == "time.Duration" {
				if value == "" {
					value = "0"
				}
			}
		case reflect.Slice:
		default:
			fmt.Printf("ERROR: Key is not a parameter (%s)\n", keyField.Type().Kind())
			return errors.New("ERROR: Key is not a parameter")
		}
	}

	// set value
	if value == "" && !reset {
		return nil
	}
	value = strings.Trim(value, " ")
	switch keyField.Type().Kind() {
	case reflect.Int, reflect.Int64:
		if keyField.Type().String() == "time.Duration" {
			v, err := time.ParseDuration(value)
			if err != nil {
				fmt.Printf("ERROR: value must be a duration (%s)\n", value)
				return errors.New("ERROR: value must be a duration")
			}
			keyField.Set(reflect.ValueOf(v))
		} else {
			v, err := strconv.Atoi(value)
			if err != nil {
				fmt.Println("ERROR: value must be an integer")
				return errors.New("ERROR: value must be an integer")
			}
			keyField.SetInt(int64(v))
		}
	case reflect.Slice:
		if keyField.Type().String() != "[]string" {
			fmt.Println("Only string slices are supported; invalid key")
			return errors.New("ERROR: Only string slices are supported; invalid key")
		}
		keyField.Set(reflect.ValueOf([]string{value}))
	case reflect.String:
		keyField.SetString(value)
	case reflect.Bool:
		switch strings.ToLower(value) {
		case "true", "yes", "t", "y":
			keyField.SetBool(true)
		case "false", "no", "f", "n":
			keyField.SetBool(false)
		default:
			fmt.Println("ERROR: value must be one of: true|false")
			return errors.New("ERROR: value must be one of: true|false")
		}
	case reflect.Float64:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			fmt.Println("ERROR: value must be a number")
			return errors.New("ERROR: value must be a number")
		}
		keyField.SetFloat(v)
	case reflect.Ptr:
		switch keyField.Type().Elem().String() {
		case "flags.Filename":
			strVal := flags.Filename(value)
			keyField.Set(reflect.ValueOf(&strVal))
		case "string":
			keyField.Set(reflect.ValueOf(&value))
		case "bool":
			boolVal := false
			switch strings.ToLower(value) {
			case "true", "yes", "t", "y":
				boolVal = true
			case "false", "no", "f", "n":
				boolVal = false
			default:
				fmt.Println("ERROR: value must be one of: true|false")
				return errors.New("ERROR: value must be one of: true|false")
			}
			keyField.Set(reflect.ValueOf(&boolVal))
		case "int", "int64":
			if keyField.Type().String() == "time.Duration" {
				v, err := time.ParseDuration(value)
				if err != nil {
					fmt.Println("ERROR: value must be a duration")
					return errors.New("ERROR: value must be a duration")
				}
				keyField.Set(reflect.ValueOf(&v))
			} else {
				v, err := strconv.Atoi(value)
				if err != nil {
					fmt.Println("ERROR: value must be an integer")
					return errors.New("ERROR: value must be an integer")
				}
				keyField.Set(reflect.ValueOf(&v))
			}
		}
	default:
		fmt.Printf("ERROR: Key is not a parameter (%s)\n", keyField.Type().Kind())
		return errors.New("ERROR: Key is not a parameter")
	}
	c.saveFile = true
	fmt.Print("OK: ")
	c.displayValues(keyField, strings.Join(keys, "."), "", onlyChanged)
	return nil
}

type ConfigValueCmd struct {
	Key   string
	Value string
}

func (c *ConfigDefaultsCmd) displayValues(keyField reflect.Value, start string, tags reflect.StructTag, onlyChanged bool) {
	ret := make(chan ConfigValueCmd, 1)
	go c.getValues(keyField, start, ret, tags, onlyChanged)
	for {
		val, ok := <-ret
		if !ok {
			return
		}
		fmt.Println(val.Key + " = " + val.Value)
	}
}

func (c *ConfigDefaultsCmd) getValues(keyField reflect.Value, start string, ret chan ConfigValueCmd, tags reflect.StructTag, onlyChanged bool) {
	defer close(ret)
	c.getValuesNext(keyField, start, ret, tags, onlyChanged)
}

func (c *ConfigDefaultsCmd) getValuesNext(keyField reflect.Value, start string, ret chan ConfigValueCmd, tags reflect.StructTag, onlyChanged bool) {
	var tagDefault string
	if tags != "" {
		tagDefault = tags.Get("default")
	}
	switch keyField.Type().Kind() {
	case reflect.Int, reflect.Int64:
		if tagDefault == "" {
			tagDefault = "0"
		}
		if !onlyChanged || tagDefault != fmt.Sprintf("%d", keyField.Int()) {
			if keyField.Type().String() != "time.Duration" {
				ret <- ConfigValueCmd{start, fmt.Sprintf("%d", keyField.Int())}
			} else {
				defDuration, err := time.ParseDuration(tagDefault)
				if !onlyChanged || err != nil || defDuration != time.Duration(keyField.Int()) {
					ret <- ConfigValueCmd{start, fmt.Sprintf("%v", time.Duration(keyField.Int()))}
				}
			}
		}
	case reflect.String:
		if !onlyChanged || keyField.String() != tagDefault {
			ret <- ConfigValueCmd{start, keyField.String()}
		}
	case reflect.Bool:
		if !onlyChanged || keyField.Bool() { // false values are the defaults
			ret <- ConfigValueCmd{start, fmt.Sprintf("%t", keyField.Bool())}
		}
	case reflect.Float64:
		val, _ := strconv.ParseFloat(tagDefault, 64)
		if !onlyChanged || val != keyField.Float() {
			ret <- ConfigValueCmd{start, fmt.Sprintf("%f", keyField.Float())}
		}
	case reflect.Struct:
		for i := 0; i < keyField.NumField(); i++ {
			fieldName := keyField.Type().Field(i).Name
			fieldTag := keyField.Type().Field(i).Tag
			if fieldTag.Get("no-default") == "true" {
				continue
			}
			if len(fieldName) > 0 && fieldName[0] >= 97 && fieldName[0] <= 122 {
				if keyField.Field(i).Type().Kind() != reflect.Struct {
					continue
				}
				c.getValuesNext(keyField.Field(i), start, ret, fieldTag, onlyChanged)
			}
			if len(fieldName) == 0 || fieldName[0] < 65 || fieldName[0] > 90 {
				continue
			}
			if start != "" {
				fieldName = start + "." + fieldName
			}
			if strings.HasPrefix(fieldName, "Config.Defaults.") || fieldName == "DryRun" {
				continue
			}
			c.getValuesNext(keyField.Field(i), fieldName, ret, fieldTag, onlyChanged)
		}
	case reflect.Slice:
		if keyField.Type().String() == "[]string" {
			val := keyField.Interface().([]string)
			if !onlyChanged || (tagDefault != "" && (len(val) != 1 || val[0] != tagDefault)) {
				ret <- ConfigValueCmd{start, fmt.Sprintf("%v", val)}
			}
		}
	case reflect.Ptr:
		if !keyField.IsNil() {
			c.getValuesNext(reflect.Indirect(keyField), start, ret, tags, onlyChanged)
		} else {
			if !onlyChanged {
				ret <- ConfigValueCmd{start, tagDefault}
			}
		}
	case reflect.Func:
		// Function fields don't have meaningful default values, skip them
	default:
		fmt.Printf("Invalid function type: %v: %v\n", keyField.Type().Kind(), start)
	}
}
