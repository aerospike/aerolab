package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"

	_ "embed"
)

var ErrExecuteError = errors.New("execute error")

type ExecuteError struct {
	Err    error
	Logger *logger.Logger
}

func (e *ExecuteError) Error() string {
	return e.Err.Error()
}

func (e *ExecuteError) Unwrap() error {
	return ErrExecuteError
}

func Error(err error, system *System, command []string, params interface{}, args []string) error {
	// telemetry hook
	TelemetryEvent(command, params, args, system, err)
	if err == nil {
		return nil
	}
	system.Logger.Error("%s", err.Error())
	return &ExecuteError{
		Err:    err,
		Logger: system.Logger,
	}
}

type System struct {
	// Logger is set as part of the Initialize function call, after-init functions can use it to log messages
	Logger *logger.Logger
	// all available commands
	Opts *Commands
	// flag parser
	Parser *flags.Parser
	// config file parser
	IniParser *flags.IniParser
	// tail arguments
	Tail []string
	// backend
	Backend backends.Backend
	// internal: log level
	logLevel logger.LogLevel
	// init options are saved here
	InitOptions *Init
	// init time
	InitTime time.Time
	// log buffer
	LogBuffer chan string
	// log buffer truncated
	LogBufferTruncated bool
}

type Init struct {
	InitBackend        bool                // initialize backend as part of startup; if trying to pollInventoryHourly without filling InitBackend, set to false, and call GetBackend() later
	RunExecuteFunction bool                // only set to true if you are initializing the application for the first time
	UpgradeCheck       bool                // check for upgrades as part of startup, and print a message if a new version is available
	Backend            *InitBackend        // backend configuration; optional, if not specified, will be auto-filled
	ExistingInventory  *backends.Inventory // existing inventory, if requested to be set by the caller
	AllBackendsHelp    bool                // if true, show help for all backends, not just the selected one
}

type InitBackend struct {
	PollInventoryHourly bool                 // whether the backend(s) should refresh inventory hourly - this is useful if running the project as a long-running service instead of CLI app
	UseCache            bool                 // whether to use local cache for the backend inventory - only use if not sharing the GCP/AWS project/account with other users
	LogMillisecond      bool                 // whether to log milliseconds - whether to enable millisecond logging
	ListAllProjects     bool                 // whether to list all projects - set to list all GCP projects in the backend inventory
	GCPAuthMethod       clouds.GCPAuthMethod // GCP authentication method to use - if not set, a default auth account will be used
	GCPBrowser          bool                 // whether to open a browser to authenticate with GCP - if false, the user will need to manually visit the auth URL with GCP
	GCPClientID         string               // GCP client ID used for authentication - if not set, a default auth account will be used
	GCPClientSecret     string               // GCP client secret used for authentication - if not set, a default auth account will be used
}

func Initialize(i *Init, command []string, params interface{}, args ...string) (*System, error) {
	s := &System{
		Logger:             logger.NewLogger(),
		Opts:               &Commands{},
		Parser:             &flags.Parser{},
		IniParser:          &flags.IniParser{},
		InitOptions:        i,
		InitTime:           time.Now(),
		LogBuffer:          make(chan string, 1000),
		LogBufferTruncated: false,
	}
	s.logLevel = logger.INFO
	switch strings.ToUpper(os.Getenv("AEROLAB_LOG_LEVEL")) {
	case "DEBUG":
		s.logLevel = logger.DEBUG
	case "INFO":
		s.logLevel = logger.INFO
	case "DETAIL":
		s.logLevel = logger.DETAIL
	case "ERROR":
		s.logLevel = logger.ERROR
	case "CRITICAL":
		s.logLevel = logger.CRITICAL
	case "WARNING":
		s.logLevel = logger.WARNING
	}
	s.Logger.SetLogLevel(s.logLevel)
	s.Logger.SinkBuffer(s.LogBuffer, &s.LogBufferTruncated)
	if command != nil {
		s.Logger = s.Logger.WithPrefix(fmt.Sprintf("[%s] ", strings.Join(command, ".")))
	}

	// capture SIGINT/SIGTERM with telemetry using the shutdown handler
	shutdown.AddLateCleanupJob("telemetry", func(isSignal bool) {
		if isSignal {
			TelemetryEvent(command, params, args, s, errors.New("signal received, exiting"))
		}
	})

	// telemetry sender
	TelemetrySend(s.Logger.WithPrefix("TELEMETRY-SHIP: "))

	if len(args) == 0 {
		args = os.Args[1:]
	}

	// initialize the parser
	s.Parser = flags.NewParser(s.Opts, flags.HelpFlag|flags.PassDoubleDash|flags.IniIncludeDefaults|flags.IniIncludeComments|flags.IniCommentDefaults)
	s.IniParser = flags.NewIniParser(s.Parser)

	// get the config file name
	cfgFile, err := ConfigFileName()
	if err != nil {
		return s, err
	}

	// create directory if it does not exist
	dir := filepath.Dir(cfgFile)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return s, err
		}
	}

	// if the config file does not exist, create it
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		err = os.WriteFile(cfgFile, []byte(""), 0644)
		if err != nil {
			return s, err
		}
	}

	// parse the ini file
	err = s.IniParser.ParseFile(cfgFile)
	if err != nil {
		return s, err
	}

	if !i.RunExecuteFunction {
		// the init is called from within an execute function, do not run any additional command logic
		s.Parser.CommandHandler = func(command flags.Commander, args []string) error {
			return nil
		}
		if i.UpgradeCheck {
			// the execute function may choose to check for upgrades and print a message if a new version is available
			i.upgradeCheck(s)
		}
	}

	// hide switches for backends that are not currently enabled
	if s.Opts.Config.Backend.Type != "" && !i.AllBackendsHelp {
		ShowHideBackend(s.Parser, []string{s.Opts.Config.Backend.Type})
	}

	// parse the command line
	s.Tail, err = s.Parser.ParseArgs(args)
	if err != nil {
		return s, err
	}

	// backend gets initialized later, as main() calls this always WITHOUT InitBackend, and the Execute functions can choose to initialize it after parsing args
	if i.InitBackend {
		s.Logger.Info("Initializing backend")
		return s, i.backend(s, false)
	}
	return s, nil
}

