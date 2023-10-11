package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/bestmethod/inslice"
)

type backendDocker struct {
	server bool
	client bool
	isArm  bool
}

func init() {
	addBackend("docker", &backendDocker{})
}

var dockerNameHeader = "aerolab-"

func (d *backendDocker) EnableServices() error {
	return nil
}

func (d *backendDocker) ExpiriesSystemInstall(intervalMinutes int, deployRegion string) error {
	return nil
}
func (d *backendDocker) ExpiriesSystemRemove() error {
	return nil
}
func (d *backendDocker) ExpiriesSystemFrequency(intervalMinutes int) error {
	return nil
}
func (d *backendDocker) ClusterExpiry(zone string, clusterName string, expiry time.Duration, nodes []int) error {
	return nil
}

func (d *backendDocker) GetInstanceTypes(minCpu int, maxCpu int, minRam float64, maxRam float64, minDisks int, maxDisks int, findArm bool, gcpZone string) ([]instanceType, error) {
	return nil, nil
}

func (d *backendDocker) Inventory(owner string, inventoryItems []int) (inventoryJson, error) {
	ij := inventoryJson{}

	if inslice.HasInt(inventoryItems, InventoryItemTemplates) {
		tmpl, err := d.ListTemplates()
		if err != nil {
			return ij, err
		}
		for _, d := range tmpl {
			arch := "amd64"
			if d.isArm {
				arch = "arm64"
			}
			ij.Templates = append(ij.Templates, inventoryTemplate{
				AerospikeVersion: d.aerospikeVersion,
				Distribution:     d.distroName,
				OSVersion:        d.distroVersion,
				Arch:             arch,
			})
		}
	}

	if inslice.HasInt(inventoryItems, InventoryItemFirewalls) {
		b := new(bytes.Buffer)
		err := d.ListNetworks(true, b)
		if err != nil {
			return ij, err
		}
		for i, line := range strings.Split(b.String(), "\n") {
			if i == 0 {
				continue
			}
			neta := strings.Split(line, ",")
			if len(neta) != 4 {
				continue
			}
			ij.FirewallRules = append(ij.FirewallRules, inventoryFirewallRule{
				Docker: &inventoryFirewallRuleDocker{
					NetworkName:   neta[0],
					NetworkDriver: neta[1],
					Subnets:       neta[2],
					MTU:           neta[3],
				},
			})
		}
	}

	nCheckList := []int{}
	if inslice.HasInt(inventoryItems, InventoryItemClusters) {
		nCheckList = []int{1}
	}
	if inslice.HasInt(inventoryItems, InventoryItemClients) {
		nCheckList = append(nCheckList, 2)
	}
	for _, i := range nCheckList {
		if i == 1 {
			d.WorkOnServers()
		} else {
			d.WorkOnClients()
		}
		out, err := exec.Command("docker", "container", "list", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Image}}\t{{.Label \"aerolab.client.type\"}}\t{{.Ports}}").CombinedOutput()
		if err != nil {
			return ij, err
		}
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			t := scanner.Text()
			t = strings.Trim(t, "'\" \t\r\n")
			tt := strings.Split(t, "\t")
			if len(tt) < 4 || len(tt) > 6 {
				continue
			}
			if !strings.HasPrefix(tt[1], dockerNameHeader) {
				continue
			}
			nameNo := strings.Split(strings.TrimPrefix(tt[1], dockerNameHeader+""), "_")
			if len(nameNo) != 2 {
				continue
			}
			outl, err := exec.Command("docker", "container", "inspect", "--format", "{{json .Config.Labels}}", tt[1]).CombinedOutput()
			if err != nil {
				return ij, err
			}
			allLabels := make(map[string]string)
			err = json.Unmarshal(outl, &allLabels)
			if err != nil {
				return ij, err
			}
			out2, err := exec.Command("docker", "container", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", tt[1]).CombinedOutput()
			if err != nil {
				return ij, err
			}
			ip := strings.Trim(string(out2), "'\" \n\r")
			arch := "amd64"
			if d.isArm {
				arch = "arm64"
			}
			var i1, asdVer string
			var i2 []string
			i3 := []string{""}
			if i == 1 {
				i1 = strings.TrimPrefix(tt[3], "aerolab-")
				i2 = strings.Split(i1, "_")
				if len(i2) > 1 {
					i3 = strings.Split(i2[1], ":")
				}
				if len(i3) > 1 {
					asdVer = i3[1]
				}
			} else {
				i2 = strings.Split(tt[3], ":")
				if len(i2) > 1 {
					i3[0] = i2[1]
				}
			}
			clientType := ""
			if len(tt) > 4 {
				clientType = tt[4]
			}
			exposePorts := ""
			intPorts := ""
			if len(tt) > 5 {
				ep1 := strings.Split(tt[5], "->")
				if len(ep1) > 1 {
					ep2 := strings.Split(ep1[0], ":")
					if len(ep2) > 1 {
						exposePorts = ep2[1]
					}
					ep2 = strings.Split(ep1[1], "/")
					intPorts = ep2[0]
				}
			}
			if i == 1 {
				features, _ := strconv.Atoi(clientType)
				ij.Clusters = append(ij.Clusters, inventoryCluster{
					ClusterName:        nameNo[0],
					NodeNo:             nameNo[1],
					PublicIp:           "",
					PrivateIp:          strings.ReplaceAll(ip, " ", ","),
					InstanceId:         tt[0],
					ImageId:            tt[3],
					State:              tt[2],
					Arch:               arch,
					Distribution:       i2[0],
					OSVersion:          i3[0],
					AerospikeVersion:   asdVer,
					DockerExposePorts:  exposePorts,
					DockerInternalPort: intPorts,
					Features:           FeatureSystem(features),
					AGILabel:           allLabels["agiLabel"],
					dockerLabels:       allLabels,
				})
			} else {
				ij.Clients = append(ij.Clients, inventoryClient{
					ClientName:         nameNo[0],
					NodeNo:             nameNo[1],
					PublicIp:           "",
					PrivateIp:          strings.ReplaceAll(ip, " ", ","),
					InstanceId:         tt[0],
					ImageId:            tt[3],
					State:              tt[2],
					Arch:               arch,
					Distribution:       i2[0],
					OSVersion:          i3[0],
					AerospikeVersion:   asdVer,
					ClientType:         clientType,
					DockerExposePorts:  exposePorts,
					DockerInternalPort: intPorts,
					dockerLabels:       allLabels,
				})
			}
		}
	}
	return ij, nil
}

