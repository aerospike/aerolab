package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
)

// if tag "command" - /command/
// if tag "long" - part of payload
// payload is json containing: {"longName":"value",...}
/* example
   http://127.0.0.1:3030/cluster/create
   {
		"name": "bob",
		"count": 3,
		"aerospike-version": "5.7.*"
   }
*/

type restCmd struct {
	Listen      string        `short:"l" long:"listen" description:"IP:PORT to listen on" default:"127.0.0.1:3030"`
	Help        attachCmdHelp `command:"help" subcommands-optional:"true" description:"Print help"`
	apiCommands []apiCommand
	stdout      *os.File
	stderr      *os.File
	logout      io.Writer
}

type apiCommand struct {
	path        string
	description string
}

func (c *restCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	keys := []string{}
	keyField := reflect.ValueOf(a.opts).Elem()
	c.makeApi(keyField, strings.Join(keys, "."), "")
	log.Printf("Listening on %s...", c.Listen)
	c.stdout = os.Stdout
	c.stderr = os.Stderr
	c.logout = log.Writer()
	go c.handleApiDo()
	return http.ListenAndServe(c.Listen, nil)
}
