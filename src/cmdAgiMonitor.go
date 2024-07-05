package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
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
	Listen agiMonitorListenCmd `command:"listen" subcommands-optional:"true" description:"Run AGI monitor listener" webicon:"fas fa-headset" simplemode:"false"`
	Create agiMonitorCreateCmd `command:"create" subcommands-optional:"true" description:"Create a client instance and run AGI monitor on it; the instance profile must allow it to run aerolab commands" webicon:"fas fa-circle-plus" invwebforce:"true" simplemode:"false"`
	Help   helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiMonitorCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

type agiMonitorListenCmd struct {
	ListenAddress       string   `long:"address" description:"address to listen on; if autocert is enabled, will also listen on :80" default:"0.0.0.0:443" yaml:"listenAddress"` // 0.0.0.0:443, not :80 is also required and will be bound to if using autocert
	NoTLS               bool     `long:"no-tls" description:"disable tls" yaml:"noTLS"`                                                                                          // enable TLS
	StrictAGITLS        bool     `long:"strict-agi-tls" description:"if set, AGI-Monitor will expect AGI instances to have a valid TLS certificate"`
	AutoCertDomains     []string `long:"autocert" description:"TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once" yaml:"autocertDomains"` // TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains
	AutoCertEmail       string   `long:"autocert-email" description:"TLS: if autocert is specified, specify a valid email address to use with letsencrypt"`
	CertFile            string   `long:"cert-file" description:"TLS: certificate file to use if not using letsencrypt; default: generate self-signed" yaml:"certFile"` // TLS: cert file (if not using autocert), default: snakeoil
	KeyFile             string   `long:"key-file" description:"TLS: key file to use if not using letsencrypt; default: generate self-signed" yaml:"keyFile"`           // TLS: key file (if not using autocert), default: snakeoil
	GCPDiskThresholdPct int      `long:"gcp-disk-thres-pct" description:"usage threshold pct at which the disk will be increased" yaml:"gcpDiskThresholdPct" default:"80"`
	GCPDiskIncreaseGB   int      `long:"gcp-disk-grow-gb" description:"when threshold is breached, grow by these many GB" yaml:"gcpDiskIncreaseGB" default:"100"`
	RAMThresUsedPct     int      `long:"ram-thres-used-pct" description:"max used PCT of RAM before instance gets sized" yaml:"ramThresholdUsedPct" default:"95"`
	RAMThresMinFreeGB   int      `long:"ram-thres-minfree-gb" description:"minimum free GB of RAM before instance gets sized" yaml:"ramThresholdMinFreeGB" default:"8"`
	SizingNoDIMFirst    bool     `long:"sizing-nodim" description:"If set, the system will first stop using data-in-memory as a sizing option before resorting to changing instance sizes" yaml:"sizingOptionNoDIMFirst"`
	DisableSizing       bool     `long:"sizing-disable" description:"Set to disable sizing of instances for more resources" yaml:"disableSizing"`
	SizingMaxRamGB      int      `long:"sizing-max-ram-gb" description:"will not size above these many GB" default:"130" yaml:"sizingMaxRamGB"`
	SizingMaxDiskGB     int      `long:"sizing-max-disk-gb" description:"will not size above these many GB" default:"400" yaml:"sizingMaxDiskGB"`
	DisableCapacity     bool     `long:"capacity-disable" description:"Set to disable rotation of spot instances with capacity issues to ondemand" yaml:"disableSpotCapacityRotation"`
	DimMultiplier       float64  `long:"sizing-multiplier-dim" description:"log size * multiplier = how much RAM is needed" yaml:"ramMultiplierDim" default:"1.8"`
	NoDimMultiplier     float64  `long:"sizing-multiplier-nodim" description:"log size * multiplier = how much RAM is needed" yaml:"ramMultiplierNoDim" default:"0.4"`
	DebugEvents         bool     `long:"debug-events" description:"Log all events for debugging purposes" yaml:"debugEvents"`
	DisablePricingAPI   bool     `long:"disable-pricing-api" description:"Set to disable pricing queries for cost tracking" yaml:"disablePricingAPI"`
	invCache            inventoryJson
	invCacheTimeout     time.Time
	invLock             *sync.Mutex
	execLock            *sync.Mutex
	Help                helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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
	Zone         string   `long:"zone" description:"zone name to deploy to" webrequired:"true"`
	NamePrefix   []string `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	InstanceRole string   `hidden:"true" long:"gcp-role" description:"instance role to assign to the instance; the role must allow at least compute access; and must be manually precreated" default:"agimonitor"`
}

type agiMonitorCreateCmdAws struct {
	InstanceType      string   `long:"aws-instance" description:"instance type to use" default:"t3a.medium"`
	SecurityGroupID   string   `short:"S" long:"secgroup-id" description:"security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID          string   `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	NamePrefix        []string `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
	InstanceRole      string   `long:"aws-role" description:"instance role to assign to the instance; the role must allow at least EC2 and EFS access; and must be manually precreated" default:"agimonitor"`
	Route53ZoneId     string   `long:"route53-zoneid" description:"if set, will automatically update a route53 DNS domain with an entry of {instanceId}.{region}.agi.; expiry system will also be updated accordingly"`
	Route53DomainName string   `long:"route53-fqdn" description:"the route domain the zone refers to; eg monitor.eu-west-1.myagi.org"`
}

