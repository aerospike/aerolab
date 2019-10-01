package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

func (c *config) F_help() (err error, ret int64) {
	ret = int64(E_HELP)
	usage := fmt.Sprintf("Usage: %s {command} [options] [-- {tail}]\n\nCommands:", os.Args[0])
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagCommandName := cField.Tag.Get("name")
		tagCommandDescription := cField.Tag.Get("description")
		if tagType == "command" {
			usage = fmt.Sprintf("%s\n\t%s\n\t\t%s", usage, tagCommandName, tagCommandDescription)
		}
	}
	usage = fmt.Sprintf("%s\n\nFor command details run:\n\t%s help {command} [--full]\nor\n\t%s {command} help [--full]\n\n\t--config={filename}\tUse config file to specify (some/all) switches instead\n\t\tCan be specified numerous times and will load in order it's specified\n\t\tDefault config path read each time as well is: /etc/aero-lab-common.conf\n\t\tSpecify parameters in [Common] section to be applied to all commands", usage, os.Args[0], os.Args[0])
	fmt.Println(usage)
	return
}

func (c *config) F_helpCommand(command string) {
	usage := "Command: " + command + "\n"
	confFile := []string{}
	TypeOfC := reflect.TypeOf(c)
	TypeOfCElem := TypeOfC.Elem()
	found := false
	var cName string
	for i := 0; i < TypeOfCElem.NumField(); i++ {
		cField := TypeOfCElem.Field(i)
		tagType := cField.Tag.Get("type")
		tagCommandName := cField.Tag.Get("name")
		cName = cField.Name
		if tagType == "command" && tagCommandName == command {
			found = true
			commandField := TypeOfCElem.Field(i)
			commandFieldType := commandField.Type
			for j := 0; j < commandFieldType.NumField(); j++ {
				tagSwitchShort := commandFieldType.Field(j).Tag.Get("short")
				tagSwitchLong := commandFieldType.Field(j).Tag.Get("long")
				tagSwitchDescription := commandFieldType.Field(j).Tag.Get("description")
				tagSwitchDefault := commandFieldType.Field(j).Tag.Get("default")
				paramName := commandFieldType.Field(j).Name
				confFile = append(confFile, paramName)
				usage = fmt.Sprintf("%s\n-%s | --%-20s\t : %s (default=%s)", usage, tagSwitchShort, tagSwitchLong, tagSwitchDescription, tagSwitchDefault)
			}
			break
		}
	}
	if found == false {
		usage = fmt.Sprintf("\nCommand not found")
	}

	if os.Args[len(os.Args)-1] == "--full" || os.Args[len(os.Args)-1] == "-f" {
		usage = fmt.Sprintf("%s\n\nConfiguration File Params:\n--------------------------\ncommand=%s\n[%s]\n%s=", usage, command, cName, strings.Join(confFile, "=\n"))
	} else {
		usage = fmt.Sprintf("%s\n", usage)
	}
	fmt.Println(usage)
}
