package main

import (
	Logger "github.com/bestmethod/go-logger"
)

type config struct {
	log              Logger.Logger
	Command          string
	comm             string
	ConfigFiles      []string
	Interactive      interactiveStruct        `type:"command" name:"interactive" method:"F_interactive" description:"Enter interactive mode"`
	MakeCluster      makeClusterStruct        `type:"command" name:"make-cluster" method:"F_makeCluster" description:"Create a new cluster"`
	ClusterStart     clusterStartStruct       `type:"command" name:"cluster-start" method:"F_clusterStart" description:"Start cluster machines"`
	ClusterStop      clusterStopStruct        `type:"command" name:"cluster-stop" method:"F_clusterStop" description:"Stop cluster machines"`
	ClusterDestroy   clusterDestroyStruct     `type:"command" name:"cluster-destroy" method:"F_clusterDestroy" description:"Destroy cluster machines"`
	ClusterList      clusterListStruct        `type:"command" name:"cluster-list" method:"F_clusterList" description:"List currently existing clusters and templates"`
	ClusterGrow      makeClusterStruct        `type:"command" name:"cluster-grow" method:"F_clusterGrow" description:"Deploy more nodes in a specific cluster"`
	UpgradeAerospike upgradeStruct            `type:"command" name:"upgrade-aerospike" method:"F_upgradeAerospike" description:"Upgrade aerospike on a node(s) in a cluster"`
	StartAerospike   startStopAerospikeStruct `type:"command" name:"start-aerospike" method:"F_startAerospike" description:"Start aerospike on cluster/nodes"`
	StopAerospike    startStopAerospikeStruct `type:"command" name:"stop-aerospike" method:"F_stopAerospike" description:"Stop aerospike on cluster/nodes"`
	RestartAerospike startStopAerospikeStruct `type:"command" name:"restart-aerospike" method:"F_restartAerospike" description:"Restart aerospike on cluster/nodes"`
	ConfFixMesh      confFixMeshStruct        `type:"command" name:"conf-fix-mesh" method:"F_confFixMesh" description:"Trigger a function to fix mesh conf in a cluster"`
	NukeTemplate     nukeTemplateStruct       `type:"command" name:"nuke-template" method:"F_nukeTemplate" description:"Destroy a template container"`
	NodeAttach       nodeAttachStruct         `type:"command" name:"node-attach" method:"F_nodeAttach" description:"Attach to a node. Can use tail to execute commands.'"`
	Aql              nodeAttachStruct         `type:"command" name:"aql" method:"F_aql" description:"Run aql on a node. Can use tail to execute commands.'"`
	Asinfo           nodeAttachStruct         `type:"command" name:"asinfo" method:"F_asinfo" description:"Run asinfo on a node. Can use tail to execute commands.'"`
	Asadm            nodeAttachStruct         `type:"command" name:"asadm" method:"F_asadm" description:"Run asadm on a node. Can use tail to execute commands.'"`
	Logs             nodeAttachStruct         `type:"command" name:"logs" method:"F_logs" description:"Get logs from a node which is using journald for logging"`
	ScHelp           scHelpStruct             `type:"command" name:"sc-help" method:"F_scHelp" description:"Strong Consistency cheat-sheet for your namespace needs"`
	GenTlsCerts      genTlsCertsStruct        `type:"command" name:"gen-tls-certs" method:"F_genTlsCerts" description:"Generate TLS certs and put them on the nodes"`
	CopyTlsCerts     copyTlsCertsStruct       `type:"command" name:"copy-tls-certs" method:"F_copyTlsCerts" description:"Copy TLS certs from nodes/clusters to nodes/clusters/clients"`
	XdrConnect       xdrConnectStruct         `type:"command" name:"xdr-connect" method:"F_xdrConnect" description:"Connect 2 or more clusters' chosen namespaces via XDR"`
	MakeXdrClusters  makeXdrClustersStruct    `type:"command" name:"make-xdr-clusters" method:"F_makeXdrClusters" description:"Quickly make clusters and join them with xdr"`
	NetBlock         netControlStruct         `type:"command" name:"net-block" method:"F_netBlock" description:"Block network communications between certain nodes/clusters"`
	NetUnblock       netControlStruct         `type:"command" name:"net-unblock" method:"F_netUnblock" description:"Unblock network communications between certain nodes/clusters"`
	NetList          netListStruct            `type:"command" name:"net-list" method:"F_netList" description:"List network blocks in communications between certain nodes/clusters"`
	NetLoss          netLossStruct            `type:"command" name:"net-loss-delay" method:"F_netLoss" description:"introduce/control/list network packet loss or delay (latency)"`
	Upload           uploadStruct             `type:"command" name:"upload" method:"F_upload" description:"Copy a file to the container"`
	Download         downloadStruct           `type:"command" name:"download" method:"F_download" description:"Copy a file from the container"`
	DeployContainer  deployContainerStruct    `type:"command" name:"deploy-container" method:"F_deployContainer" description:"Deploy an empty ubuntu container"`
	GetLogs          getLogs                  `type:"command" name:"get-logs" method:"F_getLogs" description:"Get logs from nodes in a cluster to a local directory"`
	InsertData       insertDataStruct         `type:"command" name:"insert-data" method:"F_insertData" description:"Insert data into a cluster"`
	DeleteData       deleteDataStruct         `type:"command" name:"delete-data" method:"F_deleteData" description:"Delete data from a cluster"`
	ListVersions     listVersionsStruct       `type:"command" name:"list-versions" method:"F_listVersions" description:"List available Aerospike versions"`
	Help             int                      `type:"command" name:"help" method:"F_help" description:"This help screen"`
	Version          interactiveStruct        `type:"command" name:"version" method:"F_version" description:"Display version information"`
	/*WebInterface     webInterfaceStruct       `type:"command" name:"web-interface" method:"F_webserver" description:"Launch a web interface (webserver) so you can run your aerolab tasks from the browser"`*/
	Common commonConfigStruct
	tail   []string
}

