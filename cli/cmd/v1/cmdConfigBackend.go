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

	"github.com/bestmethod/inslice"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds"
	"github.com/citrusleaf/aerolab/pkg/utils/callerip"
	flags "github.com/rglonek/go-flags"
)

const (
	DockerRegistryURLNA       = "https://storage.googleapis.com/aerospike-docker-images-na"
	DockerRegistryURLEU       = "https://storage.googleapis.com/aerospike-docker-images-eu"
	DockerRegistryURLDisabled = ""
)

type ConfigBackendCmd struct {
	Type           string         `short:"t" long:"type" description:"Supported backends: aws|docker|gcp|none" default:"" webchoice:"aws,gcp,docker,none"`
	SshKeyPath     flags.Filename `short:"p" long:"key-path" description:"Specify a custom path to store SSH keys in, default: ${HOME}/.config/aerolab" webtype:"text"`
	Region         string         `short:"r" long:"region" description:"Specify a list of regions to enable, comma-separated" default:""`
	InventoryCache bool           `short:"c" long:"inventory-cache" description:"Enable local inventory cache - use only if not sharing the GCP/AWS project/account with other users"`
	SkipPricing    bool           `long:"skip-pricing" description:"AWS/GCP: skip all cost/pricing lookups (billing/pricing APIs); instance-type and volume catalogs are still returned, just without prices. Useful under GCP Workload Identity Federation, where the billing API rejects federated tokens, or whenever the caller lacks pricing permissions"`

	SSHStrictHostKey bool `long:"ssh-strict-host-key" description:"Refuse SSH connections when an instance presents a host key different to the one AeroLab remembers for it. When unset, a changed key is only logged as a warning and then relearned. See 'aerolab config host-keys'"`

	FirewallCidr       string `long:"firewall-cidr" description:"AWS/GCP: comma-separated CIDRs which AeroLab-managed firewalls should allow inbound, instead of the discovered caller public IP. Use behind a NAT gateway or VPN, or when hosting the webui. Set to 0.0.0.0/0 to deliberately open access to everyone" default:""`
	NoFirewallAutolock bool   `long:"no-firewall-autolock" description:"AWS/GCP: do not automatically create, re-lock and attach the per-user firewall when running commands against instances. Access must then be managed manually with 'config aws|gcp lock-security-groups'"`

	AWSProfile     string `long:"aws.profile" description:"AWS: provide a profile to use; setting this ignores the AWS_PROFILE env variable"`
	AWSNoPublicIps bool   `long:"aws.no-public-ip" description:"AWS: if set, aerolab will not request public IPs, and will operate on private IPs only"`

	Project               string `long:"gcp.project" description:"GCP: specify a GCP project to use" default:""`
	GCPAuthMethod         string `long:"gcp.auth-method" description:"GCP: specify the authentication method to use (any|login|service-account)" default:"service-account" webchoice:"any,login,service-account" hidden:"true"`
	GCPNoBrowser          bool   `long:"gcp.no-browser" description:"GCP: if set, aerolab will not open a browser to authenticate with GCP when using login method" hidden:"true"`
	GCPClientID           string `long:"gcp.client-id" description:"GCP: specify a GCP client ID to use" hidden:"true"`
	GCPClientSecret       string `long:"gcp.client-secret" description:"GCP: specify a GCP client secret to use" hidden:"true" telemetry:"redact"`
	GCPNoPublicIps        bool   `long:"gcp.no-public-ip" description:"GCP: if set, aerolab will not request public IPs, and will operate on private IPs only"`
	GCPUseIAP             bool   `long:"gcp.use-iap" description:"GCP: route SSH/SFTP through IAP TCP forwarding instead of dialing the routable instance IP. Independent of --gcp.no-public-ip; aerolab does NOT auto-enable IAP when public IPs are disabled."`
	GCPAutoEnableServices bool   `long:"gcp.auto-enable-services" description:"GCP: automatically enable required GCP services (APIs) in the project when missing, without prompting. When unset, aerolab prompts interactively and errors in non-interactive contexts, listing the services to enable manually."`

	Arch                 string         `long:"docker.arch" description:"DOCKER: set to either amd64 or arm64 to force a particular architecture on docker; requires multiarch support"`
	DockerRegistryRegion string         `long:"docker.registry-region" description:"DOCKER: region for pre-built template image registry; values: na, eu, disabled" default:"na"`
	DockerRegistryURL    string         `long:"docker.registry-url" description:"DOCKER: URL for pre-built template image registry; set to empty to disable" default:"https://storage.googleapis.com/aerospike-docker-images-na" hidden:"true"`
	TmpDir               flags.Filename `short:"d" long:"temp-dir" description:"use a non-default temporary directory, when using aerolab in WSL2" default:"" webtype:"text"`
	CheckAccess          bool           `long:"check-access" description:"check access to the backend"`

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

	// Clear aws.no-public-ip unless explicitly provided
	if isWebExecMode {
		if !slices.Contains(webParams, "aws.no-public-ip") {
			c.AWSNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp.no-public-ip") {
			c.GCPNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp.use-iap") {
			c.GCPUseIAP = false
		}
		if !slices.Contains(webParams, "gcp.auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !slices.Contains(webParams, "skip-pricing") {
			c.SkipPricing = false
		}
	} else {
		if !inslice.HasString(os.Args[1:], "--aws.no-public-ip") {
			c.AWSNoPublicIps = false
		}
		if !inslice.HasString(os.Args[1:], "--gcp.no-public-ip") {
			c.GCPNoPublicIps = false
		}
		if !inslice.HasString(os.Args[1:], "--gcp.use-iap") {
			c.GCPUseIAP = false
		}
		if !inslice.HasString(os.Args[1:], "--gcp.auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !inslice.HasString(os.Args[1:], "--skip-pricing") {
			c.SkipPricing = false
		}
	}

	if c.Type == "gcp" && c.Project == "" {
		return Error(errors.New("ERROR: When using GCP backend, project name must be defined. Use: aerolab config backend -t gcp --gcp.project project-name-here"), system, []string{"config", "backend"}, c, args)
	}

	// validate docker-arch
	if c.Arch != "" && c.Arch != "amd64" && c.Arch != "arm64" && c.Arch != "unset" {
		return Error(errors.New("docker.arch must be one of: unset, amd64, arm64"), system, []string{"config", "backend"}, c, args)
	}
	if c.Arch == "unset" {
		c.Arch = ""
	}

	// validate firewall-cidr
	if c.FirewallCidr != "" {
		if _, err := callerip.ParseList(c.FirewallCidr); err != nil {
			return Error(fmt.Errorf("firewall-cidr: %w", err), system, []string{"config", "backend"}, c, args)
		}
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
		return Error(fmt.Errorf("docker.registry-region must be one of: na, eu, disabled"), system, []string{"config", "backend"}, c, args)
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
		fmt.Printf("Config.Backend.FirewallCidr = %s\n", c.FirewallCidr)
		fmt.Printf("Config.Backend.NoFirewallAutolock = %v\n", c.NoFirewallAutolock)
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
		fmt.Printf("Config.Backend.FirewallCidr = %s\n", c.FirewallCidr)
		fmt.Printf("Config.Backend.NoFirewallAutolock = %v\n", c.NoFirewallAutolock)
	}
	if c.Type == "docker" && c.Arch != "" {
		fmt.Printf("Config.Backend.Arch = %s\n", c.Arch)
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
	} else if c.regionSet == "" {
		region, err := RequireString("", "region")
		if err != nil {
			return errors.New("ERROR: Region is required for AWS and GCP backends")
		}
		c.regionSet = "yes"
		c.Region = region
	}

	if c.Type == "gcp" && c.Project == "" {
		return errors.New("ERROR: When using GCP backend, project name must be defined. Use: aerolab config backend -t gcp --gcp.project project-name-here")
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
		return errors.New("backend types supported: docker, aws, gcp, none")
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
		if !slices.Contains(webParams, "aws.no-public-ip") {
			c.AWSNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp.no-public-ip") {
			c.GCPNoPublicIps = false
		}
		if !slices.Contains(webParams, "gcp.use-iap") {
			c.GCPUseIAP = false
		}
		if !slices.Contains(webParams, "gcp.auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !slices.Contains(webParams, "gcp.no-browser") {
			c.GCPNoBrowser = false
		}
		if !slices.Contains(webParams, "skip-pricing") {
			c.SkipPricing = false
		}
		if !slices.Contains(webParams, "ssh-strict-host-key") {
			c.SSHStrictHostKey = false
		}
		if !slices.Contains(webParams, "no-firewall-autolock") {
			c.NoFirewallAutolock = false
		}
	} else {
		if !slices.Contains(os.Args, "--check-access") {
			c.CheckAccess = false
		}
		if !slices.Contains(os.Args, "--inventory-cache") {
			c.InventoryCache = false
		}
		if !slices.Contains(os.Args, "--aws.no-public-ip") {
			c.AWSNoPublicIps = false
		}
		if !slices.Contains(os.Args, "--gcp.no-public-ip") {
			c.GCPNoPublicIps = false
		}
		if !slices.Contains(os.Args, "--gcp.use-iap") {
			c.GCPUseIAP = false
		}
		if !slices.Contains(os.Args, "--gcp.auto-enable-services") {
			c.GCPAutoEnableServices = false
		}
		if !slices.Contains(os.Args, "--gcp.no-browser") {
			c.GCPNoBrowser = false
		}
		if !slices.Contains(os.Args, "--skip-pricing") {
			c.SkipPricing = false
		}
		if !slices.Contains(os.Args, "--ssh-strict-host-key") {
			c.SSHStrictHostKey = false
		}
		if !slices.Contains(os.Args, "--no-firewall-autolock") {
			c.NoFirewallAutolock = false
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