func init() {
	addBackendSwitch("agi.monitor.create", "aws", &a.opts.AGI.Monitor.Create.Aws)
	addBackendSwitch("agi.monitor.create", "gcp", &a.opts.AGI.Monitor.Create.Gcp)
}

func (c *agiMonitorCreateCmd) Execute(args []string) error {
	if earlyProcessV2(args, true) {
		return nil
	}
	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}
	return c.create(args)
}

func (c *agiMonitorCreateCmd) create(args []string) error {
	_ = args
	if a.opts.Config.Backend.Type == "aws" {
		if (c.Aws.Route53DomainName == "" && c.Aws.Route53ZoneId != "") || (c.Aws.Route53DomainName != "" && c.Aws.Route53ZoneId == "") {
			return errors.New("either both route53-zoneid and route53-domain must be fills or both must be empty")
		}
	}
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
	if len(c.AutoCertDomains) > 0 {
		log.Printf("Resolving firewalls")
		inv, err := b.Inventory("", []int{InventoryItemFirewalls})
		if err != nil {
			return err
		}
		found := false
		for _, item := range inv.FirewallRules {
			if a.opts.Config.Backend.Type == "aws" && strings.HasPrefix(item.AWS.SecurityGroupName, "agi-autocert") {
				found = true
				break
			}
			if a.opts.Config.Backend.Type == "gcp" && strings.HasPrefix(item.GCP.FirewallName, "agi-autocert") {
				found = true
				break
			}
		}
		b.WorkOnClients()
		if !found {
			err = b.CreateSecurityGroups("", "agi-autocert", true, []string{"80", "443"}, true)
			if err != nil {
				return err
			}
		}
		if a.opts.Config.Backend.Type == "aws" && !inslice.HasString(c.Aws.NamePrefix, "agi-autocert") {
			c.Aws.NamePrefix = append(c.Aws.NamePrefix, "agi-autocert")
		}
		if a.opts.Config.Backend.Type == "gcp" && !inslice.HasString(c.Gcp.NamePrefix, "agi-autocert") {
			c.Gcp.NamePrefix = append(c.Gcp.NamePrefix, "agi-autocert")
		}
	}

	log.Printf("Creating base instance")
	a.opts.Client.Create.None.ClientCount = 1
	a.opts.Client.Create.None.ClientName = TypeClientName(c.Name)
	a.opts.Client.Create.None.DistroName = "ubuntu"
	a.opts.Client.Create.None.DistroVersion = "latest"
	a.opts.Client.Create.None.Owner = c.Owner
	a.opts.Client.Create.None.Aws.SecurityGroupID = c.Aws.SecurityGroupID
	a.opts.Client.Create.None.Aws.SubnetID = c.Aws.SubnetID
	a.opts.Client.Create.None.Aws.InstanceType = guiInstanceType(c.Aws.InstanceType)
	a.opts.Client.Create.None.Aws.NamePrefix = c.Aws.NamePrefix
	a.opts.Client.Create.None.Aws.Expires = 0
	if a.opts.Config.Backend.Type == "aws" && c.Aws.Route53ZoneId != "" {
		a.opts.Client.Create.None.Aws.Tags = append(a.opts.Client.Create.None.Aws.Tags, "agimUrl="+c.Aws.Route53DomainName, "agimZone="+c.Aws.Route53ZoneId)
	}
	a.opts.Client.Create.None.instanceRole = c.Aws.InstanceRole
	if a.opts.Config.Backend.Type == "gcp" {
		a.opts.Client.Create.None.instanceRole = c.Gcp.InstanceRole
		if c.DisablePricingAPI {
			a.opts.Client.Create.None.instanceRole = c.Gcp.InstanceRole + "::nopricing"
		}
	}
	a.opts.Client.Create.None.Gcp.Expires = 0
	a.opts.Client.Create.None.Aws.Ebs = "20"
	a.opts.Client.Create.None.Gcp.Disks = []string{"pd-ssd:20"}
	a.opts.Client.Create.None.Gcp.InstanceType = guiInstanceType(c.Gcp.InstanceType)
	a.opts.Client.Create.None.Gcp.NamePrefix = c.Gcp.NamePrefix
	a.opts.Client.Create.None.Gcp.Zone = guiZone(c.Gcp.Zone)
	_, err = a.opts.Client.Create.None.createBase(nil, "agimonitor")
	if err != nil {
		return err
	}

	b.WorkOnClients()
	if a.opts.Config.Backend.Type == "aws" && c.Aws.Route53ZoneId != "" {
		log.Printf("Configuring route53")
		instIps, err := b.GetInstanceIpMap(string(c.Name), false)
		if err != nil {
			log.Printf("ERROR: Could not get node IPs, DNS will not be updated: %s", err)
		} else {
			for _, ip := range instIps {
				err := b.DomainCreate(c.Aws.Route53ZoneId, c.Aws.Route53DomainName, ip, true)
				if err != nil {
					log.Printf("ERROR creating domain in route53: %s", err)
				}
			}
		}
	}

	log.Printf("Installing aerolab")
	a.opts.Cluster.Add.AeroLab.ClusterName = TypeClusterName(c.Name)
	err = a.opts.Cluster.Add.AeroLab.run(true)
	if err != nil {
		return err
	}
	a.opts.Cluster.Add.AeroLab.alt = true
	err = a.opts.Cluster.Add.AeroLab.run(true)
	a.opts.Cluster.Add.AeroLab.alt = false
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
ExecStartPre=/usr/local/bin/aerolab config backend -t %s %s
ExecStart=/usr/local/bin/aerolab agi monitor listen

[Install]
WantedBy=multi-user.target
`
	if a.opts.Config.Backend.Type == "gcp" {
		agiSystemd = fmt.Sprintf(agiSystemd, a.opts.Config.Backend.Type, "-o "+a.opts.Config.Backend.Project)
	} else {
		agiSystemd = fmt.Sprintf(agiSystemd, a.opts.Config.Backend.Type, "-r "+a.opts.Config.Backend.Region)
	}
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
	if c.DisablePricingAPI {
		b.DisablePricingAPI()
	}
	b.DisableExpiryInstall()
	c.execLock = new(sync.Mutex)
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
		go func() {
			srv := &http.Server{
				Addr:    ":80",
				Handler: m.HTTPHandler(nil),
			}
			log.Println("AutoCert: Listening on 0.0.0.0:80")
			err := srv.ListenAndServe()
			log.Fatal(err)
		}()
		s := &http.Server{
			Addr:      c.ListenAddress,
			TLSConfig: m.TLSConfig(),
		}
		s.TLSConfig.MinVersion = tls.VersionTLS12
		s.TLSConfig.CurvePreferences = []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256}
		s.TLSConfig.CipherSuites = []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384}
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
	srv := &http.Server{Addr: c.ListenAddress}
	tlsConfig := &tls.Config{
		MinVersion:       tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
	}
	srv.TLSConfig = tlsConfig
	return srv.ListenAndServeTLS(c.CertFile, c.KeyFile)
}

func (c *agiMonitorListenCmd) isFile(s string) bool {
	_, err := os.Stat(s)
	return err == nil
}

func (c *agiMonitorListenCmd) log(uuid string, action string, line string) {
	log.Printf("tid:%s action:%s log:%s", uuid, action, line)
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
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: !c.StrictAGITLS}, // are we expecting AGI instances to have a valid certificate
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
	reqDomain := reqIp
	if cluster.AwsTags["agiDomain"] != "" {
		reqDomain = cluster.InstanceId + "." + a.opts.Config.Backend.Region + ".agi." + cluster.AwsTags["agiDomain"]
		ips, err := net.LookupIP(reqDomain)
		if err != nil {
			c.respond(w, r, uuid, 401, "auth: incorrect", fmt.Sprintf("auth:5.1 incorrect: DNS IP lookup failed (cluster:[%s,%s] req:%s)", cluster.PrivateIp, cluster.PublicIp, reqIp))
			return
		}
		domainFound := false
		for _, ip := range ips {
			if inslice.HasString([]string{cluster.PrivateIp, cluster.PublicIp}, ip.String()) {
				domainFound = true
			}
		}
		if !domainFound {
			c.respond(w, r, uuid, 401, "auth: incorrect", fmt.Sprintf("auth:5.2 incorrect: request IP does not match DNS (cluster:[%s,%s] req:%s)", cluster.PrivateIp, cluster.PublicIp, reqIp))
			return
		}
	}
	if !inslice.HasString([]string{cluster.PrivateIp, cluster.PublicIp}, reqIp) {
		c.respond(w, r, uuid, 401, "auth: incorrect", fmt.Sprintf("auth:6 incorrect: request IP does not match node IP (cluster:[%s,%s] req:%s)", cluster.PrivateIp, cluster.PublicIp, reqIp))
		return
	}
	secretChallenge := r.Header.Get("Agi-Monitor-Secret")
	var callbackFailure error
	if accepted, err := c.challengeCallback(reqDomain, secretChallenge); err != nil {
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

	if c.DebugEvents {
		debugEvent, _ := json.MarshalIndent(event, "", "  ")
		log.Printf("%s: %s", uuid, string(debugEvent))
	}

	evt, _ := json.Marshal(event)
	switch event.Event {
	case AgiEventSpotNoCapacity:
		if c.DisableCapacity {
			c.respond(w, r, uuid, 200, "ignoring: capacity handling disabled", "ignoring: capacity handling disabled")
			return
		}
		testJson := &agiCreateCmd{}
		if !c.getDeploymentJSON(uuid, event, testJson) {
			c.respond(w, r, uuid, 400, "capacity: invalid deployment json", "Capacity: abort on invalid deployment json")
			return
		}
		c.respond(w, r, uuid, 418, "capacity: rotating to on-demand", "Capacity: start on-demand rotation: "+string(evt))
		go c.handleCapacity(uuid, event)
	case AgiEventInitComplete, AgiEventDownloadComplete, AgiEventUnpackComplete, AgiEventPreProcessComplete, AgiEventResourceMonitor, AgiEventServiceDown:
		if c.DisableSizing {
			c.respond(w, r, uuid, 200, "ignoring: sizing disabled", "ignoring: sizing disabled")
			return
		}
		if callbackFailure != nil {
			c.respond(w, r, uuid, 401, "auth: incorrect", "auth:7 incorrect: callback failed: "+callbackFailure.Error())
			return
		}
		c.handleCheckSizing(w, r, uuid, event, authObj.InstanceType, cluster.Zone)
	}
}

func (c *agiMonitorListenCmd) getDeploymentJSON(uuid string, event *ingest.NotifyEvent, dst interface{}) bool {
	deployDetail, err := base64.StdEncoding.DecodeString(event.DeploymentJsonGzB64)
	if err != nil {
		c.log(uuid, "getDeploymentJson", "base64.StdEncoding.DecodeString:"+err.Error())
		return false
	}
	un, err := gzip.NewReader(bytes.NewReader(deployDetail))
	if err != nil {
		c.log(uuid, "getDeploymentJson", "gzip.NewReader:"+err.Error())
		return false
	}
	deployDetail, err = io.ReadAll(un)
	un.Close()
	if err != nil {
		c.log(uuid, "getDeploymentJson", "io.Read(gz):"+err.Error())
		return false
	}
	err = json.Unmarshal(deployDetail, dst)
	if err != nil {
		c.log(uuid, "getDeploymentJson", "json.Unmarshal:"+err.Error())
		return false
	}
	return true
}

func (c *agiMonitorListenCmd) handleCapacity(uuid string, event *ingest.NotifyEvent) {
	c.execLock.Lock()
	defer c.execLock.Unlock()
	if !c.getDeploymentJSON(uuid, event, &a.opts.AGI.Create) {
		return
	}
	a.opts.Cluster.Destroy.ClusterName = TypeClusterName(event.AGIName)
	a.opts.Cluster.Destroy.Force = true
	a.opts.Cluster.Destroy.Nodes = "1"
	err := a.opts.Cluster.Destroy.doDestroy("agi", nil)
	if err != nil {
		c.log(uuid, "capacity", fmt.Sprintf("Error destroying instance, attempting to continue (%s)", err))
		return
	}
	a.opts.AGI.Create.Aws.SpotInstance = false
	a.opts.AGI.Create.Gcp.SpotInstance = false
	a.opts.AGI.Create.uploadAuthorizedContentsGzB64 = event.SSHAuthorizedKeysFileGzB64
	a.opts.AGI.Create.SftpSkipCheck = true
	err = a.opts.AGI.Create.Execute(nil)
	a.opts.AGI.Create.uploadAuthorizedContentsGzB64 = ""
	if err != nil {
		c.log(uuid, "capacity", fmt.Sprintf("Error creating new instance (%s)", err))
		return
	}
	c.log(uuid, "capacity", "rotated to on-demand instance")
}

func (c *agiMonitorListenCmd) handleSizingDisk(uuid string, event *ingest.NotifyEvent, newSize int64) {
	c.execLock.Lock()
	defer c.execLock.Unlock()
	if !c.getDeploymentJSON(uuid, event, &a.opts.AGI.Create) {
		return
	}
	c.handleSizingDiskDo(uuid, event, newSize)
}

func (c *agiMonitorListenCmd) handleSizingRAM(uuid string, event *ingest.NotifyEvent, newType string, disableDim bool) {
	c.execLock.Lock()
	defer c.execLock.Unlock()
	if !c.getDeploymentJSON(uuid, event, &a.opts.AGI.Create) {
		return
	}
	c.handleSizingRAMDo(uuid, event, newType, disableDim)
}

func (c *agiMonitorListenCmd) handleSizingDiskAndRAM(uuid string, event *ingest.NotifyEvent, newSize int64, newType string, disableDim bool) {
	c.execLock.Lock()
	defer c.execLock.Unlock()
	if !c.getDeploymentJSON(uuid, event, &a.opts.AGI.Create) {
		return
	}
	c.handleSizingDiskDo(uuid, event, newSize)
	c.handleSizingRAMDo(uuid, event, newType, disableDim)
}

func (c *agiMonitorListenCmd) handleSizingDiskDo(uuid string, event *ingest.NotifyEvent, newSize int64) {
	_ = event
	a.opts.Volume.Resize.Zone = a.opts.AGI.Create.Gcp.Zone.String()
	a.opts.Volume.Resize.Name = string(a.opts.AGI.Create.ClusterName)
	a.opts.Volume.Resize.Size = newSize
	err := a.opts.Volume.Resize.Execute(nil)
	if err != nil {
		c.log(uuid, "volume", fmt.Sprintf("Error resizing (%s)", err))
		return
	}
}

func (c *agiMonitorListenCmd) handleSizingRAMDo(uuid string, event *ingest.NotifyEvent, newType string, disableDim bool) {
	a.opts.Cluster.Destroy.ClusterName = TypeClusterName(event.AGIName)
	a.opts.Cluster.Destroy.Force = true
	a.opts.Cluster.Destroy.Nodes = "1"
	err := a.opts.Cluster.Destroy.doDestroy("agi", nil)
	if err != nil {
		c.log(uuid, "sizing", fmt.Sprintf("Error destroying instance, attempting to continue (%s)", err))
		return
	}
	a.opts.AGI.Create.Aws.InstanceType = newType
	a.opts.AGI.Create.Gcp.InstanceType = newType
	if disableDim {
		a.opts.AGI.Create.NoDIM = true
	}
	a.opts.AGI.Create.uploadAuthorizedContentsGzB64 = event.SSHAuthorizedKeysFileGzB64
	a.opts.AGI.Create.SftpSkipCheck = true
	err = a.opts.AGI.Create.Execute(nil)
	a.opts.AGI.Create.uploadAuthorizedContentsGzB64 = ""
	if err != nil {
		c.log(uuid, "sizing", fmt.Sprintf("Error creating new instance (%s)", err))
		return
	}
	if disableDim {
		c.log(uuid, "sizing", "disabled data-in-memory, rotated to instance type: "+newType)
	} else {
		c.log(uuid, "sizing", "rotated to instance type: "+newType)
	}
}

func (c *agiMonitorListenCmd) handleCheckSizing(w http.ResponseWriter, r *http.Request, uuid string, event *ingest.NotifyEvent, currentType string, zone string) {
	// check for required disk sizing on GCP
	diskNewSize := uint64(0)
	if a.opts.Config.Backend.Type == "gcp" {
		if event.IngestStatus.System.DiskTotalBytes/1024/1024/1024 < uint64(c.SizingMaxDiskGB) {
			if event.IngestStatus.System.DiskFreeBytes > 0 && event.IngestStatus.System.DiskTotalBytes > 0 {
				if event.IngestStatus.Ingest.LogProcessorCompletePct < 100 && 1-(float64(event.IngestStatus.System.DiskFreeBytes)/float64(event.IngestStatus.System.DiskTotalBytes)) > float64(c.GCPDiskThresholdPct)/100 {
					diskNewSize = (event.IngestStatus.System.DiskTotalBytes / 1024 / 1024 / 1024) + uint64(c.GCPDiskIncreaseGB)
					if diskNewSize > uint64(c.SizingMaxDiskGB) {
						diskNewSize = uint64(c.SizingMaxDiskGB)
					}
				}
			}
		}
	}

	// check if RAM is running out and size accordingly
	newType := ""
	disableDim := false
	performRamSizing := func() (performRamSizing bool) {
		switch event.Event {
		case AgiEventServiceDown:
			if event.IsDataInMemory && c.SizingNoDIMFirst {
				disableDim = true
			} else {
				instanceTypes, itype, err := c.getInstanceTypes(zone, currentType)
				if err != nil {
					c.respond(w, r, uuid, 500, "sizing: get instance types failure", fmt.Sprintf("Sizing: getInstanceTypes: %s", err))
					return
				}
				newType, err = c.sizeInstanceType(instanceTypes, currentType, int(itype.RamGB+1))
				if err != nil {
					if !event.IsDataInMemory {
						c.respond(w, r, uuid, 400, "sizing: "+err.Error(), fmt.Sprintf("Sizing: %s", err))
						return
					} else {
						disableDim = true
					}
				}
			}
		case AgiEventPreProcessComplete:
			requiredRam := 0
			dimRequiredMemory := int(math.Ceil((float64(event.IngestStatus.Ingest.LogProcessorTotalSize)*c.DimMultiplier)/1024/1024/1024)) + c.RAMThresMinFreeGB + 2     // required memory plus minimum free RAM to keep for plugin plus 2 GB for OS/ingest overhead
			noDimRequiredMemory := int(math.Ceil((float64(event.IngestStatus.Ingest.LogProcessorTotalSize)*c.NoDimMultiplier)/1024/1024/1024)) + c.RAMThresMinFreeGB + 2 // required memory plus minimum free RAM to keep for plugin plus 2 GB for OS/ingest overhead
			if event.IsDataInMemory {
				requiredRam = dimRequiredMemory
			} else {
				requiredRam = noDimRequiredMemory
			}

			instanceTypes, itype, err := c.getInstanceTypes(zone, currentType)
			if err != nil {
				c.respond(w, r, uuid, 500, "sizing: get instance types failure", fmt.Sprintf("Sizing: getInstanceTypes: %s", err))
				return
			}

			if itype.RamGB >= float64(requiredRam) {
				return
			}

			if event.IsDataInMemory && c.SizingNoDIMFirst {
				disableDim = true
				if itype.RamGB < float64(noDimRequiredMemory) {
					newType, err = c.sizeInstanceType(instanceTypes, currentType, noDimRequiredMemory)
					if err != nil {
						c.log(uuid, "sizing", "WARNING: reached max sizing and will not have enough RAM anyways: "+err.Error())
					}
				}
			} else if event.IsDataInMemory {
				newType, err = c.sizeInstanceType(instanceTypes, currentType, requiredRam)
				if err != nil {
					disableDim = true
				}
			} else {
				newType, err = c.sizeInstanceType(instanceTypes, currentType, requiredRam)
				if err != nil {
					if newType == currentType {
						c.respond(w, r, uuid, 500, "sizing: max reached and may still run out of memory", fmt.Sprintf("Sizing: reached max and will probably still run out of RAM: %s", err))
						return
					}
					c.log(uuid, "sizing", "WARNING: reached max sizing and will not have enough RAM anyways: "+err.Error())
				}
			}
		default:
			if event.IngestStatus.System.MemoryFreeBytes/1024/1024/1024 < c.RAMThresMinFreeGB || (event.IngestStatus.System.MemoryTotalBytes > 0 && event.IngestStatus.System.MemoryFreeBytes > 0 && 1-(float64(event.IngestStatus.System.MemoryFreeBytes)/float64(event.IngestStatus.System.MemoryTotalBytes)) > float64(c.RAMThresUsedPct)/100) {
				if event.IsDataInMemory && c.SizingNoDIMFirst {
					disableDim = true
				} else {
					instanceTypes, itype, err := c.getInstanceTypes(zone, currentType)
					if err != nil {
						c.respond(w, r, uuid, 500, "sizing: get instance types failure", fmt.Sprintf("Sizing: getInstanceTypes: %s", err))
						return
					}
					newType, err = c.sizeInstanceType(instanceTypes, currentType, int(itype.RamGB+1))
					if err != nil {
						if !event.IsDataInMemory {
							c.respond(w, r, uuid, 400, "sizing: "+err.Error(), fmt.Sprintf("Sizing: %s", err))
							return
						} else {
							disableDim = true
						}
					}
				}
			} else {
				return
			}
		}
		performRamSizing = true
		return
	}()

	// perform sizing
	if !performRamSizing {
		newType = ""
		disableDim = false
	}
	if newType == "" && disableDim {
		newType = currentType
	}
	testJson := &agiCreateCmd{}
	if diskNewSize > 0 && newType == "" && !disableDim {
		if !c.getDeploymentJSON(uuid, event, testJson) {
			c.respond(w, r, uuid, 400, "sizing: invalid deployment json", "Sizing: abort on invalid deployment json")
			return
		}
		c.respond(w, r, uuid, 200, "sizing: adding disk capacity", fmt.Sprintf("Sizing: changing disk capacity from %d GiB to %d GiB", event.IngestStatus.System.DiskTotalBytes/1024/1024/1024, diskNewSize))
		go c.handleSizingDisk(uuid, event, int64(diskNewSize))
	} else if diskNewSize == 0 && (newType != "" || disableDim) {
		if !c.getDeploymentJSON(uuid, event, testJson) {
			c.respond(w, r, uuid, 400, "sizing: invalid deployment json", "Sizing: abort on invalid deployment json")
			return
		}
		c.respond(w, r, uuid, 418, "sizing: instance-ram", fmt.Sprintf("Sizing: instance-ram currentType=%s newType=%s disableDim=%t", currentType, newType, disableDim))
		go c.handleSizingRAM(uuid, event, newType, disableDim)
	} else if diskNewSize > 0 && (newType != "" || disableDim) {
		if !c.getDeploymentJSON(uuid, event, testJson) {
			c.respond(w, r, uuid, 400, "sizing: invalid deployment json", "Sizing: abort on invalid deployment json")
			return
		}
		c.respond(w, r, uuid, 418, "sizing: instance-disk-and-ram", fmt.Sprintf("Sizing: instance-disk-and-ram old-disk=%dGiB new-disk=%dGiB currentType=%s newType=%s disableDim=%t", event.IngestStatus.System.DiskTotalBytes/1024/1024/1024, diskNewSize, currentType, newType, disableDim))
		go c.handleSizingDiskAndRAM(uuid, event, int64(diskNewSize), newType, disableDim)
	} else {
		c.respond(w, r, uuid, 200, "sizing: not required", "sizing: not required")
	}
}

func (c *agiMonitorListenCmd) getInstanceTypes(zone string, currentType string) ([]instanceType, *instanceType, error) {
	instanceTypes, err := b.GetInstanceTypes(0, 0, 0, 0, 0, 0, false, zone)
	if err != nil {
		return nil, nil, err
	}
	instanceTypesX, err := b.GetInstanceTypes(0, 0, 0, 0, 0, 0, true, zone)
	if err != nil {
		return nil, nil, err
	}
	for _, t := range instanceTypesX {
		found := false
		for _, tt := range instanceTypes {
			if t.InstanceName == tt.InstanceName {
				found = true
				break
			}
		}
		if !found {
			instanceTypes = append(instanceTypes, t)
		}
	}

	var itype *instanceType
	for _, t := range instanceTypes {
		if t.InstanceName == currentType {
			itype = &t
			break
		}
	}
	if itype == nil {
		return instanceTypes, itype, fmt.Errorf("instance type '%s' not found", currentType)
	}
	return instanceTypes, itype, nil
}

// will return err if required RAM exceeds max instance sizing
// if err is set, newType will be set to max instance size available; or to currentType if family not found
func (c *agiMonitorListenCmd) sizeInstanceType(instanceTypes []instanceType, currentType string, requiredMemory int) (newType string, err error) {
	family := ""
	if a.opts.Config.Backend.Type == "aws" {
		if !strings.Contains(currentType, ".") {
			return currentType, errors.New("family not found")
		}
		family = strings.Split(currentType, ".")[0] + "."
	} else {
		fsplit := strings.Split(currentType, "-")
		if len(fsplit) != 3 {
			return currentType, errors.New("family type cannot be sized")
		}
		family = fsplit[0] + "-" + fsplit[1] + "-"
	}

	type ntype struct {
		name string
		ram  float64
	}

	ntypes := []ntype{}
	for _, t := range instanceTypes {
		if t.RamGB > float64(c.SizingMaxRamGB) {
			continue
		}
		if strings.HasPrefix(t.InstanceName, family) {
			ntypes = append(ntypes, ntype{
				name: t.InstanceName,
				ram:  t.RamGB,
			})
		}
	}
	if len(ntypes) == 0 {
		return currentType, errors.New("family not in list or list exhausted")
	}
	sort.Slice(ntypes, func(i, j int) bool {
		return ntypes[i].ram < ntypes[j].ram
	})
	for _, n := range ntypes {
		if n.ram >= float64(requiredMemory) {
			return n.name, nil
		}
		newType = n.name
	}
	return newType, errors.New("sizing exhausted")
}
