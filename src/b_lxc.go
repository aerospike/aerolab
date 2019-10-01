package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

type b_lxc struct {
	host   string
	user   string
	pubkey string
	btrfs  bool
}

// configure remote host in struct
func (b b_lxc) ConfigRemote(host string, pubkey string) backend {
	b.user = strings.Split(host, "@")[0]
	b.host = strings.Split(host, "@")[1]
	b.pubkey = pubkey
	return b
}

// return a list of cluster names
func (b b_lxc) ClusterList() ([]string, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return nil, errors.New(fmt.Sprintf("Error running lxc-ls: %s", err))
	}
	var res []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, "aero-") {
			r := strings.Split(t, " ")[0]
			r = r[5:]
			r, _ = splitClusterName(r)
			if inArray(res, r) == -1 {
				res = append(res, r)
			}
		}
	}
	return res, nil
}

func (b b_lxc) ClusterListFull() (string, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return "", errors.New(fmt.Sprintf("Error running lxc-ls: %s", err))
	}
	return string(out), nil
}

func (b b_lxc) NodeListInCluster(name string) ([]int, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return nil, errors.New(fmt.Sprintf("Error running lxc-ls: %s", err))
	}
	var res []int
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, "aero-") {
			r := strings.Split(t, " ")[0]
			ra := r[5:]
			r, na := splitClusterName(ra)
			n, _ := strconv.Atoi(na)
			if r == name {
				if inArray(res, n) == -1 {
					res = append(res, n)
				}
			}
		}
	}
	return res, nil
}

func (b b_lxc) GetClusterNodeIps(name string) ([]string, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return nil, errors.New(fmt.Sprintf("ERROR running lxc-ls: %s", err))
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	var lst []string
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, fmt.Sprintf("aero-%s", name)) {
			l := strings.Split(t, " ")
			for i := 0; i < len(l); i++ {
				if strings.HasPrefix(l[i], "10.") {
					lst = append(lst, l[i])
				}
			}
		}
	}
	return lst, nil
}

func (b b_lxc) GetNodeIpMapInternal(name string) (map[int]string, error) {
	return nil, nil
}

func (b b_lxc) GetNodeIpMap(name string) (map[int]string, error) {
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return nil, errors.New(fmt.Sprintf("ERROR running lxc-ls: %s", err))
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	lst := make(map[int]string)
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, fmt.Sprintf("aero-%s", name)) {
			l := strings.Split(t, " ")
			for i := 0; i < len(l); i++ {
				if strings.HasPrefix(l[i], "10.") {
					_, lna := splitClusterName(l[0][5:])
					ln, _ := strconv.Atoi(lna)
					lst[ln] = l[i]
				}
			}
		}
	}
	return lst, nil
}

// list all template versions available
func (b b_lxc) ListTemplates() ([]version, error) {
	v := []version{}
	var err error
	var out []byte
	if b.host == "" {
		out, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return nil, errors.New(fmt.Sprintf("ERROR running lxc-ls: %s", err))
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, "aero_tpl-") { // aero_tpl-os_ver_aeroVer
			t = strings.Split(t, " ")[0]
			r := strings.Split(strings.Split(t, "-")[1], "_")
			v = append(v, version{r[0], r[1], r[2]})
		}
	}
	return v, nil
}

func (b b_lxc) GetBackendName() string {
	return "lxc"
}