func (d *backendDocker) IsSystemArm(systemType string) (bool, error) {
	return d.isArm, nil
}

func (d *backendDocker) IsNodeArm(clusterName string, nodeNumber int) (bool, error) {
	return d.isArm, nil
}

func (d *backendDocker) Arch() TypeArch {
	if d.isArm {
		return TypeArchArm
	}
	return TypeArchAmd
}

func (d *backendDocker) AssignSecurityGroups(clusterName string, names []string, vpcOrZone string, remove bool) error {
	return nil
}

func (d *backendDocker) DeleteSecurityGroups(vpc string, namePrefix string, internal bool) error {
	return nil
}

func (d *backendDocker) CreateSecurityGroups(vpc string, namePrefix string) error {
	return nil
}

func (d *backendDocker) ListSecurityGroups() error {
	return nil
}

func (d *backendDocker) ListSubnets() error {
	return nil
}

func (d *backendDocker) LockSecurityGroups(ip string, lockSSH bool, vpc string, namePrefix string) error {
	return nil
}

func (d *backendDocker) ClusterList() ([]string, error) {
	out, err := exec.Command("docker", "container", "list", "-a", "--format", "{{json .Names}}").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var clusterList []string
	clusterList = []string{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		if strings.Contains(t, dockerNameHeader+"") {
			t = t[len(dockerNameHeader):]
			cnametmp := strings.Split(t, "_")
			cname := strings.Join(cnametmp[:len(cnametmp)-1], "_")
			//nodename = cnametmp[len(cnametmp)-1]
			if !inslice.HasString(clusterList, cname) {
				clusterList = append(clusterList, cname)
			}
		}
	}
	return clusterList, nil
}

func (d *backendDocker) NodeListInCluster(name string) ([]int, error) {
	out, err := exec.Command("docker", "container", "list", "-a", "--format", "{{json .Names}}").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var nodeList []int
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		if strings.Contains(t, dockerNameHeader+"") {
			t = t[len(dockerNameHeader):]
			cnametmp := strings.Split(t, "_")
			clusterNode0 := strings.Join(cnametmp[:len(cnametmp)-1], "_")
			clusterNode1 := cnametmp[len(cnametmp)-1]
			if clusterNode0 == name {
				node, err := strconv.Atoi(clusterNode1)
				if err != nil {
					return nil, err
				}
				nodeList = append(nodeList, node)
			}
		}
	}
	return nodeList, nil
}

