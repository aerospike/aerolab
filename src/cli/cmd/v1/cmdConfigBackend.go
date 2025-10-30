package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/go-flags"
)

type ConfigBackendCmd struct {
	Type           string         `short:"t" long:"type" description:"Supported backends: aws|docker|gcp" default:"" webchoice:"aws,gcp,docker"`
	SshKeyPath     flags.Filename `short:"p" long:"key-path" description:"Specify a custom path to store SSH keys in, default: ${HOME}/.config/aerolab" webtype:"text"`
	Region         string         `short:"r" long:"region" description:"Specify a list of regions to enable, comma-separated" default:""`
	InventoryCache bool           `short:"c" long:"inventory-cache" description:"Enable local inventory cache - use only if not sharing the GCP/AWS project/account with other users"`

	AWSProfile     string `short:"P" long:"aws-profile" description:"AWS: provide a profile to use; setting this ignores the AWS_PROFILE env variable"`
	AWSNoPublicIps bool   `long:"aws-nopublic-ip" description:"AWS: if set, aerolab will not request public IPs, and will operate on private IPs only"`

	Project         string `short:"o" long:"project" description:"GCP: specify a GCP project to use" default:""`
	GCPAuthMethod   string `short:"m" long:"gcp-auth-method" description:"GCP: specify the authentication method to use (any|login|service-account)" default:"any" webchoice:"any,login,service-account"`
	GCPNoBrowser    bool   `short:"b" long:"gcp-no-browser" description:"GCP: if set, aerolab will not open a browser to authenticate with GCP when using login method"`
	GCPClientID     string `short:"i" long:"gcp-client-id" description:"GCP: specify a GCP client ID to use"`
	GCPClientSecret string `short:"s" long:"gcp-client-secret" description:"GCP: specify a GCP client secret to use"`

	Arch        string         `short:"a" long:"docker-arch" description:"DOCKER: set to either amd64 or arm64 to force a particular architecture on docker; requires multiarch support"`
	TmpDir      flags.Filename `short:"d" long:"temp-dir" description:"use a non-default temporary directory, when using aerolab in WSL2" default:"" webtype:"text"`
	CheckAccess bool           `long:"check-access" description:"check access to the backend"`

	Help      HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	typeSet   string
	regionSet string
}

func (c *ConfigBackendCmd) Execute(args []string) error {
	// initial initialization
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, []string{"config", "backend"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"config", "backend"}, c, args)
	}

	// if we are not forcing --aws-nopublic-ip, set AWSNoPublicIps to false
	if !inslice.HasString(os.Args[1:], "--aws-nopublic-ip") {
		c.AWSNoPublicIps = false
	}

	if c.Type == "gcp" && c.Project == "" {
		return Error(errors.New("ERROR: When using GCP backend, project name must be defined. Use: aerolab config backend -t gcp -o project-name-here"), system, []string{"config", "backend"}, c, args)
	}

	// validate docker-arch
	if c.Arch != "" && c.Arch != "amd64" && c.Arch != "arm64" && c.Arch != "unset" {
		return Error(errors.New("docker-arch must be one of: unset, amd64, arm64"), system, []string{"config", "backend"}, c, args)
	}
	if c.Arch == "unset" {
		c.Arch = ""
	}

	// check if we are setting the backend type
	for _, i := range os.Args {
		if inslice.HasString([]string{"-t", "--type"}, i) {
			c.typeSet = "yes"
		}
		if inslice.HasString([]string{"-r", "--region"}, i) {
			c.regionSet = "yes"
		}
	}

	// if we are setting the backend type, execute the type set
	if c.typeSet != "" {
		err := c.ExecTypeSet(system, args)
		if err != nil {
			return Error(err, system, []string{"config", "backend"}, c, args)
		}
	}

	// display the current backend configuration
	fmt.Printf("Config.Backend.Type = %s\n", c.Type)
	if c.SshKeyPath != "" {
		fmt.Printf("Config.Backend.SshKeyPath = %s\n", c.SshKeyPath)
	} else {
		fmt.Println("Config.Backend.SshKeyPath = ${HOME}/.config/aerolab")
	}
	if c.Type == "aws" {
		fmt.Printf("Config.Backend.AWSProfile = %s\n", c.AWSProfile)
		fmt.Printf("Config.Backend.Region = %s\n", c.Region)
		fmt.Printf("Config.Backend.AWSNoPublicIps = %v\n", c.AWSNoPublicIps)
	}
	if c.Type == "gcp" {
		fmt.Printf("Config.Backend.Project = %s\n", c.Project)
		fmt.Printf("Config.Backend.Region = %s\n", c.Region)
		fmt.Printf("Config.Backend.GCPAuthMethod = %s\n", c.GCPAuthMethod)
		fmt.Printf("Config.Backend.GCPNoBrowser = %v\n", c.GCPNoBrowser)
	}
	if c.Type == "docker" && c.Arch != "" {
		fmt.Printf("Config.Backend.Arch = %s\n", c.Arch)
	}
	fmt.Printf("Config.Backend.TmpDir = %s\n", c.TmpDir)

	// check access to the backend
	if c.typeSet == "" && c.CheckAccess {
		system.Logger.Info("Checking access to the backend")
		system.InitOptions.Backend = &InitBackend{
			PollInventoryHourly: false,
			UseCache:            false,
			LogMillisecond:      false,
			ListAllProjects:     false,
			GCPAuthMethod:       clouds.GCPAuthMethod(c.GCPAuthMethod),
			GCPBrowser:          !c.GCPNoBrowser,
			GCPClientID:         c.GCPClientID,
			GCPClientSecret:     c.GCPClientSecret,
		}
		err = system.GetBackend(false)
		if err != nil {
			return Error(err, system, []string{"config", "backend"}, c, args)
		}
		system.Logger.Info("Done")
	}
	return Error(nil, system, []string{"config", "backend"}, c, args)
}

