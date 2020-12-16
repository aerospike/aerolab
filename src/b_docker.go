package main

/*
REPOSITORY          TAG                 IMAGE ID            CREATED             SIZE
hello-world         latest              4ab4c602aa5e        7 weeks ago         1.84kB

CONTAINER ID        IMAGE               COMMAND             CREATED             STATUS                      PORTS               NAMES
29ee0e81594f        hello-world         "/hello"            39 seconds ago      Exited (0) 38 seconds ago                       jolly_shirley

image listing (images are templates):
repository: aero-ubuntu_1804
tag: aerospike version (4.3.0.2)

container listing (containers are cluster):
name: cluster name and node number (aero-NAME_NO)

docker image list -a --format '{{json .ID}};{{json .Repository}};{{.Tag}}'
"4ab4c602aa5e";"hello-world";latest

docker container list -a --format '{{json .ID}};{{json .Names}}'
"de028183a458";"vigilant_saha"
*/

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type b_docker struct {
	host   string
	user   string
	pubkey string
	btrfs  bool
}

func splitClusterName(name string) (cname string, nodename string) {
	cnametmp := strings.Split(name, "_")
	cname = strings.Join(cnametmp[:len(cnametmp)-1], "_")
	nodename = cnametmp[len(cnametmp)-1]
	return
}

// return slice of strings holding cluster names, or error
func (b b_docker) ClusterList() ([]string, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("docker", "container", "list", "-a", "--format", "{{json .Names}}").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "docker container list -a --format '{{json .Names}}'")
	}
	if err != nil {
		return nil, err
	}
	var clusterList []string
	clusterList = []string{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		if strings.Contains(t, "aero-") {
			t = t[5:]
			cname, _ := splitClusterName(t)
			if inArray(clusterList, cname) == -1 {
				clusterList = append(clusterList, cname)
			}
		}
	}
	return clusterList, nil
}

func (b b_docker) GetBackendName() string {
	return "docker"
}

// accept cluster name, return slice of int holding node numbers or error
func (b b_docker) NodeListInCluster(name string) ([]int, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("docker", "container", "list", "-a", "--format", "{{json .Names}}").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "docker container list -a --format '{{json .Names}}'")
	}
	if err != nil {
		return nil, err
	}
	var nodeList []int
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		if strings.Contains(t, "aero-") {
			t = t[5:]
			clusterNode0, clusterNode1 := splitClusterName(t)
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

// return a slice of 'version' structs containing versions of templates available
func (b b_docker) ListTemplates() ([]version, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("docker", "image", "list", "-a", "--format", "{{json .Repository}};{{.Tag}}").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "docker image list -a --format '{{json .Repository}};{{.Tag}}'")
	}
	if err != nil {
		return nil, err
	}
	var templateList []version
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		repo := strings.Trim(strings.Split(t, ";")[0], "'\"")
		if strings.Contains(repo, "aero-") {
			if len(repo) > 7 {
				repo = repo[5:]
				distVer := strings.Split(repo, "_")
				if len(distVer) == 2 {
					tagList := strings.Split(t, ";")
					if len(tagList) > 1 {
						tag := strings.Trim(tagList[1], "'\"")
						templateList = append(templateList, version{distVer[0], distVer[1], tag})
					}
				}
			}
		}
	}
	return templateList, nil
}

// deploy a template, naming it with version, running 'script' inside for installation and copying 'files' into it
func (b b_docker) DeployTemplate(v version, script string, files []fileList) error {
	if v.distroName == "el" {
		v.distroName = "centos"
	}
	templName := fmt.Sprintf("aerotmpl-%s-%s-%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	// 1.deploy container with os
	//fmt.Printf("Step 1: ")
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("docker", "run", "-td", "--name", templName, fmt.Sprintf("%s:%s", v.distroName, v.distroVersion)).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker run -td --name %s %s:%s", templName, v.distroName, v.distroVersion))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Error on step 1: %s;%s", out, err))
	}
	// 2.copy add script to files list
	//fmt.Printf("Step 2: ")
	files = append(files, fileList{"/root/install.sh", []byte(script)})
	// 2.1.copy all files to container
	//fmt.Printf("Step 2.1: ")
	err = b.copyFilesToContainer(templName, files)
	if err != nil {
		return errors.New(fmt.Sprintf("Error on step 2: %s", err))
	}
	// 3.run script
	//fmt.Printf("Step 3.1: ")
	if b.host == "" {
		out, err = exec.Command("docker", "exec", "-t", templName, "chmod", "755", "/root/install.sh").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker exec -t %s chmod 755 /root/install.sh", templName))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Error on step 3.1: %s;%s", out, err))
	}
	//fmt.Printf("Step 3.2: ")
	if b.host == "" {
		out, err = exec.Command("docker", "exec", "-t", templName, "/bin/bash", "-c", "/root/install.sh").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker exec -t %s /bin/bash -c /root/install.sh", templName))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Error on step 3.2: %s;%s", out, err))
	}
	// 4.stop container
	//fmt.Printf("Step 4: ")
	if b.host == "" {
		out, err = exec.Command("docker", "stop", templName).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker stop %s", templName))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Error on step 4: %s;%s", out, err))
	}
	// 5.docker container commit container_name dist_ver:aeroVer
	//fmt.Printf("Step 5: ")
	templImg := fmt.Sprintf("aero-%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	if b.host == "" {
		out, err = exec.Command("docker", "container", "commit", templName, templImg).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker container commit %s %s", templName, templImg))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Error on step 5: %s;%s", out, err))
	}
	// 6.docker rm container_name
	//fmt.Printf("Step 6: ")
	if b.host == "" {
		out, err = exec.Command("docker", "rm", templName).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker rm %s", templName))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Error on step 6: %s;%s", out, err))
	}
	return nil
}

