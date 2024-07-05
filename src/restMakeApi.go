package main

import (
	"net/http"
	"reflect"
	"strings"
)

func (c *restCmd) makeApi(keyField reflect.Value, start string, tags reflect.StructTag) {
	http.HandleFunc("/", c.handleApi)
	defer func() {
		http.HandleFunc("/quit/", c.handleApi)
		http.HandleFunc("/quit", c.handleApi)
		c.apiCommands = append(c.apiCommands, apiCommand{
			path:        "quit",
			description: "Exit aerolab rest service",
		})
	}()
	ret := make(chan apiCommand, 1)
	go c.getCommands(keyField, start, ret, tags)
	for {
		val, ok := <-ret
		if !ok {
			return
		}
		c.apiCommands = append(c.apiCommands, val)
		http.HandleFunc("/"+val.path, c.handleApi)
		http.HandleFunc("/"+val.path+"/", c.handleApi)
	}
}

func (c *restCmd) getCommands(keyField reflect.Value, start string, ret chan apiCommand, tags reflect.StructTag) {
	defer close(ret)
	c.getCommandsNext(keyField, start, ret, tags, []string{})
}

func (c *restCmd) getCommandsNext(keyField reflect.Value, start string, ret chan apiCommand, tags reflect.StructTag, tagStack []string) {
	var tagCommand string
	if tags != "" {
		tagCommand = tags.Get("command")
	}
	if tagCommand != "" {
		tagStack = append(tagStack, tagCommand)
	}
	switch keyField.Type().Kind() {
	case reflect.Struct:
		for i := 0; i < keyField.NumField(); i++ {
			fieldName := keyField.Type().Field(i).Name
			fieldTag := keyField.Type().Field(i).Tag
			if len(fieldName) > 0 && fieldName[0] >= 97 && fieldName[0] <= 122 {
				if keyField.Field(i).Type().Kind() != reflect.Struct {
					continue
				}
				c.getCommandsNext(keyField.Field(i), start, ret, fieldTag, tagStack)
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
			c.getCommandsNext(keyField.Field(i), fieldName, ret, fieldTag, tagStack)
		}
	}
	if tagCommand == "" {
		return
	}
	isHidden := false
	if tags.Get("hidden") != "" {
		isHidden = true
	}
	isWebHidden := false
	if tags.Get("webhidden") != "" {
		isWebHidden = true
	}
	isReload := false
	if tags.Get("invwebforce") != "" {
		isReload = true
	}
	ret <- apiCommand{
		path:        strings.Join(tagStack, "/"),
		pathStack:   tagStack,
		description: tags.Get("description"),
		isHidden:    isHidden,
		isWebHidden: isWebHidden,
		icon:        tags.Get("webicon"),
		Value:       keyField,
		reload:      isReload,
		tags:        tags,
	}
}
