package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/aerospike/aerolab/scripts"
	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
	"gopkg.in/yaml.v3"
)

type clientCreateVectorCmd struct {
	clientCreateNoneCmd
	ClusterName                TypeClusterName `short:"C" long:"cluster-name" description:"aerospike cluster name to seed from" default:"mydc"`
	Seed                       string          `long:"seed" description:"specify an aerospike cluster seed IP:PORT instead of providing a ClusterName; if this parameter is provided, ClusterName is ignored"`
	ServiceListen              string          `long:"listen" description:"specify a listen IP:PORT for the service" default:"0.0.0.0:5555"`
	NoTouchServiceListen       bool            `long:"no-touch-listen" description:"set this to prevent aerolab from touching the service: configuration part"`
	NoTouchSeed                bool            `long:"no-touch-seed" description:"set this to prevent aerolab from configuring the aerospike seed ip and port"`
	NoTouchAdvertisedListeners bool            `long:"no-touch-advertised" description:"set this to prevent aerolab from configuring the advertised listeners"`
	VectorVersion              string          `long:"version" description:"vector version to install; only 0.3.1 is officially supported by aerolab (0.3.1-1 for rpm)" default:"0.3.1"`
	CustomConf                 flags.Filename  `long:"custom-conf" description:"provide a custom aerospike-proximus.yml to ship"`
	NoStart                    bool            `long:"no-start" description:"if set, service will not be started after installation"`
	FeaturesFile               flags.Filename  `short:"f" long:"featurefile" description:"Features file to install; if not provided, the features.conf from the seed aerospike cluster will be taken"`
	MetadataNamespace          string          `long:"metans" description:"configure the metadata namespace name" default:"proximus-meta"`
	JustDoIt                   bool            `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue" webdisable:"true" webset:"true"`
	seedip                     string
	seedport                   string
	seedportint                int
	serviceip                  string
	serviceport                string
	chDirCmd
}

