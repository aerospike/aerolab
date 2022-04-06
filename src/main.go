package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path"
	"reflect"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	Logger "github.com/bestmethod/go-logger"
)

func main() {
	var c config
	if err := c.log.Init(loggerHeader, loggerServiceName, Logger.LEVEL_DEBUG|Logger.LEVEL_INFO|Logger.LEVEL_WARN, Logger.LEVEL_ERROR|Logger.LEVEL_CRITICAL, Logger.LEVEL_NONE); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, ERR_MAIN_LOGGER, err)
		os.Exit(E_MAIN_LOGGER)
	}
	defer func() { _ = c.log.Destroy() }()
	os.Exit(c.main())
}

func (c *config) main() int {

	var err error
	// if aero-lab-common.conf exists in user's home or in /etc, add that to the command line parsing
	if _, err := os.Stat("/etc/aero-lab-common.conf"); err == nil {
		c.ConfigFiles = append(c.ConfigFiles, "/etc/aero-lab-common.conf")
	}
	usr, err := user.Current()
	if err == nil {
		usrPath := path.Join(usr.HomeDir, "aero-lab-common.conf")
		if _, err := os.Stat(usrPath); err == nil {
			c.ConfigFiles = append(c.ConfigFiles, usrPath)
		}
	}

	// parse command line parameters
	var commandOffset int
	if commandOffset, err = c.parseCommandLineParameters(1, nil); err != nil {
		c.log.Fatal(fmt.Sprintf(ERR_MAIN_CMDLINEPARAMS, err), E_MAIN_CMDLINEPARAMS)
	}

	// parse config file if specified
	if err := c.parseConfigFile(); err != nil {
		c.log.Fatal(fmt.Sprintf(ERR_MAIN_CONFIG, err), E_MAIN_CONFIG)
	}

	// parse command line parameters again - they override config file
	if _, err = c.parseCommandLineParameters(1, []int{commandOffset}); err != nil {
		c.log.Fatal(fmt.Sprintf(ERR_MAIN_CMDLINEPARAMS, err), E_MAIN_CMDLINEPARAMS)
	}

	// parse common config parameters
	if err = c.parseCommonConfig(); err != nil {
		c.log.Fatal(fmt.Sprintf(ERR_MAIN_CONFIG, err), E_MAIN_CONFIG)
	}

	// parse defaults
	if err := c.parseCommandLineParametersDefaults(); err != nil {
		c.log.Fatal(fmt.Sprintf(ERR_MAIN_CMDLINEPARAMS, err), E_MAIN_CMDLINEPARAMS)
	}

	ret, err := c.runCommand()
	// cycle through features and redirect to the right feature
	if err != nil {
		c.log.Fatal(err.Error(), int(ret))
	}

	return int(ret)
}

func (c *config) parseCommandLineParameters(startOffset int, ignoreOffsets []int) (commandOffset int, err error) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Fatalf(999, "Invalid command line parameters")
		}
	}()
	for i := startOffset; i < len(os.Args); i++ {
		if inArray(ignoreOffsets, i) > -1 {
			continue
		}
		param := os.Args[i]
		if param == "--" {
			c.tail = os.Args[i+1:]
			i = len(os.Args)
		} else if param[:2] == "--" {
			p := strings.Split(param[2:], "=")
			v := ""
			if len(p) > 2 {
				err = fmt.Errorf(ERR_MAIN_INVALIDPARAM, param)
			} else if len(p) == 2 {
				v = p[1]
			}
			if p[0] == "config" {
				c.ConfigFiles = append(c.ConfigFiles, v)
			} else {
				if err := c.parseCommandLineParametersSwitch("long", p[0], v); err != nil {
					return -1, err
				}
			}
		} else if param[:1] == "-" {
			ntype, err := c.parseCommandLineParametersGetType("short", param[1:])
			if err != nil {
				return -1, err
			}
			i = i + 1
			if i == len(os.Args) && ntype != "bool" {
				return -1, fmt.Errorf("Parameter is missing argument: %s", os.Args[i-1])
			}
			if i == len(os.Args) {
				if err := c.parseCommandLineParametersSwitch("short", param[1:], ""); err != nil {
					return -1, err
				}
			} else if os.Args[i] == "0" || os.Args[i] == "1" || ntype != "bool" {
				value := os.Args[i]
				if err := c.parseCommandLineParametersSwitch("short", param[1:], value); err != nil {
					return -1, err
				}
			} else {
				i = i - 1
				if err := c.parseCommandLineParametersSwitch("short", param[1:], ""); err != nil {
					return -1, err
				}
			}
		} else {
			if i > 1 {
				if c.Command == "help" {
					c.F_helpCommand(param)
					os.Exit(E_HELP)
				} else if param == "help" {
					c.F_helpCommand(c.Command)
					os.Exit(E_HELP)
				} else {
					if c.Command != "" {
						err = errors.New(ERR_MAIN_MANY_COMMANDS)
						return -1, err
					}
					if err := c.parseCommandLineParametersCommand(param); err != nil {
						return -1, err
					} else {
						commandOffset = i
					}
				}
			} else {
				if c.Command != "" {
					err = errors.New(ERR_MAIN_MANY_COMMANDS)
					return
				}
				if err := c.parseCommandLineParametersCommand(param); err != nil {
					return -1, err
				} else {
					commandOffset = i
				}
			}
		}
	}
	return commandOffset, nil
}

