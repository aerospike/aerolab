package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/bestmethod/inslice"
	"github.com/jessevdk/go-flags"
)

type clientCreateVectorCmd struct {
	clientCreateNoneCmd
	ClusterName          TypeClusterName `short:"C" long:"cluster-name" description:"cluster name to seed from" default:"mydc"`
	Seed                 string          `long:"seed" description:"specify a seed IP:PORT instead of providing a ClusterName; if this parameter is provided, ClusterName is ignored"`
	ServiceListen        string          `long:"listen" description:"specify a listen IP:PORT for the service" default:"0.0.0.0:5000"`
	NoTouchServiceListen bool            `long:"no-touch-listen" description:"set this to prevent aerolab from touching the service: configuration part"`
	NoTouchSeed          bool            `long:"no-touch-seed" description:"set this to prevent aerolab from configuring the aerospike seed ip and port"`
	JustDoIt             bool            `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue" webdisable:"true" webset:"true"`
	VectorVersion        string          `long:"version" description:"vector version to install; only 0.3.1 is officially supported by aerolab" default:"0.3.1"`
	CustomConf           flags.Filename  `long:"custom-conf" description:"provide a custom aerospike-proximus.yml to ship"`
	seedip               string
	seedport             string
	serviceip            string
	serviceport          string
	chDirCmd
}

func (c *clientCreateVectorCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
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
	returns := parallelize.MapLimit(machines, c.ParallelThreads, func(node int) error {
		/* TODO: advertised listeners - valid only for docker - if running via expose, set to 127.0.0.1
		# The Proximus service listening ports, TLS and network interface.
		service:
		  ports:
		    5000: {}
		  # Required when running behind NAT
		  advertised-listeners:
		   default:
		     # List of externally accessible addresses and ports for this Proximus instance.
		     - address: {EXTERNAL_IP}
		       port: 5000

		*/
		// TODO make version string configurable
		/* NOTES
		Vector download one of:
		 * https://aerospike.jfrog.io/artifactory/deb/aerospike-proximus-0.3.1.deb
		 * https://aerospike.jfrog.io/artifactory/rpm/aerospike-proximus-0.3.1-1.noarch.rpm

		set -e
		[ -f /tmp/installer.deb ] && apt update && apt -y install openjdk-21-jdk-headless /tmp/installer.deb
		[ -f /tmp/installer.rpm ] && yum -y install java-21-openjdk && yum localinstall /tmp/installer.rpm
		aerolab cluster attach -n $NEW_TRIAL_NAME -- systemctl enable --now aerospike-proximus

		Cloud:
		 * systemctl enable --now aerospike-proximus
		Docker:
		 * /opt/aerospike-proximus/bin/aerospike-proximus -f /etc/aerospike-proximus/aerospike-proximus.yml

		custom /etc/aerospike-proximus/aerospike-proximus.yml
		custom /etc/aerospike-proximus
		port 5000 - allow custom configuration

		version - allow custom

		auto-patch yaml to change port:
		config["service"]["ports"] = map[string]interface{}{
			"5000": map[string]interface{}{
				"addresses": []string{
					"0.0.0.0"
				},
			},
		}
		--
		config["aerospike"]["seeds"] = []map[string]interface{}{
			map[string]interface{}{
				"1.2.3.4": map[string]interface{}{
					"port": 3000,
				},
			},
		}
		*/
		/* TODO b.CopyFilesToCluster
		optional custom yaml configuration file OR option custom whole configuration directory
		upload the installer-download and installation script
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/aerospike-graph.properties", string(graphConfig), len(graphConfig)}, {"/tmp/install-graph.sh", string(graphScript), len(graphScript)}}, []int{node})
			if err != nil {
				return err
			}
		*/
		/* TODO a.opts.Attach
		run installation script (it should contain, for docker only, the /opt/autoload vector starter)
			a.opts.Attach.Client.ClientName = c.ClientName
			a.opts.Attach.Client.Machine = TypeMachines(strconv.Itoa(node))
			defer backendRestoreTerminal()
			err = a.opts.Attach.Client.run([]string{"/bin/bash", "/tmp/install-graph.sh"})
			if err != nil {
				return err
			}
		*/
		/* TODO download/path/upload
		patch configuration file with service listen and aerospike seed ip/ports
		*/
		/* TODO run the service (docker/cloud specific run command)
		 */
		/* YAML PATCHER
		config := make(map[interface{}]interface{})
		err := yaml.NewDecoder(strings.NewReader(yml)).Decode(config)
		if err != nil {
			log.Fatal(err)
		}
		err = mapDelete(config, []interface{}{"service", "ports"})
		if err != nil {
			log.Fatal(err)
		}
		err = mapMakeSet(config, []interface{}{"service", "ports", 6000, "addresses"}, []string{"0.0.0.0"})
		if err != nil {
			log.Fatal(err)
		}
		err = mapDelete(config, []interface{}{"aerospike", "seeds"})
		if err != nil {
			log.Fatal(err)
		}
		err = mapMakeSet(config, []interface{}{"aerospike", "seeds"}, []map[string]interface{}{
			{
				"1.2.3.4": map[string]int{
					"port": 3000,
				},
			},
		})
		if err != nil {
			log.Fatal(err)
		}
		yaml.NewEncoder(os.Stdout).Encode(config)
		*/
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
		default:
			return errors.New("invalid map type, must be map[string]interface{}")
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
		default:
			return errors.New("invalid map type, must be map[interface{}]interface{}")
		}
	}
	return nil
}
