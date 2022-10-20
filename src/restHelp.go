package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	flags "github.com/rglonek/jeddevdk-goflags"
)

func (c *restCmd) handleHelp(w http.ResponseWriter, r *http.Request) {
	urlpath := strings.TrimSuffix(strings.Trim(r.URL.Path, "/"), "/help")
	if urlpath == "help" {
		urlpath = ""
	}
	comms := []apiCommand{}
	maxlen := 7
	desclen := 11
	for _, command := range c.apiCommands {
		if !strings.HasSuffix(command.path, "/help") && strings.HasPrefix(command.path, urlpath) && len(command.path) > len(urlpath) && !strings.Contains(strings.Trim(strings.TrimPrefix(command.path, urlpath), "/"), "/") {
			comms = append(comms, command)
			if len(command.path)+1 > maxlen {
				maxlen = len(command.path) + 1
			}
			if len(command.description) > desclen {
				desclen = len(command.description)
			}
		}
	}

	// commands exist, print list of commands as help
	if len(comms) > 0 {
		m1 := "-------"
		for len(m1) < maxlen {
			m1 = m1 + "-"
		}
		m2 := "-----------"
		for len(m2) < desclen {
			m2 = m2 + "-"
		}
		fmt.Fprintf(w, "%-"+strconv.Itoa(maxlen)+"s | %s\n", "Command", "Description")
		fmt.Fprintf(w, "%-"+strconv.Itoa(maxlen)+"s-+-%s\n", m1, m2)
		for _, comm := range comms {
			fmt.Fprintf(w, "%-"+strconv.Itoa(maxlen)+"s | %s\n", "/"+comm.path, comm.description)
		}
		return
	}

	// subcommands do not exist, extract json and provide help for the given command payload parameters
	var na = &aerolab{
		opts: new(commands),
	}
	na.parser = flags.NewParser(na.opts, flags.HelpFlag|flags.PassDoubleDash)
	na.iniParser = flags.NewIniParser(na.parser)
	na.parseFile()
	na.parser.ParseArgs([]string{})
	command := strings.Split(urlpath, "/")
	keys := []string{}
	keyField := reflect.ValueOf(na.opts).Elem()
	v, err := c.findCommand(keyField, strings.Join(keys, "."), "", []string{}, command)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, "=== JSON payload with default values ===\n")
	out, err := json.MarshalIndent(v.Interface(), "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out = []byte(strings.ReplaceAll(string(out), ",\n  \"Help\": {}", ""))
	w.Write(out)
	fmt.Fprint(w, "\n=== Payload Parameter descriptions ===\n")

	vals := []helpValue{}
	keylen := 3
	desclen = 11
	kindlen := 4
	ret := make(chan helpValue, 1)
	go c.helpValueDescriptions(*v, "", ret, "")
	for {
		val, ok := <-ret
		if !ok {
			break
		}
		vals = append(vals, val)
		if len(val.key) > keylen {
			keylen = len(val.key)
		}
		if len(val.description) > desclen {
			desclen = len(val.description)
		}
		if len(val.kind) > kindlen {
			kindlen = len(val.kind)
		}
	}

	m1 := "---"
	for len(m1) < keylen {
		m1 = m1 + "-"
	}
	m2 := "-----------"
	for len(m2) < desclen {
		m2 = m2 + "-"
	}
	m3 := "----"
	for len(m3) < kindlen {
		m3 = m3 + "-"
	}
	fmt.Fprintf(w, "%-"+strconv.Itoa(keylen)+"s | %-"+strconv.Itoa(kindlen)+"s | %s\n", "Key", "Kind", "Description")
	fmt.Fprintf(w, "%-"+strconv.Itoa(keylen)+"s-+-%-"+strconv.Itoa(kindlen)+"s-+-%s\n", m1, m3, m2)
	for _, val := range vals {
		fmt.Fprintf(w, "%-"+strconv.Itoa(keylen)+"s | %-"+strconv.Itoa(kindlen)+"s | %s\n", val.key, val.kind, val.description)
	}
}

type helpValue struct {
	key         string
	description string
	kind        string
}

func (c *restCmd) helpValueDescriptions(keyField reflect.Value, start string, ret chan helpValue, tags reflect.StructTag) {
	defer close(ret)
	c.helpValueDescriptionsDo(keyField, start, ret, tags)
}

func (c *restCmd) helpValueDescriptionsDo(keyField reflect.Value, start string, ret chan helpValue, tags reflect.StructTag) {
	var tagDefault string
	if tags != "" {
		tagDefault = tags.Get("default")
	}
	tagDescription := tags.Get("description")
	switch keyField.Type().Kind() {
	case reflect.Int:
		if tagDefault == "" {
			tagDefault = "0"
		}
		ret <- helpValue{start, tagDescription, "integer"}
	case reflect.String:
		ret <- helpValue{start, tagDescription, "string"}
	case reflect.Bool:
		ret <- helpValue{start, tagDescription, "boolean"}
	case reflect.Float64:
		ret <- helpValue{start, tagDescription, "float64"}
	case reflect.Struct:
		for i := 0; i < keyField.NumField(); i++ {
			fieldName := keyField.Type().Field(i).Name
			fieldTag := keyField.Type().Field(i).Tag
			if len(fieldName) > 0 && fieldName[0] >= 97 && fieldName[0] <= 122 {
				if keyField.Field(i).Type().Kind() != reflect.Struct {
					continue
				}
				c.helpValueDescriptionsDo(keyField.Field(i), start, ret, fieldTag)
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
			c.helpValueDescriptionsDo(keyField.Field(i), fieldName, ret, fieldTag)
		}
	case reflect.Slice:
		ret <- helpValue{start, tagDescription, "list(strings)"}
	default:
		fmt.Printf("Invalid function type: %v: %v\n", keyField.Type().Kind(), start)
	}
}
