package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	flags "github.com/rglonek/jeddevdk-goflags"
)

/*
curl -X POST http://127.0.0.1:3030/attach/shell -d '{"Tail":["ls"]}'
curl -vX POST http://127.0.0.1:3030/cluster/create -d '{}'
curl -vX POST http://127.0.0.1:3030/cluster/destroy -d '{"Docker":{"Force":true}}'
/attach/shell/help
/help (same as / or nothing)
/cluster/help
/cluster/create/help
--
print commands
print json struct representing data
for each json struct item, print the item and the description
*/

var apiQueue = make(chan apiQueueItem, 1024)

type apiQueueItem struct {
	w      http.ResponseWriter
	r      *http.Request
	finish chan int
}

func (c *restCmd) handleApi(w http.ResponseWriter, r *http.Request) {
	f := make(chan int, 1)
	apiQueue <- apiQueueItem{
		w:      w,
		r:      r,
		finish: f,
	}
	<-f
}

func (c *restCmd) handleApiDo() {
	for {
		c.handleApiDoLoop()
	}
}

func (c *restCmd) handleApiDoLoop() {
	queueItem := <-apiQueue
	w := queueItem.w
	r := queueItem.r
	defer close(queueItem.finish)
	urlpath := strings.Trim(r.URL.Path, "/")
	if urlpath == "quit" {
		os.Exit(0)
	}
	if strings.HasSuffix(urlpath, "/help") || urlpath == "help" {
		c.handleHelp(w, r)
		return
	}

	subcommands := false
	for _, command := range c.apiCommands {
		if !strings.HasSuffix(command.path, "/help") && strings.HasPrefix(command.path, urlpath) && len(command.path) > len(urlpath) {
			fmt.Fprint(w, command.path+"\n")
			subcommands = true
		}
	}
	if subcommands {
		return
	}

	// command = []string{"xdr","connect"}
	// handle and parse payload to command struct
	// execute command struct .Execute
	pr, pw, err := os.Pipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	os.Stdout = pw
	os.Stderr = pw
	log.SetOutput(pw)
	buf := new(bytes.Buffer)
	defer func() {
		os.Stdout = c.stdout
		os.Stderr = c.stderr
		log.SetOutput(c.logout)
	}()
	defer pr.Close()
	defer pw.Close()
	go c.copy(buf, pr)

	command := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	c.resetBools()
	a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)
	a.iniParser = flags.NewIniParser(a.parser)
	a.parser.ParseArgs([]string{})
	a.parseFile()
	a.parser.ParseArgs([]string{})

	keys := []string{}
	keyField := reflect.ValueOf(a.opts).Elem()
	v, err := c.findCommand(keyField, strings.Join(keys, "."), "", []string{}, command)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if string(body) == "" {
		body = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	err = dec.Decode(v.Addr().Interface())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if command[0] == "attach" {
		if len(c.getTail(v)) == 0 {
			http.Error(w, "Tail is not optional for attach commands via the rest api", http.StatusBadRequest)
			return
		}
	}

	if len(command) == 2 && command[0] == "config" && command[1] == "backend" && strings.Contains(string(body), "\"Type\"") {
		a.opts.Config.Backend.typeSet = "yes"
	} else {
		a.opts.Config.Backend.typeSet = ""
	}
	c.apiRunCommand(v, w, buf)
}

func (c *restCmd) copy(buf *bytes.Buffer, pr *os.File) {
	for {
		_, err := io.Copy(buf, pr)
		if err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *restCmd) getTail(v *reflect.Value) []string {
	tail := v.FieldByName("Tail")
	if !tail.IsValid() {
		return []string{}
	}
	return tail.Interface().([]string)
}

func (c *restCmd) apiRunCommand(v *reflect.Value, w http.ResponseWriter, buf *bytes.Buffer) {
	// run the execute command and stream results back to response.data, if error from command, also set response.err
	tailv := c.getTail(v)
	tail := []reflect.Value{reflect.ValueOf(tailv)}
	outv := v.Addr().MethodByName("Execute").Call(tail)
	out := outv[0].Interface()
	switch out.(type) {
	case error:
		w.WriteHeader(http.StatusInternalServerError)
	}
	time.Sleep(20 * time.Millisecond)
	io.Copy(w, buf)
	switch err := out.(type) {
	case error:
		w.Write([]byte(err.Error()))
	}
}

func (c *restCmd) findCommand(keyField reflect.Value, start string, tags reflect.StructTag, tagStack []string, command []string) (v *reflect.Value, err error) {
	tagCommand := tags.Get("command")
	if tagCommand != "" {
		tagStack = append(tagStack, tagCommand)
	}
	for i, t := range tagStack {
		if (len(command) > i && command[i] != t) || len(command) <= i {
			return
		}
	}
	if testEq(command, tagStack) {
		return &keyField, nil
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
				v, err = c.findCommand(keyField.Field(i), start, fieldTag, tagStack, command)
				if err != nil || v != nil {
					return
				}
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
			v, err = c.findCommand(keyField.Field(i), fieldName, fieldTag, tagStack, command)
			if err != nil || v != nil {
				return
			}
		}
	}
	return
}

func testEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
