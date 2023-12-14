package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type agiMonitorCmd struct {
	Listen agiMonitorListenCmd `command:"listen" subcommands-optional:"true" description:"Run AGI monitor listener"`
	Create agiMonitorCreateCmd `command:"create" subcommands-optional:"true" description:"Create a client instance and run AGI monitor on it"`
}

type agiMonitorListenCmd struct {
	ListenAddress    string   `long:"address" description:"address to listen on; if autocert is enabled, will also listen on :80" default:"0.0.0.0:443" yaml:"listenAddress"`                                       // 0.0.0.0:443, not :80 is also required and will be bound to if using autocert
	NoTLS            bool     `long:"no-tls" description:"disable tls" yaml:"noTLS"`                                                                                                                                // enable TLS
	AutoCertDomains  []string `long:"autocert" description:"TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once" yaml:"autocertDomains"`     // TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains
	CertFile         string   `long:"cert-file" description:"TLS: certificate file to use if not using letsencrypt; default: generate self-signed" yaml:"certFile"`                                                 // TLS: cert file (if not using autocert), default: snakeoil
	KeyFile          string   `long:"key-file" description:"TLS: key file to use if not using letsencrypt; default: generate self-signed" yaml:"keyFile"`                                                           // TLS: key file (if not using autocert), default: snakeoil
	AWSSizingOptions string   `long:"aws-sizing" description:"specify instance types, comma-separated to use for sizing; same.auto means same family, auto increase the size" default:"same.auto" yaml:"awsSizing"` // if r6g.2xlarge, size above using r6g family
	GCPSizingOptions string   `long:"gcp-sizing" description:"specify instance types, comma-separated to use for sizing; same.auto means same family, auto increase the size" default:"same.auto" yaml:"gcpSizing"` // if c2d-highmem-4, size above using c2d-highmem family
	SizingNoDIMFirst bool     `long:"sizing-nodim" description:"If set, the system will first stop using data-in-memory as a sizing option before resorting to changing instance sizes" yaml:"sizingOptionNoDIMFirst"`
	DisableSizing    bool     `long:"sizing-disable" description:"Set to disable sizing of instances for more resources" yaml:"disableSizing"`
	DisableCapacity  bool     `long:"capacity-disable" description:"Set to disable rotation of spot instances with capacity issues to ondemand" yaml:"disableSpotCapacityRotation"`
}

type agiMonitorCreateCmd struct {
	Name  string `short:"n" long:"name" description:"monitor client name" default:"agimonitor"`
	Owner string `long:"owner" description:"AWS/GCP only: create owner tag with this value"`
	agiMonitorListenCmd
	Aws agiMonitorCreateCmdAws `no-flag:"true"`
	Gcp agiMonitorCreateCmdGcp `no-flag:"true"`
}

type agiMonitorCreateCmdGcp struct {
	InstanceType string   `long:"gcp-instance" description:"instance type to use" default:"e2-medium"`
	Zone         string   `long:"zone" description:"zone name to deploy to"`
	NamePrefix   []string `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
}

type agiMonitorCreateCmdAws struct {
	InstanceType    string   `long:"aws-instance" description:"instance type to use" default:"t3a.medium"`
	SecurityGroupID string   `short:"S" long:"secgroup-id" description:"security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID        string   `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	NamePrefix      []string `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
}

func init() {
	addBackendSwitch("agi.monitor.create", "aws", &a.opts.AGI.Monitor.Create.Aws)
	addBackendSwitch("agi.monitor.create", "gcp", &a.opts.AGI.Monitor.Create.Gcp)
}