func (d *backendDocker) ListTemplates() ([]backendVersion, error) {
	out, err := exec.Command("docker", "image", "list", "-a", "--format", "{{json .Repository}};{{.Tag}}").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var templateList []backendVersion
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		repo := strings.Trim(strings.Split(t, ";")[0], "'\"")
		if strings.Contains(repo, dockerNameHeader+"") {
			if len(repo) > len(dockerNameHeader)+2 {
				repo = repo[len(dockerNameHeader):]
				distVer := strings.Split(repo, "_")
				if len(distVer) == 2 {
					tagList := strings.Split(t, ";")
					if len(tagList) > 1 {
						tag := strings.Trim(tagList[1], "'\"")
						templateList = append(templateList, backendVersion{distVer[0], distVer[1], tag, d.isArm})
					}
				}
			}
		}
	}
	return templateList, nil
}

func (d *backendDocker) WorkOnClients() {
	d.server = false
	d.client = true
	dockerNameHeader = "aerolab_c-"
}

func (d *backendDocker) WorkOnServers() {
	d.server = true
	d.client = false
	dockerNameHeader = "aerolab-"
}

func (d *backendDocker) Init() error {
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer ctxCancel()
	out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker command not found, or docker appears to be unreachable or down: %s", string(out))
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.Trim(line, "\r\n\t ")
		if !strings.HasPrefix(line, "Architecture: ") {
			continue
		}
		arch := strings.Split(line, ": ")[1]
		if strings.Contains(arch, "arm") || strings.Contains(arch, "aarch") {
			d.isArm = true
		}
		break
	}
	d.WorkOnServers()
	return nil
}

func (d *backendDocker) versionToReal(v *backendVersion) error {
	switch v.distroName {
	case "ubuntu", "centos", "debian":
	case "amazon":
		v.distroName = "centos"
		v.distroVersion = "7"
	default:
		return errors.New("unsupported distro")
	}
	return nil
}

func (d *backendDocker) VacuumTemplates() error {
	out, err := exec.Command("docker", "container", "list", "-a").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker command failed: %s", err)
	}
	s := bufio.NewScanner(strings.NewReader(string(out)))
	ids := []string{}
	for s.Scan() {
		line := s.Text()
		if strings.Contains(line, " aerotmpl-") || strings.Contains(line, "\taerotmpl-") {
			id := strings.Split(strings.Trim(line, "\t\r\n "), " ")[0]
			id = strings.Split(id, "\t")[0]
			ids = append(ids, id)
		}
	}
	errs := ""
	for _, id := range ids {
		exec.Command("docker", "stop", "-t", "1", id).CombinedOutput()
		out, err := exec.Command("docker", "rm", "-f", id).CombinedOutput()
		if err != nil {
			errs = errs + err.Error() + "\n" + string(out) + "\n"
		}
	}
	if errs == "" {
		return nil
	}
	return errors.New(errs)
}

func (d *backendDocker) VacuumTemplate(v backendVersion) error {
	if err := d.versionToReal(&v); err != nil {
		return err
	}
	templName := fmt.Sprintf("aerotmpl-%s-%s-%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	out, err := exec.Command("docker", "stop", "-t", "1", templName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not stop temporary template container: %s;%s", out, err)
	}
	out, err = exec.Command("docker", "rm", "-f", templName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not destroy temporary template container: %s;%s", out, err)
	}
	return nil
}

var deployTemplateShutdownMaking = make(chan int, 1)