// deploy a given version of template, use script to install aerospike
func (b b_lxc) DeployTemplate(v version, script string, files []fileList) error {
	var err error
	var out []byte
	comm := "lxc-create"
	var nick string
	if v.distroVersion == "18.04" {
		nick = "bionic"
	} else if v.distroVersion == "16.04" {
		nick = "xenial"
	} else if v.distroVersion == "14.04" {
		nick = "trusty"
	} else {
		nick = v.distroVersion
	}
	var nameNick string
	if v.distroName == "el" {
		nameNick = "centos"
	} else {
		nameNick = v.distroName
	}
	templatePath := fmt.Sprintf("/var/lib/lxc/aero_tpl-%s_%s_%s/rootfs", v.distroName, v.distroVersion, v.aerospikeVersion)
	templateName := fmt.Sprintf("aero_tpl-%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	var params []string
	if b.btrfs == false {
		params = []string{"-t", "download", "-n", templateName, "--", "-d", nameNick, "-r", nick, "-a", "amd64"}
	} else {
		params = []string{"-B", "btrfs", "-t", "download", "-n", templateName, "--", "-d", nameNick, "-r", nick, "-a", "amd64"}
	}
	if b.host == "" {
		out, err = exec.Command(comm, params...).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("%s %s", comm, strings.Join(params, " ")))
	}
	if check_exec_retcode(err) != 0 {
		return errors.New(fmt.Sprintf("ERROR running lxc-create: %s\n%s", err, out))
	}

	files = append(files, fileList{"/root/aero-script.sh", []byte(script)})

	for i := range files {
		files[i].filePath = path.Join(templatePath, files[i].filePath)
	}

	if b.host != "" {
		err = scp(b.user, b.host, b.pubkey, files)
		if err != nil {
			return err
		}
	} else {
		for _, file := range files {
			ioutil.WriteFile(file.filePath, file.fileContents, 0755)
		}
	}

	if b.host == "" {
		out, err = exec.Command("lxc-start", "-n", templateName).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-start -n "+templateName)
	}
	if check_exec_retcode(err) != 0 {
		return errors.New(fmt.Sprintf("ERROR running lxc-start: %s\n%s", err, out))
	}

	err = b.lxc_ips_assigned(templateName)
	if err != nil {
		return err
	}

	if b.host == "" {
		out, err = exec.Command("lxc-attach", "-n", templateName, "--", "/bin/bash", "/root/aero-script.sh").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-attach -n "+templateName+" -- /bin/bash /root/aero-script.sh")
	}
	if check_exec_retcode(err) != 0 {
		return errors.New(fmt.Sprintf("ERROR running lxc-attach: %s\n%s", err, out))
	}

	if b.host == "" {
		out, err = exec.Command("lxc-stop", "-n", templateName).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-stop -n "+templateName)
	}
	if check_exec_retcode(err) != 0 {
		return errors.New(fmt.Sprintf("ERROR running lxc-stop: %s\n%s", err, out))
	}

	return nil
}

// deploy cluster from template
func (b b_lxc) DeployCluster(v version, name string, nodeCount int, exposePorts []string) error {
	return b.DeployClusterWithLimits(v, name, nodeCount, exposePorts, "", "", "", false)
}

func (b b_lxc) DeployClusterWithLimits(v version, name string, nodeCount int, exposePorts []string, cpuLimit string, ramLimit string, swapLimit string, privileged bool) error {
	if cpuLimit != "" || ramLimit != "" || swapLimit != "" {
		return errors.New("Sorry, LXC does not currently support setting quotas. This feature will be enabled at some point using cgroups.")
	}
	if len(exposePorts) > 0 {
		fmt.Println("WARN: lxc backend does not support port exposure. To expose port, invoke an iptables PREROUTING rule directly on the host.")
	}
	templateName := fmt.Sprintf("aero_tpl-%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	clusterPrefix := "aero-" + name + "_"
	var err error
	var out []byte
	var nodeList []int
	var highestNode int
	list, err := b.ClusterList()
	if err != nil {
		return err
	}
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

	for i := highestNode; i < nodeCount+highestNode; i++ {
		node := clusterPrefix + strconv.Itoa(i)
		if b.host == "" {
			if b.btrfs == false {
				//out, err = exec.Command("lxc-copy", "-B", "overlayfs","-s","-n", templateName, "-N", node).CombinedOutput()
				out, err = exec.Command("lxc-copy", "-n", templateName, "-N", node).CombinedOutput()
			} else {
				out, err = exec.Command("lxc-copy", "-B", "btrfs", "-n", templateName, "-N", node).CombinedOutput()
			}
		} else {
			if b.btrfs == false {
				//out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-copy -B overlayfs -s -n "+templateName+" -N "+node)
				out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-copy -n "+templateName+" -N "+node)
			} else {
				out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-copy -B btrfs -n "+templateName+" -N "+node)
			}
		}
		if check_exec_retcode(err) != 0 {
			return errors.New(fmt.Sprintf("ERROR running lxc-copy: %s\n%s", err, out))
		}
	}

	err = b.cleanup_dnsmasq()

	//TODO: impose limits on CPU/RAM/SWAP

	return err
}

func (b b_lxc) lxc_ip_assigned_count(name_prefix string) (string, int, error) {
	var out []byte
	var err error
	if b.host == "" {
		out, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return "", -1, errors.New(fmt.Sprintf("ERROR running lxc-ls --fancy: %s\n", err))
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	count := 0
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, name_prefix) && strings.Contains(t, " 10.") {
			count = count + 1
		}
	}
	return string(out), count, nil
}

