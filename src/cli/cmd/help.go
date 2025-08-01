package cmd

import (
	"fmt"
	"os"

	"github.com/aerospike/aerolab/pkg/utils/shutdown"
)

type HelpCmd struct{}

func (c *HelpCmd) Execute(args []string) error {
	PrintHelp("")
	return nil
}

func PrintHelp(extraInfo string) error {
	args := []string{}
	for _, arg := range os.Args[1:] {
		if arg == "help" {
			continue
		}
		args = append(args, arg)
	}
	if len(args) == 0 {
		args = []string{"-h"}
	}
	system, err := Initialize(&Init{}, []string{"help"}, nil, args...)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
		return nil
	}
	system.Parser.WriteHelp(os.Stdout)
	fmt.Println("")
	if extraInfo != "" {
		fmt.Print(extraInfo)
	}
	shutdown.WaitJobs()
	os.Exit(1)
	return nil
}