func (d *backendDocker) DeployTemplate(v backendVersion, script string, files []fileListReader, extra *backendExtra) error {
	if err := d.versionToReal(&v); err != nil {
		return err
	}
	templName := fmt.Sprintf("aerotmpl-%s-%s-%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	addShutdownHandler("deployTemplate", func(os.Signal) {
		for len(deployTemplateShutdownMaking) > 0 {
			time.Sleep(time.Second)
		}
		exec.Command("docker", "rm", "-f", templName).CombinedOutput()
	})
	defer delShutdownHandler("deployTemplate")
	// deploy container with os
	deployTemplateShutdownMaking <- 1
	out, err := exec.Command("docker", "run", "-td", "--name", templName, d.centosNaming(v)).CombinedOutput()
	<-deployTemplateShutdownMaking
	if err != nil {
		return fmt.Errorf("could not start vanilla container: %s;%s", out, err)
	}
	// copy add script to files list
	files = append(files, fileListReader{"/root/install.sh", strings.NewReader(script), len(script)})
	// copy all files to container
	err = d.copyFilesToContainer(templName, files)
	if err != nil {
		return fmt.Errorf("could not copy files to container: %s", err)
	}
	// run script
	out, err = exec.Command("docker", "exec", "-t", templName, "chmod", "755", "/root/install.sh").CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not chmod 755 /root/install.sh: %s;%s", out, err)
	}
	out, err = exec.Command("docker", "exec", "-t", templName, "/bin/bash", "-c", "/root/install.sh").CombinedOutput()
	if err != nil {
		return fmt.Errorf("script /root/install.sh failed with: %s;%s", out, err)
	}
	// stop container
	out, err = exec.Command("docker", "stop", templName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed stopping container: %s;%s", out, err)
	}
	// docker container commit container_name dist_ver:aeroVer
	templImg := fmt.Sprintf(dockerNameHeader+"%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	out, err = exec.Command("docker", "container", "commit", templName, templImg).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to commit container to image: %s;%s", out, err)
	}
	// docker rm container_name
	out, err = exec.Command("docker", "rm", templName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove temporary container: %s;%s", out, err)
	}
	return nil
}

func (d *backendDocker) TemplateDestroy(v backendVersion) error {
	if v.distroName == "el" {
		v.distroName = "centos"
	}
	name := fmt.Sprintf(dockerNameHeader+"%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	out, err := exec.Command("docker", "image", "list", "--format", "{{json .ID}}", fmt.Sprintf("--filter=reference=%s", name)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get image list: %s;%s", string(out), err)
	}
	imageId := strings.Trim(string(out), "\"' \n\r")
	out, err = exec.Command("docker", "rmi", imageId).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to rmi '%s': %s;%s", imageId, string(out), err)
	}
	return nil
}

