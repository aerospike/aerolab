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

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bvagrant"
	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

// vagrantPreflightBackend is the subset of the vagrant Cloud implementation's methods
// needed to run a preflight check ahead of persisting a candidate backend config. It is
// declared locally (rather than added to backends.Cloud) because it is CLI-only surface,
// and PreflightCheck's return type is specific to the bvagrant package.
type vagrantPreflightBackend interface {
	SetConfig(configDir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error, listAllProjects bool) error
	PreflightCheck(force bool) (*bvagrant.PreflightInfo, error)
}

const (
	DockerRegistryURLNA       = "https://storage.googleapis.com/aerospike-docker-images-na"
	DockerRegistryURLEU       = "https://storage.googleapis.com/aerospike-docker-images-eu"
	DockerRegistryURLDisabled = ""
)

type ConfigBackendCmd struct {
	Type           string         `short:"t" long:"type" description:"Supported backends: aws|docker|gcp|vagrant|none" default:"" webchoice:"aws,gcp,docker,vagrant,none"`
	SshKeyPath     flags.Filename `short:"p" long:"key-path" description:"Specify a custom path to store SSH keys in, default: ${HOME}/.config/aerolab" webtype:"text"`
	Region         string         `short:"r" long:"region" description:"Specify a list of regions to enable, comma-separated" default:""`
	InventoryCache bool           `short:"c" long:"inventory-cache" description:"Enable local inventory cache - use only if not sharing the GCP/AWS project/account with other users"`
	SkipPricing    bool           `long:"skip-pricing" description:"AWS/GCP: skip all cost/pricing lookups (billing/pricing APIs); instance-type and volume catalogs are still returned, just without prices. Useful under GCP Workload Identity Federation, where the billing API rejects federated tokens, or whenever the caller lacks pricing permissions"`

	AWSProfile     string `short:"P" long:"aws-profile" description:"AWS: provide a profile to use; setting this ignores the AWS_PROFILE env variable"`
	AWSNoPublicIps bool   `long:"aws-nopublic-ip" description:"AWS: if set, aerolab will not request public IPs, and will operate on private IPs only"`

	Project               string `short:"o" long:"project" description:"GCP: specify a GCP project to use" default:""`
	GCPAuthMethod         string `short:"m" long:"gcp-auth-method" description:"GCP: specify the authentication method to use (any|login|service-account)" default:"service-account" webchoice:"any,login,service-account" hidden:"true"`
	GCPNoBrowser          bool   `short:"b" long:"gcp-no-browser" description:"GCP: if set, aerolab will not open a browser to authenticate with GCP when using login method" hidden:"true"`
	GCPClientID           string `short:"i" long:"gcp-client-id" description:"GCP: specify a GCP client ID to use" hidden:"true"`
	GCPClientSecret       string `short:"s" long:"gcp-client-secret" description:"GCP: specify a GCP client secret to use" hidden:"true"`
	GCPNoPublicIps        bool   `long:"gcp-nopublic-ip" description:"GCP: if set, aerolab will not request public IPs, and will operate on private IPs only"`
	GCPUseIAP             bool   `long:"gcp-use-iap" description:"GCP: route SSH/SFTP through IAP TCP forwarding instead of dialing the routable instance IP. Independent of --gcp-nopublic-ip; aerolab does NOT auto-enable IAP when public IPs are disabled."`
	GCPAutoEnableServices bool   `long:"gcp-auto-enable-services" description:"GCP: automatically enable required GCP services (APIs) in the project when missing, without prompting. When unset, aerolab prompts interactively and errors in non-interactive contexts, listing the services to enable manually."`

	Arch                 string `short:"a" long:"docker-arch" description:"DOCKER: set to either amd64 or arm64 to force a particular architecture on docker; requires multiarch support"`
	DockerRegistryRegion string `long:"docker-registry-region" description:"DOCKER: region for pre-built template image registry; values: na, eu, disabled" default:"na"`
	DockerRegistryURL    string `long:"docker-registry-url" description:"DOCKER: URL for pre-built template image registry; set to empty to disable" default:"https://storage.googleapis.com/aerospike-docker-images-na" hidden:"true"`

	VagrantProvider string `long:"vagrant-provider" description:"VAGRANT: default provider to use (virtualbox, libvirt, vmware_desktop, hyperv); empty lets vagrant pick"`
	VagrantSubnet   string `long:"vagrant-subnet" description:"VAGRANT: CIDR for the private-network subnet assigned to instances" default:""`
	VagrantBinary   string `long:"vagrant-binary" description:"VAGRANT: path to the vagrant binary; empty uses PATH lookup"`

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

	// Determine which parameters were explicitly provided.
	// In WebUI subprocess mode, params come via JSON and their keys are tracked
	// in the AEROLAB_WEBUI_EXEC_PARAMS env var. In CLI mode, check os.Args.
	webParamsRaw, isWebExecMode := os.LookupEnv("AEROLAB_WEBUI_EXEC_PARAMS")
	var webParams []string
	if isWebExecMode && webParamsRaw != "" {
		webParams = strings.Split(webParamsRaw, ",")
	}

	// Clear aws-nopublic-ip unless explicitly provided
	if isWebExecMode {
		if !slices.Contains(webParams, "aws-nopublic-ip") {
			c.AWSNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp-nopublic-ip") {
			c.GCPNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp-use-iap") {
			c.GCPUseIAP = false
		}
		if !slices.Contains(webParams, "gcp-auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !slices.Contains(webParams, "skip-pricing") {
			c.SkipPricing = false
		}
	} else {
		if !inslice.HasString(os.Args[1:], "--aws-nopublic-ip") {
			c.AWSNoPublicIps = false
		}
		if !inslice.HasString(os.Args[1:], "--gcp-nopublic-ip") {
			c.GCPNoPublicIps = false
		}
		if !inslice.HasString(os.Args[1:], "--gcp-use-iap") {
			c.GCPUseIAP = false
		}
		if !inslice.HasString(os.Args[1:], "--gcp-auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !inslice.HasString(os.Args[1:], "--skip-pricing") {
			c.SkipPricing = false
		}
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

	// map DockerRegistryRegion to DockerRegistryURL
	switch strings.ToLower(c.DockerRegistryRegion) {
	case "na":
		c.DockerRegistryURL = DockerRegistryURLNA
	case "eu":
		c.DockerRegistryURL = DockerRegistryURLEU
	case "disabled", "":
		c.DockerRegistryURL = DockerRegistryURLDisabled
	default:
		return Error(fmt.Errorf("docker-registry-region must be one of: na, eu, disabled"), system, []string{"config", "backend"}, c, args)
	}

	// check if we are setting the backend type
	if isWebExecMode {
		if slices.Contains(webParams, "type") {
			c.typeSet = "yes"
		}
		if slices.Contains(webParams, "region") {
			c.regionSet = "yes"
		}
	} else {
		for _, i := range os.Args {
			if inslice.HasString([]string{"-t", "--type"}, i) || strings.HasPrefix(i, "--type=") {
				c.typeSet = "yes"
			}
			if inslice.HasString([]string{"-r", "--region"}, i) || strings.HasPrefix(i, "--region=") {
				c.regionSet = "yes"
			}
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
		fmt.Printf("Config.Backend.SkipPricing = %v\n", c.SkipPricing)
	}
	if c.Type == "gcp" {
		fmt.Printf("Config.Backend.Project = %s\n", c.Project)
		fmt.Printf("Config.Backend.Region = %s\n", c.Region)
		fmt.Printf("Config.Backend.GCPAuthMethod = %s\n", c.GCPAuthMethod)
		fmt.Printf("Config.Backend.GCPNoBrowser = %v\n", c.GCPNoBrowser)
		fmt.Printf("Config.Backend.GCPNoPublicIps = %v\n", c.GCPNoPublicIps)
		fmt.Printf("Config.Backend.GCPUseIAP = %v\n", c.GCPUseIAP)
		fmt.Printf("Config.Backend.GCPAutoEnableServices = %v\n", c.GCPAutoEnableServices)
		fmt.Printf("Config.Backend.SkipPricing = %v\n", c.SkipPricing)
	}
	if c.Type == "docker" && c.Arch != "" {
		fmt.Printf("Config.Backend.Arch = %s\n", c.Arch)
	}
	if c.Type == "vagrant" {
		fmt.Printf("Config.Backend.VagrantProvider = %s\n", c.VagrantProvider)
		fmt.Printf("Config.Backend.VagrantSubnet = %s\n", c.VagrantSubnet)
		fmt.Printf("Config.Backend.VagrantBinary = %s\n", c.VagrantBinary)
	}
	/*
		if c.Type == "docker" {
			fmt.Printf("Config.Backend.DockerRegistryURL = %s\n", c.DockerRegistryURL)
		}
	*/
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
			SkipPricing:         c.SkipPricing,
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
	if c.Type == "docker" || c.Type == "none" {
		c.Region = ""
	} else if c.Type == "vagrant" {
		// vagrant has exactly one fixed zone; record it so the region
		// reconciliation in Initialize matches the backend's enabled set.
		c.Region = "local"
	} else if c.regionSet == "" {
		return errors.New("ERROR: Region is required for AWS and GCP backends")
	}

	if c.Type == "gcp" && c.Project == "" {
		return errors.New("ERROR: When using GCP backend, project name must be defined. Use: aerolab config backend -t gcp -o project-name-here")
	}
	if c.Type == "aws" || c.Type == "gcp" || c.Type == "docker" || c.Type == "vagrant" {
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
		return errors.New("backend types supported: docker, aws, gcp, vagrant, none")
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
	// Clear bools that weren't explicitly provided (prevents saved config values
	// from being treated as user input). In WebUI mode, check the env var;
	// in CLI mode, check os.Args.
	webParamsRaw, isWebExecMode := os.LookupEnv("AEROLAB_WEBUI_EXEC_PARAMS")
	if isWebExecMode {
		var webParams []string
		if webParamsRaw != "" {
			webParams = strings.Split(webParamsRaw, ",")
		}
		if !slices.Contains(webParams, "check-access") {
			c.CheckAccess = false
		}
		if !slices.Contains(webParams, "inventory-cache") {
			c.InventoryCache = false
		}
		if !slices.Contains(webParams, "aws-nopublic-ip") {
			c.AWSNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp-nopublic-ip") {
			c.GCPNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp-use-iap") {
			c.GCPUseIAP = false
		}
		if !slices.Contains(webParams, "gcp-auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !slices.Contains(webParams, "gcp-no-browser") {
			c.GCPNoBrowser = false
		}
		if !slices.Contains(webParams, "skip-pricing") {
			c.SkipPricing = false
		}
	} else {
		if !slices.Contains(os.Args, "--check-access") {
			c.CheckAccess = false
		}
		if !slices.Contains(os.Args, "--inventory-cache") {
			c.InventoryCache = false
		}
		if !slices.Contains(os.Args, "--aws-nopublic-ip") {
			c.AWSNoPublicIps = false
		}
		if !slices.Contains(os.Args, "--gcp-nopublic-ip") {
			c.GCPNoPublicIps = false
		}
		if !slices.Contains(os.Args, "--gcp-use-iap") {
			c.GCPUseIAP = false
		}
		if !slices.Contains(os.Args, "--gcp-auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !slices.Contains(os.Args, "--gcp-no-browser") && !slices.Contains(os.Args, "-b") {
			c.GCPNoBrowser = false
		}
		if !slices.Contains(os.Args, "--skip-pricing") {
			c.SkipPricing = false
		}
	}

	// Run a vagrant preflight check BEFORE persisting the candidate config, so a broken
	// local vagrant environment (missing/too-old binary, unavailable default provider)
	// never gets written as the active backend. This reaches the registered vagrant
	// backend directly via backends.LookupBackend rather than through system.GetBackend,
	// since GetBackend would only run after (and thus require) the config write below.
	if c.Type == "vagrant" {
		if err := c.vagrantPreflight(system); err != nil {
			return err
		}
	}

	// force (re)initialize the backend
	system.Opts.Config.Backend = *c

	// Save config FIRST so that even if backend initialization fails the user
	// is not stuck in a deadlock with a broken saved config.
	err := writeConfigFile(system)
	if err != nil {
		log.Printf("ERROR: Could not save config file: %s", err)
	}

	// Skip backend initialization for "none" type
	if c.Type == "none" {
		system.Logger.Info("Backend type set to 'none' - no backend will be initialized")
		return nil
	}

	// Clear stale region state for the target backend. A previous (possibly
	// buggy) run may have written regions from another backend type into this
	// backend's regions.json. Since we are about to reconfigure regions from
	// scratch, remove the file so backend.New doesn't try to poll stale regions.
	rootDir, rootErr := AerolabRootDir()
	if rootErr == nil {
		project := os.Getenv("AEROLAB_PROJECT")
		if project == "" {
			project = "default"
		}
		staleRegionsFile := path.Join(rootDir, "projects", project, "config", c.Type, "regions.json")
		os.Remove(staleRegionsFile) // best-effort; ignore errors
	}

	system.Logger.Info("Initializing backend")
	system.InitOptions.Backend = &InitBackend{
		PollInventoryHourly:   false,
		UseCache:              false,
		LogMillisecond:        false,
		ListAllProjects:       false,
		GCPAuthMethod:         clouds.GCPAuthMethod(c.GCPAuthMethod),
		GCPBrowser:            !c.GCPNoBrowser,
		GCPClientID:           c.GCPClientID,
		GCPClientSecret:       c.GCPClientSecret,
		GCPUseIAP:             c.GCPUseIAP,
		GCPAutoEnableServices: c.GCPAutoEnableServices,
		SkipPricing:           c.SkipPricing,
	}
	err = system.GetBackend(false)
	if err != nil {
		return err
	}

	system.Logger.Info("Backend initialized")
	UpdateDiskCacheNow(system)
	return nil
}

// vagrantPreflight configures the registered vagrant backend with the candidate
// (not-yet-persisted) credentials from c, then runs a forced preflight check. It returns
// an error describing the detected issues if the environment isn't usable, so callers can
// abort before writing this config out as the active backend.
func (c *ConfigBackendCmd) vagrantPreflight(system *System) error {
	cloud, ok := backends.LookupBackend(backends.BackendTypeVagrant)
	if !ok {
		return errors.New("vagrant backend is not available in this build")
	}
	vb, ok := cloud.(vagrantPreflightBackend)
	if !ok {
		return errors.New("vagrant backend does not support preflight checks")
	}

	rootDir, err := AerolabRootDir()
	if err != nil {
		return err
	}
	project := os.Getenv("AEROLAB_PROJECT")
	if project == "" {
		project = "default"
	}
	aver, _, _, _ := GetAerolabVersion()

	sshKeyPath := path.Join(rootDir, "projects", project, "ssh-keys", "vagrant")
	if c.SshKeyPath != "" {
		sshKeyPath = path.Join(string(c.SshKeyPath), project, "ssh-keys", "vagrant")
	}
	configDir := path.Join(rootDir, "projects", project, "config", "vagrant")
	workDir := path.Join(rootDir, "projects", project, "workdir", "vagrant")

	creds := &clouds.Credentials{
		VAGRANT: clouds.VAGRANT{
			DefaultProvider: c.VagrantProvider,
			BinaryPath:      c.VagrantBinary,
			Subnet:          c.VagrantSubnet,
		},
	}
	if err := vb.SetConfig(configDir, creds, project, sshKeyPath, system.Logger.WithPrefix("vagrant "), aver, workDir, nil, false); err != nil {
		return fmt.Errorf("vagrant: could not initialize backend for preflight: %w", err)
	}

	system.Logger.Info("Running vagrant preflight check")
	info, err := vb.PreflightCheck(true)
	if err != nil {
		return fmt.Errorf("vagrant: preflight check failed: %w", err)
	}
	if !info.OK {
		return fmt.Errorf("vagrant preflight failed:\n  - %s", strings.Join(info.Issues, "\n  - "))
	}
	system.Logger.Info("Vagrant preflight OK (version %s, providers: %v)", info.VagrantVersion, info.Providers)
	return nil
}