func (c *clientCreateVectorCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("WARNING: Vector is in early development stage; things may break and stop working. If so, please report.")
	if c.DistroName == "ubuntu" && c.DistroVersion == "latest" {
		log.Println("WARNING: Ubuntu version 24.04 is not yet supported by prism/python process. Using 22.04 instead.")
		c.DistroVersion = "22.04"
	}
	if !c.NoTouchServiceListen {
		addr, err := net.ResolveTCPAddr("tcp", c.ServiceListen)
		if err != nil {
			return err
		}
		c.serviceport = strconv.Itoa(addr.Port)
		c.serviceip = addr.IP.String()
	} else {
		c.serviceport = "5000"
		c.serviceip = "localhost"
	}

	if a.opts.Config.Backend.Type == "docker" && !strings.Contains(c.Docker.ExposePortsToHost, ":8998") {
		if !c.Docker.NoAutoExpose {
			c.Docker.ExposePortsToHost = strings.Trim("8998:8998,"+c.Docker.ExposePortsToHost, ",")
		}
	}
	if a.opts.Config.Backend.Type == "docker" && !strings.Contains(c.Docker.ExposePortsToHost, ":"+c.serviceport) {
		if c.Docker.NoAutoExpose {
			fmt.Println("Docker backend is in use, but vector access port is not being forwarded. If using Docker Desktop, use '-e " + c.serviceport + ":" + c.serviceport + "' parameter in order to forward port " + c.serviceport + ". Press ENTER to continue regardless.")
			if !c.JustDoIt {
				var ignoreMe string
				fmt.Scanln(&ignoreMe)
			}
		} else {
			c.Docker.ExposePortsToHost = strings.Trim(c.serviceport+":"+c.serviceport+","+c.Docker.ExposePortsToHost, ",")
		}
	}

	// early checks
	if c.FeaturesFile != "" {
		if _, err := os.Stat(string(c.FeaturesFile)); err != nil {
			return err
		}
	}
	if c.CustomConf != "" {
		if _, err := os.Stat(string(c.CustomConf)); err != nil {
			return err
		}
	}

	// features file
	ffc := []byte{}
	var err error
	if c.FeaturesFile != "" {
		ffc, err = os.ReadFile(string(c.FeaturesFile))
		if err != nil {
			return err
		}
	} else if c.Seed == "" {
		log.Print("Downloading features file from a cluster node")
		b.WorkOnServers()
		nodes, err := b.NodeListInCluster(string(c.ClusterName))
		if err != nil {
			return err
		}
		if len(nodes) == 0 {
			return errors.New("specified cluster has 0 nodes, or does not exist")
		}
		out, err := b.RunCommands(string(c.ClusterName), [][]string{{"cat", "/etc/aerospike/features.conf"}}, []int{nodes[0]})
		if err != nil {
			if len(out) > 0 {
				err = fmt.Errorf("%s\n%s", err, out[0])
			}
			return err
		}
		ffc = out[0]
	} else {
		return errors.New("no feature file provided")
	}

	// test the feaures file for the correct enabled feature
	ffcscanner := bufio.NewScanner(bytes.NewReader(ffc))
	ffcenabled := false
	for ffcscanner.Scan() {
		line := strings.ToLower(strings.Trim(ffcscanner.Text(), "\r\n\t "))
		if strings.HasPrefix(line, "vector-service") && strings.HasSuffix(line, "true") {
			ffcenabled = true
			break
		}
	}
	if !ffcenabled {
		if !c.JustDoIt {
			fmt.Println("\nWARNING: The given feature key file does not have vector-service enabled. This will not work.\nPlease cancel (CTRL+C) and provide a feature key file using `-f /path/to/file`.")
			fmt.Println("Press ENTER to continue regardless.")
			reader := bufio.NewReader(os.Stdin)
			_, err := reader.ReadString('\n')
			if err != nil {
				logExit(err)
			}
		} else {
			fmt.Println("\nWARNING: The given feature key file does not have vector-service enabled. This will not work.\nTo provide a feature key file, use `-f /path/to/file` in the create command. Continuing regardless...")
		}
	}

	// confirm at least 2 namespaces exist in cluster, and one has the same name as c.MetadataNamespace and has nsup enabled
	if !c.JustDoIt {
		fmt.Printf("\n-> The Vector client (Proximus) requires at least 2 namespaces, one of which must be called '%s' and have `nsup-period` enabled.\n-> If this is not the case, CTRL+C now and reconfigure the cluster.\n-> Example `aerospike.conf` can be obtained by running `aerolab conf generate` and ticking the `vector` checkbox.\n\nPress ENTER to continue...", c.MetadataNamespace)
		reader := bufio.NewReader(os.Stdin)
		_, err := reader.ReadString('\n')
		if err != nil {
			logExit(err)
		}
	} else {
		fmt.Printf("\n-> The Vector client (Proximus) requires at least 2 namespaces, one of which must be called '%s' and have `nsup-period` enabled.\n-> Example `aerospike.conf` can be obtained by running `aerolab conf generate` and ticking the `vector` checkbox.\n\n", c.MetadataNamespace)
	}

	if c.Seed == "" && !c.NoTouchSeed {
		fmt.Println("Getting cluster list")
		b.WorkOnServers()
		clist, err := b.ClusterList()
		if err != nil {
			return err
		}
		if !inslice.HasString(clist, string(c.ClusterName)) {
			return errors.New("cluster not found")
		}
		ips, err := b.GetNodeIpMap(string(c.ClusterName), true)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			ips, err = b.GetNodeIpMap(string(c.ClusterName), false)
			if err != nil {
				return err
			}
			if len(ips) == 0 {
				return errors.New("node IPs not found")
			}
		}
		for _, ip := range ips {
			if ip != "" {
				c.seedip = ip
				break
			}
		}
		c.seedport = "3000"
		if a.opts.Config.Backend.Type == "docker" {
			inv, err := b.Inventory("", []int{InventoryItemClusters})
			if err != nil {
				return err
			}
			for _, item := range inv.Clusters {
				if item.ClusterName == c.ClusterName.String() {
					if item.PrivateIp != "" && item.DockerExposePorts != "" {
						c.seedport = item.DockerExposePorts
						c.seedip = item.PrivateIp
					}
				}
			}
		}
	} else if !c.NoTouchSeed {
		addr, err := net.ResolveTCPAddr("tcp", c.Seed)
		if err != nil {
			return err
		}
		c.seedport = strconv.Itoa(addr.Port)
		c.seedip = addr.IP.String()
	}
	c.seedportint, err = strconv.Atoi(c.seedport)
	if err != nil {
		return err
	}
	b.WorkOnClients()
	if !c.NoTouchSeed && c.seedip == "" {
		return errors.New("could not find an IP for a node in the given cluster - are all the nodes down?")
	}

	// install
	c.Docker.Labels = append(c.Docker.Labels, "lport="+c.serviceport)
	c.Aws.Tags = append(c.Aws.Tags, "lport="+c.serviceport)
	c.Gcp.Labels = append(c.Gcp.Labels, "lport="+c.serviceport)
	machines, err := c.createBase(args, "vector")
	if err != nil {
		return err
	}
	if c.PriceOnly {
		return nil
	}
	log.Println("Continuing vector installation...")
	var extIps map[int]string
	var intIps map[int]string
	if !c.NoTouchAdvertisedListeners {
		extIps, err = b.GetNodeIpMap(string(c.ClientName), false)
		if err != nil {
			return err
		}
		intIps, err = b.GetNodeIpMap(string(c.ClientName), true)
		if err != nil {
			return err
		}
	}

	// find download URL and generate script
	dlUrl := "https://aerospike.jfrog.io/artifactory/deb/aerospike-proximus-" + c.VectorVersion + ".deb"
	fExt := "deb"
	if c.DistroName != "ubuntu" && c.DistroName != "debian" {
		if !strings.Contains(c.VectorVersion, "-") {
			c.VectorVersion = c.VectorVersion + "-1"
		}
		dlUrl = "https://aerospike.jfrog.io/artifactory/rpm/aerospike-proximus-" + c.VectorVersion + ".noarch.rpm"
		fExt = "rpm"
	}
	script := scripts.GetVectorScript(a.opts.Config.Backend.Type == "docker", fExt, dlUrl)
	log.Printf("Download URL: %s", dlUrl)

	aoptslock := new(sync.Mutex)
	returns := parallelize.MapLimit(machines, c.ParallelThreads, func(node int) error {
		// handle advertised listeners
		listeners := make(vectorAdvertisedListeners)
		if !c.NoTouchAdvertisedListeners {
			listeners["localhost"] = &vectorAdvertisedListener{
				Address: "127.0.0.1",
				Port:    c.serviceport,
			}
			if v, ok := extIps[node]; ok && v != "" {
				listeners["default"] = &vectorAdvertisedListener{
					Address: v,
					Port:    c.serviceport,
				}
			}
			if v, ok := intIps[node]; ok && v != "" {
				listeners["local"] = &vectorAdvertisedListener{
					Address: v,
					Port:    c.serviceport,
				}
			}
			if _, ok := listeners["local"]; !ok {
				listeners["local"] = listeners["default"]
			}
			if _, ok := listeners["default"]; !ok {
				listeners["default"] = listeners["local"]
			}
		}

		if c.CustomConf != "" {
			log.Printf("client=%d read custom conf file", node)
			// read the custom conf
			fc, err := os.ReadFile(string(c.CustomConf))
			if err != nil {
				return err
			}
			newconf := fc
			if !c.NoTouchSeed || !c.NoTouchServiceListen || c.MetadataNamespace != "proximus-meta" || !c.NoTouchAdvertisedListeners {
				log.Printf("client=%d patch custom conf file", node)
				// patch the custom conf
				newconf, err = c.vectorConfigPatch(fc, listeners)
				if err != nil {
					return err
				}
			}
			// upload custom conf
			log.Printf("client=%d upload custom conf file", node)
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/aerospike-proximus/aerospike-proximus.yml", string(newconf), len(newconf)}}, []int{node})
			if err != nil {
				return err
			}
		}

		// upload and run install script
		log.Printf("client=%d upload and run install script", node)
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/opt/install-vector.sh", string(script), len(script)}}, []int{node})
		if err != nil {
			return err
		}
		out, err := b.RunCommands(string(c.ClientName), [][]string{{"/bin/bash", "/opt/install-vector.sh"}}, []int{node})
		if err != nil {
			if len(out) > 0 {
				err = fmt.Errorf("%s\n%s", err, out[0])
			}
			return err
		}

		if c.CustomConf == "" && (!c.NoTouchSeed || !c.NoTouchServiceListen || c.MetadataNamespace != "proximus-meta" || !c.NoTouchAdvertisedListeners) {
			log.Printf("client=%d download, patch and upload conf file", node)
			// download, patch and reupload config file /etc/aerospike-proximus/aerospike-proximus.yml
			// download
			oldconf, err := b.RunCommands(string(c.ClientName), [][]string{{"cat", "/etc/aerospike-proximus/aerospike-proximus.yml"}}, []int{node})
			if err != nil {
				if len(oldconf) > 0 {
					err = fmt.Errorf("%s\n%s", err, oldconf[0])
				}
				return err
			}
			// patch config file
			newconf, err := c.vectorConfigPatch(oldconf[0], listeners)
			if err != nil {
				return err
			}
			// upload custom conf
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/aerospike-proximus/aerospike-proximus.yml", string(newconf), len(newconf)}}, []int{node})
			if err != nil {
				return err
			}
		}

		// features file
		log.Printf("client=%d upload features file", node)
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/aerospike-proximus/features.conf", string(ffc), len(ffc)}}, []int{node})
		if err != nil {
			return err
		}

		// startup
		// if docker - also detach-run the autoload /opt/autoload/10-proximus
		// if cloud - systemctl start aerospike-proximus
		if !c.NoStart {
			// make a lock on this as doing a.opts in parallelize causes a race condition!
			aoptslock.Lock()
			log.Printf("client=%d start service", node)
			if a.opts.Config.Backend.Type == "docker" {
				a.opts.Attach.Client.ClientName = c.ClientName
				a.opts.Attach.Client.Machine = TypeMachines(strconv.Itoa(node))
				a.opts.Attach.Client.Detach = true
				defer backendRestoreTerminal()
				err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/autoload/10-proximus"})
				if err != nil {
					aoptslock.Unlock()
					return err
				}
			} else {
				a.opts.Attach.Client.ClientName = c.ClientName
				a.opts.Attach.Client.Machine = TypeMachines(strconv.Itoa(node))
				defer backendRestoreTerminal()
				err = a.opts.Attach.Client.run([]string{"systemctl", "start", "aerospike-proximus"})
				if err != nil {
					aoptslock.Unlock()
					return err
				}
			}
			aoptslock.Unlock()
		}

		// upload prism-example script
		log.Printf("client=%d upload example script", node)
		listenPort := "8080"
		if a.opts.Config.Backend.Type == "docker" {
			listenPort = "8998"
		}
		ex := scripts.GetVectorExampleScript(c.serviceport, listenPort)
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/opt/prism-example.sh", string(ex), len(ex)}}, []int{node})
		if err != nil {
			return err
		}
		log.Printf("client=%d done", node)
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", machines[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}
	log.Println("Done")
	log.Println(" * Proximus python client manual: https://github.com/aerospike/aerospike-proximus-client-python")
	log.Println(" * Proximus usage examples:       https://github.com/aerospike/proximus-examples")
	fmt.Printf("\nTo download examples:\n  $ aerolab attach client -n %s\n  $ apt -y install python3 python3-pip git\n  $ cd /opt && git clone https://github.com/aerospike/proximus-examples.git\n  $ cd /opt/proximus-examples\n  $ ls\n", c.ClientName)
	fmt.Println("\nPrism image search example installation script and manual are provided. For instructions, see https://github.com/aerospike/aerolab/blob/master/docs/deploy_clients/vector.md")
	return nil
}

func mapDelete(mp interface{}, keys []interface{}) error {
	for i, key := range keys {
		switch m := mp.(type) {
		case map[interface{}]interface{}:
			if i < len(keys)-1 {
				if _, ok := m[key]; !ok {
					return nil
				}
			} else {
				delete(m, key)
			}
			mp = m[key]
		case map[string]interface{}:
			switch k := key.(type) {
			case string:
				if i < len(keys)-1 {
					if _, ok := m[k]; !ok {
						return nil
					}
				} else {
					delete(m, k)
				}
				mp = m[k]
			default:
				return fmt.Errorf("type mismatch key=%s map type=%s", reflect.TypeOf(key).String(), reflect.TypeOf(mp).String())
			}
		default:
			return fmt.Errorf("delete: invalid map type (%s), must be map[interface{}]interface{} (keys:%v mp:%v)", reflect.TypeOf(mp).String(), keys, mp)
		}
	}
	return nil
}

func mapMakeSet(mp interface{}, keys []interface{}, value interface{}) error {
	for i, key := range keys {
		switch m := mp.(type) {
		case map[interface{}]interface{}:
			if i < len(keys)-1 {
				if _, ok := m[key]; !ok {
					m[key] = make(map[interface{}]interface{})
				}
			} else {
				m[key] = value
			}
			mp = m[key]
		case map[string]interface{}:
			switch k := key.(type) {
			case string:
				if i < len(keys)-1 {
					if _, ok := m[k]; !ok {
						m[k] = make(map[interface{}]interface{})
					}
				} else {
					m[k] = value
				}
				mp = m[k]
			default:
				return fmt.Errorf("type mismatch key=%s map type=%s", reflect.TypeOf(key).String(), reflect.TypeOf(mp).String())
			}
		default:
			return fmt.Errorf("set: invalid map type, must be map[interface{}]interface{} (keys:%v mp:%v)", keys, mp)
		}
	}
	return nil
}

type vectorAdvertisedListener struct {
	Address string `yaml:"address"`
	Port    string `yaml:"port"`
}

type vectorAdvertisedListeners map[string]*vectorAdvertisedListener

func (c *clientCreateVectorCmd) vectorConfigPatch(fc []byte, listeners vectorAdvertisedListeners) ([]byte, error) {
	config := make(map[interface{}]interface{})
	err := yaml.NewDecoder(bytes.NewReader(fc)).Decode(config)
	if err != nil {
		return fc, err
	}
	if !c.NoTouchAdvertisedListeners && len(listeners) > 0 {
		err = mapDelete(config, []interface{}{"service", "advertised-listeners"})
		if err != nil {
			return fc, fmt.Errorf("%v\n%v\n%v", err, config, []interface{}{"service", "advertised-listeners"})
		}
		err = mapMakeSet(config, []interface{}{"service", "advertised-listeners"}, listeners)
		if err != nil {
			return fc, fmt.Errorf("%v\n%v\n%v", err, config, []interface{}{"service", "advertised-listeners"})
		}
	}
	if !c.NoTouchServiceListen {
		err = mapDelete(config, []interface{}{"service", "ports"})
		if err != nil {
			return fc, fmt.Errorf("%v\n%v\n%v", err, config, []interface{}{"service", "ports"})
		}
		err = mapMakeSet(config, []interface{}{"service", "ports", c.serviceport, "addresses"}, []string{c.serviceip})
		if err != nil {
			return fc, fmt.Errorf("%v\n%v\n%v", err, config, []interface{}{"service", "ports", c.serviceport, "addresses"})
		}
	}
	if !c.NoTouchSeed {
		err = mapDelete(config, []interface{}{"aerospike", "seeds"})
		if err != nil {
			return fc, fmt.Errorf("%v\n%v\n%v", err, config, []interface{}{"aerospike", "seeds"})
		}
		err = mapMakeSet(config, []interface{}{"aerospike", "seeds"}, []map[string]interface{}{
			{
				c.seedip: map[string]int{
					"port": c.seedportint,
				},
			},
		})
		if err != nil {
			return fc, fmt.Errorf("%v\n%v\n%v", err, config, []interface{}{"aerospike", "seeds"})
		}
	}
	if c.MetadataNamespace != "proximus-meta" {
		err = mapMakeSet(config, []interface{}{"aerospike", "metadata-namespace"}, c.MetadataNamespace)
		if err != nil {
			return fc, fmt.Errorf("%v\n%v\n%v", err, config, []interface{}{"aerospike", "metadata-namespace"})
		}
	}
	buf := &bytes.Buffer{}
	err = yaml.NewEncoder(buf).Encode(config)
	if err != nil {
		return fc, err
	}
	return buf.Bytes(), nil
}