func (d *backendDocker) DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error {
	name = strings.Trim(name, "\r\n\t ")
	if extra.network != "" {
		b := new(bytes.Buffer)
		err := d.ListNetworks(true, b)
		if err != nil {
			return err
		}
		found := false
		for i, line := range strings.Split(b.String(), "\n") {
			if i == 0 {
				continue
			}
			netName := strings.Split(line, ",")[0]
			if netName == extra.network {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Network %s not found! Create (y/n)? ", extra.network)
			reader := bufio.NewReader(os.Stdin)
			answer := ""
			for strings.ToLower(answer) != "y" && strings.ToLower(answer) != "n" && strings.ToLower(answer) != "yes" && strings.ToLower(answer) != "no" {
				answer, _ = reader.ReadString('\n')
				answer = strings.Trim(answer, "\t\r\n ")
				if strings.ToLower(answer) != "y" && strings.ToLower(answer) != "n" && strings.ToLower(answer) != "yes" && strings.ToLower(answer) != "no" {
					fmt.Println("Invalid input: answer either 'y' or 'n'")
					fmt.Printf("Network %s not found! Create (y/n)? ", extra.network)
				}
			}
			if strings.HasPrefix(answer, "n") {
				return fmt.Errorf("network not found, choose another network or create one first with: aerolab config docker help")
			}
			ok := false
			for !ok {
				fmt.Printf("Subnet (empty=default): ")
				subnet, _ := reader.ReadString('\n')
				subnet = strings.Trim(subnet, "\t\r\n ")
				fmt.Printf("Driver (empty=default): ")
				driver, _ := reader.ReadString('\n')
				driver = strings.Trim(driver, "\t\r\n ")
				fmt.Printf("MTU (empty=default): ")
				mtu, _ := reader.ReadString('\n')
				mtu = strings.Trim(mtu, "\t\r\n ")
				fmt.Printf("OK (y/n/q)? ")
				answer := ""
				for strings.ToLower(answer) != "y" && strings.ToLower(answer) != "n" && strings.ToLower(answer) != "yes" && strings.ToLower(answer) != "no" && strings.ToLower(answer) != "q" && strings.ToLower(answer) != "quit" {
					answer, _ = reader.ReadString('\n')
					answer = strings.Trim(answer, "\t\r\n ")
					if strings.ToLower(answer) != "y" && strings.ToLower(answer) != "n" && strings.ToLower(answer) != "yes" && strings.ToLower(answer) != "no" && strings.ToLower(answer) != "q" && strings.ToLower(answer) != "quit" {
						fmt.Println("Invalid input: answer either 'y' or 'n'")
						fmt.Printf("OK (y/n/q)? ")
					}
				}
				if strings.HasPrefix(answer, "q") {
					return fmt.Errorf("network not found, choose another network or create one first with: aerolab config docker help")
				}
				if strings.HasPrefix(answer, "y") {
					if driver == "" {
						driver = "bridge"
					}
					err = d.CreateNetwork(extra.network, driver, subnet, mtu)
					if err != nil {
						return err
					}
					ok = true
				}
			}
		}
	}
	if err := d.versionToReal(&v); err != nil {
		return err
	}
	if !d.client {
		templ, err := d.ListTemplates()
		if err != nil {
			return err
		}
		inArray, err := inslice.Reflect(templ, v, 1)
		if err != nil {
			return err
		}
		if len(inArray) == 0 {
			return errors.New("template not found")
		}
	}
	list, err := d.ClusterList()
	if err != nil {
		return err
	}
	var nodeList []int
	var highestNode int
	if inslice.HasString(list, name) {
		nodeList, err = d.NodeListInCluster(name)
		if err != nil {
			return err
		}
		for _, i := range nodeList {
			if i > highestNode {
				highestNode = i
			}
		}
	} else {
		highestNode = 0
	}
	highestNode = highestNode + 1

	var exposedList []int
	if extra.autoExpose {
		abc := d.server
		abd := d.client
		abe := dockerNameHeader
		invJson, err := b.Inventory("", []int{InventoryItemClusters, InventoryItemClients})
		d.server = abc
		d.client = abd
		dockerNameHeader = abe
		if err != nil {
			return err
		}
		for _, item := range invJson.Clusters {
			p, _ := strconv.Atoi(item.DockerExposePorts)
			if p > 0 {
				exposedList = append(exposedList, p)
			}
		}
		for _, item := range invJson.Clients {
			p, _ := strconv.Atoi(item.DockerExposePorts)
			if p > 0 {
				exposedList = append(exposedList, p)
			}
		}
	}
	var exposeFreeList []int
	for i := 3100; i < 3500; i++ {
		if !inslice.HasInt(exposedList, i) {
			exposeFreeList = append(exposeFreeList, i)
		}
	}
	exposeFreeListNext := -1
	for node := highestNode; node < nodeCount+highestNode; node = node + 1 {
		exposeFreeListNext++
		var out []byte
		exposeList := []string{"run"}
		if extra.clientType != "" {
			exposeList = append(exposeList, "--label", "aerolab.client.type="+extra.clientType)
		}
		for _, newlabel := range extra.labels {
			exposeList = append(exposeList, "--label", newlabel)
		}
		tmplName := fmt.Sprintf(dockerNameHeader+"%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion)
		if d.client {
			tmplName = d.centosNaming(v)
		}
		if extra.dockerHostname {
			exposeList = append(exposeList, "--hostname", name+"-"+strconv.Itoa(node))
		}
		if len(extra.switches) > 0 {
			exposeList = append(exposeList, extra.switches...)
		}
		if extra.autoExpose {
			nPort := strconv.Itoa(exposeFreeList[exposeFreeListNext])
			exposeList = append(exposeList, "-p", nPort+":"+nPort)
		}
		for _, ep := range extra.exposePorts {
			exposeList = append(exposeList, "-p", ep)
		}
		if extra.cpuLimit != "" {
			exposeList = append(exposeList, fmt.Sprintf("--cpus=%s", extra.cpuLimit))
		}
		if extra.ramLimit != "" {
			exposeList = append(exposeList, "-m", extra.ramLimit)
		}
		if extra.swapLimit != "" {
			exposeList = append(exposeList, "--memory-swap", extra.swapLimit)
		}
		if extra.network != "" {
			exposeList = append(exposeList, "--network", extra.network)
		}
		if extra.privileged {
			fmt.Println("WARNING: privileged container")
			exposeList = append(exposeList, "--device-cgroup-rule=b 7:* rmw", "--privileged=true", "--cap-add=NET_ADMIN", "--cap-add=NET_RAW", "-td", "--name", fmt.Sprintf(dockerNameHeader+"%s_%d", name, node), tmplName)
		} else {
			exposeList = append(exposeList, "--cap-add=NET_ADMIN", "--cap-add=NET_RAW", "-td", "--name", fmt.Sprintf(dockerNameHeader+"%s_%d", name, node), tmplName)
		}
		out, err = exec.Command("docker", exposeList...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("error running container: %s;%s", out, err)
		}
	}
	return nil
}

func (d *backendDocker) centosNaming(v backendVersion) (templName string) {
	if v.distroName != "centos" {
		return fmt.Sprintf("%s:%s", v.distroName, v.distroVersion)
	}
	switch v.distroVersion {
	case "6":
		return "quay.io/centos/centos:6"
	case "7":
		return "quay.io/centos/centos:7"
	default:
		return "quay.io/centos/centos:stream" + v.distroVersion
	}
}

func (d *backendDocker) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
	fr := []fileListReader{}
	for _, f := range files {
		fr = append(fr, fileListReader{
			filePath:     f.filePath,
			fileSize:     f.fileSize,
			fileContents: strings.NewReader(f.fileContents),
		})
	}
	return d.CopyFilesToClusterReader(name, fr, nodes)
}

func (d *backendDocker) CopyFilesToClusterReader(name string, files []fileListReader, nodes []int) error {
	var err error
	if nodes == nil {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, file := range files {
		var tmpfile *os.File
		var tmpfileName string
		tmpfile, err = os.CreateTemp(string(a.opts.Config.Backend.TmpDir), "aerolab-tmp")
		if err != nil {
			return err
		}
		tmpfileName = tmpfile.Name()
		_, err = io.Copy(tmpfile, file.fileContents)
		if err != nil {
			return fmt.Errorf("error making tmpfile: %s", err)
		}
		err = tmpfile.Close()
		if err != nil {
			return fmt.Errorf("error closing tmpfile: %s", err)
		}
		for _, node := range nodes {
			nodeName := fmt.Sprintf(dockerNameHeader+"%s_%d", name, node)
			var out []byte
			out, err = exec.Command("docker", "cp", tmpfileName, fmt.Sprintf("%s:%s", nodeName, file.filePath)).CombinedOutput()
			if err != nil {
				return fmt.Errorf("error with docker cp: %s;%s\ntmpfileName: %s\nfilePath: %s", string(out), err, tmpfileName, fmt.Sprintf("%s:%s", nodeName, file.filePath))
			}
		}
		err = os.Remove(tmpfileName)
		if err != nil {
			return fmt.Errorf("error removing tmpfile: %s", err)
		}
	}
	return err
}

func (d *backendDocker) RunCommands(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
	var fout [][]byte
	var err error
	if nodes == nil {
		nodes, err = d.NodeListInCluster(clusterName)
		if err != nil {
			return nil, err
		}
	}
	for _, node := range nodes {
		name := fmt.Sprintf(dockerNameHeader+"%s_%d", clusterName, node)
		var out []byte
		var err error
		for _, command := range commands {
			head := []string{"exec", "-e", fmt.Sprintf("NODE=%d", node), name}
			command = append(head, command...)
			out, err = exec.Command("docker", command...).CombinedOutput()
			fout = append(fout, out)
			if checkExecRetcode(err) != 0 {
				return fout, fmt.Errorf("error running %s: %s", command, err)
			}
		}
	}
	return fout, nil
}

func (d *backendDocker) GetClusterNodeIps(name string) ([]string, error) {
	clusters, err := d.ClusterList()
	if err != nil {
		return nil, err
	}
	if !inslice.HasString(clusters, name) {
		return nil, errors.New("cluster not found")
	}
	nodes, err := d.NodeListInCluster(name)
	if err != nil {
		return nil, err
	}
	ips := []string{}
	var out []byte
	for _, node := range nodes {
		containerName := fmt.Sprintf(dockerNameHeader+"%s_%d", name, node)
		out, err = exec.Command("docker", "container", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", containerName).CombinedOutput()
		if err != nil {
			return nil, err
		}
		ip := strings.Trim(string(out), "'\" \n\r")
		if ip != "" {
			ips = append(ips, strings.Split(ip, " ")[0])
		}
	}
	return ips, nil
}

func (d *backendDocker) GetNodeIpMap(name string, internalIPs bool) (map[int]string, error) {
	if internalIPs {
		return nil, nil
	}
	clusters, err := d.ClusterList()
	if err != nil {
		return nil, err
	}
	if !inslice.HasString(clusters, name) {
		return nil, errors.New("cluster not found")
	}
	nodes, err := d.NodeListInCluster(name)
	if err != nil {
		return nil, err
	}
	ips := make(map[int]string)
	var out []byte
	for _, node := range nodes {
		containerName := fmt.Sprintf(dockerNameHeader+"%s_%d", name, node)
		out, err = exec.Command("docker", "container", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", containerName).CombinedOutput()
		if err != nil {
			return nil, err
		}
		ip := strings.Trim(string(out), "'\" \n\r")
		if ip != "" {
			ips[node] = strings.Split(ip, " ")[0]
		}
	}
	return ips, nil
}

func (d *backendDocker) ClusterStart(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, node := range nodes {
		var out []byte
		name := fmt.Sprintf(dockerNameHeader+"%s_%d", name, node)
		out, err = exec.Command("docker", "start", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s;%s", string(out), err)
		}
	}
	return nil
}

func (d *backendDocker) ClusterStop(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, node := range nodes {
		var out []byte
		name := fmt.Sprintf(dockerNameHeader+"%s_%d", name, node)
		out, err = exec.Command("docker", "stop", "-t", "1", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s;%s", string(out), err)
		}
	}
	return nil
}

func (d *backendDocker) ClusterDestroy(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, node := range nodes {
		var out []byte
		name := fmt.Sprintf(dockerNameHeader+"%s_%d", name, node)
		out, err = exec.Command("docker", "rm", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s;%s", string(out), err)
		}
	}
	return nil
}

func (d *backendDocker) Upload(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
	name := fmt.Sprintf(dockerNameHeader+"%s_%d", clusterName, node)
	cmd := []string{"cp", source, name + ":" + destination}
	out, err := exec.Command("docker", cmd...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.ReplaceAll(string(out), "\n", "; "))
	}
	return nil
}

func (d *backendDocker) Download(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
	name := fmt.Sprintf(dockerNameHeader+"%s_%d", clusterName, node)
	cmd := []string{"cp", name + ":" + source, destination}
	out, err := exec.Command("docker", cmd...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.ReplaceAll(string(out), "\n", "; "))
	}
	return nil
}

func (d *backendDocker) AttachAndRun(clusterName string, node int, command []string) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr)
}