func (b b_lxc) Init() (backend, error) {
	var out bool
	var err error
	out, err = b.check_dependencies()
	if err != nil {
		return b, err
	}
	if out == true {
		out, err = b.check_dependencies()
		if err != nil {
			return b, err
		}
		if out == true {
			return b, errors.New("ERROR: Something isn't right. Tried installing twice and got installed twice? Run manually: apt-get -y install lxc1 lxc-templates")
		}
	}
	//stat -f -c %T /var/lib/lxc
	var nout []byte
	if b.host == "" {
		nout, err = exec.Command("stat", "-f", "-c", "%T", "/var/lib/lxc").CombinedOutput()
	} else {
		nout, err = remoteRun(b.user, b.host, b.pubkey, "stat -f -c %T /var/lib/lxc")
	}
	if check_exec_retcode(err) != 0 {
		return b, errors.New(fmt.Sprintf("ERROR running stat on /var/lib/lxc: %s\n", err))
	}
	if strings.HasPrefix(string(nout), "btrfs") {
		b.btrfs = true
	}
	// disabling btrfs as 18.04 has broken btrfs lxc-copy, re-enable when bug is fixed. for now will use overlayfs everywhere
	b.btrfs = false
	err = b.cleanup_dnsmasq()
	return b, err
}

func (b b_lxc) AttachAndRun(clusterName string, node int, command []string) (err error) {
	name := fmt.Sprintf("aero-%s_%d", clusterName, node)
	var cmd *exec.Cmd
	if b.host == "" {
		head := []string{"-n", name, "--"}
		command = append(head, command...)
		cmd = exec.Command("lxc-attach", command...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
	} else {
		err = remoteAttachAndRun(b.user, b.host, b.pubkey, fmt.Sprintf("lxc-attach -n %s -- %s", name, strings.Join(command, " ")))
	}
	return err
}

// run command in cluster on nodes
func (b b_lxc) RunCommand(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
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
				head := []string{"-n", name, "--"}
				command = append(head, command...)
				out, err = exec.Command("lxc-attach", command...).CombinedOutput()
			} else {
				out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("lxc-attach -n %s -- %s", name, strings.Join(command, " ")))
			}
			fout = append(fout, out)
			if check_exec_retcode(err) != 0 {
				return fout, errors.New(fmt.Sprintf("ERROR running %s: %s\n", command, err))
			}
		}
	}
	return fout, nil
}

func (b b_lxc) ClusterStart(name string, nodes []int) error {
	var err error
	if nodes == nil {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	nodeIpMap, err := b.GetNodeIpMap(name)
	if err != nil {
		return err
	}

	totalIps := len(nodeIpMap)
	for _, i := range nodes {
		if _, ok := nodeIpMap[i]; ok == false {
			totalIps = totalIps + 1
		}
	}

	err = b.cluster_stop_start(name, nodes, "lxc-start")
	if err != nil {
		return err
	}
	err = b.lxc_ips_assigned_cluster(name, totalIps)
	if err != nil {
		return err
	}
	return nil
}

func (b b_lxc) ClusterStop(name string, nodes []int) error {
	return b.cluster_stop_start(name, nodes, "lxc-stop")
}

func (b b_lxc) ClusterDestroy(name string, nodes []int) error {
	return b.cluster_stop_start(name, nodes, "lxc-destroy")
}

func (b b_lxc) cluster_stop_start(name string, nodes []int, action string) error {
	var err error
	var out []byte
	if nodes == nil {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}
	for _, node := range nodes {
		nname := fmt.Sprintf("aero-%s_%d", name, node)
		if b.host == "" {
			out, err = exec.Command(action, "-n", nname).CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("%s -n %s", action, nname))
		}
		if check_exec_retcode(err) != 0 {
			return errors.New(fmt.Sprintf("ERROR running %s: %s\n%s", action, err, out))
		}
	}
	err = b.cleanup_dnsmasq()
	return err
}