func (c *config) parseCommandLineParametersDefaults() (err error) {
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		if tagType == "command" && cField.Type.Kind() == reflect.Struct {
			commandField := TypeOfCElem.Field(i)
			commandFieldType := commandField.Type
			for j := 0; j < commandFieldType.NumField(); j++ {
				tagDefault := commandFieldType.Field(j).Tag.Get("default")
				if tagDefault != "" {
					if commandFieldType.Field(j).Type.Kind() == reflect.String {
						if reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).String() == "" {
							reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).SetString(tagDefault)
						}
					} else if commandFieldType.Field(j).Type.Kind() == reflect.Int {
						if reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).Int() == 0 {
							num, err := strconv.Atoi(tagDefault)
							if err != nil {
								return err
							}
							reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).SetInt(int64(num))
						}
					} else {
						err = errors.New(ERR_MAIN_PARSE_TYPE)
						return
					}
				}
			}
		}
	}
	return
}

func (c *config) parseCommandLineParametersSwitch(paramType string, param string, value string) (err error) {
	if c.Command == "" {
		err = errors.New(ERR_MAIN_COMM_FIRST)
		return
	}
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	found := false
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagCommandName := cField.Tag.Get("name")
		if tagType == "command" && tagCommandName == c.Command && cField.Type.Kind() == reflect.Struct {
			commandField := TypeOfCElem.Field(i)
			commandFieldType := commandField.Type
			for j := 0; j < commandFieldType.NumField(); j++ {
				tagSwitch := commandFieldType.Field(j).Tag.Get(paramType)
				tagFieldType := commandFieldType.Field(j).Tag.Get("type")
				if tagSwitch == param {
					found = true
					if commandFieldType.Field(j).Type.Kind() == reflect.String {
						reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).SetString(value)
					} else if commandFieldType.Field(j).Type.Kind() == reflect.Int {
						if value == "" && tagFieldType == "bool" {
							value = "1"
						}
						num, err := strconv.Atoi(value)
						if err != nil {
							return err
						}
						reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).SetInt(int64(num))
					} else {
						err = errors.New(ERR_MAIN_PARSE_TYPE)
						return
					}
					break
				}
			}
		}
		if found {
			break
		}
	}
	if !found {
		err = fmt.Errorf(ERR_MAIN_UNKNOWN_PARAM, param)
	}
	return
}

