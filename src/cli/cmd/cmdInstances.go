package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type InstancesCmd struct {
	Create          InstancesCreateCmd          `command:"create" subcommands-optional:"true" description:"Create a new instance cluster" webicon:"fas fa-plus"`
	Grow            InstancesGrowCmd            `command:"grow" subcommands-optional:"true" description:"Grow an existing instance cluster" webicon:"fas fa-plus"`
	List            InstancesListCmd            `command:"list" subcommands-optional:"true" description:"List instances" webicon:"fas fa-list"`
	Attach          InstancesAttachCmd          `command:"attach" subcommands-optional:"true" description:"Attach to an instance or cluster" webicon:"fas fa-terminal"`
	Start           InstancesStartCmd           `command:"start" subcommands-optional:"true" description:"Start an instance or cluster" webicon:"fas fa-play"`
	Stop            InstancesStopCmd            `command:"stop" subcommands-optional:"true" description:"Stop an instance or cluster" webicon:"fas fa-stop"`
	Restart         InstancesRestartCmd         `command:"restart" subcommands-optional:"true" description:"Restart an instance or cluster" webicon:"fas fa-sync"`
	UpdateHostsFile InstancesUpdateHostsFileCmd `command:"update-hosts-file" subcommands-optional:"true" description:"Update the hosts file on the instances" webicon:"fas fa-file-alt"`
	Destroy         InstancesDestroyCmd         `command:"destroy" subcommands-optional:"true" description:"Destroy an instance or cluster" webicon:"fas fa-trash"`
	Help            HelpCmd                     `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

// expand node numbers, given as string, ex: 1,2,3,10-15
// returns a sorted list of node numbers
func expandNodeNumbers(nodeNumbers string) ([]int, error) {
	items := strings.Split(nodeNumbers, ",")
	nn := []int{}
	for _, item := range items {
		if strings.Contains(item, "-") {
			parts := strings.Split(item, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid node number range: %s", item)
			}
			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, err
			}
			if start > end {
				return nil, fmt.Errorf("invalid node number range: %s", item)
			}
			for i := start; i <= end; i++ {
				if !slices.Contains(nn, i) {
					nn = append(nn, i)
				}
			}
		} else {
			n, err := strconv.Atoi(item)
			if err != nil {
				return nil, err
			}
			if !slices.Contains(nn, n) {
				nn = append(nn, n)
			}
		}
	}
	slices.Sort(nn)
	return nn, nil
}