func (b b_lxc) TemplateDestroy(v version) error {
	if v.distroName == "rhel" {
		v.distroName = "el"
	}
	tname := fmt.Sprintf("aero_tpl-%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion)
	var out []byte
	var err error
	if b.host == "" {
		exec.Command("lxc-stop", "-n", tname).CombinedOutput()
	} else {
		remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("lxc-stop -n %s", tname))
	}
	if b.host == "" {
		out, err = exec.Command("lxc-destroy", "-n", tname).CombinedOutput()
	} else {
		out, err = remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("lxc-destroy -n %s", tname))
	}
	if check_exec_retcode(err) != 0 {
		return errors.New(fmt.Sprintf("ERROR running lxc-destroy: %s\n%s", err, out))
	}
	return nil
}

// deploy extra files to cluster nodes
func (b b_lxc) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
	var err error
	if nodes == nil {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return err
		}
	}

	var filepaths []fileList
	for _, node := range nodes {
		var nDir string
		if b.host == "" {
			nDir = fmt.Sprintf("/var/lib/lxc/aero-%s_%d/delta0/", name, node)
			if _, err := os.Stat(nDir); os.IsNotExist(err) {
				nDir = fmt.Sprintf("/var/lib/lxc/aero-%s_%d/rootfs/", name, node)
			}
		} else {
			// TODO: not checking delta0 here vs rootfs
			nDir = fmt.Sprintf("/var/lib/lxc/aero-%s_%d/rootfs/", name, node)
		}
		for _, file := range files {
			filepaths = append(filepaths, fileList{path.Join(nDir, file.filePath), file.fileContents})
		}
	}

	if b.host != "" {
		for _, filepath := range filepaths {
			nDir := path.Dir(filepath.filePath)
			remoteRun(b.user, b.host, b.pubkey, fmt.Sprintf("mkdir -p '%s'", nDir))
		}
		err = scp(b.user, b.host, b.pubkey, filepaths)
		if err != nil {
			return err
		}
	} else {
		for _, filepath := range filepaths {
			nDir := path.Dir(filepath.filePath)
			os.MkdirAll(nDir, 0755)
			err = ioutil.WriteFile(filepath.filePath, []byte(filepath.fileContents), 0755)
			if err != nil {
				return err
			}
		}
	}

	return err
}

func (b b_lxc) lxc_ips_assigned(templateName string) error {
	tst := false
	for i := 0; i < 20; i++ {
		_, count, err := b.lxc_ip_assigned_count(templateName)
		if err != nil {
			return err
		}
		if count == 1 {
			tst = true
			break
		} else {
			time.Sleep(1 * time.Second)
		}
	}
	if tst == false {
		return errors.New("ERROR: Waited for 20 seconds, but IP still not assigned. Quitting. You may want to check: lxc-ls --fancy")
	}
	return nil
}

func (b b_lxc) lxc_ips_assigned_cluster(name string, count int) error {
	tst := false
	name = fmt.Sprintf("aero-%s_", name)
	for i := 0; i < 20; i++ {
		_, cc, err := b.lxc_ip_assigned_count(name)
		if err != nil {
			return err
		}
		if count == cc {
			tst = true
			break
		} else {
			time.Sleep(1 * time.Second)
		}
	}
	if tst == false {
		return errors.New("ERROR: Waited for 20 seconds, but IP still not assigned. Quitting. You may want to check: lxc-ls --fancy")
	}
	return nil
}

// ----------------------------- INIT & CLEANUP_DNSMASQ ---------------------------------- //
func (b b_lxc) reload_dnsmasq() error {
	var err error
	if b.host == "" {
		_, err = exec.Command("killall", "-s", "SIGHUP", "dnsmasq").CombinedOutput()
	} else {
		_, err = remoteRun(b.user, b.host, b.pubkey, "killall -s SIGHUP dnsmasq")
	}
	if check_exec_retcode(err) != 0 {
		return errors.New(fmt.Sprintf("ERROR: Could not reload dnsmasq, IP stickiness won't work until restarted: %s\n", err))
	}
	return nil
}

