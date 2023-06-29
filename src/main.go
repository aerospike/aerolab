package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bestmethod/inslice"
	"github.com/google/uuid"
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

Default file path is ${HOME}/.aerolab/conf

To specify a custom configuration file, set the environment variable:
   $ export AEROLAB_CONFIG_FILE=/path/to/file.conf

`

func (a *aerolab) main(name string, args []string) {
	a.createDefaults()
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
	go telemetry()
	/*
		err = telemetry()
		if err != nil {
			log.Printf("TELEMETRY:%s", err)
		} else {
			log.Print("TELEMETRY:OK")
		}
	*/
	return false
}

func telemetry() error {
	// basic checks
	if len(os.Args) < 2 {
		return nil
	}
	// do not ship config defaults command usage
	if os.Args[1] == "config" && os.Args[2] == "defaults" {
		return nil
	}
	// only enable if a feature file is present and belongs to Aerospike internal users
	if a.opts.Cluster.Create.FeaturesFilePath == "" {
		return nil
	}
	enableTelemetry := false
	err := filepath.WalkDir(string(a.opts.Cluster.Create.FeaturesFilePath), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if err = scanner.Err(); err != nil {
				return err
			}
			line := strings.ToLower(strings.Trim(scanner.Text(), "\r\n\t "))
			if strings.HasPrefix(line, "account-name") && (strings.HasSuffix(line, "aerospike") || strings.Contains(line, " aerospike") || strings.Contains(line, "aerospike_test") || strings.Contains(line, "\taerospike")) {
				enableTelemetry = true
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !enableTelemetry {
		return nil
	}

	// resolve telemetry directory
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	telemetryDir := path.Join(home, ".aerolab/telemetry")

	// check if telemetry is disabled
	if _, err := os.Stat(path.Join(home, ".aerolab/telemetry/disable")); err == nil {
		return err
	}

	// create telemetry dir
	if _, err := os.Stat(telemetryDir); err != nil {
		err = os.MkdirAll(telemetryDir, 0755)
		if err != nil {
			return err
		}
	}

	// generate uid for telemetry if it doesn't exist
	var uuidx []byte
	uuidFile := path.Join(telemetryDir, "uuid")
	if _, err := os.Stat(uuidFile); err != nil {
		uuidx = []byte(uuid.New().String())
		err = os.WriteFile(uuidFile, uuidx, 0644)
		if err != nil {
			return err
		}
	} else {
		uuidx, err = os.ReadFile(uuidFile)
		if err != nil {
			return err
		}
	}

	// get telemetry items
	telemetryFiles := []string{}
	err = filepath.WalkDir(telemetryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "item-") {
			return nil
		}
		telemetryFiles = append(telemetryFiles, path)
		return nil
	})
	if err != nil {
		return err
	}

	// if telemetry count > 1000, abort
	if len(telemetryFiles) > 1000 {
		return nil
	}

	// sort telemetryFiles
	sort.Strings(telemetryFiles)

	// create telemetry item
	item := telemetryItem{
		UUID:    string(uuidx),
		Time:    time.Now().UnixMicro(),
		CmdLine: os.Args[1:],
	}

	// add changed default values to the item
	ret := make(chan configValueCmd, 1)
	a.opts.Config.Defaults.OnlyChanged = true
	keyField := reflect.ValueOf(a.opts).Elem()
	go a.opts.Config.Defaults.getValues(keyField, "", ret, "")
	for {
		val, ok := <-ret
		if !ok {
			break
		}
		if strings.HasSuffix(val.key, ".Password") || strings.HasSuffix(val.key, ".Pass") || strings.HasSuffix(val.key, ".User") || strings.HasSuffix(val.key, ".Username") {
			continue
		}
		item.Defaults = append(item.Defaults, telemetryDefault{
			Key:   val.key,
			Value: val.value,
		})
	}

	// add current command to telemetry
	telemetryString, err := json.Marshal(item)
	if err != nil {
		return err
	}
	newFile := path.Join(telemetryDir, "item-"+strconv.Itoa(int(item.Time)))
	err = os.WriteFile(newFile, telemetryString, 0644)
	if err != nil {
		return err
	}
	telemetryFiles = append(telemetryFiles, newFile)

	// ship telemetryFiles oldest to newest and remove from disk
	for _, file := range telemetryFiles {
		if err = telemetryShip(file); err == nil {
			err = os.Remove(file)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func telemetryShip(file string) error {
	contents, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	url := "https://us-central1-aerospike-gaia.cloudfunctions.net/aerolab-telemetrics"
	ret, err := http.Post(url, "application/json", bytes.NewReader(contents))
	if err != nil {
		return err
	}
	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("returned ret code: %d:%s", ret.StatusCode, ret.Status)
	}
	return nil
}

type telemetryItem struct {
	CmdLine  []string
	UUID     string
	Time     int64
	Defaults []telemetryDefault
}

type telemetryDefault struct {
	Key   string
	Value string
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
		cfgFile = path.Join(home, ".aerolab/conf")
	}
	return
}

func (a *aerolab) createDefaults() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("WARN could not determine user's home directory: %s", err)
		return
	}
	ahome := path.Join(home, ".aerolab")
	if _, err := os.Stat(ahome); err != nil {
		err = os.MkdirAll(ahome, 0755)
		if err != nil {
			log.Printf("WARN could not create %s, configuration files may not be available: %s", ahome, err)
			return
		}
	}
	if _, err := os.Stat(path.Join(ahome, "telemetry")); err != nil {
		os.Mkdir(path.Join(ahome, "telemetry"), 0755)
	}
	if _, err := os.Stat(path.Join(ahome, "conf")); err != nil {
		if _, err := os.Stat(path.Join(home, ".aerolab.conf")); err == nil {
			err = os.Rename(path.Join(home, ".aerolab.conf"), path.Join(ahome, "conf"))
			if err != nil {
				log.Printf("WARN failed to migrate ~/.aerolab.conf to ~/.aerolab/conf")
			}
		}
	}
}
