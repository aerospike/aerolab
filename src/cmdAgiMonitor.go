package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/notifier"
	"github.com/bestmethod/inslice"
	"github.com/lithammer/shortuuid"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v3"
)

type agiMonitorCmd struct {
	Listen agiMonitorListenCmd `command:"listen" subcommands-optional:"true" description:"Run AGI monitor listener"`
	Create agiMonitorCreateCmd `command:"create" subcommands-optional:"true" description:"Create a client instance and run AGI monitor on it; the instance profile must allow it to run aerolab commands"`
	Help   helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiMonitorCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

type agiMonitorListenCmd struct {
	ListenAddress    string   `long:"address" description:"address to listen on; if autocert is enabled, will also listen on :80" default:"0.0.0.0:443" yaml:"listenAddress"`                                   // 0.0.0.0:443, not :80 is also required and will be bound to if using autocert
	NoTLS            bool     `long:"no-tls" description:"disable tls" yaml:"noTLS"`                                                                                                                            // enable TLS
	AutoCertDomains  []string `long:"autocert" description:"TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once" yaml:"autocertDomains"` // TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains
	AutoCertEmail    string   `long:"autocert-email" description:"TLS: if autocert is specified, specify a valid email address to use with letsencrypt"`
	CertFile         string   `long:"cert-file" description:"TLS: certificate file to use if not using letsencrypt; default: generate self-signed" yaml:"certFile"`                                                 // TLS: cert file (if not using autocert), default: snakeoil
	KeyFile          string   `long:"key-file" description:"TLS: key file to use if not using letsencrypt; default: generate self-signed" yaml:"keyFile"`                                                           // TLS: key file (if not using autocert), default: snakeoil
	AWSSizingOptions string   `long:"aws-sizing" description:"specify instance types, comma-separated to use for sizing; same.auto means same family, auto increase the size" default:"same.auto" yaml:"awsSizing"` // if r6g.2xlarge, size above using r6g family
	GCPSizingOptions string   `long:"gcp-sizing" description:"specify instance types, comma-separated to use for sizing; same.auto means same family, auto increase the size" default:"same.auto" yaml:"gcpSizing"` // if c2d-highmem-4, size above using c2d-highmem family
	SizingNoDIMFirst bool     `long:"sizing-nodim" description:"If set, the system will first stop using data-in-memory as a sizing option before resorting to changing instance sizes" yaml:"sizingOptionNoDIMFirst"`
	DisableSizing    bool     `long:"sizing-disable" description:"Set to disable sizing of instances for more resources" yaml:"disableSizing"`
	DisableCapacity  bool     `long:"capacity-disable" description:"Set to disable rotation of spot instances with capacity issues to ondemand" yaml:"disableSpotCapacityRotation"`
	invCache         inventoryJson
	invCacheTimeout  time.Time
	invLock          *sync.Mutex
	Help             helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type agiMonitorCreateCmd struct {
	Name  string `short:"n" long:"name" description:"monitor client name" default:"agimonitor"`
	Owner string `long:"owner" description:"AWS/GCP only: create owner tag with this value"`
	agiMonitorListenCmd
	Aws  agiMonitorCreateCmdAws `no-flag:"true"`
	Gcp  agiMonitorCreateCmdGcp `no-flag:"true"`
	Help helpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

type agiMonitorCreateCmdGcp struct {
	InstanceType string   `long:"gcp-instance" description:"instance type to use" default:"e2-medium"`
	Zone         string   `long:"zone" description:"zone name to deploy to"`
	NamePrefix   []string `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	InstanceRole string   `hidden:"true" long:"gcp-role" description:"instance role to assign to the instance; the role must allow at least compute access; and must be manually precreated" default:"agimonitor"`
}

type agiMonitorCreateCmdAws struct {
	InstanceType    string   `long:"aws-instance" description:"instance type to use" default:"t3a.medium"`
	SecurityGroupID string   `short:"S" long:"secgroup-id" description:"security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID        string   `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	NamePrefix      []string `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
	InstanceRole    string   `long:"aws-role" description:"instance role to assign to the instance; the role must allow at least EC2 and EFS access; and must be manually precreated" default:"agimonitor"`
}

func init() {
	addBackendSwitch("agi.monitor.create", "aws", &a.opts.AGI.Monitor.Create.Aws)
	addBackendSwitch("agi.monitor.create", "gcp", &a.opts.AGI.Monitor.Create.Gcp)
}

func (c *agiMonitorCreateCmd) Execute(args []string) error {
	if earlyProcessV2(args, true) {
		return nil
	}
	return c.create(args)
}

func (c *agiMonitorCreateCmd) create(args []string) error {
	if a.opts.Config.Backend.Type == "docker" {
		return errors.New("this feature can only be deployed on GCP or AWS")
	}
	if len(c.AutoCertDomains) > 0 && c.AutoCertEmail == "" {
		return errors.New("if autocert domains is in use, a valid email must be provided for letsencrypt registration")
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
	a.opts.Client.Create.None.instanceRole = c.Aws.InstanceRole
	if a.opts.Config.Backend.Type == "gcp" {
		a.opts.Client.Create.None.instanceRole = c.Gcp.InstanceRole
	}
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
ExecStartPre=/usr/local/bin/aerolab config backend -t %s -r %s -o %s
ExecStart=/usr/local/bin/aerolab agi monitor listen

[Install]
WantedBy=multi-user.target
`
	agiSystemd = fmt.Sprintf(agiSystemd, a.opts.Config.Backend.Type, a.opts.Config.Backend.Region, a.opts.Config.Backend.Project)
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
	if earlyProcess(args) {
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
	c.invLock = new(sync.Mutex)
	if len(c.AutoCertDomains) > 0 && c.AutoCertEmail == "" {
		return errors.New("if autocert domains is in use, a valid email must be provided for letsencrypt registration")
	}
	http.HandleFunc("/", c.handle)
	if c.NoTLS {
		log.Printf("Listening on http://%s", c.ListenAddress)
		return http.ListenAndServe(c.ListenAddress, nil)
	}
	if _, err := os.Stat("autocert-cache"); err != nil {
		err = os.Mkdir("autocert-cache", 0755)
		if err != nil {
			return err
		}
	}
	if len(c.AutoCertDomains) > 0 {
		m := &autocert.Manager{
			Cache:      autocert.DirCache("autocert-cache"),
			Prompt:     autocert.AcceptTOS,
			Email:      c.AutoCertEmail,
			HostPolicy: autocert.HostWhitelist(c.AutoCertDomains...),
		}
		s := &http.Server{
			Addr:      c.ListenAddress,
			TLSConfig: m.TLSConfig(),
		}
		log.Printf("Listening on https://%s", c.ListenAddress)
		return s.ListenAndServeTLS("", "")
	}
	if c.CertFile == "" && c.KeyFile == "" {
		c.CertFile = "/etc/ssl/certs/ssl-cert-snakeoil.pem"
		c.KeyFile = "/etc/ssl/private/ssl-cert-snakeoil.key"
		if !c.isFile(c.CertFile) || !c.isFile(c.KeyFile) {
			snakeScript := `which apt
ISAPT=$?
set -e
if [ $ISAPT -eq 0 ]
then
    apt update && apt -y install ssl-cert
else
    yum install -y wget mod_ssl
    mkdir -p /etc/ssl/certs /etc/ssl/private
    openssl req -new -x509 -nodes -out /etc/ssl/certs/ssl-cert-snakeoil.pem -keyout /etc/ssl/private/ssl-cert-snakeoil.key -days 3650 -subj '/CN=www.example.com'
fi
`
			err = os.WriteFile("/tmp/snakeoil.sh", []byte(snakeScript), 0755)
			if err != nil {
				return err
			}
			out, err := exec.Command("/bin/bash", "/tmp/snakeoil.sh").CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s: %s", err, string(out))
			}
		}
	}
	log.Printf("Listening on https://%s", c.ListenAddress)
	return http.ListenAndServeTLS(c.ListenAddress, c.CertFile, c.KeyFile, nil)
}

func (c *agiMonitorListenCmd) isFile(s string) bool {
	_, err := os.Stat(s)
	return err == nil
}

func (c *agiMonitorListenCmd) respond(w http.ResponseWriter, r *http.Request, uuid string, code int, value string, logmsg string) {
	log.Printf("tid:%s remoteAddr:%s requestUri:%s method:%s returnCode:%d log:%s", uuid, r.RemoteAddr, r.RequestURI, r.Method, code, logmsg)
	if code > 299 || code < 200 {
		http.Error(w, value, code)
	} else {
		w.WriteHeader(code)
		w.Write([]byte(value))
	}
}

func (c *agiMonitorListenCmd) inventory(forceRefresh bool) inventoryJson {
	c.invLock.Lock()
	defer c.invLock.Unlock()
	if forceRefresh || c.invCacheTimeout.Before(time.Now()) {
		inv, err := b.Inventory("", []int{InventoryItemAGI, InventoryItemClusters, InventoryItemVolumes})
		if err == nil {
			c.invCache = inv
			c.invCacheTimeout = time.Now().Add(10 * time.Second)
		} else {
			log.Printf("WARNING: INVENTORY CACHE: %s", err)
		}
	}
	return c.invCache
}

func (c *agiMonitorListenCmd) challengeCallback(ip string, secret string) (confirmed bool, err error) {
	ret, err := c.challengeCallbackDo("https", ip, secret)
	if err != nil {
		ret, err = c.challengeCallbackDo("http", ip, secret)
	}
	return ret, err
}

func (c *agiMonitorListenCmd) challengeCallbackDo(prot string, ip string, secret string) (confirmed bool, err error) {
	req, err := http.NewRequest(http.MethodGet, prot+"://"+ip+"/agi/monitor-challenge", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Agi-Monitor-Secret", secret)
	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   10 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTeapot {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, fmt.Errorf("wrong error code: %d", resp.StatusCode)
	}
	return true, nil
}

func (c *agiMonitorListenCmd) handle(w http.ResponseWriter, r *http.Request) {
	uuid := shortuuid.New()
	authHeader := r.Header.Get("Agi-Monitor-Auth")
	if authHeader == "" {
		c.respond(w, r, uuid, 401, "auth header missing", "auth header missing")
		return
	}
	authObj, err := notifier.DecodeAuthJson(authHeader)
	if err != nil {
		c.respond(w, r, uuid, 401, "auth header invalid json", "auth header invalid json: "+err.Error())
		log.Print(authHeader)
		return
	}
	inv := c.inventory(false)
	found := false
	var cluster inventoryCluster
	for _, cl := range inv.Clusters {
		if cl.InstanceId != authObj.InstanceId {
			continue
		}
		cluster = cl
		found = true
	}

	if !found {
		inv = c.inventory(true)
		for _, cl := range inv.Clusters {
			if cl.InstanceId != authObj.InstanceId {
				continue
			}
			cluster = cl
			found = true
		}
	}

	var logJson struct {
		Cluster inventoryCluster
		AuthObj *notifier.AgiMonitorAuth
	}
	logJson.Cluster = cluster
	logJson.AuthObj = authObj
	v, _ := json.Marshal(logJson)
	if !found {
		c.respond(w, r, uuid, 401, "auth: instance not found", "auth: instance not found: "+string(v))
		return
	}
	if a.opts.Config.Backend.Type == "aws" && authObj.ImageId != cluster.ImageId {
		c.respond(w, r, uuid, 401, "auth: incorrect", "auth:1 incorrect: "+string(v))
		return
	} else if a.opts.Config.Backend.Type == "gcp" && !strings.HasSuffix(cluster.ImageId, "/"+authObj.ImageId) {
		c.respond(w, r, uuid, 401, "auth: incorrect", "auth:1 incorrect: "+string(v))
		return
	}
	if authObj.PrivateIp != cluster.PrivateIp {
		c.respond(w, r, uuid, 401, "auth: incorrect", "auth:2 incorrect: "+string(v))
		return
	}
	if !strings.HasPrefix(authObj.AvailabilityZoneName, cluster.Zone) {
		c.respond(w, r, uuid, 401, "auth: incorrect", "auth:3 incorrect: "+string(v))
		return
	}
	sgMatch := true
	for _, sg := range cluster.Firewalls {
		if inslice.HasString(authObj.SecurityGroups, sg) {
			continue
		}
		sgMatch = false
	}
	if !sgMatch {
		c.respond(w, r, uuid, 401, "auth: incorrect", "auth:4 incorrect: "+string(v))
		return
	}
	if a.opts.Config.Backend.Type == "gcp" {
		clt := strings.Split(cluster.InstanceType, "/")
		cluster.InstanceType = clt[len(clt)-1]
	}
	if authObj.InstanceType != cluster.InstanceType {
		c.respond(w, r, uuid, 401, "auth: incorrect", "auth:5 incorrect: "+string(v))
		return
	}

	reqIp := strings.Split(r.RemoteAddr, ":")[0]
	if !inslice.HasString([]string{cluster.PrivateIp, cluster.PublicIp}, reqIp) {
		c.respond(w, r, uuid, 401, "auth: incorrect", fmt.Sprintf("auth:6 incorrect: request IP does not match (cluster:[%s,%s] req:%s)", cluster.PrivateIp, cluster.PublicIp, reqIp))
		return
	}
	secretChallenge := r.Header.Get("Agi-Monitor-Secret")
	var callbackFailure error
	if accepted, err := c.challengeCallback(reqIp, secretChallenge); err != nil {
		callbackFailure = err
	} else if !accepted {
		c.respond(w, r, uuid, 401, "auth: incorrect", "auth:7 incorrect: challenge callback not accepted")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		c.respond(w, r, uuid, 400, "message body read error", "io.ReadAll(r.Body):"+err.Error())
		return
	}
	event := &ingest.NotifyEvent{}
	err = json.Unmarshal(body, event)
	if err != nil {
		c.respond(w, r, uuid, 400, "message json malformed", "json.Unmarshal(body):"+err.Error())
		return
	}

	// TODO: handle event
	switch event.Event {
	case AgiEventSpotNoCapacity:
		//- this is the only event on which we ignore callback failure, as we could have been simply too late
		//- respond 200 ok or 418 teapot, stop on this event is not possible
		//- terminate the instance
		//- restart the instance as ondemand
	case AgiEventInitComplete, AgiEventDownloadComplete, AgiEventUnpackComplete, AgiEventPreProcessComplete, AgiEventResourceMonitor, AgiEventServiceDown:
		if callbackFailure != nil {
			c.respond(w, r, uuid, 401, "auth: incorrect", "auth:7 incorrect: callback failed: "+err.Error())
			return
		}
		//- check log sizes, available disk space (GCP) and RAM
		//- if disk size too small - grow it
		//- if RAM too small, tell agi to stop, shutdown the instance and restart it as larger instance accordingly (configurable sizing options)
		//- if we grew instances already and are out of options, disable DIM
		//- allow config option to set max limit for instance growth in size
		//- respond with 418 when we are wanting processing to stop
	}
}