func (c *agiMonitorCreateCmd) Execute(args []string) error {
	if earlyProcessV2(args, true) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" {
		return errors.New("this feature can only be deployed on GCP or AWS")
	}
	log.Printf("Running agi.monitor.create")
	agiConfigYaml, err := yaml.Marshal(c.agiMonitorListenCmd)
	if err != nil {
		return err
	}
	if a.opts.Config.Backend.Type == "gcp" {
		printPrice(c.Gcp.Zone, c.Gcp.InstanceType, 1, false)
	} else if a.opts.Config.Backend.Type == "aws" {
		printPrice("", c.Aws.InstanceType, 1, false)
	}
	log.Printf("Creating base instance")
	a.opts.Client.Create.None.ClientCount = 1
	a.opts.Client.Create.None.ClientName = TypeClientName(c.Name)
	a.opts.Client.Create.None.DistroName = "ubuntu"
	a.opts.Client.Create.None.DistroVersion = "latest"
	a.opts.Client.Create.None.Owner = c.Owner
	a.opts.Client.Create.None.Aws.SecurityGroupID = c.Aws.SecurityGroupID
	a.opts.Client.Create.None.Aws.SubnetID = c.Aws.SubnetID
	a.opts.Client.Create.None.Aws.InstanceType = c.Aws.InstanceType
	a.opts.Client.Create.None.Aws.NamePrefix = c.Aws.NamePrefix
	a.opts.Client.Create.None.Aws.Expires = 0
	a.opts.Client.Create.None.Gcp.Expires = 0
	a.opts.Client.Create.None.Aws.Ebs = "20"
	a.opts.Client.Create.None.Gcp.Disks = []string{"pd-ssd:20"}
	a.opts.Client.Create.None.Gcp.InstanceType = c.Gcp.InstanceType
	a.opts.Client.Create.None.Gcp.NamePrefix = c.Gcp.NamePrefix
	a.opts.Client.Create.None.Gcp.Zone = c.Gcp.Zone
	_, err = a.opts.Client.Create.None.createBase(nil, "agimonitor")
	if err != nil {
		return err
	}

	log.Printf("Installing aerolab")
	a.opts.Cluster.Add.AeroLab.ClusterName = TypeClusterName(c.Name)
	err = a.opts.Cluster.Add.AeroLab.run(true)
	if err != nil {
		return err
	}

	log.Printf("Installing config and systemd unit file")
	b.WorkOnClients()
	agiSystemd := `[Unit]
Description=AeroLab AGI Monitor
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
ExecStartPre=/usr/local/bin/aerolab config backend -t none
ExecStart=/usr/local/bin/aerolab agi monitor listen

[Install]
WantedBy=multi-user.target
`
	err = b.CopyFilesToClusterReader(c.Name, []fileListReader{{"/usr/lib/systemd/system/agimonitor.service", strings.NewReader(agiSystemd), len(agiSystemd)}, {"/etc/agimonitor.yaml", bytes.NewReader(agiConfigYaml), len(agiConfigYaml)}}, []int{1})
	if err != nil {
		return err
	}

	log.Printf("Starting agimonitor")
	out, err := b.RunCommands(c.Name, [][]string{{"systemctl", "enable", "--now", "agimonitor"}}, []int{1})
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out[0]))
	}
	return nil
}

func (c *agiMonitorListenCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	log.Print("Starting agi-monitor")
	err := os.MkdirAll("/var/lib/agimonitor", 0755)
	if err != nil {
		return err
	}
	err = os.Chdir("/var/lib/agimonitor")
	if err != nil {
		return err
	}
	if _, err := os.Stat("/etc/agimonitor.yaml"); err == nil {
		data, err := os.ReadFile("/etc/agimonitor.yaml")
		if err != nil {
			return err
		}
		err = yaml.Unmarshal(data, c)
		if err != nil {
			return err
		}
	}
	log.Print("Configuration:")
	yaml.NewEncoder(os.Stderr).Encode(c)

	log.Print("Initializing")
	// TODO: configure routing etc

	log.Printf("Listening on %s", c.ListenAddress)
	// TODO: start listener
	return nil
}

/* TODO:
auth:
  call: notifier.DecodeAuthJson("") to get the auth json values
  get the instance details from backend
  compare
receive events from agi-proxy http notifier
authenticate them
if event is sizing:
 - check log sizes, available disk space (GCP) and RAM
 - if disk size too small - grow it
 - if RAM too small, tell agi to stop, shutdown the instance and restart it as larger instance accordingly (configurable sizing options)
if event is spot termination:
 - respond 200 ok, stop on this event is not possible
 - terminate the instance
 - restart the instance as ondemand
*/

/*
* TODO: Document agi instance state monitor.
  * document what it's for: to run sizing for agi instances in AWS/GCP which use volume backing, and to cycle spot to on-demand if capacity becomes unavailable
  * document running monitor locally
  * document usage with AGI instances (need to specify `--monitor-url` and must have a backing volume)
*/
