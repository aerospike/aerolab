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
	"sync"
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

func (d *backendDocker) GetAZName(subnetId string) (string, error) {
	return "", nil
}

func (d *backendDocker) AttachVolume(name string, zone string, clusterName string, node int) error {
	return nil
}

func (d *backendDocker) TagVolume(fsId string, tagName string, tagValue string, zone string) error {
	return nil
}

func (d *backendDocker) CreateMountTarget(volume *inventoryVolume, subnet string, secGroups []string) (inventoryMountTarget, error) {
	return inventoryMountTarget{}, nil
}

func (d *backendDocker) MountTargetAddSecurityGroup(mountTarget *inventoryMountTarget, volume *inventoryVolume, addGroups []string) error {
	return nil
}

func (d *backendDocker) DetachVolume(name string, clusterName string, node int, zone string) error {
	return nil
}

func (d *backendDocker) ResizeVolume(name string, zone string, newSize int64) error {
	return nil
}

func (d *backendDocker) DeleteVolume(name string, zone string) error {
	return nil
}

func (d *backendDocker) CreateVolume(name string, zone string, tags []string, expires time.Duration, size int64, desc string) error {
	return nil
}

func (d *backendDocker) SetLabel(clusterName string, key string, value string, gcpZone string) error {
	return errors.New("docker does not support changing of container labels")
}

func (d *backendDocker) GetKeyPath(clusterName string) (keyPath string, err error) {
	return "", fmt.Errorf("feature not supported on docker")
}

func (d *backendDocker) EnableServices() error {
	return nil
}

