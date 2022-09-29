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
}

func init() {
	addBackend("docker", &backendDocker{})
}

var dockerNameHeader = "aerolab-"

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
						templateList = append(templateList, backendVersion{distVer[0], distVer[1], tag})
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
	_, err := exec.Command("/bin/bash", "-c", "command -v docker").CombinedOutput()
	if err != nil {
		return errors.New("docker command not found; install docker first")
	}
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer ctxCancel()
	out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker command exists, but docker appears to be unreachable or down: %s", string(out))
	}
	d.WorkOnServers()
	return nil
}

func (d *backendDocker) versionToReal(v *backendVersion) error {
	switch v.distroName {
	case "ubuntu", "centos":
	case "amazon":
		v.distroName = "centos"
		v.distroVersion = "7"
	default:
		return errors.New("unsupported distro")
	}
	return nil
}

func (d *backendDocker) DeployTemplate(v backendVersion, script string, files []fileList, extra *backendExtra) error {
	if err := d.versionToReal(&v); err != nil {
		return err
	}
	templName := fmt.Sprintf("aerotmpl-%s-%s-%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	// deploy container with os
	out, err := exec.Command("docker", "run", "-td", "--name", templName, fmt.Sprintf("%s:%s", v.distroName, v.distroVersion)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not start vanilla container: %s;%s", out, err)
	}
	// copy add script to files list
	scriptReader := bytes.NewReader([]byte(script))
	files = append(files, fileList{"/root/install.sh", scriptReader, len(script)})
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

	for node := highestNode; node < nodeCount+highestNode; node = node + 1 {
		var out []byte
		exposeList := []string{"run"}
		tmplName := fmt.Sprintf(dockerNameHeader+"%s_%s:%s", v.distroName, v.distroVersion, v.aerospikeVersion)
		if d.client {
			tmplName = fmt.Sprintf("%s:%s", v.distroName, v.distroVersion)
		}
		if extra.dockerHostname {
			exposeList = append(exposeList, "--hostname", name+"-"+strconv.Itoa(node))
		}
		if len(extra.switches) > 0 {
			exposeList = append(exposeList, strings.Split(extra.switches, " ")...)
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

func (d *backendDocker) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
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
		tmpfile, err = os.CreateTemp("", "aerolab-tmp")
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
			head := []string{"exec", name}
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
		out, err = exec.Command("docker", "container", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", containerName).CombinedOutput()
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
		out, err = exec.Command("docker", "container", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", containerName).CombinedOutput()
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
		out, err = exec.Command("docker", "stop", name).CombinedOutput()
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

func (d *backendDocker) Upload(clusterName string, node int, source string, destination string, verbose bool) error {
	name := fmt.Sprintf(dockerNameHeader+"%s_%d", clusterName, node)
	cmd := []string{"cp", source, name + ":" + destination}
	out, err := exec.Command("docker", cmd...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.ReplaceAll(string(out), "\n", "; "))
	}
	return nil
}

func (d *backendDocker) Download(clusterName string, node int, source string, destination string, verbose bool) error {
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

func (d *backendDocker) copyFilesToContainer(name string, files []fileList) error {
	var err error
	for _, file := range files {
		var tmpfile *os.File
		var tmpfileName string
		tmpfile, err = os.CreateTemp("", dockerNameHeader+"tmp")
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
func (d *backendDocker) ClusterListFull(isJson bool) (string, error) {
	if !isJson {
		return d.clusterListFullNoJson()
	}
	jsonOut := []clusterListFull{}
	out, err := exec.Command("docker", "container", "list", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Status}}").CombinedOutput()
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\" \t\r\n")
		tt := strings.Split(t, "\t")
		if len(tt) != 3 {
			continue
		}
		nameNo := strings.Split(strings.TrimPrefix(tt[1], dockerNameHeader+""), "_")
		if len(nameNo) != 2 {
			continue
		}
		out2, err := exec.Command("docker", "container", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", tt[1]).CombinedOutput()
		if err != nil {
			return "", err
		}
		ip := strings.Trim(string(out2), "'\" \n\r")
		jsonOut = append(jsonOut, clusterListFull{
			ClusterName: nameNo[0],
			NodeNumber:  nameNo[1],
			IpAddress:   ip,
			PublicIp:    "",
			InstanceId:  tt[0],
			State:       tt[2],
		})
	}
	out, err = json.MarshalIndent(jsonOut, "", "    ")
	return string(out), err
}

func (d *backendDocker) clusterListFullNoJson() (string, error) {
	var err error
	var out []byte
	var response string
	out, err = exec.Command("docker", "container", "list", "-a").CombinedOutput()
	if err != nil {
		return string(out), err
	}
	response = "Containers (clusters):\n======================\n"
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\"")
		if strings.HasPrefix(t, "CONTAINER ID") || strings.Contains(t, " "+dockerNameHeader) {
			response = response + t + "\n"
		}
	}

	out, err = exec.Command("docker", "container", "list", "-a", "--format", "{{json .Names}}").CombinedOutput()
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
		if strings.HasPrefix(cluster, dockerNameHeader+"") {
			out, err = exec.Command("docker", "container", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", cluster).CombinedOutput()
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

// returns an unformatted string with list of clusters, to be printed to user
func (d *backendDocker) TemplateListFull(isJson bool) (string, error) {
	jsonRes := []templateListFull{}
	var err error
	var out []byte
	out, err = exec.Command("docker", "image", "list").CombinedOutput()
	if err != nil {
		return string(out), err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))

	if !isJson {
		var response string
		response = "Images (templates):\n===================\n"
		for scanner.Scan() {
			t := scanner.Text()
			t = strings.Trim(t, "'\"")
			if strings.HasPrefix(t, "REPOSITORY") || strings.HasPrefix(t, dockerNameHeader+"") {
				response = response + t + "\n"
			}
		}
		response = response + "\n\nTo see all docker containers (including base OS images), not just those specific to aerolab:\n$ docker container list -a\n$ docker image list -a\n"
		return response, nil
	}

	for scanner.Scan() {
		t := scanner.Text()
		t = strings.Trim(t, "'\" \t\r\n")
		if !strings.HasPrefix(t, dockerNameHeader+"") {
			continue
		}
		rep := strings.TrimPrefix(cut(t, 1, " "), dockerNameHeader+"")
		osVer := strings.Split(rep, "_")
		if len(osVer) != 2 {
			continue
		}
		asdVer := cut(t, 2, " ")
		jsonRes = append(jsonRes, templateListFull{
			OsName:           osVer[0],
			OsVersion:        osVer[1],
			AerospikeVersion: asdVer,
			ImageId:          cut(t, 3, " "),
		})
	}
	out, err = json.MarshalIndent(jsonRes, "", "    ")
	return string(out), err
}