func (c *ConfigBackendCmd) ExecTypeSet(system *System, args []string) error {
	system.Logger.Info("Configuring backend")
	if c.Type == "gcp" && (c.GCPAuthMethod != "any" && c.GCPAuthMethod != "login" && c.GCPAuthMethod != "service-account") {
		return errors.New("ERROR: Invalid GCP authentication method: " + c.GCPAuthMethod)
	}
	if c.Type == "docker" {
		c.Region = ""
	} else if c.regionSet == "" {
		return errors.New("ERROR: Region is required for AWS and GCP backends")
	}

	if c.Type == "gcp" && c.Project == "" {
		return errors.New("ERROR: When using GCP backend, project name must be defined. Use: aerolab config backend -t gcp -o project-name-here")
	}
	if c.Type == "aws" || c.Type == "gcp" || c.Type == "docker" {
		if c.SshKeyPath != "" {
			if strings.Contains(string(c.SshKeyPath), "${HOME}") {
				ch, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				c.SshKeyPath = flags.Filename(strings.ReplaceAll(string(c.SshKeyPath), "${HOME}", ch))
			}
			if _, err := os.Stat(string(c.SshKeyPath)); err != nil {
				err = os.MkdirAll(string(c.SshKeyPath), 0700)
				if err != nil {
					return err
				}
			}
		}
	} else if c.Type != "none" {
		return errors.New("backend types supported: docker, aws, gcp")
	}
	if c.TmpDir == "" {
		out, err := exec.Command("uname", "-r").CombinedOutput()
		if err != nil {
			log.Println("WARNING: `uname` not found, if running in WSL2, specify the temporary directory as part of this command using `-d /path/to/tmpdir`")
		} else {
			if strings.Contains(string(out), "-WSL2") && strings.Contains(string(out), "microsoft") {
				ch, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				err = os.MkdirAll(path.Join(ch, ".aerolab.tmp"), 0755)
				if err != nil {
					return err
				}
				c.TmpDir = flags.Filename(path.Join(ch, ".aerolab.tmp"))
			}
		}
	}

	// handle bools - sticky flags
	if !slices.Contains(os.Args, "--check-access") {
		c.CheckAccess = false
	}
	if !slices.Contains(os.Args, "--inventory-cache") {
		c.InventoryCache = false
	}
	if !slices.Contains(os.Args, "--aws-nopublic-ip") {
		c.AWSNoPublicIps = false
	}
	if !slices.Contains(os.Args, "--gcp-no-browser") && !slices.Contains(os.Args, "-b") {
		c.GCPNoBrowser = false
	}

	// force (re)initialize the backend
	system.Logger.Info("Initializing backend")
	system.Opts.Config.Backend = *c
	system.InitOptions.Backend = &InitBackend{
		PollInventoryHourly: false,
		UseCache:            false,
		LogMillisecond:      false,
		ListAllProjects:     false,
		GCPAuthMethod:       clouds.GCPAuthMethod(c.GCPAuthMethod),
		GCPBrowser:          !c.GCPNoBrowser,
		GCPClientID:         c.GCPClientID,
		GCPClientSecret:     c.GCPClientSecret,
	}
	err := system.GetBackend(false)
	if err != nil {
		return err
	}

	system.Logger.Info("Backend initialized")
	err = writeConfigFile(system)
	if err != nil {
		log.Printf("ERROR: Could not save file: %s", err)
	}
	UpdateDiskCache(system)
	return nil
}