func (b b_lxc) check_dependencies() (bool, error) {
	ret := false
	pkg := []string{"lxc-utils", "lxc-templates", "wget"}
	for i := 0; i < len(pkg); i++ {
		var err error
		var out []byte
		if b.host == "" {
			out, err = exec.Command("/bin/bash", "-c", "command -v dpkg").CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, "/bin/bash -c 'command -v dpkg'")
		}
		if err != nil {
			return false, errors.New("Not an ubuntu box. Could not find dpkg.")
		}
		if b.host == "" {
			out, err = exec.Command("/bin/bash", "-c", "command -v lxc-ls").CombinedOutput()
		} else {
			out, err = remoteRun(b.user, b.host, b.pubkey, "/bin/bash -c 'command -v lxc-ls'")
		}
		if err != nil {
			fmt.Println("Installing dependencies with package manager apt, please wait...")
			if b.host == "" {
				out, err = exec.Command("apt-get", "-y", "install", "lxc-utils", "lxc-templates", "wget").CombinedOutput()
			} else {
				out, err = remoteRun(b.user, b.host, b.pubkey, "apt-get -y install lxc-utils lxc-templates wget")
			}
			if err != nil {
				return false, errors.New(fmt.Sprintf("ERROR installing dependencies with apt: %s\n%s", err, out))
			}
			fmt.Println("Installed: lxc-utils, lxc-templates, wget")
			ret = true
		}
	}

	var err error
	var lxcnet []byte
	if b.host == "" {
		lxcnet, err = ioutil.ReadFile("/etc/default/lxc-net")
	} else {
		lxcnet, err = remoteRun(b.user, b.host, b.pubkey, "echo -n '===STARTFILEOUTHERE===' && cat /etc/default/lxc-net")
		if err == nil {
			lxcnet = []byte(strings.Split(string(lxcnet), "===STARTFILEOUTHERE===")[1])
		}
	}
	if err != nil {
		return false, errors.New(fmt.Sprintf("ERROR reading /etc/default/lxc-net: %s\n", err))
	}
	found := false
	restart := false
	scanner := bufio.NewScanner(strings.NewReader(string(lxcnet)))
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, "LXC_DHCP_CONFILE") {
			found = true
			break
		}
	}
	if found == false {
		lxcnet1 := string(lxcnet) + "\nLXC_DHCP_CONFILE=/etc/lxc/dnsmasq.conf\n"

		if b.host == "" {
			err = ioutil.WriteFile("/etc/default/lxc-net", []byte(lxcnet1), 0666)
		} else {
			err = scp(b.user, b.host, b.pubkey, []fileList{fileList{"/etc/default/lxc-net", []byte(lxcnet1)}})
		}
		restart = true
		if err != nil {
			return false, errors.New(fmt.Sprintf("ERROR writing to /etc/default/lxc-net: %s\n", err))
		}
	}

	if b.host == "" {
		if _, err := os.Stat("/etc/lxc/dnsmasq.conf"); os.IsNotExist(err) {
			r := "dhcp-hostsfile=/etc/lxc/dnsmasq-hosts.conf"
			err = ioutil.WriteFile("/etc/lxc/dnsmasq.conf", []byte(r), 0666)
			restart = true
			if err != nil {
				return false, errors.New(fmt.Sprintf("ERROR writing to /etc/lxc/dnsmasq.conf: %s\n", err))
			}
		}
	} else {
		var out []byte
		out, err = remoteRun(b.user, b.host, b.pubkey, "echo -n '===STARTFILEOUTHERE===' && ls /etc/lxc/dnsmasq.conf 2>/dev/null 1>/dev/null ; echo $?")
		if err != nil {
			return false, errors.New(fmt.Sprintf("ERROR writing to /etc/lxc/dnsmasq.conf: %s\n", err))
		}
		out = []byte(strings.Split(string(out), "===STARTFILEOUTHERE===")[1])
		if strings.Contains(string(out), "0") != true {
			r := "dhcp-hostsfile=/etc/lxc/dnsmasq-hosts.conf"
			err = scp(b.user, b.host, b.pubkey, []fileList{fileList{"/etc/lxc/dnsmasq.conf", []byte(r)}})
			restart = true
			if err != nil {
				return false, errors.New(fmt.Sprintf("ERROR writing to /etc/lxc/dnsmasq.conf: %s\n", err))
			}
		}
	}

	if b.host == "" {
		if _, err := os.Stat("/etc/lxc/dnsmasq-hosts.conf"); os.IsNotExist(err) {
			r := ""
			err = ioutil.WriteFile("/etc/lxc/dnsmasq-hosts.conf", []byte(r), 0666)
			restart = true
			if err != nil {
				return false, errors.New(fmt.Sprintf("ERROR writing to /etc/lxc/dnsmasq-hosts.conf: %s\n", err))
			}
		}
	} else {
		_, err = remoteRun(b.user, b.host, b.pubkey, "echo '' >> /etc/lxc/dnsmasq-hosts.conf")
		if err != nil {
			return false, errors.New(fmt.Sprintf("ERROR writing to /etc/lxc/dnsmasq-hosts.conf: %s\n", err))
		}
	}

	if restart == true {
		if b.host == "" {
			_, err = exec.Command("service", "lxc-net", "restart").CombinedOutput()
		} else {
			_, err = remoteRun(b.user, b.host, b.pubkey, "service lxc-net restart")
		}
		if err != nil {
			return false, errors.New(fmt.Sprintf("Could not restart service lxc-net, reboot machine: %s\n", err))
		}
		err = b.reload_dnsmasq()
		if err != nil {
			return false, err
		}
	}
	return ret, nil
}

