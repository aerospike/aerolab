//go:build noeks

package cmd

import "fmt"

type ClientCreateEksCtlCmd struct {
	ClientCreateNoneCmd
}

func (c *ClientCreateEksCtlCmd) Execute(args []string) error {
	return fmt.Errorf("EKS support is not available in this build")
}
