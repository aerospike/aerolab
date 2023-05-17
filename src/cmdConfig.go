package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type configCmd struct {
	Backend  configBackendCmd  `command:"backend" subcommands-optional:"true" description:"Show or change backend"`
	Defaults configDefaultsCmd `command:"defaults" subcommands-optional:"true" description:"Show or change defaults in the configuration file"`
	Aws      configAwsCmd      `command:"aws" subcommands-optional:"true" description:"AWS-only related management commands"`
	Docker   configDockerCmd   `command:"docker" subcommands-optional:"true" description:"DOCKER-only related management commands"`
	Help     helpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *configCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type configBackendCmd struct {
	Type       string         `short:"t" long:"type" description:"Supported backends: aws|docker" default:""`
	SshKeyPath flags.Filename `short:"p" long:"key-path" description:"AWS and GCP backends: specify a path to store SSH keys in, default: ${HOME}/aerolab-keys/" default:"${HOME}/aerolab-keys/"`
	Region     string         `short:"r" long:"region" description:"AWS backend: override default aws configured region" default:""`
	Project    string         `short:"o" long:"project" description:"GCP backend: override default gcp configured project" default:""`
	TmpDir     flags.Filename `short:"d" long:"temp-dir" description:"use a non-default temporary directory" default:""`
	Help       helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
	typeSet    string
}

type configDefaultsCmd struct {
	Key         string         `short:"k" long:"key" description:"Key to modify or show, character '*' expansion is supported" default:""`
	OnlyChanged bool           `short:"o" long:"only-changed" description:"Set to only display values different from application default"`
	Value       flags.Filename `short:"v" long:"value" description:"Value to set" default:""`
	Reset       bool           `short:"r" long:"reset" description:"Reset to default value. Use instead of --value"`
	Help        helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *configBackendCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		for _, i := range os.Args {
			if inslice.HasString([]string{"-t", "--type"}, i) {
				c.typeSet = "yes"
			}
		}
		a.forceFileOptional = true
		return nil
	}
	if c.typeSet != "" {
		err := c.ExecTypeSet(args)
		if err != nil {
			return err
		}
	}
	fmt.Printf("Config.Backend.Type = %s\n", c.Type)
	if c.Type == "aws" {
		fmt.Printf("Config.Backend.Region = %s\n", c.Region)
	}
	if c.Type == "gcp" {
		fmt.Printf("Config.Backend.Project = %s\n", c.Project)
	}
	fmt.Printf("Config.Backend.TmpDir = %s\n", c.TmpDir)
	return nil
}

