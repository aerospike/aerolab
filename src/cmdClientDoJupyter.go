package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreateJupyterCmd struct {
	clientCreateBaseCmd
	ConnectCluster TypeClusterName `short:"s" long:"seed" description:"cluster name to prefill as seed node IPs (default seed:127.0.0.1)"`
	Kernels        string          `short:"k" long:"kernels" description:"comma-separated list; options: go,python,java,dotnet; default: all kernels"`
	chDirCmd
}

type clientAddJupyterCmd struct {
	ClientName  TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	osSelectorCmd
	nodes          map[string][]string // destination map[cluster][]nodeIPs
	ConnectCluster TypeClusterName     `short:"s" long:"seed" description:"cluster name to prefill as seed node IPs (default seed:127.0.0.1)"`
	Kernels        string              `short:"k" long:"kernels" description:"comma-separated list; options: go,python,java,dotnet; default: all kernels"`
	Help           helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateJupyterCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroVersion == "latest" {
		c.DistroVersion = "20.04"
	}
	if c.DistroName != TypeDistro("ubuntu") || c.DistroVersion != TypeDistroVersion("20.04") {
		return fmt.Errorf("jupyter is only supported on ubuntu:20.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	nodes := make(map[string][]string)
	var err error
	if c.ConnectCluster == "" {
		c.ConnectCluster = "none"
		nodes["none"] = []string{"127.0.0.1"}
	} else {
		nodes, err = c.checkClustersExist(c.ConnectCluster.String())
		if err != nil {
			return err
		}
	}
	machines, err := c.createBase(args, "jupyter")
	if err != nil {
		return err
	}
	a.opts.Client.Add.Jupyter.nodes = nodes
	a.opts.Client.Add.Jupyter.ClientName = c.ClientName
	a.opts.Client.Add.Jupyter.StartScript = c.StartScript
	a.opts.Client.Add.Jupyter.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.Jupyter.ConnectCluster = c.ConnectCluster
	a.opts.Client.Add.Jupyter.DistroName = c.DistroName
	a.opts.Client.Add.Jupyter.DistroVersion = c.DistroVersion
	a.opts.Client.Add.Jupyter.Kernels = c.Kernels
	return a.opts.Client.Add.Jupyter.addJupyter(args)
}

func (c *clientConfigureJupyterCmd) parseKernelsToSwitches(k string) ([]string, error) {
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
		case "node":
			rval = append(rval, "-n")
		case "dotnet":
			rval = append(rval, "-d")
		default:
			return nil, errors.New("unsupported kernel selected")
		}
	}
	rval = append(rval, "-o", "-s")
	return rval, nil
}

func (c *clientAddJupyterCmd) parseKernelsToSwitches(k string) ([]string, error) {
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
		case "node":
			rval = append(rval, "-n")
		case "dotnet":
			rval = append(rval, "-d")
		default:
			return nil, errors.New("unsupported kernel selected")
		}
	}
	rval = append(rval, "-s")
	return rval, nil
}

// return map[clusterName][]nodeIPs
func (c *clientCreateJupyterCmd) checkClustersExist(clusters string) (map[string][]string, error) {
	return a.opts.Client.Add.Jupyter.checkClustersExist(clusters)
}

// return map[clusterName][]nodeIPs
func (c *clientAddJupyterCmd) checkClustersExist(clusters string) (map[string][]string, error) {
	cnames := []string{}
	clusters = strings.Trim(clusters, "\r\n\t ")
	if len(clusters) > 0 {
		cnames = strings.Split(clusters, ",")
	}
	ret := make(map[string][]string)
	clist, err := b.ClusterList()
	if err != nil {
		return nil, err
	}
	// first pass check clusters exist
	for _, cname := range cnames {
		if !inslice.HasString(clist, cname) {
			return nil, fmt.Errorf("cluster %s does not exist", cname)
		}
	}
	// 2nd pass enumerate node IPs
	for _, cname := range cnames {
		ips, err := b.GetClusterNodeIps(cname)
		if err != nil {
			return nil, err
		}
		ret[cname] = ips
	}
	return ret, nil
}