// destroy template for a given version
func (b b_docker) TemplateDestroy(v version) error {
	if v.distroName == "el" {
		v.distroName = "centos"
	}
	var err error
	var out []byte
	name := fmt.Sprintf("aero-%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	if b.host == "" {
		out, err = exec.Command("docker", "image", "list", "--format", "{{json .ID}}", fmt.Sprintf("--filter=reference=%s", name)).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker image list --format '{{json .ID}}' --filter=reference='%s'", name))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Failed to get image list: %s;%s", string(out), err))
	}
	imageId := strings.Trim(string(out), "\"' \n\r")
	if b.host == "" {
		out, err = exec.Command("docker", "rmi", imageId).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker rmi %s", imageId))
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Failed to rmi '%s': %s;%s", imageId, string(out), err))
	}
	return nil
}

// deploy cluster from template, requires version, name of new cluster and node count to deploy
func (b b_docker) DeployCluster(v version, name string, nodeCount int, exposePorts []string) error {
	return b.DeployClusterWithLimits(v, name, nodeCount, exposePorts, "", "", "", false)
}

func (b b_docker) DeployClusterWithLimits(v version, name string, nodeCount int, exposePorts []string, cpuLimit string, ramLimit string, swapLimit string, privileged bool) error {
	if v.distroName == "el" {
		v.distroName = "centos"
	}
	templ, err := b.ListTemplates()
	if err != nil {
		return err
	}
	if inArray(templ, v) == -1 {
		return errors.New("Template not found")
	}
	list, err := b.ClusterList()
	if err != nil {
		return err
	}
	var nodeList []int
	var highestNode int
	if inArray(list, name) != -1 {
		nodeList, err = b.NodeListInCluster(name)
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

	for node := highestNode; node < nodeCount+highestNode; node = node + 1 {
		var out []byte
		if b.host == "" {
			exposeList := []string{"run"}
			for _, ep := range exposePorts {
				exposeList = append(exposeList, "-p", ep)
			}
			if cpuLimit != "" {
				exposeList = append(exposeList, fmt.Sprintf("--cpus=%s", cpuLimit))
			}
			if ramLimit != "" {
				exposeList = append(exposeList, "-m", ramLimit)
			}
			if swapLimit != "" {
				exposeList = append(exposeList, "--memory-swap", swapLimit)
			}
			if privileged == true {
				fmt.Println("WARNING: privileged container")
				exposeList = append(exposeList, "--device-cgroup-rule=b 7:* rmw", "--privileged=true", "--cap-add=NET_ADMIN", "--cap-add=NET_RAW", "-td", "--name", fmt.Sprintf("aero-%s_%d", name, node), fmt.Sprintf("aero-%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion))
			} else {
				exposeList = append(exposeList, "--cap-add=NET_ADMIN", "--cap-add=NET_RAW", "-td", "--name", fmt.Sprintf("aero-%s_%d", name, node), fmt.Sprintf("aero-%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion))
			}
			out, err = exec.Command("docker", exposeList...).CombinedOutput()
		} else {
			var exposeList string
			for _, ep := range exposePorts {
				exposeList = exposeList + " -p " + ep
			}
			if cpuLimit != "" {
				exposeList = exposeList + fmt.Sprintf(" --cpus=%s", cpuLimit)
			}
			if ramLimit != "" {
				exposeList = exposeList + " -m " + ramLimit
			}
			if swapLimit != "" {
				exposeList = exposeList + " --memory-swap " + swapLimit
			}
			if privileged == true {
				out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker run%s --device-cgroup-rule='b 7:* rmw' --privileged=true --cap-add=NET_ADMIN --cap-add=NET_RAW -td --name aero-%s_%d aero-%s_%s:%s", exposeList, name, node, v.distroName, v.distroVersion, v.aerospikeVersion))
			} else {
				out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker run%s --cap-add=NET_ADMIN --cap-add=NET_RAW -td --name aero-%s_%d aero-%s_%s:%s", exposeList, name, node, v.distroName, v.distroVersion, v.aerospikeVersion))
			}
		}
		if err != nil {
			return errors.New(fmt.Sprintf("Error running container: %s;%s", out, err))
		}
	}
	return nil
}

// copy files to cluster, requires cluster name, list of files to copy and list of nodes in cluster to copy to
func (b b_docker) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
	var err error
	if nodes == nil {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, file := range files {
		var tmpfile *os.File
		var tmpfileName string
		var fpath string
		if b.host == "" {
			tmpfile, err = ioutil.TempFile("", "aero-lab-tmp")
			if err != nil {
				return err
			}
			tmpfileName = tmpfile.Name()
			_, err = tmpfile.Write(file.fileContents)
			if err != nil {
				return errors.New(fmt.Sprintf("Error making tmpfile: %s", err))
			}
			err = tmpfile.Close()
			if err != nil {
				return errors.New(fmt.Sprintf("Error closing tmpfile: %s", err))
			}
		} else {
			now := time.Now().UnixNano()
			fpath = fmt.Sprintf("/tmp/aero-lab-tmp-%d", now)
			err = scp(b.user, b.host, b.pubkey, []fileList{fileList{fpath, file.fileContents}})
		}
		for _, node := range nodes {
			nodeName := fmt.Sprintf("aero-%s_%d", name, node)
			if b.host == "" {
				var out []byte
				out, err = exec.Command("docker", "cp", tmpfileName, fmt.Sprintf("%s:%s", nodeName, file.filePath)).CombinedOutput()
				if err != nil {
					return errors.New(fmt.Sprintf("Error with docker cp: %s;%s\ntmpfileName: %s\nfilePath: %s", string(out), err, tmpfileName, fmt.Sprintf("%s:%s", nodeName, file.filePath)))
				}
			} else {
				var out []byte
				out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker cp %s %s:%s", fpath, nodeName, file.filePath))
				if err != nil {
					return errors.New(fmt.Sprintf("Error with docker cp: %s;%s", string(out), err))
				}
			}
		}
		if b.host == "" {
			err = os.Remove(tmpfileName)
			if err != nil {
				return errors.New(fmt.Sprintf("Error removing tmpfile: %s", err))
			}
		} else {
			_, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("rm %s", fpath))
			if err != nil {
				return errors.New(fmt.Sprintf("Error remote rm tmpfile: %s", err))
			}
		}
	}
	return err
}

// run command(s) inside node(s) in cluster. Requires cluster name, commands as slice of command slices, and nodes list slice
// returns a slice of byte slices containing each node/command output and error
func (b b_docker) RunCommand(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
	var fout [][]byte
	var err error
	if nodes == nil {
		nodes, err = b.NodeListInCluster(clusterName)
		if err != nil {
			return nil, err
		}
	}
	for _, node := range nodes {
		name := fmt.Sprintf("aero-%s_%d", clusterName, node)
		var out []byte
		var err error
		for _, command := range commands {
			if b.host == "" {
				head := []string{"exec", name}
				command = append(head, command...)
				out, err = exec.Command("docker", command...).CombinedOutput()
			} else {
				out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker exec -t %s %s", name, strings.Join(command, " ")))
			}
			fout = append(fout, out)
			if check_exec_retcode(err) != 0 {
				return fout, errors.New(fmt.Sprintf("ERROR running %s: %s\n", command, err))
			}
		}
	}
	return fout, nil
}

// returns a string slice containing IPs of given cluster name
func (b b_docker) GetClusterNodeIps(name string) ([]string, error) {
	clusters, err := b.ClusterList()
	if err != nil {
		return nil, err
	}
	if inArray(clusters, name) == -1 {
		return nil, errors.New("Cluster not found")
	}
	nodes, err := b.NodeListInCluster(name)
	if err != nil {
		return nil, err
	}
	ips := []string{}
	var out []byte
	for _, node := range nodes {
		containerName := fmt.Sprintf("aero-%s_%d", name, node)
		if b.host == "" {
			out, err = exec.Command("docker", "container", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", containerName).CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker container inspect --format='{{.NetworkSettings.IPAddress}}' %s", containerName))
		}
		if err != nil {
			return nil, err
		}
		ip := strings.Trim(string(out), "'\" \n\r")
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

// returns a map of [int]string for a given cluster, where int is node number and string is the IP of said node
func (b b_docker) GetNodeIpMap(name string) (map[int]string, error) {
	clusters, err := b.ClusterList()
	if err != nil {
		return nil, err
	}
	if inArray(clusters, name) == -1 {
		return nil, errors.New("Cluster not found")
	}
	nodes, err := b.NodeListInCluster(name)
	if err != nil {
		return nil, err
	}
	ips := make(map[int]string)
	var out []byte
	for _, node := range nodes {
		containerName := fmt.Sprintf("aero-%s_%d", name, node)
		if b.host == "" {
			out, err = exec.Command("docker", "container", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", containerName).CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker container inspect --format='{{.NetworkSettings.IPAddress}}' %s", containerName))
		}
		if err != nil {
			return nil, err
		}
		ip := strings.Trim(string(out), "'\" \n\r")
		if ip != "" {
			ips[node] = ip
		}
	}
	return ips, nil
}

// used by backend to configure itself for remote access. e.g. store b.user, b.host, b.pubkey from given parameters
func (b b_docker) ConfigRemote(host string, pubkey string) backend {
	b.user = strings.Split(host, "@")[0]
	b.host = strings.Split(host, "@")[1]
	b.pubkey = pubkey
	return b
}

// used to initialize the backend, for example check if docker is installed and install it if not on linux (error on mac)
func (b b_docker) Init() (backend, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("/bin/bash", "-c", "command -v docker").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "/bin/bash -c 'command -v docker'")
	}
	if err != nil {
		if b.host == "" {
			out, err = exec.Command("/bin/bash", "-c", "uname").CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, "uname")
		}
		if err != nil {
			return b, errors.New(fmt.Sprintf("Could not determine platform with uname: %s, %s", err, string(out)))
		}
		if strings.Contains(string(out), "Darwin") {
			return b, errors.New("ERROR: docker is not present. See https://docs.docker.com/docker-for-mac/install/")
		} else {
			commands := [][]string{}
			commands = append(commands, []string{"apt-get", "update"})
			commands = append(commands, []string{"apt-get", "-y", "install", "apt-transport-https", "ca-certificates", "curl", "software-properties-common"})
			commands = append(commands, []string{"bash", "-c", "curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -"})
			commands = append(commands, []string{"add-apt-repository", "deb [arch=amd64] https://download.docker.com/linux/ubuntu    xenial    stable"})
			commands = append(commands, []string{"apt-get", "update"})
			commands = append(commands, []string{"apt-get", "-y", "install", "docker-ce"})
			for _, command := range commands {
				if b.host == "" {
					out, err = exec.Command(command[0], command[1:]...).CombinedOutput()
				} else {
					comm := command[0]
					for _, c := range command[1:] {
						if strings.Contains(c, " ") {
							comm = comm + " \"" + c + "\""
						} else {
							comm = comm + " " + c
						}
					}
					out, err = remoteRun(b.user, b.host, b.pubkey, comm)
				}
				if err != nil {
					return b, errors.New(fmt.Sprintf("Could not install docker: %s, %s", err, string(out)))
				}
			}
		}
	}

	if b.host == "" {
		out, err = exec.Command("docker", "info").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "docker info")
	}
	if err != nil {
		return b, errors.New(fmt.Sprintf("Error: docker command exists, but docker appears to be stopped. Got this:\n%s\n%s", string(out), err))
	}
	return b, nil
}

// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
func (b b_docker) ClusterStart(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, node := range nodes {
		var out []byte
		name := fmt.Sprintf("aero-%s_%d", name, node)
		if b.host == "" {
			out, err = exec.Command("docker", "start", name).CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker start %s", name))
		}
		if err != nil {
			return errors.New(fmt.Sprintf("%s;%s", string(out), err))
		}
	}
	return nil
}

// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
func (b b_docker) ClusterStop(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, node := range nodes {
		var out []byte
		name := fmt.Sprintf("aero-%s_%d", name, node)
		if b.host == "" {
			out, err = exec.Command("docker", "stop", name).CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker stop %s", name))
		}
		if err != nil {
			return errors.New(fmt.Sprintf("%s;%s", string(out), err))
		}
	}
	return nil
}

// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
func (b b_docker) ClusterDestroy(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, node := range nodes {
		var out []byte
		name := fmt.Sprintf("aero-%s_%d", name, node)
		if b.host == "" {
			out, err = exec.Command("docker", "rm", name).CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker rm %s", name))
		}
		if err != nil {
			return errors.New(fmt.Sprintf("%s;%s", string(out), err))
		}
	}
	return nil
}

// returns an unformatted string with list of clusters, to be printed to user
func (b b_docker) ClusterListFull() (string, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("docker", "image", "list").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "docker image list")
	}
	if err != nil {
		return string(out), err
	}
	var response string
	response = "Images (templates):\n===================\n"
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		if strings.HasPrefix(t, "REPOSITORY") || strings.HasPrefix(t, "aero-") {
			response = response + t + "\n"
		}
	}
	if b.host == "" {
		out, err = exec.Command("docker", "container", "list", "-a").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "docker container list -a")
	}
	if err != nil {
		return string(out), err
	}
	response = response + "\n\nContainers (clusters):\n======================\n"
	scanner = bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		if strings.HasPrefix(t, "CONTAINER ID") || strings.Contains(t, " aero-") {
			response = response + t + "\n"
		}
	}

	if b.host == "" {
		out, err = exec.Command("docker", "container", "list", "-a", "--format", "{{json .Names}}").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "docker container list -a --format '{{json .Names}}'")
	}
	if err != nil {
		return "", err
	}
	var clusterList []string
	scanner = bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		clusterList = append(clusterList, t)
	}
	response = response + "\n\nNODE_NAME | NODE_IP\n===================\n"
	for _, cluster := range clusterList {
		if strings.HasPrefix(cluster, "aero-") {
			if b.host == "" {
				out, err = exec.Command("docker", "container", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", cluster).CombinedOutput()
			} else {
				out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker container inspect --format='{{.NetworkSettings.IPAddress}}' %s", cluster))
			}
			if err != nil {
				return "", err
			}
			ip := strings.Trim(string(out), "'\" \n\r")
			response = response + cluster + " | " + ip + "\n"
		}
	}
	response = response + "\n\nTo see all docker containers (including base OS images), not just those specific to aerolab:\n$ docker container list -a\n$ docker image list -a\n"
	return response, nil
}