func (c *configBackendCmd) ExecTypeSet(args []string) error {
	for command, switchList := range backendSwitches {
		keys := strings.Split(strings.ToLower(string(command)), ".")
		var nCmd *flags.Command
		for i, key := range keys {
			if i == 0 {
				nCmd = a.parser.Find(key)
			} else {
				nCmd = nCmd.Find(key)
			}
		}
		for backend, switches := range switchList {
			_, err := nCmd.AddGroup(string(backend), string(backend), switches)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	if c.Type == "aws" || c.Type == "gcp" {
		if strings.Contains(string(c.SshKeyPath), "${HOME}") {
			ch, err := os.UserHomeDir()
			if err != nil {
				log.Fatal(err)
			}
			c.SshKeyPath = flags.Filename(strings.ReplaceAll(string(c.SshKeyPath), "${HOME}", ch))
		}
		if _, err := os.Stat(string(c.SshKeyPath)); err != nil {
			err = os.MkdirAll(string(c.SshKeyPath), 0755)
			if err != nil {
				return err
			}
		}
	} else if c.Type != "docker" {
		return errors.New("backend types supported: docker, aws, gcp")
	}
	if c.TmpDir == "" {
		out, err := exec.Command("uname", "-r").CombinedOutput()
		if err != nil {
			log.Println("WARNING: `uname` not found, if running in WSL2, specify the temporary directory as part of this command using `-d /path/to/tmpdir`")
		} else {
			if strings.Contains(string(out), "-WSL2") && strings.Contains(string(out), "microsoft") {
				ch, err := os.UserHomeDir()
				if err != nil {
					log.Fatal(err)
				}
				err = os.MkdirAll(path.Join(ch, ".aerolab.tmp"), 0755)
				if err != nil {
					log.Fatal(err)
				}
				c.TmpDir = flags.Filename(path.Join(ch, ".aerolab.tmp"))
			}
		}
	}

	err := writeConfigFile()
	if err != nil {
		log.Printf("ERROR: Could not save file: %s", err)
	}
	fmt.Print("OK: ")
	return nil
}

func (c *configDefaultsCmd) handleRecursive(args []string) error {
	ret := make(chan configValueCmd, 1)
	value := c.Value
	reset := c.Reset
	keyField := reflect.ValueOf(a.opts).Elem()
	findKey := c.Key
	findKeyR := strings.ReplaceAll(strings.ReplaceAll(findKey, ".", "\\."), "*", ".*")
	if strings.HasSuffix(findKeyR, ".*") && c.Value != "" {
		return errors.New("when specifying a value, key cannot end with .*")
	}
	findKeyRegex, err := regexp.Compile("^" + findKeyR + "$")
	if err != nil {
		return err
	}
	go c.getValues(keyField, "", ret, "")
	for {
		val, ok := <-ret
		if !ok {
			return nil
		}
		if !findKeyRegex.MatchString(val.key) {
			continue
		}
		c.Key = val.key
		c.Value = value
		c.Reset = reset
		if !c.Reset && c.Value == "" {
			fmt.Println(val.key + " = " + val.value)
		} else {
			err = c.Execute(args)
			if err != nil {
				return err
			}
		}
	}
}

func (c *configDefaultsCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	if strings.Contains(c.Key, "*") {
		return c.handleRecursive(args)
	}
	var keys []string
	if c.Key == "" {
		keys = []string{}
	} else {
		keys = strings.Split(c.Key, ".")
	}
	c.Key = ""

	value := string(c.Value)
	c.Value = ""

	reset := c.Reset
	c.Reset = false

	keyField := reflect.ValueOf(a.opts).Elem()
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
		c.displayValues(keyField, strings.Join(keys, "."), tags)
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
		case reflect.Int, reflect.String, reflect.Float64:
		default:
			fmt.Println("ERROR: Key is not a parameter")
			os.Exit(1)
		}
	}

	// set value
	if value == "" && !reset {
		return nil
	}
	value = strings.Trim(value, " ")
	switch keyField.Type().Kind() {
	case reflect.Int:
		v, err := strconv.Atoi(value)
		if err != nil {
			fmt.Println("ERROR: value must be an integer")
			os.Exit(1)
		}
		keyField.SetInt(int64(v))
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
			os.Exit(1)
		}
	case reflect.Float64:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			fmt.Println("ERROR: value must be a number")
			os.Exit(1)
		}
		keyField.SetFloat(v)
	default:
		fmt.Println("ERROR: Key is not a parameter")
		os.Exit(1)
	}
	err := writeConfigFile()
	if err != nil {
		fmt.Printf("ERROR writing configuration file: %s\n", err)
		os.Exit(1)
	}
	fmt.Print("OK: ")
	c.displayValues(keyField, strings.Join(keys, "."), "")
	return nil
}

type configValueCmd struct {
	key   string
	value string
}

func (c *configDefaultsCmd) displayValues(keyField reflect.Value, start string, tags reflect.StructTag) {
	ret := make(chan configValueCmd, 1)
	go c.getValues(keyField, start, ret, tags)
	for {
		val, ok := <-ret
		if !ok {
			return
		}
		fmt.Println(val.key + " = " + val.value)
	}
}

func (c *configDefaultsCmd) getValues(keyField reflect.Value, start string, ret chan configValueCmd, tags reflect.StructTag) {
	defer close(ret)
	c.getValuesNext(keyField, start, ret, tags)
}

func (c *configDefaultsCmd) getValuesNext(keyField reflect.Value, start string, ret chan configValueCmd, tags reflect.StructTag) {
	var tagDefault string
	if tags != "" {
		tagDefault = tags.Get("default")
	}
	switch keyField.Type().Kind() {
	case reflect.Int:
		if tagDefault == "" {
			tagDefault = "0"
		}
		if !c.OnlyChanged || tagDefault != fmt.Sprintf("%d", keyField.Int()) {
			ret <- configValueCmd{start, fmt.Sprintf("%d", keyField.Int())}
		}
	case reflect.String:
		if !c.OnlyChanged || keyField.String() != tagDefault {
			ret <- configValueCmd{start, keyField.String()}
		}
	case reflect.Bool:
		if !c.OnlyChanged || keyField.Bool() { // false values are the defaults
			ret <- configValueCmd{start, fmt.Sprintf("%t", keyField.Bool())}
		}
	case reflect.Float64:
		val, _ := strconv.ParseFloat(tagDefault, 64)
		if !c.OnlyChanged || val != keyField.Float() {
			ret <- configValueCmd{start, fmt.Sprintf("%f", keyField.Float())}
		}
	case reflect.Struct:
		for i := 0; i < keyField.NumField(); i++ {
			fieldName := keyField.Type().Field(i).Name
			fieldTag := keyField.Type().Field(i).Tag
			if len(fieldName) > 0 && fieldName[0] >= 97 && fieldName[0] <= 122 {
				if keyField.Field(i).Type().Kind() != reflect.Struct {
					continue
				}
				c.getValuesNext(keyField.Field(i), start, ret, fieldTag)
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
			c.getValuesNext(keyField.Field(i), fieldName, ret, fieldTag)
		}
	case reflect.Slice:
	default:
		fmt.Printf("Invalid function type: %v: %v\n", keyField.Type().Kind(), start)
	}
}
