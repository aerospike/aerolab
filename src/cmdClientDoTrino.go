package main

import (
	"fmt"
	"log"
	"os"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreateTrinoCmd struct {
	clientCreateBaseCmd
	ConnectCluster   string `short:"s" long:"seed" description:"seed IP:PORT (can be changed later using client configure command)" default:"127.0.0.1:3000"`
	TrinoVersion     string `long:"trino-version" description:"trino version" default:"399"`
	ConnectorVersion string `long:"connector-version" description:"aerospike connector version" default:"4.2.1-391"`
	chDirCmd
}

type clientAddTrinoCmd struct {
	ClientName  TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	osSelectorCmd
	ConnectCluster   string  `short:"s" long:"seed" description:"seed IP:PORT (can be changed later using client configure command)" default:"127.0.0.1:3000"`
	TrinoVersion     string  `long:"trino-version" description:"trino version" default:"399"`
	ConnectorVersion string  `long:"connector-version" description:"aerospike connector version" default:"4.2.1-391"`
	Help             helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateTrinoCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroVersion == "latest" {
		c.DistroVersion = "22.04"
	}
	if c.DistroName != TypeDistro("ubuntu") || c.DistroVersion != TypeDistroVersion("22.04") {
		return fmt.Errorf("trino is only supported on ubuntu:22.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	machines, err := c.createBase(args, "trino")
	if err != nil {
		return err
	}
	if c.PriceOnly {
		return nil
	}
	a.opts.Client.Add.Trino.ClientName = c.ClientName
	a.opts.Client.Add.Trino.StartScript = c.StartScript
	a.opts.Client.Add.Trino.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.Trino.ConnectCluster = c.ConnectCluster
	a.opts.Client.Add.Trino.DistroName = c.DistroName
	a.opts.Client.Add.Trino.DistroVersion = c.DistroVersion
	a.opts.Client.Add.Trino.ConnectorVersion = c.ConnectorVersion
	a.opts.Client.Add.Trino.TrinoVersion = c.TrinoVersion
	return a.opts.Client.Add.Trino.addTrino(args)
}

func (c *clientAddTrinoCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroVersion == "latest" {
		c.DistroVersion = "22.04"
	}
	if c.DistroName != TypeDistro("ubuntu") || c.DistroVersion != TypeDistroVersion("22.04") {
		return fmt.Errorf("trino is only supported on ubuntu:20.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	return c.addTrino(args)
}

func (c *clientAddTrinoCmd) addTrino(args []string) error {
	b.WorkOnClients()
	f, err := os.CreateTemp(string(a.opts.Config.Backend.TmpDir), "")
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
	nargs := []string{"/bin/bash", "/install.sh", "-t", c.TrinoVersion, "-h", c.ConnectCluster, "-a", c.ConnectorVersion}
	err = a.opts.Attach.Client.run(nargs)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *clientAddTrinoCmd) installScript() string {
	return `#!/bin/bash

if [ ! -f /opt/trino.sh ]
then
cat <<'EOF' > /opt/trino.sh
#!/bin/bash

function stop_process() {
        pname="$1"
    timeout=$2
    tout=0
        pkill -f "${pname}"
        RET=1
        while [ ${RET} -eq 0 ]
        do
                tout=$(( ${tout} + 1 ))
                pgrep -f "${pname}"
                RET=$?
                if [ ${RET} -eq 0 ]
                then
                        [ ${tout} -gt ${timeout} ] && pkill -f -9 "${pname}"
                fi
                sleep 1
        done
}

if [ "$1" = "start" ]
then
  echo "Starting trino"
  /opt/autoload/trino-start.sh
elif [ "$1" = "stop" ]
then
  echo "Stopping trino"
  stop_process java 10
elif [ "$1" = "reconfigure" ]
then
  echo "Reconfiguring trino"
  host_list=$2
  sed -i.bak "s/aerospike.hostlist=.*/aerospike.hostlist=${host_list}/g" /home/trino/trino-server/etc/catalog/aerospike.properties
  $0 stop
  sleep 1
  $0 start
fi
EOF
chmod 755 /opt/trino.sh
fi

# Assign arguments
while getopts t:h:a: flag
do
    case "${flag}" in
        t) trino_version=${OPTARG};; # Trino server version, e.g. 399
        h) host_list=${OPTARG};; # Comma separated list of Aerospike seed nodes, e.g. 172.17.0.3:3000,172.17.0.4:3000
        a) aerospike_trino=${OPTARG};; # Aerospike Trino connector version, e.g. 4.2.1-391 
    esac
done

# Set defaults
if [ -z "$trino_version" ]; then trino_version="399"; fi
if [ -z "$host_list" ]; then host_list="127.0.0.1:3000"; fi
if [ -z "$aerospike_trino" ]; then aerospike_trino="4.2.1-391"; fi

# Split aerospike_trino argument 
aerospike=(${aerospike_trino//-/ })

# Install startup script
mkdir /opt/autoload

cat << EOF > /opt/autoload/trino-start.sh
#!/bin/bash
rm -f /var/log/trino
touch /var/log/trino
chown trino /var/log/trino
# Start trino server
su - trino -c "nohup bash ./trino-server/bin/launcher start > /var/log/trino 2>&1 &"
EOF

chmod +x /opt/autoload/trino-start.sh

# Add trino user
useradd -m -d /home/trino -s /bin/bash trino
cd /home/trino

# Update and install necessary packages
apt-get update
apt-get install unzip openjdk-17-jdk python-is-python3 uuid-runtime -y

# Update limits.conf with suggested limits
sed -i.bak 's/# End of file/trino      soft        nofile      131072\ntrino       hard        nofile      131072\n\n# End of file/' /etc/security/limits.conf

# Get Trino server and install
wget -O trino.tar.gz https://repo1.maven.org/maven2/io/trino/trino-server/$trino_version/trino-server-$trino_version.tar.gz
tar -xvf trino.tar.gz
rm -rf trino.tar.gz
mv trino-server-$trino_version trino-server

# Create necessary server directories
mkdir data trino-server/etc trino-server/etc/catalog

# Add Trino config files
cat << EOF > trino-server/etc/node.properties
node.environment=aerolab
node.id=$(uuidgen)
node.data-dir=/home/trino/data
EOF

cat << EOF > trino-server/etc/jvm.config
-server
-Xmx16G
-XX:InitialRAMPercentage=80
-XX:MaxRAMPercentage=80
-XX:G1HeapRegionSize=32M
-XX:+ExplicitGCInvokesConcurrent
-XX:+ExitOnOutOfMemoryError
-XX:+HeapDumpOnOutOfMemoryError
-XX:-OmitStackTraceInFastThrow
-XX:ReservedCodeCacheSize=512M
-XX:PerMethodRecompilationCutoff=10000
-XX:PerBytecodeRecompilationCutoff=10000
-Djdk.attach.allowAttachSelf=true
-Djdk.nio.maxCachedBufferSize=2000000
-XX:+UnlockDiagnosticVMOptions
-XX:+UseAESCTRIntrinsics
EOF

cat << EOF > trino-server/etc/config.properties
coordinator=true
node-scheduler.include-coordinator=true
http-server.http.port=8080
discovery.uri=http://127.0.0.1:8080
EOF

cat << EOF > trino-server/etc/log.properties
io.trino=DEBUG
EOF

# Get Trino CLI tool
wget -O trino https://repo1.maven.org/maven2/io/trino/trino-cli/$trino_version/trino-cli-$trino_version-executable.jar
chmod +x trino

# Get and install Aerospike connector
wget -O aerospike.zip https://download.aerospike.com/artifacts/enterprise/aerospike-trino/${aerospike[0]}/aerospike-trino-$aerospike_trino.zip
mkdir aerotmp trino-server/plugin/aerospike
unzip aerospike.zip -d aerotmp
find aerotmp/trino-aerospike-$aerospike_trino -name '*.jar' -exec cp {} trino-server/plugin/aerospike \;

mkdir -p /home/trino/schema
cat << EOF > trino-server/etc/catalog/aerospike.properties
connector.name=aerospike
aerospike.hostlist=$host_list
aerospike.split-number=4
aerospike.strict-schemas=false
aerospike.record-key-hidden=false
aerospike.enable-statistics=false
aerospike.insert-require-key=false
aerospike.table-desc-dir=/home/trino/schema
aerospike.clientpolicy.tls.enabled=false
aerospike.scanpolicy.recordsPerSecond=0
aerospike.clientpolicy.timeout=60000
aerospike.clientpolicy.tendInterval=1000
aerospike.clientpolicy.maxSocketIdle=30
aerospike.clientpolicy.sharedThreadPool=true
aerospike.clientpolicy.connPoolsPerNode=1
aerospike.scanpolicy.maxConcurrentNodes=0
aerospike.policy.socketTimeout=600000
aerospike.event-group-size=72
aerospike.clientpolicy.maxConnsPerNode=100
EOF

rm -rf aerotmp aerospike.zip

# Change ownership to trino user
chown -R trino /home/trino

# run
/opt/autoload/trino-start.sh
`
}