func (c *config) parseCommandLineParametersGetType(paramType string, param string) (ntype string, err error) {
	if c.Command == "" {
		err = errors.New(ERR_MAIN_COMM_FIRST)
		return
	}
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	found := false
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagCommandName := cField.Tag.Get("name")
		if tagType == "command" && tagCommandName == c.Command && cField.Type.Kind() == reflect.Struct {
			commandField := TypeOfCElem.Field(i)
			commandFieldType := commandField.Type
			for j := 0; j < commandFieldType.NumField(); j++ {
				tagSwitch := commandFieldType.Field(j).Tag.Get(paramType)
				tagFieldType := commandFieldType.Field(j).Tag.Get("type")
				if tagSwitch == param {
					found = true
					if commandFieldType.Field(j).Type.Kind() == reflect.String {
						ntype = "string"
					} else if commandFieldType.Field(j).Type.Kind() == reflect.Int {
						if tagFieldType == "bool" {
							ntype = "bool"
						} else {
							ntype = "int"
						}
					} else {
						err = errors.New(ERR_MAIN_PARSE_TYPE)
						return
					}
					break
				}
			}
		}
		if found {
			break
		}
	}
	if !found {
		err = fmt.Errorf(ERR_MAIN_UNKNOWN_PARAM, param)
	}
	return
}

func (c *config) parseCommandLineParametersCommand(param string) (err error) {
	for _, p := range strings.Split(param, ",") {
		TypeOfC := reflect.TypeOf(c)
		TypeOfCElem := TypeOfC.Elem()
		found := false
		for i := 0; i < TypeOfCElem.NumField(); i++ {
			cField := TypeOfCElem.Field(i)
			tagType := cField.Tag.Get("type")
			if tagType == "command" {
				tagName := cField.Tag.Get("name")
				if tagName == p {
					found = true
					break
				}
			}
		}
		if !found {
			err = fmt.Errorf(ERR_MAIN_INVALIDPARAM, param)
		} else {
			c.Command = p
		}
	}
	return
}

func (c *config) parseConfigFile() (err error) {
	if len(c.ConfigFiles) != 0 {
		for _, ConfigFile := range c.ConfigFiles {
			c.comm = c.Command
			_, err = toml.DecodeFile(ConfigFile, c)
			if c.comm != "" {
				c.Command = c.comm
			}
		}
	}
	return err
}

func (c *config) parseCommonConfig() (err error) {
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	var valueS string
	var valueI int64
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagCommandName := cField.Tag.Get("name")
		if tagType == "command" && tagCommandName == c.Command && cField.Type.Kind() == reflect.Struct {
			commandField := TypeOfCElem.Field(i)
			commandFieldType := commandField.Type
			for j := 0; j < commandFieldType.NumField(); j++ {
				fieldName := commandFieldType.Field(j).Name
				// get same field value from common
				TypeOfCommon := reflect.TypeOf(&(c.Common))
				TypeOfCommonElem := TypeOfCommon.Elem()
				valueS = ""
				valueI = 0
				for k := 0; k < TypeOfCommonElem.NumField(); k++ {
					commonField := TypeOfCommonElem.Field(k)
					if commonField.Name == fieldName {
						if commonField.Type.Kind() == reflect.String {
							valueS = reflect.ValueOf(&(c.Common)).Elem().Field(k).String()
						} else if commonField.Type.Kind() == reflect.Int {
							valueI = reflect.ValueOf(&(c.Common)).Elem().Field(k).Int()
						}
						break
					}
				}
				// end
				if valueI > 0 || valueS != "" {
					if commandFieldType.Field(j).Type.Kind() == reflect.String && reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).String() == "" {
						reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).SetString(valueS)
					} else if commandFieldType.Field(j).Type.Kind() == reflect.Int && reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).Int() == 0 {
						reflect.ValueOf(c).Elem().FieldByName(commandField.Name).FieldByName(commandFieldType.Field(j).Name).SetInt(int64(valueI))
					}
				}
			}
		}
	}
	return
}

func (c *config) runCommand() (ret int64, err error) {
	if c.Command == "" {
		c.log.Info("No command specified. Try running: %s help", os.Args[0])
		return
	}
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagName := cField.Tag.Get("name")
		if tagType == "command" && tagName == c.Command {
			tagMethod := cField.Tag.Get("method")
			reta := reflect.ValueOf(c).MethodByName(tagMethod).Call([]reflect.Value{})
			if !reta[1].IsNil() {
				err = errors.New(fmt.Sprint(reta[1].Interface()))
			}
			ret = reta[0].Int()
		}
	}
	return
}
