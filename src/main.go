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
	"os/user"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
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

type Exit struct{ Code int }

var beepCount = 0

// exit code handler
func handleExit() {
	if e := recover(); e != nil {
		if exit, ok := e.(Exit); ok {
			for i := 0; i < beepCount; i++ {
				fmt.Printf("\a")
			}
			if _, ok := os.LookupEnv("JPY_SESSION_NAME"); ok {
				fmt.Printf("\nERROR EXIT_CODE=%d\n", exit.Code)
			}
			os.Exit(exit.Code)
		}
		panic(e)
	}
}

type logWriter struct {
}

func (writer logWriter) Write(bytes []byte) (int, error) {
	return fmt.Fprintf(os.Stderr, time.Now().Format("2006/01/02 15:04:05 -0700")+" "+string(bytes))
}

func main() {
	log.SetFlags(0)
	log.SetOutput(new(logWriter))
	if installSelf() {
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "upgrade" {
		if len(os.Args) < 3 || os.Args[1] != "agi" || os.Args[2] != "exec" {
			if !inslice.HasString(os.Args, "--webui") {
				go a.isLatestVersion()
			}
		}
	}
	_, command := path.Split(os.Args[0])
	switch command {
	case "showsysinfo", "showconf", "showinterrupts":
		showcommands()
	default:
		if beepenv := os.Getenv("AEROLAB_BEEP"); beepenv != "" {
			bp, err := strconv.Atoi(beepenv)
			if err != nil {
				log.Printf("ERROR: AEROLAB_BEEP must be an integer")
			} else if bp > 0 {
				beepCount += bp
				defer func() {
					fmt.Printf("\a")
				}()
			}
		}
		if beepenv := os.Getenv("AEROLAB_BEEPF"); beepenv != "" {
			bp, err := strconv.Atoi(beepenv)
			if err != nil {
				log.Printf("ERROR: AEROLAB_BEEPF must be an integer")
			} else if bp > 0 {
				beepCount += bp
			}
		}
		args := []string{}
		for _, arg := range os.Args[1:] {
			if arg == "--beep" {
				if beepCount == 0 {
					defer func() {
						fmt.Printf("\a")
					}()
				}
				beepCount++
			} else if arg == "--beepf" {
				beepCount++
			} else {
				args = append(args, arg)
			}
		}
		err := a.main(args)
		if err != nil {
			defer handleExit()
			panic(Exit{1})
		}
	}
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

func (a *aerolab) main(args []string) error {
	setOwners()
	defer backendRestoreTerminal()
	a.createDefaults()
	a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)

	// preload file to load parsers based on backend
	a.iniParser = flags.NewIniParser(a.parser)
	ffo := a.forceFileOptional
	a.forceFileOptional = true
	a.parseFile()
	a.forceFileOptional = ffo

	a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)
	populateAllBackends := false
	if len(args) >= 2 && args[0] == "config" && args[1] == "defaults" {
		populateAllBackends = true
	}
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
				logExit(err)
			}
			if string(backend) != a.opts.Config.Backend.Type && !populateAllBackends {
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
	if err != nil && os.Args[1] != "webui" {
		_, fna := path.Split(os.Args[0])
		fmt.Printf(chooseBackendHelpMsg, fna, fna, fna)
		os.Exit(1)
	}
	if !a.forceFileOptional && a.opts.Config.Backend.Type == "" && os.Args[1] != "webui" && os.Args[1] != "webrun" {
		_, fna := path.Split(os.Args[0])
		fmt.Printf(chooseBackendHelpMsg, fna, fna, fna)
		os.Exit(1)
	}

	err = a.parseArgs(args)
	return err
}

var currentOwnerUser = ""