func (d *backendDocker) ExpiriesSystemInstall(intervalMinutes int, deployRegion string) error {
	return nil
}
func (d *backendDocker) ExpiriesSystemRemove(region string) error {
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
		lineWait := new(sync.WaitGroup)
		var lineError error
		lineErrorLock := new(sync.Mutex)
		invLock := new(sync.Mutex)
		for scanner.Scan() {
			t := scanner.Text()
			lineWait.Add(1)
			go func(t string) {
				defer lineWait.Done()
				t = strings.Trim(t, "'\" \t\r\n")
				tt := strings.Split(t, "\t")
				if len(tt) < 4 || len(tt) > 6 {
					return
				}
				if !strings.HasPrefix(tt[1], dockerNameHeader) {
					return
				}
				nameNo := strings.Split(strings.TrimPrefix(tt[1], dockerNameHeader+""), "_")
				if len(nameNo) < 2 {
					return
				}
				nno := nameNo[len(nameNo)-1]
				nname := strings.Join(nameNo[0:len(nameNo)-1], "_")
				nameNo = []string{nname, nno}
				outl, err := exec.Command("docker", "container", "inspect", "--format", "{{json .Config.Labels}}", tt[1]).CombinedOutput()
				if err != nil {
					lineErrorLock.Lock()
					lineError = err
					lineErrorLock.Unlock()
					return
				}
				allLabels := make(map[string]string)
				err = json.Unmarshal(outl, &allLabels)
				if err != nil {
					lineErrorLock.Lock()
					lineError = err
					lineErrorLock.Unlock()
					return
				}
				out2, err := exec.Command("docker", "container", "inspect", "--format", "{{json .Config.ExposedPorts}} {{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", tt[1]).CombinedOutput()
				if err != nil {
					lineErrorLock.Lock()
					lineError = err
					lineErrorLock.Unlock()
					return
				}
				ipport := strings.Split(strings.Trim(string(out2), "'\" \n\r"), " ")
				ip := ""
				exposePort := ""
				intPort := ""
				for _, it := range ipport {
					if strings.HasPrefix(it, "{") {
						nports := make(map[string]interface{})
						err = json.Unmarshal([]byte(it), &nports)
						if err == nil {
							for k := range nports {
								k = strings.Split(k, "/")[0]
								exposePort = k
								intPort = k
							}
						}
					} else if it != "null" && it != "" {
						ip = it
						break
					}
				}
				arch := "amd64"
				if d.isArm {
					arch = "arm64"
				}
				var i1, asdVer string
				var i2 []string
				i3 := []string{""}
				tt[3] = strings.TrimPrefix(tt[3], "localhost/")
				if i == 1 {
					i1 = strings.TrimPrefix(tt[3], "aerolab-")
					i2 = strings.Split(i1, "_")
					if len(i2) > 1 {
						i3 = strings.Split(i2[1], ":")
					}
					i4 := strings.Split(tt[3], ":")
					if len(i4) > 1 {
						asdVer = i4[1]
					}
				} else {
					i2 = strings.Split(tt[3], ":")
					if len(i2) > 1 {
						i3[0] = i2[1]
					}
					ix := strings.Split(i2[0], "/")
					if len(ix) > 1 {
						i2[0] = ix[1]
						arch = ix[0]
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
				if exposePorts == "" {
					exposePorts = exposePort
				}
				if intPorts == "" {
					intPorts = intPort
				}
				if strings.Contains(tt[3], "_amd64") {
					arch = "amd64"
				} else if strings.Contains(tt[3], "_arm64") {
					arch = "arm64"
				}
				invLock.Lock()
				defer invLock.Unlock()
				if i == 1 {
					features, _ := strconv.Atoi(clientType)
					ij.Clusters = append(ij.Clusters, inventoryCluster{
						ClusterName:        nameNo[0],
						NodeNo:             nameNo[1],
						PublicIp:           "",
						PrivateIp:          strings.ReplaceAll(ip, " ", ","),
						InstanceId:         tt[0],
						ImageId:            tt[3],
						State:              strings.ReplaceAll(tt[2], " ", "_"),
						Arch:               arch,
						Distribution:       i2[0],
						OSVersion:          i3[0],
						AerospikeVersion:   asdVer,
						DockerExposePorts:  exposePorts,
						DockerInternalPort: intPorts,
						Features:           FeatureSystem(features),
						AGILabel:           allLabels["agiLabel"],
						dockerLabels:       allLabels,
						Owner:              allLabels["owner"],
					})
				} else {
					ij.Clients = append(ij.Clients, inventoryClient{
						ClientName:         nameNo[0],
						NodeNo:             nameNo[1],
						PublicIp:           "",
						PrivateIp:          strings.ReplaceAll(ip, " ", ","),
						InstanceId:         tt[0],
						ImageId:            tt[3],
						State:              strings.ReplaceAll(tt[2], " ", "_"),
						Arch:               arch,
						Distribution:       i2[0],
						OSVersion:          i3[0],
						AerospikeVersion:   asdVer,
						ClientType:         clientType,
						DockerExposePorts:  exposePorts,
						DockerInternalPort: intPorts,
						dockerLabels:       allLabels,
						Owner:              allLabels["owner"],
					})
				}
			}(t)
		}
		lineWait.Wait()
		if lineError != nil {
			return ij, err
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

func (d *backendDocker) CreateSecurityGroups(vpc string, namePrefix string, isAgi bool) error {
	return nil
}

func (d *backendDocker) ListSecurityGroups() error {
	return nil
}

func (d *backendDocker) ListSubnets() error {
	return nil
}

func (d *backendDocker) LockSecurityGroups(ip string, lockSSH bool, vpc string, namePrefix string, isAgi bool) error {
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
	return d.nodeListInClusterDo(name, false)
}

func (d *backendDocker) nodeListInClusterDo(name string, onlyRunning bool) ([]int, error) {
	var out []byte
	var err error
	if onlyRunning {
		out, err = exec.Command("docker", "container", "list", "--format", "{{json .Names}}").CombinedOutput()
	} else {
		out, err = exec.Command("docker", "container", "list", "-a", "--format", "{{json .Names}}").CombinedOutput()
	}
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
		repo = strings.TrimPrefix(repo, "localhost/")
		if strings.Contains(repo, dockerNameHeader+"") {
			if len(repo) > len(dockerNameHeader)+2 {
				repo = repo[len(dockerNameHeader):]
				distVer := strings.Split(repo, "_")
				if len(distVer) > 1 {
					isArm := d.isArm
					if len(distVer) > 2 {
						if distVer[2] == "arm64" {
							isArm = true
						} else {
							isArm = false
						}
					}
					tagList := strings.Split(t, ";")
					if len(tagList) > 1 {
						tag := strings.Trim(tagList[1], "'\"")
						templateList = append(templateList, backendVersion{distVer[0], distVer[1], tag, isArm})
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
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*10)
	defer ctxCancel()
	out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker command not found, or docker appears to be unreachable or down: %s", string(out))
	}
	switch a.opts.Config.Backend.Arch {
	case "amd64":
		d.isArm = false
	case "arm64":
		d.isArm = true
	default:
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
	arch := "amd64"
	if v.isArm {
		arch = "arm64"
	}
	templName := fmt.Sprintf("aerotmpl-%s-%s-%s-%s", v.distroName, v.distroVersion, v.aerospikeVersion, arch)
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
	arch := "amd64"
	if v.isArm {
		arch = "arm64"
	}
	templName := fmt.Sprintf("aerotmpl-%s-%s-%s-%s", v.distroName, v.distroVersion, v.aerospikeVersion, arch)
	addShutdownHandler("deployTemplate", func(os.Signal) {
		for len(deployTemplateShutdownMaking) > 0 {
			time.Sleep(time.Second)
		}
		exec.Command("docker", "rm", "-f", templName).CombinedOutput()
	})
	defer delShutdownHandler("deployTemplate")
	// deploy container with os
	deployTemplateShutdownMaking <- 1
	out, err := exec.Command("docker", "run", "-td", "--name", templName, d.imageNaming(v)).CombinedOutput()
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
	templImg := fmt.Sprintf(dockerNameHeader+"%s_%s_%s:%s", v.distroName, v.distroVersion, arch, v.aerospikeVersion)
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
	arch := "amd64"
	if v.isArm {
		arch = "arm64"
	}
	name := fmt.Sprintf(dockerNameHeader+"%s_%s_%s:%s", v.distroName, v.distroVersion, arch, v.aerospikeVersion)
	out, err := exec.Command("docker", "image", "list", "--format", "{{json .ID}}", fmt.Sprintf("--filter=reference=%s", name)).CombinedOutput()
	if err != nil || len(strings.Trim(string(out), "\t\r\n ")) == 0 {
		name = fmt.Sprintf(dockerNameHeader+"%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion)
		out, err = exec.Command("docker", "image", "list", "--format", "{{json .ID}}", fmt.Sprintf("--filter=reference=%s", name)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get image list: %s;%s", string(out), err)
		}
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
		arch := "amd64"
		if v.isArm {
			arch = "arm64"
		}
		tmplName := fmt.Sprintf(dockerNameHeader+"%s_%s_%s:%s", v.distroName, v.distroVersion, arch, v.aerospikeVersion)
		if d.client {
			tmplName = d.imageNaming(v)
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
		if extra.limitNoFile > 0 {
			exposeList = append(exposeList, "--ulimit", fmt.Sprintf("nofile=%d:%d", extra.limitNoFile, extra.limitNoFile))
		}
		if extra.privileged {
			fmt.Println("WARNING: privileged container")
			exposeList = append(exposeList, "--device-cgroup-rule=b 7:* rmw", "--privileged=true", "--cap-add=NET_ADMIN", "--cap-add=NET_RAW", "-td", "--name", fmt.Sprintf(dockerNameHeader+"%s_%d", name, node), tmplName, "/bin/bash", "-c", "while true; do [ -f /tmp/poweroff.now ] && rm -f /tmp/poweroff.now && exit; sleep 1; done")
		} else {
			exposeList = append(exposeList, "--cap-add=NET_ADMIN", "--cap-add=NET_RAW", "-td", "--name", fmt.Sprintf(dockerNameHeader+"%s_%d", name, node), tmplName, "/bin/bash", "-c", "while true; do [ -f /tmp/poweroff.now ] && rm -f /tmp/poweroff.now && exit; sleep 1; done")
		}
		out, err = exec.Command("docker", exposeList...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("error running container: %s;%s", out, err)
		}
	}
	return nil
}

func (d *backendDocker) imageNaming(v backendVersion) (templName string) {
	switch v.distroName {
	case "centos":
		switch v.distroVersion {
		case "6":
			return "quay.io/centos/centos:6"
		case "7":
			return "quay.io/centos/centos:7"
		default:
			switch a.opts.Config.Backend.Arch {
			case "amd64":
				return "quay.io/centos/amd64:stream" + v.distroVersion
			case "arm64":
				return "quay.io/centos/arm64v8:stream" + v.distroVersion
			default:
				return "quay.io/centos/centos:stream" + v.distroVersion
			}
		}
	case "ubuntu", "debian":
		switch a.opts.Config.Backend.Arch {
		case "amd64":
			return fmt.Sprintf("amd64/%s:%s", v.distroName, v.distroVersion)
		case "arm64":
			return fmt.Sprintf("arm64v8/%s:%s", v.distroName, v.distroVersion)
		}
		fallthrough
	default:
		return fmt.Sprintf("%s:%s", v.distroName, v.distroVersion)
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

func (d *backendDocker) DisablePricingAPI() {
}

func (d *backendDocker) DisableExpiryInstall() {
}

func (d *backendDocker) CopyFilesToClusterReader(name string, files []fileListReader, nodes []int) error {
	var err error
	if nodes == nil {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	if a.opts.Config.Backend.TmpDir != "" {
		os.MkdirAll(string(a.opts.Config.Backend.TmpDir), 0755)
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
	nodes, err := d.nodeListInClusterDo(name, true)
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

func (d *backendDocker) AttachAndRun(clusterName string, node int, command []string, isInteractive bool) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr, isInteractive)
}

func (d *backendDocker) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, isInteractive bool) (err error) {
	name := fmt.Sprintf(dockerNameHeader+"%s_%d", clusterName, node)
	var cmd *exec.Cmd
	termMode := "-t"
	if isInteractive {
		termMode = "-ti"
	}
	head := []string{"exec", "-e", fmt.Sprintf("NODE=%d", node), termMode, name}
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
	if a.opts.Config.Backend.TmpDir != "" {
		os.MkdirAll(string(a.opts.Config.Backend.TmpDir), 0755)
	}
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
func (d *backendDocker) ClusterListFull(isJson bool, owner string, pager bool, isPretty bool, sort []string, renderer string) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.Pager = pager
	a.opts.Inventory.List.JsonPretty = isPretty
	a.opts.Inventory.List.SortBy = sort
	a.opts.Inventory.List.RenderType = renderer
	return "", a.opts.Inventory.List.run(d.server, d.client, false, false, false)
}

// returns an unformatted string with list of clusters, to be printed to user
func (d *backendDocker) TemplateListFull(isJson bool, pager bool, isPretty bool, sort []string, renderer string) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.Pager = pager
	a.opts.Inventory.List.JsonPretty = isPretty
	a.opts.Inventory.List.SortBy = sort
	a.opts.Inventory.List.RenderType = renderer
	return "", a.opts.Inventory.List.run(false, false, true, false, false)
}
