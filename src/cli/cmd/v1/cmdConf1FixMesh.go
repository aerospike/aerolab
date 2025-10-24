package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/logger"
)

type ConfFixMeshCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	ConfigPath      string          `short:"c" long:"config-path" description:"path to a custom aerospike config file to use for the configuration" default:"/etc/aerospike/aerospike.conf"`
	ParallelThreads int             `short:"p" long:"parallel-threads" description:"number of threads to use for parallel execution" default:"10"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfFixMeshCmd) Execute(args []string) error {
	cmd := []string{"conf", "fix-mesh"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.FixMesh(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ConfFixMeshCmd) FixMesh(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"conf", "fix-mesh"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	instances := inventory.Instances.WithState(backends.LifeCycleStateRunning).WithClusterName(c.ClusterName.String())
	if c.Nodes != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return err
		}
		instances = instances.WithNodeNo(nodes...)
		if instances.Count() != len(nodes) {
			return fmt.Errorf("some nodes not found: %s", c.Nodes.String())
		}
	}
	lanIPs := []string{}
	for _, i := range instances.Describe() {
		lanIPs = append(lanIPs, i.IP.Private)
	}
	var hasErr bool
	parallelize.ForEachLimit(instances.Describe(), c.ParallelThreads, func(i *backends.Instance) {
		conf, err := i.GetSftpConfig("root")
		if err != nil {
			logger.Error("Failed to fix mesh for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
		client, err := sshexec.NewSftp(conf)
		if err != nil {
			logger.Error("Failed to fix mesh for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
		defer client.Close()
		var buf bytes.Buffer
		err = client.ReadFile(&sshexec.FileReader{
			SourcePath:  c.ConfigPath,
			Destination: &buf,
		})
		if err != nil {
			logger.Error("Failed to fix mesh for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
		newConfig, err := fixHeartbeats(buf.Bytes(), "mesh", "", "", lanIPs)
		if err != nil {
			logger.Error("Failed to fix mesh for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
		newConfig, err = fixAccessAddress(newConfig, i.IP.Private)
		if err != nil {
			logger.Error("Failed to fix mesh for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath: c.ConfigPath,
			Source:   bytes.NewReader(newConfig),
		})
		if err != nil {
			logger.Error("Failed to fix mesh for node %s: %s", i.Name, err)
			hasErr = true
			return
		}
	})
	if hasErr {
		return errors.New("some nodes failed to fix mesh")
	}
	return nil
}

// modes:
// - default: do nothing
// - auto: determine the best mode based on current config, or otherwise default to mesh
// - mesh: use mesh mode
// - mcast: use multicast mode
func fixHeartbeats(conf []byte, mode string, addr string, port string, intIps []string) (data []byte, err error) {
	if mode == "default" {
		return conf, nil
	}
	ac, err := aeroconf.Parse(bytes.NewReader(conf))
	if err != nil {
		return nil, err
	}
	if ac.Type("network") == aeroconf.ValueNil {
		ac.NewStanza("network")
	}
	if ac.Stanza("network").Type("heartbeat") == aeroconf.ValueNil {
		ac.Stanza("network").NewStanza("heartbeat")
	}
	if ac.Stanza("network").Stanza("heartbeat").Type("interval") == aeroconf.ValueNil {
		ac.Stanza("network").Stanza("heartbeat").SetValue("interval", "150")
	}
	if ac.Stanza("network").Stanza("heartbeat").Type("timeout") == aeroconf.ValueNil {
		ac.Stanza("network").Stanza("heartbeat").SetValue("timeout", "10")
	}

	if mode == "auto" {
		if ac.Stanza("network").Stanza("heartbeat").Type("mode") == aeroconf.ValueNil {
			mode = "mesh"
		} else {
			vals, err := ac.Stanza("network").Stanza("heartbeat").GetValues("mode")
			if err != nil {
				return conf, err
			}
			for _, val := range vals {
				if strings.Trim(*val, "\r\n\t ") == "mesh" {
					mode = "mesh"
					break
				} else if strings.Trim(*val, "\r\n\t ") == "multicast" {
					mode = "mcast"
					break
				}
			}
			if mode == "auto" {
				mode = "mesh"
			}
		}
	}

	switch mode {
	case "mesh":
		ac.Stanza("network").Stanza("heartbeat").Delete("multicast-group")
		ac.Stanza("network").Stanza("heartbeat").SetValue("mode", "mesh")
		ac.Stanza("network").Stanza("heartbeat").Delete("mesh-seed-address-port")
		ac.Stanza("network").Stanza("heartbeat").Delete("tls-mesh-seed-address-port")
		if ac.Stanza("network").Stanza("heartbeat").Type("port") == aeroconf.ValueNil && ac.Stanza("network").Stanza("heartbeat").Type("tls-port") == aeroconf.ValueNil {
			ac.Stanza("network").Stanza("heartbeat").SetValue("port", "3002")
		}
		if ac.Stanza("network").Stanza("heartbeat").Type("port") != aeroconf.ValueNil {
			vals, err := ac.Stanza("network").Stanza("heartbeat").GetValues("port")
			if err != nil {
				return conf, err
			}
			port := "3002"
			for _, val := range vals {
				valx := strings.Trim(*val, "\r\n\t ")
				if strings.HasPrefix(valx, "9") {
					ac.Stanza("network").Stanza("heartbeat").SetValue("port", "3002")
					break
				} else {
					port = valx
				}
			}
			vals = []*string{}
			for j := 0; j < len(intIps); j++ {
				val := fmt.Sprintf("%s %s", intIps[j], port)
				vals = append(vals, &val)
			}
			ac.Stanza("network").Stanza("heartbeat").SetValues("mesh-seed-address-port", vals)
		}
		if ac.Stanza("network").Stanza("heartbeat").Type("tls-port") != aeroconf.ValueNil {
			vals, err := ac.Stanza("network").Stanza("heartbeat").GetValues("tls-port")
			if err != nil {
				return conf, err
			}
			port := "3012"
			for _, val := range vals {
				valx := strings.Trim(*val, "\r\n\t ")
				if strings.HasPrefix(valx, "9") {
					ac.Stanza("network").Stanza("heartbeat").SetValue("tls-port", "3012")
					break
				} else {
					port = valx
				}
			}
			vals = []*string{}
			for j := 0; j < len(intIps); j++ {
				val := fmt.Sprintf("%s %s", intIps[j], port)
				vals = append(vals, &val)
			}
			ac.Stanza("network").Stanza("heartbeat").SetValues("tls-mesh-seed-address-port", vals)
		}
	case "mcast", "multicast":
		ac.Stanza("network").Stanza("heartbeat").SetValue("mode", "multicast")
		ac.Stanza("network").Stanza("heartbeat").SetValue("multicast-group", addr)
		ac.Stanza("network").Stanza("heartbeat").Delete("mesh-seed-address-port")
		ac.Stanza("network").Stanza("heartbeat").Delete("tls-mesh-seed-address-port")
		if ac.Stanza("network").Stanza("heartbeat").Type("port") == aeroconf.ValueNil && ac.Stanza("network").Stanza("heartbeat").Type("tls-port") == aeroconf.ValueNil {
			ac.Stanza("network").Stanza("heartbeat").SetValue("port", port)
		} else {
			vals, err := ac.Stanza("network").Stanza("heartbeat").GetValues("port")
			if err != nil {
				return conf, err
			}
			for _, val := range vals {
				valx := strings.Trim(*val, "\r\n\t ")
				if strings.HasPrefix(valx, "3") {
					ac.Stanza("network").Stanza("heartbeat").SetValue("port", port)
					break
				}
			}
		}
	}

	buf := bytes.NewBuffer(nil)
	err = ac.Write(buf, "", "  ", true)
	if err != nil {
		return nil, err
	}
	data = buf.Bytes()
	return data, nil
}

func fixAccessAddress(old []byte, newIp string) (new []byte, err error) {
	conf, err := aeroconf.Parse(bytes.NewReader(old))
	if err != nil {
		return old, err
	}
	s := conf.Stanza("network")
	if s == nil {
		return old, errors.New("network stanza not found")
	}
	s = s.Stanza("service")
	if s == nil {
		return old, errors.New("network.service stanza not found")
	}
	for _, str := range []string{"access-address", "tls-access-address"} {
		if s.Type(str) == aeroconf.ValueString {
			vals, err := s.GetValues(str)
			if err != nil {
				return old, err
			}
			for i, val := range vals {
				if val == nil || strings.HasPrefix(*val, "127.") {
					continue
				}
				valIP := net.ParseIP(*val)
				if valIP.IsPrivate() {
					vals[i] = &newIp
				}
			}
		}
	}
	if s.Type("alternate-access-address") == aeroconf.ValueString {
		vals, err := s.GetValues("alternate-access-address")
		if err != nil {
			return old, err
		}
		for i, val := range vals {
			if val == nil || strings.HasPrefix(*val, "127.") {
				continue
			}
			valIP := net.ParseIP(*val)
			if valIP.IsPrivate() {
				vals[i] = &newIp
			}
		}
	}
	var buf bytes.Buffer
	err = conf.Write(&buf, "", "    ", true)
	if err != nil {
		return old, err
	}
	return buf.Bytes(), nil
}