func setOwners() {
	user, err := user.Current()
	if err != nil {
		return
	}
	uname := ""
	for _, r := range user.Username {
		if r < 48 {
			continue
		}
		if r > 57 && r < 65 {
			continue
		}
		if r > 90 && r < 97 {
			continue
		}
		if r > 122 {
			continue
		}
		uname = uname + string(r)
	}
	if runtime.GOOS == "windows" {
		unamex := strings.Split(uname, "\\")
		uname = unamex[len(unamex)-1]
	}
	if len(uname) > 63 {
		uname = uname[:63]
	}
	currentOwnerUser = strings.ToLower(uname)
}

func earlyProcessNoBackend(tail []string) (early bool) {
	if inslice.HasString(tail, "help") {
		a.parser.WriteHelp(os.Stderr)
		//a.parser.WriteManPage(os.Stderr)
		os.Exit(1)
	}
	return a.early
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
			logExit("Failed to write config file: %s", err)
		}
	}
	if a.opts.DryRun {
		fmt.Println("OK(dry-run)")
		os.Exit(0)
	}
	var err error
	b, err = getBackend()
	if err != nil {
		logExit("Could not get backend: %s", err)
	}
	if initBackend {
		if b == nil {
			logExit("Invalid backend")
		}
		err = b.Init()
		if err != nil {
			logExit("Could not init backend: %s", err)
		}
	}
	if b != nil {
		b.WorkOnServers()
	}
	log.SetFlags(0)
	log.SetOutput(&tStderr{})
	// webui call: exit early, webrun will trigger telemetry separately
	if len(os.Args) >= 1 && os.Args[1] == "webrun" {
		return false
	}

	telemetryNoSaveMutex.Lock()
	expiryTelemetryLock.Lock()
	go a.telemetry("")
	/*
		err = a.telemetry()
		if err != nil {
			log.Printf("TELEMETRY:%s", err)
		} else {
			log.Print("TELEMETRY:OK")
		}
	*/
	return false
}

var currentTelemetry telemetryItem
var telemetryDir string
var telemetryNoSave = true
var telemetryNoSaveMutex = new(sync.Mutex)
var telemetryMutex = new(sync.Mutex)
var expiryTelemetryUUID = ""
var expiryTelemetryLock = new(sync.Mutex)

type tStderr struct {
	OutSize int
}

func (t *tStderr) Write(b []byte) (int, error) {
	telemetryMutex.Lock()
	defer telemetryMutex.Unlock()
	if t.OutSize > 100 {
		currentTelemetry.StderrTruncated = true
		return os.Stderr.Write(b)
	}
	currentTelemetry.Stderr = append(currentTelemetry.Stderr, string(b))
	t.OutSize++
	bb := make([]byte, 0, 26+len(b))
	bb = append(bb, []byte(time.Now().Format("2006/01/02 15:04:05 -0700"))...)
	bb = append(bb, ' ')
	bb = append(bb, b...)
	return os.Stderr.Write(bb)
}