type listVersionsStruct struct {
	Prefix    string `short:"v" long:"version" description:"Version Prefix to search for" default:""`
	Community int    `short:"c" long:"community" description:"Set this switch to list community editions" default:"0" type:"bool"`
	Reverse   int    `short:"r" long:"reverse" description:"Reverse-sort the results" default:"0" type:"bool"`
	Url       int    `short:"l" long:"show-url" description:"Show direct access url instead of version number" default:"0" type:"bool"`
}

type deleteDataStruct struct {
	Namespace               string `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Set                     string `short:"s" long:"set" description:"Set name." default:"myset"`
	PkPrefix                string `short:"p" long:"pk-prefix" description:"Prefix to add to primary key." default:""`
	PkStartNumber           int    `short:"a" long:"pk-start-number" description:"The start ID of the unique PK names" default:"1"`
	PkEndNumber             int    `short:"z" long:"pk-end-number" description:"The end ID of the unique PK names" default:"1000"`
	RunDirect               int    `short:"d" long:"run-direct" description:"If set, will ignore backend, cluster name and node ID and connect to SeedNode directly from running machine. To enable: -d 1" default:"0" type:"bool"`
	UseMultiThreaded        int    `short:"u" long:"multi-thread" description:"If set, will use multithreading. Set to the number of threads you want processing." default:"0"`
	UserPassword            string `short:"q" long:"userpass" description:"If set, will use this user-pass to authenticate to aerospike cluster. Format: username:password" default:""`
	AuthType                int    `short:"Q" long:"auth-external" description:"if set, will use external auth method" default:"0" type:"bool"`
	Version                 string `short:"v" long:"version" description:"which aerospike library version to use: 4|5" default:"5"`
	TlsCaCert               string `short:"y" long:"tls-ca-cert" description:"Tls CA certificate path" default:""`
	TlsClientCert           string `short:"w" long:"tls-client-cert" description:"Tls client cerrtificate path" default:""`
	TlsServerName           string `short:"i" long:"tls-server-name" description:"Tls ServerName" default:""`
	TTL                     int    `short:"T" long:"ttl" description:"set ttl for records. Set to -1 to use server default, 0=don't expire" default:"-1"`
	Durable                 int    `short:"D" long:"durable-delete" description:"if set, will use durable deletes" default:"0" type:"bool"`
	ClusterName             string `short:"n" long:"name" description:"Cluster name of cluster to run aerolab on" default:"mydc"`
	Node                    int    `short:"l" long:"node" description:"Node to run aerolab on to do inserts" default:"1"`
	SeedNode                string `short:"g" long:"seed-node" description:"Seed node IP:PORT. Only use if you are inserting data from different node to another one." default:"127.0.0.1:3000"`
	LinuxBinaryPath         string `short:"t" long:"path" description:"Path to the linux compiled aerolab binary. This is required if -d isn't set, unless using the osx-aio version." default:""`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type netLossStruct struct {
	SourceClusterName       string `short:"s" long:"source" description:"Source Cluster name" default:"mydc"`
	SourceNodeList          string `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	DestinationClusterName  string `short:"d" long:"destination" description:"Destination Cluster name" default:"mydc-xdr"`
	DestinationNodeList     string `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	Action                  string `short:"a" long:"action" description:"One of: set|del|delall|show. delall does not require dest dc, as it removes all rules" default:"show"`
	ShowNames               int    `short:"n" long:"show-names" description:"if action is show, this will cause IPs to resolve to names in output" default:"0" type:"bool"`
	Delay                   string `short:"p" long:"delay" description:"Delay (packet latency), e.g. 100ms or 0.5sec" default:""`
	Loss                    string `short:"L" long:"loss" description:"Network loss in % packets. E.g. 0.1% or 20%" default:""`
	RunOnDestination        int    `short:"D" long:"on-destination" description:"if set, the rules will be created on destination nodes (avoid EPERM on source, true simulation)" default:"0" type:"bool"`
	Rate                    string `short:"R" long:"rate" description:"Max link speed, e.g. 100Kbps" default:""`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

/*
type webInterfaceStruct struct {
	ListenIp      string `short:"l" long:"listen" description:"IP:PORT to listen on" default:"127.0.0.1:8089"`
	BasicAuthUser string `short:"u" long:"user" description:"If set, basic HTTP auth will be required to access the web interface. Set required username" default:""`
	BasicAuthPass string `short:"p" long:"pass" description:"If set, basic HTTP auth will be required to access the web interface. Set required password" default:""`
}
*/

type upgradeStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	DistroName              string `short:"d" long:"distro" description:"OS distro to use. One of: ubuntu, rhel. rhel" default:"ubuntu"`
	DistroVersion           string `short:"i" long:"distro-version" description:"Version of distro. E.g. 7, 6 for RHEL/centos, 20.04, 18.04, 16.04 for ubuntu" default:"best"`
	AerospikeVersion        string `short:"v" long:"aerospike-version" description:"Version of aerospike to use (add 'c' to denote community, e.g. 3.13.0.1c)" default:"latest"`
	AutoStartAerospike      string `short:"s" long:"start" description:"Auto-start aerospike after creation of cluster (y/n)" default:"y"`
	Username                string `short:"U" long:"username" description:"Required for downloading enterprise edition"`
	Password                string `short:"P" long:"password" description:"Required for downloading enterprise edition"`
	ChDir                   string `short:"W" long:"work-dir" description:"Specify working directory. This is where all installers will download and CA certs will initially generate to."`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type insertDataStruct struct {
	Namespace               string `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Set                     string `short:"s" long:"set" description:"Set name. Either 'name' or 'random:SIZE'" default:"myset"`
	Bin                     string `short:"b" long:"bin" description:"Bin name. Either 'static:NAME' or 'unique:PREFIX' or 'random:LENGTH'" default:"static:mybin"`
	BinContents             string `short:"c" long:"bin-contents" description:"Bin contents. Either 'static:NAME' or 'unique:PREFIX' or 'random:LENGTH'" default:"unique:bin_"`
	PkPrefix                string `short:"p" long:"pk-prefix" description:"Prefix to add to primary key." default:""`
	PkStartNumber           int    `short:"a" long:"pk-start-number" description:"The start ID of the unique PK names" default:"1"`
	PkEndNumber             int    `short:"z" long:"pk-end-number" description:"The end ID of the unique PK names" default:"1000"`
	ReadAfterWrite          int    `short:"f" long:"read-after-write" description:"Should we read (get) after write. Set -f 1" default:"0" type:"bool"`
	RunDirect               int    `short:"d" long:"run-direct" description:"If set, will ignore backend, cluster name and node ID and connect to SeedNode directly from running machine. To enable: -d 1" default:"0" type:"bool"`
	UseMultiThreaded        int    `short:"u" long:"multi-thread" description:"If set, will use multithreading. Set to the number of threads you want processing." default:"0"`
	UserPassword            string `short:"q" long:"userpass" description:"If set, will use this user-pass to authenticate to aerospike cluster. Format: username:password" default:""`
	Version                 string `short:"v" long:"version" description:"which aerospike library version to use: 4|5" default:"5"`
	AuthType                int    `short:"Q" long:"auth-external" description:"if set, will use external auth method" default:"0" type:"bool"`
	TlsCaCert               string `short:"y" long:"tls-ca-cert" description:"Tls CA certificate path" default:""`
	TlsClientCert           string `short:"w" long:"tls-client-cert" description:"Tls client cerrtificate path" default:""`
	TlsServerName           string `short:"i" long:"tls-server-name" description:"Tls ServerName" default:""`
	TTL                     int    `short:"T" long:"ttl" description:"set ttl for records. Set to -1 to use server default, 0=don't expire" default:"-1"`
	InsertToNodes           string `short:"N" long:"to-nodes" description:"insert to specific node(s); provide comma-separated node IDs" default:""`
	InsertToPartitions      int    `short:"P" long:"to-partitions" description:"insert to X number of partitions at most. to-partitions/to-nodes=partitions-per-node" default:"0"`
	InsertToPartitionList   string `short:"L" long:"to-partition-list" description:"comma-separated list of partition numbers to insert data to. -P and -L  are ignore if this is specified" default:""`
	ExistsAction            string `short:"E" long:"exists-action" description:"action policy: CREATE_ONLY | REPLACE_ONLY | REPLACE | UPDATE_ONLY | UPDATE" default:""`
	ClusterName             string `short:"n" long:"name" description:"Cluster name of cluster to run aerolab on" default:"mydc"`
	Node                    int    `short:"l" long:"node" description:"Node to run aerolab on to do inserts" default:"1"`
	SeedNode                string `short:"g" long:"seed-node" description:"Seed node IP:PORT. Only use if you are inserting data from different node to another one." default:"127.0.0.1:3000"`
	LinuxBinaryPath         string `short:"t" long:"path" description:"Path to the linux compiled aerolab binary. This is required if -d isn't set, unless using the osx-aio version." default:""`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type copyTlsCertsStruct struct {
	SourceClusterName       string `short:"s" long:"source" description:"Source Cluster name" default:"mydc"`
	SourceNode              int    `short:"l" long:"source-node" description:"Source node from which to copy the TLS certificates" default:"1"`
	DestinationClusterName  string `short:"d" long:"destination" description:"Destination Cluster name." default:"client"`
	DestinationNodeList     string `short:"a" long:"destination-nodes" description:"List of destination nodes to copy the TLS certs to, comma separated. Empty=ALL." default:""`
	TlsName                 string `short:"t" long:"tls-name" description:"Common Name (tlsname) to copy over." default:"tls1"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type deployContainerStruct struct {
	ContainerName           string `short:"n" long:"name" description:"container name" default:"container"`
	ExposePorts             string `short:"p" long:"ports" description:"Which ports to expose, format HOST_PORT:CONTAINER_PORT,HOST_PORT:CONTAINER_PORT,..."`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
	Privileged              int    `short:"B" long:"privileged" description:"Docker only: run container in privileged mode" default:"0" type:"bool"`
	ChDir                   string `short:"W" long:"work-dir" description:"Specify working directory. This is where all installers will download and CA certs will initially generate to."`
}

type getLogs struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	InputFile               string `short:"i" long:"input-file" description:"Path on each node, e.g. /var/log/aerospike.log" default:"/var/log/aerospike.log"`
	OutputDir               string `short:"o" long:"output-dir" description:"Directory to copy the files to on the local machine." default:"./"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type uploadStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	InputFile               string `short:"i" long:"input-file" description:"File to be copied"`
	OutputFile              string `short:"o" long:"output-file" description:"Location to be copied to"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type downloadStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Node                    int    `short:"l" long:"node" description:"Node to copy from." default:"1"`
	InputFile               string `short:"i" long:"input-file" description:"File to be copied"`
	OutputFile              string `short:"o" long:"output-file" description:"Location to be copied to"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type netListStruct struct {
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type netControlStruct struct {
	SourceClusterName       string `short:"s" long:"source" description:"Source Cluster name" default:"mydc"`
	SourceNodeList          string `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	DestinationClusterName  string `short:"d" long:"destination" description:"Destination Cluster name" default:"mydc-xdr"`
	DestinationNodeList     string `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	Type                    string `short:"t" long:"type" description:"Block type (reject|drop)." default:"reject"`
	Ports                   string `short:"p" long:"ports" description:"Comma separated list of ports to block." default:"3000"`
	BlockOn                 string `short:"b" long:"block-on" description:"Block where (input|output). Input=on destination, output=on source." default:"input"`
	StatisticMode           string `short:"M" long:"statistic-mode" description:"for partial packet loss, supported are: random | nth. Not set: drop all packets." default:""`
	StatisticProbability    string `short:"P" long:"probability" description:"for partial packet loss mode random. Supported values are between 0.0 and 1.0 (0% to 100%)" default:"0.5"`
	StatisticEvery          string `short:"E" long:"every" description:"for partial packet loss mode nth. Match one every nth packet. Default: 2 (50% loss)" default:"2"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type xdrConnectStruct struct {
	SourceClusterName       string `short:"s" long:"source" description:"Source Cluster name" default:"mydc"`
	DestinationClusterNames string `short:"d" long:"destinations" description:"Destination Cluster names, comma separated." default:"mydc-xdr"`
	Version                 string `short:"v" long:"xdr-version" description:"specify aerospike xdr configuration version (4|5|auto)" default:"auto"`
	Restart                 string `short:"t" long:"restart-source" description:"restart source nodes after connecting (y/n)" default:"y"`
	Namespaces              string `short:"m" long:"namespaces" description:"Comma-separated list of namespaces to connect." default:"test"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type makeXdrClustersStruct struct {
	SourceClusterName       string `short:"s" long:"source" description:"Source Cluster name" default:"dc1"`
	SourceNodeCount         int    `short:"c" long:"source-node-count" description:"Number of source nodes." default:"2"`
	DestinationClusterNames string `short:"x" long:"destinations" description:"Destination Cluster names, comma separated." default:"dc2"`
	DestinationNodeCount    int    `short:"a" long:"destination-node-count" description:"Number of destination nodes per cluster." default:"2"`
	Namespaces              string `short:"m" long:"namespaces" description:"Comma-separated list of namespaces to connect." default:"test"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
	AerospikeVersion        string `short:"v" long:"aerospike-version" description:"Version of aerospike to use (add 'c' to denote community, e.g. 3.13.0.1c)" default:"latest"`
	DistroName              string `short:"d" long:"distro" description:"OS distro to use. One of: ubuntu, rhel. rhel" default:"ubuntu"`
	DistroVersion           string `short:"i" long:"distro-version" description:"Version of distro. E.g. 7, 6 for RHEL/centos, 20.04, 18.04, 16.04 for ubuntu" default:"best"`
	Restart                 string `short:"t" long:"restart-source" description:"restart source nodes after connecting (y/n)" default:"y"`
	CustomConfigFilePath    string `short:"o" long:"customconf" description:"Custom config file path to install"`
	FeaturesFilePath        string `short:"f" long:"featurefile" description:"Features file to install"`
	AutoStartAerospike      string `short:"S" long:"start" description:"Auto-start aerospike after creation of cluster (y/n)" default:"y"`
	Username                string `short:"U" long:"username" description:"Required for downloading enterprise edition"`
	Password                string `short:"P" long:"password" description:"Required for downloading enterprise edition"`
	Privileged              int    `short:"B" long:"privileged" description:"Docker only: run container in privileged mode" default:"0" type:"bool"`
	ChDir                   string `short:"W" long:"work-dir" description:"Specify working directory. This is where all installers will download and CA certs will initially generate to."`
}

type interactiveStruct struct {
}

type scHelpStruct struct {
	Namespace string `short:"t" long:"namespace" description:"Namespace name" default:"bar"`
}

type genTlsCertsStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	TlsName                 string `short:"t" long:"tls-name" description:"Common Name (tlsname)" default:"tls1"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
	ChDir                   string `short:"W" long:"work-dir" description:"Specify working directory. This is where all installers will download and CA certs will initially generate to."`
}

type nodeAttachStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Node                    string `short:"l" long:"node" description:"Node to attach to (or comma-separated list, when using '-- ...'). Example: 'node-attach --node=all -- /some/command' will execute command on all nodes" default:"1"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type nukeTemplateStruct struct {
	AerospikeVersion        string `short:"v" long:"aerospike-version" description:"Version of aerospike to use (add 'c' to denote community, e.g. 3.13.0.1c) OR 'all'" default:"latest"`
	DistroName              string `short:"d" long:"distro" description:"OS distro to use. One of: ubuntu, rhel. rhel OR 'all'" default:"ubuntu"`
	DistroVersion           string `short:"i" long:"distro-version" description:"Version of distro. E.g. 7, 6 for RHEL/centos, 20.04, 18.04, 16.04 for ubuntu OR 'all'" default:"20.04"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type startStopAerospikeStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type clusterStartStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	NoFixMesh               int    `short:"f" long:"no-fix-mesh" description:"Set to avoid running conf-fix-mesh" default:"0" type:"bool"`
	NoStart                 int    `short:"s" long:"no-start" description:"Set to prevent Aerospike from starting on cluster-start" default:"0" type:"bool"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type clusterStopStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type clusterDestroyStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Nodes                   string `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Force                   int    `short:"f" long:"force" description:"set to --force=1 to force stop before destroy" default:"0" type:"bool"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type clusterListStruct struct {
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type confFixMeshStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
}

type makeClusterStruct struct {
	ClusterName             string `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	NodeCount               int    `short:"c" long:"count" description:"Number of nodes to create" default:"1"`
	AerospikeVersion        string `short:"v" long:"aerospike-version" description:"Aerospike version, full or partial: '5.7.*' to start latest 5.7. (add 'c' to denote community, e.g. 3.13.0.1c)" default:"latest"`
	DistroName              string `short:"d" long:"distro" description:"OS distro to use. One of: ubuntu, rhel. rhel" default:"ubuntu"`
	DistroVersion           string `short:"i" long:"distro-version" description:"Version of distro. E.g. 7, 6 for RHEL/centos, 20.04, 18.04, 16.04 for ubuntu" default:"best"`
	CustomConfigFilePath    string `short:"o" long:"customconf" description:"Custom config file path to install"`
	FeaturesFilePath        string `short:"f" long:"featurefile" description:"Features file to install"`
	HeartbeatMode           string `short:"m" long:"mode" description:"Heartbeat mode, values are: mcast|mesh|default. Default:don't touch" default:"default"`
	MulticastAddress        string `short:"a" long:"mcast-address" description:"Multicast address to change to in config file"`
	MulticastPort           string `short:"p" long:"mcast-port" description:"Multicast port to change to in config file"`
	AutoStartAerospike      string `short:"s" long:"start" description:"Auto-start aerospike after creation of cluster (y/n)" default:"y"`
	ExposePortsToHost       string `short:"h" long:"expose-ports" description:"Only on docker, if a single machine is being deployed, port forward. Format: HOST_PORT:NODE_PORT,HOST_PORT:NODE_PORT" default:""`
	PublicIP                int    `short:"A" long:"aws-public-ip" description:"if set, will install systemd script which will set access-address and alternate-access address to allow public IP connections" default:"0" type:"bool"`
	DeployOn                string `short:"e" long:"deploy-on" description:"Deploy where (aws|docker)" default:""`
	RemoteHost              string `short:"r" long:"remote-host" description:"Remote host to use for deployment, as user@ip:port (empty=locally)"`
	AccessPublicKeyFilePath string `short:"k" long:"pubkey" description:"Public key to use to login to hosts when installing to remote"`
	CpuLimit                string `short:"l" long:"cpu-limit" description:"Impose CPU speed limit. Values acceptable could be '1' or '2' or '0.5' etc." default:""`
	RamLimit                string `short:"t" long:"ram-limit" description:"Limit RAM available to each node, e.g. 500m, or 1g." default:""`
	SwapLimit               string `short:"w" long:"swap-limit" description:"Limit the amount of total memory (ram+swap) each node can use, e.g. 600m. If ram-limit==swap-limit, no swap is available." default:""`
	Username                string `short:"U" long:"username" description:"Required for downloading enterprise edition"`
	Password                string `short:"P" long:"password" description:"Required for downloading enterprise edition"`
	Privileged              int    `short:"B" long:"privileged" description:"Docker only: run container in privileged mode" default:"0" type:"bool"`
	OverrideASClusterName   int    `short:"O" long:"override-as-cluster-name" description:"Override setting a cluster name in the aerospike.conf file" default:"0" type:"bool"`
	ChDir                   string `short:"W" long:"work-dir" description:"Specify working directory. This is where all installers will download and CA certs will initially generate to."`
}

type commonConfigStruct struct {
	ClusterName             string
	Node                    string
	Nodes                   string
	NodeCount               int
	AerospikeVersion        string
	DistroName              string
	DistroVersion           string
	CustomConfigFilePath    string
	FeaturesFilePath        string
	HeartbeatMode           string
	MulticastAddress        string
	MulticastPort           string
	AutoStartAerospike      string
	DeployOn                string
	RemoteHost              string
	AccessPublicKeyFilePath string
	Username                string
	Password                string
	Namespace               string
	ClientName              string
	Language                string
	Force                   int
	TlsName                 string
	SourceClusterName       string
	DestinationClusterNames string
	Namespaces              string
	SourceNodeCount         string
	DestinationNodeCount    string
	InputFile               string
	OutputFile              string
	AmcName                 string
	ExposePorts             string
	AmcVersion              string
	AutoStart               string
	SourceNodeList          string
	DestinationClusterName  string
	DestinationNodeList     string
	Type                    string
	Ports                   string
	BlockOn                 string
	ExposePortsToHost       string
	CpuLimit                string
	RamLimit                string
	SwapLimit               string
	ChDir                   string
	OutputDir               string
	SourceNode              int
	SeedNode                string
	Set                     string
	Bin                     string
	PkPrefix                string
	PkStartNumber           int
	PkEndNumber             int
	ReadAfterWrite          int
	RunDirect               int
	LinuxBinaryPath         string
	UserPassword            string
	Privileged              int
}
