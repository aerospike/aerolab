package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strings"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type helpCmd struct{}

type commandsDefaults struct {
	MakeConfig bool    `hidden:"true" long:"make-config" description:"Make configuration file with current parameters"`
	DryRun     bool    `hidden:"true" long:"dry-run" description:"Do not run the command (useful with --make-config parameter)"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *helpCmd) Execute(args []string) error {
	return printHelp("")
}

func printHelp(extraInfo string) error {
	params := []string{}
	for _, comm := range os.Args[1:] {
		if strings.HasPrefix(comm, "-") {
			continue
		}
		if comm == "help" {
			continue
		}
		params = append(params, comm)
	}
	params = append(params, "-h")
	cmd := exec.Command(os.Args[0], params...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Run()
	if extraInfo != "" {
		fmt.Print(extraInfo)
	}
	os.Exit(1)
	return nil
}

type commandPath string

type backendName string

var backendSwitches = make(map[commandPath]map[backendName]interface{})

var b backend

func addBackendSwitch(command string, backend string, switches interface{}) {
	if _, ok := backendSwitches[commandPath(command)]; !ok {
		backendSwitches[commandPath(command)] = make(map[backendName]interface{})
	}
	backendSwitches[commandPath(command)][backendName(backend)] = switches
}

type aerolab struct {
	opts              *commands
	parser            *flags.Parser
	iniParser         *flags.IniParser
	forceFileOptional bool
	early             bool
}

var a = &aerolab{
	opts: new(commands),
}

func main() {
	a.main(os.Args[0], os.Args[1:])
}

var chooseBackendHelpMsg = `
Create a config file and select a backend first using one of:

$ %s config backend -t docker [-d /path/to/tmpdir/for-aerolab/to/use]
$ %s config backend -t aws [-r region] [-p /custom/path/to/store/ssh/keys/in/] [-d /path/to/tmpdir/for-aerolab/to/use]
$ %s config backend -t gcp -o project-name [-d /path/to/tmpdir/for-aerolab/to/use]

Default file path is ${HOME}/.aerolab.conf

To specify a custom configuration file, set the environment variable:
   $ export AEROLAB_CONFIG_FILE=/path/to/file.conf

`

func (a *aerolab) main(name string, args []string) {
	a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)

	// preload file to load parsers based on backend
	a.iniParser = flags.NewIniParser(a.parser)
	ffo := a.forceFileOptional
	a.forceFileOptional = true
	a.parseFile()
	a.forceFileOptional = ffo

	a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)
	for command, switchList := range backendSwitches {
		keys := strings.Split(strings.ToLower(string(command)), ".")
		var nCmd *flags.Command
		for i, key := range keys {
			if i == 0 {
				nCmd = a.parser.Find(key)
			} else {
				nCmd = nCmd.Find(key)
			}
		}
		for backend, switches := range switchList {
			grp, err := nCmd.AddGroup(string(backend), string(backend), switches)
			if err != nil {
				log.Fatal(err)
			}
			if string(backend) != a.opts.Config.Backend.Type {
				grp.Hidden = true
			}
		}
	}

	// start loading
	a.iniParser = flags.NewIniParser(a.parser)
	a.early = true
	a.parseArgs(args)
	a.early = false
	_, err := a.parseFile()
	if err != nil {
		_, fna := path.Split(os.Args[0])
		fmt.Printf(chooseBackendHelpMsg, fna, fna)
		os.Exit(1)
	}
	if !a.forceFileOptional && a.opts.Config.Backend.Type == "" {
		_, fna := path.Split(os.Args[0])
		fmt.Printf(chooseBackendHelpMsg, fna, fna)
		os.Exit(1)
	}

	a.parseArgs(args)
}

func earlyProcess(tail []string) (early bool) {
	return earlyProcessV2(tail, true)
}

func earlyProcessV2(tail []string, initBackend bool) (early bool) {
	if inslice.HasString(tail, "help") {
		a.parser.WriteHelp(os.Stderr)
		//a.parser.WriteManPage(os.Stderr)
		os.Exit(1)
	}
	if a.early {
		return true
	}
	if a.opts.MakeConfig {
		err := writeConfigFile()
		if err != nil {
			log.Fatalf("Failed to write config file: %s", err)
		}
	}
	if a.opts.DryRun {
		fmt.Println("OK(dry-run)")
		os.Exit(0)
	}
	var err error
	b, err = getBackend()
	if err != nil {
		log.Fatalf("Could not get backend: %s", err)
	}
	if initBackend {
		if b == nil {
			log.Fatalf("Invalid backend")
		}
		err = b.Init()
		if err != nil {
			log.Fatalf("Could not init backend: %s", err)
		}
	}
	if b != nil {
		b.WorkOnServers()
	}
	return false
}

func writeConfigFile() error {
	cfgFile, _, err := a.configFileName()
	if err != nil {
		return err
	}
	opts := flags.IniOptions(flags.IniIncludeComments | flags.IniIncludeDefaults | flags.IniCommentDefaults)
	prev := a.opts.MakeConfig
	prev2 := a.opts.DryRun
	a.opts.DryRun = false
	a.opts.MakeConfig = false
	err = a.iniParser.WriteFile(cfgFile, opts)
	a.opts.MakeConfig = prev
	a.opts.DryRun = prev2
	if err != nil {
		return err
	}
	return nil
}

func (a *aerolab) parseArgs(args []string) {
	_, err := a.parser.ParseArgs(args)
	if a.early {
		return
	}
	if err != nil {
		if reflect.TypeOf(err).Elem().String() == "flags.Error" {
			flagsErr := err.(*flags.Error)
			if flagsErr.Type == flags.ErrCommandRequired {
				a.parser.WriteHelp(os.Stderr)
			} else {
				fmt.Println(err)
			}
		} else {
			fmt.Println(err)
		}
		os.Exit(1)
	}
}

func (a *aerolab) parseFile() (cfgFile string, err error) {
	var optional bool
	cfgFile, optional, err = a.configFileName()
	if err != nil {
		return
	}

	if a.forceFileOptional {
		optional = true
	}

	if _, err = os.Stat(cfgFile); err != nil && os.IsNotExist(err) {
		if optional {
			err = nil
			return
		} else {
			return
		}
	}

	a.parser.Options = flags.HelpFlag | flags.PassDoubleDash | flags.IgnoreUnknown
	err = a.iniParser.ParseFile(cfgFile)
	a.parser.Options = flags.HelpFlag | flags.PassDoubleDash
	if err != nil {
		log.Print(err)
	}
	return
}

func (a *aerolab) configFileName() (cfgFile string, optional bool, err error) {
	cfgFile, _ = os.LookupEnv("AEROLAB_CONFIG_FILE")
	optional = false
	if a.opts.MakeConfig {
		optional = true
	}
	if cfgFile == "" {
		optional = true
		var home string
		home, err = os.UserHomeDir()
		if err != nil {
			return
		}
		cfgFile = path.Join(home, ".aerolab.conf")
	}
	return
}