func (c *clientAddJupyterCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroName != TypeDistro("ubuntu") || c.DistroVersion != TypeDistroVersion("20.04") {
		return fmt.Errorf("jupyter is only supported on ubuntu:20.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	b.WorkOnServers()
	var err error
	if c.ConnectCluster == "" {
		c.ConnectCluster = "none"
		c.nodes = make(map[string][]string)
		c.nodes["none"] = []string{"127.0.0.1"}
	} else {
		c.nodes, err = c.checkClustersExist(c.ConnectCluster.String())
		if err != nil {
			return err
		}
	}
	return c.addJupyter(args)
}

func (c *clientAddJupyterCmd) addJupyter(args []string) error {
	b.WorkOnClients()
	f, err := os.CreateTemp(string(a.opts.Config.Backend.TmpDir), "")
	if err != nil {
		return err
	}
	fn := f.Name()
	_, err = f.WriteString(c.installScript(c.nodes[string(c.ConnectCluster)]))
	f.Close()
	if err != nil {
		return err
	}
	a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
	a.opts.Files.Upload.IsClient = true
	a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
	a.opts.Files.Upload.Files.Source = flags.Filename(fn)
	a.opts.Files.Upload.Files.Destination = flags.Filename("/install.sh")
	a.opts.Files.Upload.doLegacy = true
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
	if a.opts.Config.Backend.Type == "aws" {
		log.Print("NOTE: if allowing for AeroLab to manage AWS Security Group, if not already done so, consider restricting access by using: aerolab config aws lock-security-groups")
	}
	if a.opts.Config.Backend.Type == "gcp" {
		log.Print("NOTE: if not already done so, consider restricting access by using: aerolab config gcp lock-firewall-rules")
	}
	return err
}

func (c *clientAddJupyterCmd) installScript(ips []string) string {
	nips := "\"" + strings.Join(ips, "\" \"") + "\""
	return strings.ReplaceAll(`#!/bin/bash

#host_list=("172.17.0.2" "172.17.0.3")
host_list=(CLUSTERIPLISTHERE)

function kpython() {
cat <<'EOF' > /python.ipynb
{
 "cells": [
  {
   "cell_type": "code",
   "execution_count": 1,
   "id": "349f9b91-189f-4b74-a36e-33ae449bd692",
   "metadata": {},
   "outputs": [],
   "source": [
    "import aerospike\n"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": 2,
   "id": "885e41b3-80dc-43f1-982c-740becc80960",
   "metadata": {},
   "outputs": [
    {
     "data": {
      "text/plain": [
       "<aerospike.Client at 0x157d660>"
      ]
     },
     "execution_count": 2,
     "metadata": {},
     "output_type": "execute_result"
    }
   ],
   "source": [
    "config = {\n",
    "    'hosts': [\n",
REPLACEHOSTSHERE
    "    ],\n",
    "    'policies': {\n",
    "        'timeout': 1000 # milliseconds\n",
    "    }\n",
    "}\n",
    "\n",
    "client = aerospike.client(config)\n",
    "\n",
    "client.connect()"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3 (ipykernel)",
   "language": "python",
   "name": "python3"
  },
  "language_info": {
   "codemirror_mode": {
    "name": "ipython",
    "version": 3
   },
   "file_extension": ".py",
   "mimetype": "text/x-python",
   "name": "python",
   "nbconvert_exporter": "python",
   "pygments_lexer": "ipython3",
   "version": "3.8.10"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
EOF

pip3 install aerospike

for str in ${host_list[@]}; do
  sed -i.bak "s/REPLACEHOSTSHERE/    \"        (\\\\\"${str}\\\\\", 3000),\\\\n\",\nREPLACEHOSTSHERE/g" /python.ipynb
done
sed -i.bak "s/REPLACEHOSTSHERE//g" /python.ipynb
mv /python.ipynb /home/jovyan/python.ipynb
chown -R jovyan /home/jovyan
}

function kjava() {
cat <<'EOF' > /java.ipynb
{
 "cells": [
  {
   "cell_type": "code",
   "execution_count": 0,
   "id": "0794f230",
   "metadata": {
    "vscode": {
     "languageId": "java"
    }
   },
   "outputs": [],
   "source": [
    "import io.github.spencerpark.ijava.IJava;\n",
    "import io.github.spencerpark.jupyter.kernel.magic.common.Shell;\n",
    "IJava.getKernelInstance().getMagics().registerMagics(Shell.class);"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": 0,
   "id": "f1d68081",
   "metadata": {
    "vscode": {
     "languageId": "java"
    }
   },
   "outputs": [],
   "source": [
    "%%loadFromPOM\n",
    "<dependencies>\n",
    "  <dependency>\n",
    "    <groupId>org.slf4j</groupId>\n",
    "    <artifactId>slf4j-simple</artifactId>\n",
    "    <version>1.7.25</version>\n",
    "  </dependency>\n",
    "  <dependency>\n",
    "    <groupId>com.aerospike</groupId>\n",
    "    <artifactId>aerospike-client</artifactId>\n",
    "    <version>6.1.4</version>\n",
    "  </dependency>\n",
    "  <dependency>\n",
    "    <groupId>com.aerospike</groupId>\n",
    "    <artifactId>aerospike-document-api</artifactId>\n",
    "    <version>1.2.0</version>\n",
    "  </dependency>\n",
    "  <dependency>\n",
    "    <groupId>commons-io</groupId>\n",
    "    <artifactId>commons-io</artifactId>\n",
    "    <version>2.6</version>\n",
    "  </dependency>\n",
    "</dependencies>"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": 0,
   "id": "c4ca2e5f",
   "metadata": {
    "vscode": {
     "languageId": "java"
    }
   },
   "outputs": [],
   "source": [
    "import org.apache.commons.io.FileUtils;\n",
    "import java.nio.charset.StandardCharsets;\n",
    "import com.fasterxml.jackson.databind.JsonNode;\n",
    "import com.jayway.jsonpath.JsonPath;\n",
    "\n",
    "import com.aerospike.client.AerospikeClient;\n",
    "import com.aerospike.client.Host;\n",
    "import com.aerospike.client.Key;\n",
    "import com.aerospike.documentapi.*;\n",
    "import com.aerospike.documentapi.JsonConverters;\n",
    "\n",
    "\n",
    "Host[] hosts = new Host[] {\n",
REPLACEHOSTSHERE
    "};\n",
    "\n",
    "AerospikeClient client = new AerospikeClient(null, hosts);\n",
    "System.out.println(\"Initialized Aerospike client and connected to the cluster.\");\n",
    "\n",
    "AerospikeDocumentClient docClient = new AerospikeDocumentClient(client);\n",
    "System.out.println(\"Initialized document client from the Aerospike client.\");"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "0c58179c",
   "metadata": {
    "vscode": {
     "languageId": "java"
    }
   },
   "outputs": [],
   "source": []
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Java",
   "language": "java",
   "name": "java"
  },
  "language_info": {
   "codemirror_mode": "java",
   "file_extension": ".jshell",
   "mimetype": "text/x-java-source",
   "name": "Java",
   "pygments_lexer": "java",
   "version": "17.0.4+8-Ubuntu-122.04"
  },
  "toc": {
   "base_numbering": 1,
   "nav_menu": {},
   "number_sections": true,
   "sideBar": true,
   "skip_h1_title": false,
   "title_cell": "Table of Contents",
   "title_sidebar": "Contents",
   "toc_cell": false,
   "toc_position": {},
   "toc_section_display": true,
   "toc_window_display": false
  }
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
EOF

apt-get update || return
apt-get -y install openjdk-17-jdk || return

wget "https://github.com/SpencerPark/IJava/releases/download/v1.3.0/ijava-1.3.0.zip" -O ijava-kernel.zip || return
unzip ijava-kernel.zip -d ijava-kernel || return
python3 ijava-kernel/install.py --sys-prefix || return
rm -f ijava-kernel.zip

for str in ${host_list[@]}; do
  sed -i.bak "s/REPLACEHOSTSHERE/    \"    new Host(\\\\\"${str}\\\\\", 3000),\\\\n\",\nREPLACEHOSTSHERE/g" /java.ipynb
done
sed -i.bak "s/REPLACEHOSTSHERE//g" /java.ipynb
mv /java.ipynb /home/jovyan/java.ipynb
chown -R jovyan /home/jovyan
}

function knode() {
  cat <<'EOF' > /node.ipynb
{
 "cells": [
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "ceae5b67-d7ee-4a24-bdd3-01ee713bf8bd",
   "metadata": {},
   "outputs": [],
   "source": [
    "const Aerospike = require('aerospike')\n",
    "\n",
    "const config = {\n",
    "  hosts: 'REPLACEHOSTSHERE',\n",
    "  // Timeouts disabled, latency varies with hardware selection. Configure as needed.\n",
    "  policies: {\n",
    "    read : new Aerospike.WritePolicy({socketTimeout : 0, totalTimeout: 0}),\n",
    "    write : new Aerospike.ReadPolicy({socketTimeout : 0, totalTimeout: 0})\n",
    "  }\n",
    "}"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "1694e3dd-09f0-4676-9e14-98950a4b7408",
   "metadata": {},
   "outputs": [],
   "source": [
    ";(async () => {\n",
    "    // Establishes a connection to the server\n",
    "    let client = await Aerospike.connect(config);\n",
    "\n",
    "    //await client.put(key, bins, [], writePolicy);\n",
    "\n",
    "    // Close the connection to the server\n",
    "    client.close();\n",
    "})();"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "97fd1064-58e1-430b-80be-a5437e27f82e",
   "metadata": {},
   "outputs": [],
   "source": []
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "JavaScript (Node.js)",
   "language": "javascript",
   "name": "javascript"
  },
  "language_info": {
   "file_extension": ".js",
   "mimetype": "application/javascript",
   "name": "javascript",
   "version": "16.18.0"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
EOF

cd /
su -c "cd /home/jovyan; npm install aerospike@5.0.3" jovyan || return
npm install -g --unsafe-perm zeromq || return
npm install -g --unsafe-perm ijavascript || return
su -c "ijsinstall --spec-path=full --working-dir=/home/jovyan" jovyan || return

bar=$(printf ",%s" "${host_list[@]}")
bar=${bar:1}
sed -i.bak "s/REPLACEHOSTSHERE/${bar}/g" /node.ipynb
mv /node.ipynb /home/jovyan/node.ipynb
chown -R jovyan /home/jovyan
}

function kgo() {
cat <<'EOF' > /go.ipynb
{
 "cells": [
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "95721125-aebb-48f3-a3c8-2c6c9987fcde",
   "metadata": {},
   "outputs": [],
   "source": [
    "import \"github.com/aerospike/aerospike-client-go/v6\"\n",
    "import \"os\"\n",
    "import \"fmt\""
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "ef6fba52-56bb-48bd-9c72-52630eab7a7f",
   "metadata": {},
   "outputs": [],
   "source": [
    "fmt.Println(\"Starting\")\n",
    "host := \"REPLACEHOSTSHERE\"\n",
    "client, err := aerospike.NewClient(host, 3000)\n",
    "if err != nil {\n",
    "    fmt.Println(err)\n",
    "} else {\n",
    "    fmt.Println(\"Client connected\")\n",
    "}"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "e2721137-7219-4279-b1ab-71435b044c72",
   "metadata": {},
   "outputs": [],
   "source": [
    "count, err := client.WarmUp(100)\n",
    "if err != nil {\n",
    "    fmt.Printf(\"Failed to warm up client: %s\\n\", err)\n",
    "} else {\n",
    "    fmt.Printf(\"Created %v new connections (one would have already existed)\", count)\n",
    "}"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "00ba1de6-b850-470c-9c66-38927760ecf0",
   "metadata": {},
   "outputs": [],
   "source": [
    "client.Close()"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "562c5e55-dc40-4537-9c61-81c4458ffecf",
   "metadata": {},
   "outputs": [],
   "source": []
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Go",
   "language": "go",
   "name": "gophernotes"
  },
  "language_info": {
   "codemirror_mode": "",
   "file_extension": ".go",
   "mimetype": "",
   "name": "go",
   "nbconvert_exporter": "",
   "pygments_lexer": "",
   "version": "go1.19.3"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
EOF

apt-get install -y gcc
url="https://go.dev/dl/go1.19.3.linux-amd64.tar.gz"
uname -p |egrep 'x86_64|amd64'
[ $? -ne 0 ] && url="https://go.dev/dl/go1.19.3.linux-arm64.tar.gz"
cd /
wget -O go.tgz ${url} || return
tar -C /usr/local -xzf go.tgz || return
ln -s /usr/local/go/bin/go /usr/local/bin/go || return
ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt || return
su -c "go install github.com/gopherdata/gophernotes@v0.7.5" jovyan || return
su -c "mkdir -p ~/.local/share/jupyter/kernels/gophernotes" jovyan || return
su -c "cd ~/.local/share/jupyter/kernels/gophernotes && cp \$(go env GOPATH)/pkg/mod/github.com/gopherdata/gophernotes@v0.7.5/kernel/* ." jovyan || return
su -c "cd ~/.local/share/jupyter/kernels/gophernotes && chmod 644 kernel.json && sed \"s_gophernotes_\$(go env GOPATH)/bin/gophernotes_\" kernel.json.in >kernel.json" jovyan || return
chmod 755 -R /home/jovyan/go || return
su -c "cd \$(go env GOPATH)/pkg/mod/github.com/go-zeromq/zmq4@v0.14.1 && go get -u && go mod tidy && cd \$(go env GOPATH)/pkg/mod/github.com/gopherdata/gophernotes@v0.7.5 && go get -u && go mod tidy" jovyan

bar=${host_list[0]}
sed -i.bak "s/REPLACEHOSTSHERE/${bar}/g" /go.ipynb
mv /go.ipynb /home/jovyan/go.ipynb
chown -R jovyan /home/jovyan
}

function knet() {
cat <<'EOF' > /dotnet.ipynb
{
 "cells": [
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "ee0110ac-3a0a-4b16-8b25-48d5232f1538",
   "metadata": {},
   "outputs": [],
   "source": [
    "#r \"nuget:Aerospike.Client\""
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "c1bdd8a9-eb79-4793-b695-be7555814cfc",
   "metadata": {},
   "outputs": [],
   "source": [
    "using Aerospike.Client;\n",
    "\n",
    "string host = new string(\"REPLACEHOSTSHERE\");\n",
    "\n",
    "Host config = new Host(host, 3000);\n",
    "\n",
    "AerospikeClient client = new AerospikeClient(null, config);\n",
    "\n",
    "client.Close();"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "id": "2a4ff983-74e4-4f75-b4c4-efdbd10834d3",
   "metadata": {},
   "outputs": [],
   "source": []
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": ".NET (C#)",
   "language": "C#",
   "name": ".net-csharp"
  },
  "language_info": {
   "file_extension": ".cs",
   "mimetype": "text/x-csharp",
   "name": "C#",
   "pygments_lexer": "csharp",
   "version": "10.0"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
EOF

url=https://download.visualstudio.microsoft.com/download/pr/dc930bff-ef3d-4f6f-8799-6eb60390f5b4/1efee2a8ea0180c94aff8f15eb3af981/dotnet-sdk-6.0.300-linux-x64.tar.gz
uname -p |egrep 'x86_64|amd64'
[ $? -ne 0 ] && url=https://download.visualstudio.microsoft.com/download/pr/7c62b503-4ede-4ff2-bc38-50f250a86d89/3b5e9db04cbe0169e852cb050a0dffce/dotnet-sdk-6.0.300-linux-arm64.tar.gz
cd /home/jovyan
su -c "wget -O dotnet.tar.gz ${url} && \
mkdir -p /home/jovyan/dotnet && tar zxf dotnet.tar.gz -C /home/jovyan/dotnet && \
export DOTNET_ROOT=/home/jovyan/dotnet && \
/home/jovyan/dotnet/dotnet tool install --global Microsoft.dotnet-interactive && \
/home/jovyan/.dotnet/tools/dotnet-interactive jupyter install && \
rm -rf /tmp/NuGetScratch/lock/* && \
rm -f dotnet.tar.gz" jovyan

bar=${host_list[0]}
sed -i.bak "s/REPLACEHOSTSHERE/${bar}/g" /dotnet.ipynb
mv /dotnet.ipynb /home/jovyan/dotnet.ipynb
chown -R jovyan /home/jovyan
}

function install() {
# need curl
apt-get update || exit 2
apt-get -y install curl || exit 3

# Update and install necessary packages
curl -sL https://deb.nodesource.com/setup_16.x | bash - || exit 1

apt-get update || exit 2
apt-get install unzip python-is-python3 python3-pip python3-dev nodejs libuv1 libuv1-dev g++ libssl1.1 libssl-dev libz-dev -y || exit 3

# Add Jupyter user
useradd -m -d /home/jovyan -s /bin/bash jovyan || exit 4
cd /home/jovyan || exit 5

# Install Jupyter
pip3 install jupyterlab || exit 6

# Change ownership of $HOME
chown -R jovyan /home/jovyan || exit 18

# run script
mkdir -p /opt/autoload
cat << 'EOF' > /opt/autoload/start-jupyter.sh
#!/bin/bash
rm -f /var/log/jupyter
touch /var/log/jupyter
chown jovyan /var/log/jupyter
if [ -d /home/jovyan/dotnet ]
then
	su - jovyan -c "export DOTNET_ROOT=/home/jovyan/dotnet && export PATH=$PATH:/home/jovyan/dotnet:/home/jovyan/.dotnet/tools && nohup jupyter lab --no-browser --ip=0.0.0.0 --port=8888 --ServerApp.token='' > /var/log/jupyter 2>&1 &"
else
	su - jovyan -c "nohup jupyter lab --no-browser --ip=0.0.0.0 --port=8888 --ServerApp.token='' > /var/log/jupyter 2>&1 &"
fi
EOF

chmod +x /opt/autoload/start-jupyter.sh
}

function start() {
/opt/autoload/start-jupyter.sh
echo "Started"
}

function stop() {
  kill $(pgrep jupyter-lab)
  RET=1
  timeout=10
  t=0
  while [ ${RET} -eq 0 ]
  do
    pgrep jupyter-lab >/dev/null 2>&1
    RET=$?
    if [ ${RET} -eq 0 ]
    then
      t=$(( $t + 1 ))
      [ ${t} -eq ${timeout} ] && kill -9 $(pgrep jupyter-lab)
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

while getopts ":ijpngdso" o; do
    case "${o}" in
        i) optinstall=true ;;
        j) optjava=true ;;
        p) optpython=true ;;
        n) optnode=true ;;
        g) optgo=true ;;
        d) optdotnet=true ;;
        s) optstart=true ;;
        o) optstop=true ;;
    esac
done
shift $((OPTIND-1))

mkdir -p /opt/steps
$optinstall && [ ! -f /opt/steps/install ] && install && touch /opt/steps/install
$optjava && [ ! -f /opt/steps/kjava ] && kjava && touch /opt/steps/kjava
$optpython && [ ! -f /opt/steps/kpython ] && kpython && touch /opt/steps/kpython
$optnode && [ ! -f /opt/steps/knode ] && knode && touch /opt/steps/knode
$optgo && [ ! -f /opt/steps/kgo ] && kgo && touch /opt/steps/kgo
$optdotnet && [ ! -f /opt/steps/knet ] && knet && touch /opt/steps/knet
$optstop && stop
$optstart && start

# ./install.sh -i -j -p -n -g -d -s # install, install kernels, start
# ./install.sh -i                   # install
# ./install.sh -j -p -n -g -d       # install kernels
# ./install.sh -s                   # start
# ./install.sh -o                   # stop
# ./install.sh -o -s                # restart
`, "CLUSTERIPLISTHERE", nips)
}