type upgradeCheck struct {
	Timestamp          time.Time
	CurrentVersion     string
	IsUpgradeAvailable bool
	NewVersion         string
}

func (i *Init) upgradeCheck(s *System) {
	if os.Getenv("AEROLAB_DISABLE_UPGRADE_CHECK") != "" {
		return
	}
	log := s.Logger.WithPrefix("AEROLAB-VERSION: ")
	rootDir, err := AerolabRootDir()
	if err != nil {
		log.Detail("Could not determine user's home directory: %s", err)
		return
	}
	_, _, currentEdition, currentVersion := GetAerolabVersion()
	uc := &upgradeCheck{
		Timestamp:          time.Now().Add(-24 * time.Hour),
		CurrentVersion:     currentVersion,
		IsUpgradeAvailable: false,
		NewVersion:         "",
	}

	// try to read the upgrade check file
	upgradeCheckFile := path.Join(rootDir, "upgrade-check.json")
	if _, err := os.Stat(upgradeCheckFile); err == nil {
		upgradeCheckBytes, err := os.ReadFile(upgradeCheckFile)
		if err == nil {
			uca := &upgradeCheck{}
			err = json.Unmarshal(upgradeCheckBytes, uca)
			if err != nil {
				log.Detail("Could not parse upgrade check timestamp file: %s", err)
			} else {
				uc = uca
			}
		} else {
			log.Detail("Could not read upgrade check timestamp file: %s", err)
		}
	}

	// if the last check was done on this version, and upgrade is available, inform the user
	if uc.CurrentVersion == currentVersion && uc.IsUpgradeAvailable {
		log.Info("A new AeroLab version %s is available, install using `aerolab upgrade`, or download link: https://github.com/aerospike/aerolab/releases", uc.NewVersion)
	}

	// check if we need to pull latest version details, and do so if required
	// if the version on which the check was done is not the current one, we need to check for upgrades
	// if the check versions match, but the check is more than 24 hours old, AND upgrade was not available last time we checked, we need to check for upgrades
	if uc.CurrentVersion != currentVersion || (uc.Timestamp.Add(24*time.Hour).Before(time.Now()) && !uc.IsUpgradeAvailable) {
		shutdown.AddJob()
		go func() {
			defer shutdown.DoneJob()
			a := &UpgradeCmd{}
			// if we are on a pre-release, also check if new pre-release is available, otherwise only check stable releases
			// if we are on unofficial, also check pre-releases
			if currentEdition == "-prerelease" || currentEdition == "-unofficial" {
				a.Edge = true
			}
			install, newVersion, _, err := a.CheckForUpgrade()
			if err == nil {
				uc.IsUpgradeAvailable = install
				uc.NewVersion = newVersion
			}
			uc.Timestamp = time.Now()
			uc.CurrentVersion = currentVersion
			ucBytes, err := json.Marshal(uc)
			if err != nil {
				log.Detail("Could not marshal upgrade check: %s", err)
			} else {
				err = os.WriteFile(upgradeCheckFile, ucBytes, 0644)
				if err != nil {
					log.Detail("Could not write upgrade check file: %s", err)
				}
			}
		}()
	}
}

//go:embed initialize-backend-error.txt
var initializeBackendError string
var errNoBackendConfigured = fmt.Errorf(initializeBackendError, os.Args[0], os.Args[0], os.Args[0], os.Args[0])

// force (re)initialize the backend
func (s *System) GetBackend(pollInventoryHourly bool) error {
	return s.InitOptions.backend(s, pollInventoryHourly)
}

