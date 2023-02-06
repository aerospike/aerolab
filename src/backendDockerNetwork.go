package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
)

type dockerListNetwork struct {
	Name   string
	Driver string
	IPAM   struct {
		Config []struct {
			Subnet  string
			Gateway string
		}
	}
	Options map[string]string
}

func (d *backendDocker) CreateNetwork(name string, driver string, subnet string, mtu string) error {
	if driver == "" {
		driver = "bridge"
	}
	opts := []string{"network", "create", "--attachable", "-d", driver}
	if subnet != "" {
		opts = append(opts, "--subnet", subnet)
	}
	opts = append(opts, "--opt", "com.docker.network.bridge.enable_icc=true", "--opt", "com.docker.network.bridge.enable_ip_masquerade=true", "--opt", "com.docker.network.bridge.host_binding_ipv4=0.0.0.0", "--opt", "com.docker.network.bridge.name="+name)
	if mtu != "" {
		opts = append(opts, "--opt", "com.docker.network.driver.mtu="+mtu)
	}
	opts = append(opts, name)
	out, err := exec.Command("docker", opts...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func (d *backendDocker) DeleteNetwork(name string) error {
	out, err := exec.Command("docker", "network", "rm", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func (d *backendDocker) PruneNetworks() error {
	out, err := exec.Command("docker", "network", "prune", "-f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}
func (d *backendDocker) ListNetworks(csv bool, writer io.Writer) error {
	out, err := exec.Command("docker", "network", "list", "--format", "{{.Name}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	networks := []string{"network", "inspect"}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.Trim(line, "\r\t\n ")
		if line == "" {
			continue
		}
		networks = append(networks, line)
	}
	out, err = exec.Command("docker", networks...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}

	netlist := []dockerListNetwork{}
	err = json.Unmarshal(out, &netlist)
	if err != nil {
		return err
	}
	if writer == nil {
		writer = os.Stdout
	}
	w := tabwriter.NewWriter(writer, 1, 1, 4, ' ', 0)
	if !csv {
		fmt.Fprintln(w, "NAME\tDRIVER\tSUBNETS\tMTU")
		fmt.Fprintln(w, "----\t------\t-------\t---")
	} else {
		fmt.Fprintln(writer, "name,driver,subnets,mtu")
	}
	for _, net := range netlist {
		subnets := []string{}
		for _, sub := range net.IPAM.Config {
			subnets = append(subnets, sub.Subnet)
		}
		mtuOpt, ok := net.Options["com.docker.network.driver.mtu"]
		if !ok {
			mtuOpt = "default"
		}
		if !csv {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", net.Name, net.Driver, strings.Join(subnets, ","), mtuOpt)
		} else {
			fmt.Fprintf(writer, "%s,%s,%s,%s\n", net.Name, net.Driver, strings.Join(subnets, ","), mtuOpt)
		}
	}
	if !csv {
		w.Flush()
	}
	return nil
}