// attach to a node in cluster and run a single command. does not return output of command.
func (b b_docker) AttachAndRun(clusterName string, node int, command []string) (err error) {
	name := fmt.Sprintf("aero-%s_%d", clusterName, node)
	var cmd *exec.Cmd
	if b.host == "" {
		head := []string{"exec", "-e", fmt.Sprintf("NODE=%d", node), "-ti", name}
		if len(command) == 0 {
			command = append(head, "/bin/bash")
		} else {
			command = append(head, command...)
		}
		cmd = exec.Command("docker", command...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
	} else {
		err = remoteAttachAndRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker exec -e NODE=%d -ti %s %s", node, name, strings.Join(command, " ")))
	}
	return err
}

func (b b_docker) copyFilesToContainer(name string, files []fileList) error {
	var err error
	for _, file := range files {
		var tmpfile *os.File
		var tmpfileName string
		var fpath string
		if b.host == "" {
			tmpfile, err = ioutil.TempFile("", "aero-lab-tmp")
			if err != nil {
				return err
			}
			tmpfileName = tmpfile.Name()
			_, err = tmpfile.Write(file.fileContents)
			if err != nil {
				return makeError("Error making tmpfile: %s", err)
			}
			err = tmpfile.Close()
			if err != nil {
				return makeError("Error closing tmpfile: %s", err)
			}
		} else {
			now := time.Now().UnixNano()
			fpath = fmt.Sprintf("/tmp/aero-lab-tmp-%d", now)
			err = scp(b.user, b.host, b.pubkey, []fileList{fileList{fpath, file.fileContents}})
		}
		nodeName := name
		if b.host == "" {
			var out []byte
			out, err = exec.Command("docker", "cp", tmpfileName, fmt.Sprintf("%s:%s", nodeName, file.filePath)).CombinedOutput()
			if err != nil {
				return errors.New(fmt.Sprintf("Error with docker cp: %s;%s", string(out), err))
			}
		} else {
			var out []byte
			out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("docker cp %s %s:%s", fpath, nodeName, file.filePath))
			if err != nil {
				return errors.New(fmt.Sprintf("Error with docker cp: %s;%s", string(out), err))
			}
		}
		if b.host == "" {
			err = os.Remove(tmpfileName)
			if err != nil {
				return makeError("Error deleting tmpfile: %s", err)
			}
		} else {
			_, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("rm %s", fpath))
			if err != nil {
				return makeError("Error running remote rm: %s", err)
			}
		}
	}
	return err
}

func (b b_docker) GetNodeIpMapInternal(name string) (map[int]string, error) {
	return nil, nil
}