func (a *aerolab) telemetry(webuiData string) error {
	defer expiryTelemetryLock.Unlock()
	defer telemetryNoSaveMutex.Unlock()
	// basic checks
	if len(os.Args) < 2 {
		return nil
	}
	// do not ship config defaults command usage
	if (os.Args[1] == "config" && os.Args[2] == "defaults") || (os.Args[1] == "webrun" && webuiData == "") || (slices.Equal(os.Args[1:], webuiInventoryListParams)) {
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
	home, err := a.aerolabRootDir()
	if err != nil {
		return err
	}
	telemetryDir = path.Join(home, "telemetry")

	// check if telemetry is disabled
	if _, err := os.Stat(path.Join(telemetryDir, "disable")); err == nil {
		return err
	}

	// create telemetry dir
	if _, err := os.Stat(telemetryDir); err != nil {
		err = os.MkdirAll(telemetryDir, 0700)
		if err != nil {
			return err
		}
	}

	// generate uid for telemetry if it doesn't exist
	var uuidx []byte
	uuidFile := path.Join(telemetryDir, "uuid")
	if _, err := os.Stat(uuidFile); err != nil {
		uuidx = []byte(uuid.New().String())
		err = os.WriteFile(uuidFile, uuidx, 0600)
		if err != nil {
			return err
		}
	} else {
		uuidx, err = os.ReadFile(uuidFile)
		if err != nil {
			return err
		}
	}
	expiryTelemetryUUID = string(uuidx)
	// create telemetry item
	currentTelemetry.UUID = string(uuidx)
	currentTelemetry.StartTime = time.Now().UnixMicro()
	currentTelemetry.CmdLine = os.Args[1:]
	currentTelemetry.Version = telemetryVersion
	currentTelemetry.AeroLabVersion = version
	webCmdParams := strings.Split(webuiData, "-=-=-=-")
	currentTelemetry.WebRun.Command = strings.Split(strings.Trim(webCmdParams[0], "/"), "/")
	if len(webCmdParams) > 1 {
		webParams := make(map[string]interface{})
		err = json.Unmarshal([]byte(webCmdParams[1]), &webParams)
		if err != nil {
			webParams["ERROR-UNMARSHAL"] = err.Error()
		}
		currentTelemetry.WebRun.Params = webParams
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
		currentTelemetry.Defaults = append(currentTelemetry.Defaults, telemetryDefault{
			Key:   val.key,
			Value: val.value,
		})
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
	if len(telemetryFiles) < 1000 {
		telemetryNoSave = false
	}

	go func() {
		// sort telemetryFiles
		sort.Strings(telemetryFiles)

		// ship telemetryFiles oldest to newest and remove from disk
		for _, file := range telemetryFiles {
			if err = telemetryShip(file); err == nil {
				err = os.Remove(file)
				if err != nil {
					return
				}
			}
		}
	}()
	return nil
}

func telemetrySaveCurrent(returnError error) error {
	telemetryNoSaveMutex.Lock()
	if telemetryNoSave {
		telemetryNoSaveMutex.Unlock()
		return nil
	}
	telemetryNoSaveMutex.Unlock()
	if returnError != nil {
		currentTelemetry.Error = aws.String(returnError.Error())
	}
	currentTelemetry.EndTime = time.Now().UnixMicro()
	telemetryString, err := json.Marshal(currentTelemetry)
	if err != nil {
		return err
	}
	newFile := path.Join(telemetryDir, "item-"+strconv.Itoa(int(currentTelemetry.StartTime)))
	err = os.WriteFile(newFile, telemetryString, 0600)
	if err != nil {
		return err
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
	CmdLine         []string
	UUID            string
	StartTime       int64
	EndTime         int64
	Defaults        []telemetryDefault
	Version         string
	AeroLabVersion  string
	Error           *string
	Stderr          []string
	StderrTruncated bool
	WebRun          struct {
		Command []string
		Params  map[string]interface{}
	}
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
	os.WriteFile(cfgFile+".ts", []byte(time.Now().Format(time.RFC3339)), 0644)
	return nil
}

func (a *aerolab) parseArgs(args []string) error {
	a.opts.Config.Defaults.Reset = false
	a.opts.Config.Defaults.OnlyChanged = false
	a.opts.Config.Defaults.Key = ""
	a.opts.Config.Defaults.Value = ""
	_, err := a.parser.ParseArgs(args)
	if a.early {
		return nil
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
			log.Println(err)
		}
		telemetrySaveCurrent(err)
		return err
	}
	telemetrySaveCurrent(err)
	return nil
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
		home, err = a.aerolabRootDir()
		if err != nil {
			return
		}
		cfgFile = path.Join(home, "conf")
	}
	return
}

func (a *aerolab) createDefaults() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("WARN could not determine user's home directory: %s", err)
		return
	}
	ahome, err := a.aerolabRootDir()
	if err != nil {
		log.Printf("WARN could not determine user's home directory: %s", err)
		return
	}
	if _, err := os.Stat(ahome); err != nil {
		err = os.MkdirAll(ahome, 0700)
		if err != nil {
			log.Printf("WARN could not create %s, configuration files may not be available: %s", ahome, err)
			return
		}
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
