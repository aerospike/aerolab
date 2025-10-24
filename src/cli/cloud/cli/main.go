package main

import (
	"fmt"
	"os"

	"github.com/aerospike/aerolab/cli/cloud/cloud"
)

func main() {
	err := cloud.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