func (d *backendDocker) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (err error) {
	name := fmt.Sprintf(dockerNameHeader+"%s_%d", clusterName, node)
	var cmd *exec.Cmd
	head := []string{"exec", "-e", fmt.Sprintf("NODE=%d", node), "-ti", name}
	if len(command) == 0 {
		command = append(head, "/bin/bash")
	} else {
		command = append(head, command...)
	}
	cmd = exec.Command("docker", command...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	return err
}

func (d *backendDocker) copyFilesToContainer(name string, files []fileListReader) error {
	var err error
	for _, file := range files {
		var tmpfile *os.File
		var tmpfileName string
		tmpfile, err = os.CreateTemp(string(a.opts.Config.Backend.TmpDir), dockerNameHeader+"tmp")
		if err != nil {
			return err
		}
		tmpfileName = tmpfile.Name()
		_, err = io.Copy(tmpfile, file.fileContents)
		if err != nil {
			return fmt.Errorf("error making tmpfile: %s", err)
		}
		err = tmpfile.Close()
		if err != nil {
			return fmt.Errorf("error closing tmpfile: %s", err)
		}
		nodeName := name
		var out []byte
		out, err = exec.Command("docker", "cp", tmpfileName, fmt.Sprintf("%s:%s", nodeName, file.filePath)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("error with docker cp: %s;%s", string(out), err)
		}
		err = os.Remove(tmpfileName)
		if err != nil {
			return fmt.Errorf("error deleting tmpfile: %s", err)
		}
	}
	return err
}

// returns an unformatted string with list of clusters, to be printed to user
func (d *backendDocker) ClusterListFull(isJson bool, owner string, noPager bool) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.NoPager = noPager
	return "", a.opts.Inventory.List.run(d.server, d.client, false, false, false)
}

// returns an unformatted string with list of clusters, to be printed to user
func (d *backendDocker) TemplateListFull(isJson bool, noPager bool) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.NoPager = noPager
	return "", a.opts.Inventory.List.run(false, false, true, false, false)
}