func (b b_lxc) cleanup_dnsmasq() error {
	var containers []string
	var outa []byte
	var err error
	if b.host == "" {
		outa, err = exec.Command("lxc-ls", "--fancy").CombinedOutput()
	} else {
		outa, err = remoteRun(b.user, b.host, b.pubkey, "lxc-ls --fancy")
	}
	if check_exec_retcode(err) != 0 {
		return errors.New(fmt.Sprintf("WARNING running lxc-ls --fancy during dnsmasq cleanup, dnsmasq could be dirty: %s\n", err))
	}

	//get containers list
	scanner := bufio.NewScanner(strings.NewReader(string(outa)))
	for scanner.Scan() {
		l := strings.Split(scanner.Text(), " ")
		container := l[0]
		ip := ""
		for i := 0; i < len(l); i++ {
			if strings.HasPrefix(l[i], "10.") {
				ip = l[i]
				break
			}
		}
		containers = append(containers, fmt.Sprintf("%s,%s", container, ip))
	}

	cleanfile := ""
	changed := false
	var dm []byte
	if b.host == "" {
		dm, err = ioutil.ReadFile("/etc/lxc/dnsmasq-hosts.conf")
		if err != nil {
			return errors.New(fmt.Sprintf("Cannot read dnsmasq-hosts.conf during cleanup, dnsmasq could be dirty: %s\n", err))
		}
	} else {
		dm, err = remoteRun(b.user, b.host, b.pubkey, "echo -n '===STARTFILEOUTHERE===' && cat /etc/lxc/dnsmasq-hosts.conf")
		if err != nil {
			return errors.New(fmt.Sprintf("Cannot read dnsmasq-hosts.conf during cleanup, dnsmasq could be dirty: %s\n", err))
		}
		dm = []byte(strings.Split(string(dm), "===STARTFILEOUTHERE===")[1])
	}
	scanner = bufio.NewScanner(strings.NewReader(string(dm)))
	for scanner.Scan() {
		t := scanner.Text()
		l := strings.Split(t, ",")
		found := false
		for i := 0; i < len(containers); i++ {
			aa := strings.Split(containers[i], ",")
			if aa[0] == l[0] && (aa[1] == "" || aa[1] == l[1]) {
				found = true
				cleanfile = cleanfile + t + "\n"
				break
			}
		}
		if found == false {
			changed = true
		}
	}

	//add missing containers
	dml := strings.Split(string(dm), "\n")

	for i := 0; i < len(containers); i++ {
		if strings.HasPrefix(containers[i], "aero-") {
			found := false
			cname := strings.Split(containers[i], ",")[0]
			for j := 0; j < len(dml); j++ {
				if cname == strings.Split(dml[j], ",")[0] {
					found = true
					break
				}
			}
			if found == false {
				cleanfile = cleanfile + containers[i] + "\n"
				changed = true
			}
		}
	}

	if changed == true {
		if b.host == "" {
			err = ioutil.WriteFile("/etc/lxc/dnsmasq-hosts.conf", []byte(cleanfile), 0666)
		} else {
			err = scp(b.user, b.host, b.pubkey, []fileList{fileList{"/etc/lxc/dnsmasq-hosts.conf", []byte(cleanfile)}})
		}
		if err != nil {
			fmt.Printf("WARNING cannot write dnsmasq-hosts.conf during cleanup, dnsmasq is dirty: %s\n", err)
		}
		err = b.reload_dnsmasq()
		if err != nil {
			return err
		}
	}

	return nil
}
