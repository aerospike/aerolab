package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
)

type webRunCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *webRunCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	j, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	js := strings.Split(string(j), "-=-=-=-")
	if len(js) != 2 {
		return errors.New("malform request")
	}
	command := strings.Split(strings.Trim(js[0], "/"), "/")
	keys := []string{}
	keyField := reflect.ValueOf(a.opts).Elem()
	v, err := a.opts.Rest.findCommand(keyField, strings.Join(keys, "."), "", []string{}, command)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(js[1])))
	dec.DisallowUnknownFields()
	err = dec.Decode(v.Addr().Interface())
	if err != nil {
		return err
	}

	if len(command) == 2 && command[0] == "config" && command[1] == "backend" && strings.Contains(js[1], "\"Type\"") {
		a.opts.Config.Backend.typeSet = "yes"
	} else {
		a.opts.Config.Backend.typeSet = ""
	}

	tailx := v.FieldByName("Tail")
	tailv := []string{}
	if tailx.IsValid() {
		tailv = tailx.Interface().([]string)
	}
	tail := []reflect.Value{reflect.ValueOf(tailv)}

	outv := v.Addr().MethodByName("Execute").Call(tail)
	out := outv[0].Interface()
	switch out.(type) {
	case error:
		return err
	}
	return nil
}
