package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreateVSCodeCmd struct {
	clientCreateBaseCmd
	Kernels string `short:"k" long:"kernels" description:"comma-separated list; options: go,python,java,dotnet; default: all kernels"`
	chDirCmd
}

type clientAddVSCodeCmd struct {
	ClientName  TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	osSelectorCmd
	Kernels string  `short:"k" long:"kernels" description:"comma-separated list; options: go,python,java,dotnet; default: all kernels"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateVSCodeCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroVersion == "latest" {
		c.DistroVersion = "20.04"
	}
	if c.DistroName != TypeDistro("ubuntu") || c.DistroVersion != TypeDistroVersion("20.04") {
		return fmt.Errorf("VSCode is only supported on ubuntu:20.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	var err error
	machines, err := c.createBase(args, "VSCode")
	if err != nil {
		return err
	}
	a.opts.Client.Add.VSCode.ClientName = c.ClientName
	a.opts.Client.Add.VSCode.StartScript = c.StartScript
	a.opts.Client.Add.VSCode.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.VSCode.DistroName = c.DistroName
	a.opts.Client.Add.VSCode.DistroVersion = c.DistroVersion
	a.opts.Client.Add.VSCode.Kernels = c.Kernels
	return a.opts.Client.Add.VSCode.addVSCode(args)
}

func (c *clientConfigureVSCodeCmd) parseKernelsToSwitches(k string) ([]string, error) {
	kernels := strings.Split(k, ",")
	if len(kernels) == 0 || kernels[0] == "" || k == "" {
		// return []string{"-j", "-p", "-n", "-g", "-d", "-o", "-s"}, nil
		return []string{"-j", "-p", "-g", "-d", "-o", "-s"}, nil
	}
	rval := []string{}
	for _, kernel := range kernels {
		switch kernel {
		case "go":
			rval = append(rval, "-g")
		case "python":
			rval = append(rval, "-p")
		case "java":
			rval = append(rval, "-j")
		case "dotnet":
			rval = append(rval, "-d")
		default:
			return nil, errors.New("unsupported kernel selected")
		}
	}
	rval = append(rval, "-o", "-s")
	return rval, nil
}

func (c *clientAddVSCodeCmd) parseKernelsToSwitches(k string) ([]string, error) {
	kernels := strings.Split(k, ",")
	if len(kernels) == 0 || kernels[0] == "" || k == "" {
		// return []string{"-i", "-j", "-p", "-n", "-g", "-d", "-s"}, nil
		return []string{"-i", "-j", "-p", "-g", "-d", "-s"}, nil
	}
	rval := []string{"-i"}
	for _, kernel := range kernels {
		switch kernel {
		case "go":
			rval = append(rval, "-g")
		case "python":
			rval = append(rval, "-p")
		case "java":
			rval = append(rval, "-j")
		case "dotnet":
			rval = append(rval, "-d")
		default:
			return nil, errors.New("unsupported kernel selected")
		}
	}
	rval = append(rval, "-s")
	return rval, nil
}

func (c *clientAddVSCodeCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroName != TypeDistro("ubuntu") || c.DistroVersion != TypeDistroVersion("20.04") {
		return fmt.Errorf("VSCode is only supported on ubuntu:20.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	b.WorkOnServers()
	return c.addVSCode(args)
}

func (c *clientAddVSCodeCmd) addVSCode(args []string) error {
	b.WorkOnClients()
	f, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}
	fn := f.Name()
	_, err = f.WriteString(c.installScript())
	f.Close()
	if err != nil {
		return err
	}
	a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
	a.opts.Files.Upload.IsClient = true
	a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
	a.opts.Files.Upload.Files.Source = flags.Filename(fn)
	a.opts.Files.Upload.Files.Destination = flags.Filename("/install.sh")
	err = a.opts.Files.Upload.runUpload(nil)
	if err != nil {
		return err
	}
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	switches, err := c.parseKernelsToSwitches(c.Kernels)
	if err != nil {
		return err
	}
	nargs := append([]string{"/bin/bash", "/install.sh"}, switches...)
	err = a.opts.Attach.Client.run(nargs)
	if err != nil {
		return err
	}
	log.Print("Done, to access vscode, run `aerolab client list` to get the IP, and then visit http://IP:8080 in your browser")
	return nil
}

func (c *clientAddVSCodeCmd) installScript() string {
	return `function install_code() {
	cd /
	apt-get update && apt-get -y install curl wget git || return 1
	wget -O installcode.sh https://code-server.dev/install.sh || return 2
	bash installcode.sh || return 3
}

function install_start_script() {
	mkdir -p /opt/autoload || return 1
	echo '#!/bin/bash' > /opt/autoload/code.sh || return 2
	echo 'export DOTNET_ROOT=/root/dotnet' >> /opt/autoload/code.sh || return 3
	echo 'export PATH=$PATH:/root/dotnet:/root/.dotnet/tools' >> /opt/autoload/code.sh || return 4
	echo 'nohup code-server --disable-workspace-trust --disable-telemetry --disable-getting-started-override > /var/log/code-server.log 2>&1 &' >> /opt/autoload/code.sh || return 5
	chmod 755 /opt/autoload/code.sh || return 6
}

function conf_code() {
mkdir -p /opt/code
cd /opt/code
git clone -b code-server-examples https://github.com/aerospike/aerolab.git && \
mv aerolab/* . && \
mv aerolab/.vscode . && \
rm -rf aerolab
mkdir -p /root/.config/code-server
mkdir -p /root/.local/share/code-server/User
cat <<'EOF' > /root/.config/code-server/config.yaml
bind-addr: 0.0.0.0:8080
auth: none
cert: false
EOF
cat <<'EOF' > /root/.local/share/code-server/coder.json 
{
  "query": {
    "folder": "/opt/code"
  },
  "update": {
    "checked": 1668550936677,
    "version": "4.8.3"
  }
}
EOF
cat <<'EOF' > /root/.local/share/code-server/User/settings.json 
{
    "window.menuBarVisibility": "classic",
    "workbench.colorTheme": "Default Dark+",
    "workbench.startupEditor": "none",
    "omnisharp.dotnetPath": "/root/dotnet",
    "omnisharp.sdkPath": "/root/dotnet/sdk/6.0.300",
}
EOF
}

function kgo() {
	apt-get install -y gcc || return 1
	url="https://go.dev/dl/go1.19.3.linux-amd64.tar.gz"
	uname -p |egrep 'x86_64|amd64'
	[ $? -ne 0 ] && url="https://go.dev/dl/go1.19.3.linux-arm64.tar.gz"
	cd /
	wget -O go.tgz ${url} || return 2
	tar -C /usr/local -xzf go.tgz || return 3
	ln -s /usr/local/go/bin/go /usr/local/bin/go || return 4
	ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt || return 5
	code-server --install-extension golang.go || return 6
	go install github.com/cweill/gotests/gotests@v1.6.0
	go install github.com/fatih/gomodifytags@v1.16.0
	go install github.com/josharian/impl@v1.1.0
	go install github.com/haya14busa/goplay/cmd/goplay@v1.0.0
	go install github.com/go-delve/delve/cmd/dlv@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/tools/gopls@latest
	go install github.com/ramya-rao-a/go-outline@v0.0.0-20210608161538-9736a4bde949
}

function kpython() {
	apt-get update && apt-get -y install python3 python3-pip python3-wheel python3-setuptools || return 1
	pip3 install aerospike || return 2
	code-server --install-extension ms-python.python || return 3
}

function kjava() {
	apt-get update || return 1
	DEBIAN_FRONTEND=noninteractive apt-get -qq -y install openjdk-17-jdk || return 2
	code-server --install-extension vscjava.vscode-java-debug || return 3
	code-server --install-extension vscjava.vscode-maven || return 4
	code-server --install-extension vscjava.vscode-java-dependency || return 5
	code-server --install-extension vscjava.vscode-java-test || return 6
	code-server --install-extension redhat.java || return 7
	cd /tmp && \
	wget https://dlcdn.apache.org/maven/maven-3/3.8.6/binaries/apache-maven-3.8.6-bin.tar.gz && \
	tar xvf apache-maven-3.8.6-bin.tar.gz && \
	mkdir -p /usr/share/maven && \
	cd /usr/share/maven && \
	cp -r /tmp/apache-maven-3.8.6/* . && \
	ln -s /usr/share/maven/bin/mvn /usr/bin/mvn
}

function knet() {
	url=https://download.visualstudio.microsoft.com/download/pr/dc930bff-ef3d-4f6f-8799-6eb60390f5b4/1efee2a8ea0180c94aff8f15eb3af981/dotnet-sdk-6.0.300-linux-x64.tar.gz
	uname -p |egrep 'x86_64|amd64'
	[ $? -ne 0 ] && url=https://download.visualstudio.microsoft.com/download/pr/7c62b503-4ede-4ff2-bc38-50f250a86d89/3b5e9db04cbe0169e852cb050a0dffce/dotnet-sdk-6.0.300-linux-arm64.tar.gz
	cd /root
	wget -O dotnet.tar.gz ${url} || return 1
	mkdir -p /root/dotnet && tar zxf dotnet.tar.gz -C /root/dotnet || return 2
	export DOTNET_ROOT=/root/dotnet
	/root/dotnet/dotnet tool install --global Microsoft.dotnet-interactive || return 3
	code-server --install-extension ms-dotnettools.vscode-dotnet-runtime || return 4
	code-server --install-extension muhammad-sammy.csharp || return 5
	cd /opt/code/dotnet && /root/dotnet/dotnet restore || echo "notdotnet"
}

function start() {
cd /
bash /opt/autoload/code.sh
sleep 1
pgrep node
[ $? -ne 0 ] && bash /opt/autoload/code.sh
echo "Started"
}

function stop() {
  kill $(pgrep node)
  RET=1
  timeout=10
  t=0
  while [ ${RET} -eq 0 ]
  do
    pgrep node >/dev/null 2>&1
    RET=$?
    if [ ${RET} -eq 0 ]
    then
      t=$(( $t + 1 ))
      [ ${t} -eq ${timeout} ] && kill -9 $(pgrep node)
      sleep 1
    fi
  done
}

optinstall=false
optjava=false
optpython=false
optnode=false
optgo=false
optdotnet=false
optstart=false
optstop=false

while getopts ":ijpgdso" o; do
    case "${o}" in
        i) optinstall=true ;;
        j) optjava=true ;;
        p) optpython=true ;;
        g) optgo=true ;;
        d) optdotnet=true ;;
        s) optstart=true ;;
        o) optstop=true ;;
    esac
done
shift $((OPTIND-1))

mkdir -p /opt/steps
$optinstall && [ ! -f /opt/steps/install ] && install_code && install_start_script && conf_code && touch /opt/steps/install
$optgo && [ ! -f /opt/steps/kgo ] && kgo && touch /opt/steps/kgo
$optpython && [ ! -f /opt/steps/kpython ] && kpython && touch /opt/steps/kpython
$optjava && [ ! -f /opt/steps/kjava ] && kjava && touch /opt/steps/kjava
$optdotnet && [ ! -f /opt/steps/knet ] && knet && touch /opt/steps/knet
$optstop && stop
$optstart && start

# ./install.sh -i -j -p -g -d -s    # install, install kernels, start
# ./install.sh -i                   # install
# ./install.sh -j -p -g -d          # install kernels
# ./install.sh -s                   # start
# ./install.sh -o                   # stop
# ./install.sh -o -s                # restart
`
}