func (i *Init) backend(s *System, pollInventoryHourly bool) error {
	if i.Backend == nil {
		i.Backend = &InitBackend{
			PollInventoryHourly: pollInventoryHourly,
			UseCache:            s.Opts.Config.Backend.InventoryCache,
			LogMillisecond:      false,
			ListAllProjects:     false,
			GCPAuthMethod:       clouds.GCPAuthMethod(s.Opts.Config.Backend.GCPAuthMethod),
			GCPBrowser:          !s.Opts.Config.Backend.GCPNoBrowser,
			GCPClientID:         s.Opts.Config.Backend.GCPClientID,
			GCPClientSecret:     s.Opts.Config.Backend.GCPClientSecret,
		}
	}
	if s.Opts.Config.Backend.Type == "" {
		return errNoBackendConfigured
	}
	backendList := []backends.BackendType{backends.BackendType(s.Opts.Config.Backend.Type)}
	rootDir, err := AerolabRootDir()
	if err != nil {
		return err
	}
	aver, _, _, _ := GetAerolabVersion()
	var gcpSecrets *clouds.LoginGCPSecrets
	if i.Backend.GCPClientID != "" && i.Backend.GCPClientSecret != "" {
		gcpSecrets = &clouds.LoginGCPSecrets{
			ClientID:     i.Backend.GCPClientID,
			ClientSecret: i.Backend.GCPClientSecret,
		}
	}
	project := os.Getenv("AEROLAB_PROJECT")
	if project == "" {
		project = "default"
	}
	tokenCacheFilePath := path.Join(rootDir, "projects", project, "config", "gcp")
	if _, err := os.Stat(tokenCacheFilePath); os.IsNotExist(err) {
		err = os.MkdirAll(tokenCacheFilePath, 0755)
		if err != nil {
			return fmt.Errorf("could not create token cache directory: %w", err)
		}
	}
	config := &backend.Config{
		RootDir: path.Join(rootDir, "projects"),
		Cache:   i.Backend.UseCache,
		Credentials: &clouds.Credentials{
			AWS: clouds.AWS{
				AuthMethod: clouds.AWSAuthMethodShared,
				Shared: clouds.SharedAWSConfig{
					Profile: s.Opts.Config.Backend.AWSProfile,
				},
			},
			GCP: clouds.GCP{
				Project:    s.Opts.Config.Backend.Project,
				AuthMethod: i.Backend.GCPAuthMethod,
				Login: clouds.LoginGCPConfig{
					Secrets:            gcpSecrets,
					Browser:            i.Backend.GCPBrowser,
					TokenCacheFilePath: path.Join(tokenCacheFilePath, "token-cache.json"),
				},
			},
			DOCKER: clouds.DOCKER{
				EnableDefaultFromEnv: true,
			},
		},
		LogLevel:         s.logLevel,
		LogMillisecond:   i.Backend.LogMillisecond,
		AerolabVersion:   aver,
		ListAllProjects:  i.Backend.ListAllProjects,
		CustomSSHKeyPath: string(s.Opts.Config.Backend.SshKeyPath),
	}
	b, err := backend.New(project, config, i.Backend.PollInventoryHourly, backendList, i.ExistingInventory)
	if err != nil {
		return fmt.Errorf("could not initialize backend: %w", err)
	}

	// ensure only the selected region(s) are enabled
	regionList := strings.Split(s.Opts.Config.Backend.Region, ",")
	regions, err := b.ListEnabledRegions(backendList[0])
	if err != nil {
		return fmt.Errorf("could not list enabled regions: %w", err)
	}
	// remove regions not on the list
	for _, region := range regions {
		if !slices.Contains(regionList, region) {
			err = b.RemoveRegion(backendList[0], region)
			if err != nil {
				return fmt.Errorf("could not remove region %s: %w", region, err)
			}
		}
	}
	// add missing regions
	for _, region := range regionList {
		if !slices.Contains(regions, region) {
			err = b.AddRegion(backendList[0], region)
			if err != nil {
				return fmt.Errorf("could not add region %s: %w", region, err)
			}
		}
	}

	// success
	s.Backend = b
	return nil
}

func (s *System) WriteConfigFile() error {
	return writeConfigFile(s)
}

func writeConfigFile(system *System) error {
	cfgFile, err := ConfigFileName()
	if err != nil {
		return err
	}
	opts := flags.IniOptions(flags.IniIncludeComments | flags.IniIncludeDefaults | flags.IniCommentDefaults)
	err = system.IniParser.WriteFile(cfgFile, opts)
	if err != nil {
		return err
	}
	err = os.WriteFile(cfgFile+".ts", []byte(time.Now().Format(time.RFC3339)), 0644)
	if err != nil {
		return err
	}
	return nil
}

func ConfigFileName() (cfgFile string, err error) {
	cfgFile = os.Getenv("AEROLAB_CONFIG_FILE")
	if cfgFile == "" {
		var home string
		home, err = AerolabRootDir()
		if err != nil {
			return
		}
		cfgFile = path.Join(home, "conf")
	}
	return
}
