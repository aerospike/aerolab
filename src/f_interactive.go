package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type read struct {
	reader *bufio.Reader
}

func (r read) read(prompt string, default_string string) string {
	if prompt != "" {
		fmt.Print(prompt)
	}
	ret, _ := r.reader.ReadString('\n')
	if ret == "" || ret == "\n" {
		ret = default_string
	}
	l := len(ret)
	if l > 0 {
		if ret[l-1] == '\n' {
			ret = ret[:l-1]
		}
	}
	return ret
}

func (c *config) F_interactive() (err error, ret int64) {

	// setup reader
	reader := bufio.NewReader(os.Stdin)
	r := read{reader: reader}

	// get command name
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	commands := []string{}
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagCommandName := cField.Tag.Get("name")
		if tagType == "command" && tagCommandName != "interactive" {
			commands = append(commands, tagCommandName)
		}
	}
	cmds := fmt.Sprintf("Command (%s): ", strings.Join(commands, "|"))
	command := r.read(cmds, "")
	c.Command = command

	// get all parameters for given command
	found := false
	var tagMethod string
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagCommandName := cField.Tag.Get("name")
		tagMethod = cField.Tag.Get("method")
		if tagType == "command" && tagCommandName == command {
			found = true
			if command != "help" {
				commandField := TypeOfCElem.Field(i)
				commandFieldType := commandField.Type
				for j := 0; j < commandFieldType.NumField(); j++ {
					tagSwitchDescription := commandFieldType.Field(j).Tag.Get("description")
					tagSwitchLong := commandFieldType.Field(j).Tag.Get("long")
					tagSwitchDefault := commandFieldType.Field(j).Tag.Get("default")
					value := r.read(fmt.Sprintf("%s (%s) [%s]: ", tagSwitchLong, tagSwitchDescription, tagSwitchDefault), tagSwitchDefault)
					c.parseCommandLineParametersSwitch("long", tagSwitchLong, value)
				}
			}
			break
		}
	}
	if found == false {
		err = errors.New("Command not found")
		return err, E_INTERACTIVE
	}
	reta := reflect.ValueOf(c).MethodByName(tagMethod).Call([]reflect.Value{})
	if reta[0].IsNil() == false {
		err = errors.New(fmt.Sprint(reta[0].Interface()))
	}
	ret = reta[1].Int()
	return err, ret
}
