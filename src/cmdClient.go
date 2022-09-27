package main

type clientCmd struct {
	Create  clientCreateCmd  `command:"create" subcommands-optional:"true" description:"Create new client machines"`
	List    clientListCmd    `command:"list" subcommands-optional:"true" description:"List client machine groups"`
	Start   clientStartCmd   `command:"start" subcommands-optional:"true" description:"Start a client machine group"`
	Stop    clientStopCmd    `command:"stop" subcommands-optional:"true" description:"Stop a client machine group"`
	Grow    clientGrowCmd    `command:"grow" subcommands-optional:"true" description:"Grow a client machine group"`
	Destroy clientDestroyCmd `command:"destroy" subcommands-optional:"true" description:"Destroy client(s)"`
	Help    helpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientCreateCmd struct {
	Basic clientCreateBasicCmd `command:"basic" subcommands-optional:"true" description:"basic machine with a few linux tools"`
	Tools clientCreateToolsCmd `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	Help  helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientCreateBasicCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}
type clientCreateToolsCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientListCmd struct{}
type clientStartCmd struct{}
type clientStopCmd struct{}
type clientGrowCmd struct{}
type clientDestroyCmd struct{}
