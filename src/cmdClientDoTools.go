package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreateToolsCmd struct {
	clientCreateBaseCmd
	CustomToolsFilePath flags.Filename `short:"z" long:"toolsconf" description:"Custom astools config file path to install"`
	aerospikeVersionCmd
	chDirCmd
}

type clientAddToolsCmd struct {
	ClientName          TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines            TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	CustomToolsFilePath flags.Filename `short:"z" long:"toolsconf" description:"Custom astools config file path to install"`
	StartScript         flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	aerospikeVersionCmd
	osSelectorCmd
	chDirCmd
	Aws  clientAddToolsAwsCmd `no-flag:"true"`
	Help helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddToolsAwsCmd struct {
	IsArm bool `long:"arm" hidden:"true" description:"indicate installing on an arm instance"`
}

func init() {
	addBackendSwitch("client.add.tools", "aws", &a.opts.Client.Add.Tools.Aws)
}

func (c *clientCreateToolsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	var err error
	if string(c.CustomToolsFilePath) != "" {
		if _, err := os.Stat(string(c.CustomToolsFilePath)); os.IsNotExist(err) {
			return logFatal("File %s does not exist", string(c.CustomToolsFilePath))
		}
	}
	// arm fill
	c.Aws.IsArm, err = b.IsSystemArm(c.Aws.InstanceType)
	if err != nil {
		return fmt.Errorf("IsSystemArm check: %s", err)
	}

	isArm := c.Aws.IsArm
	if a.opts.Config.Backend.Type == "docker" {
		if b.Arch() == TypeArchArm {
			isArm = true
		} else {
			isArm = false
		}
	}
	bv := &backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String(), isArm}
	if strings.HasPrefix(c.AerospikeVersion.String(), "latest") || strings.HasSuffix(c.AerospikeVersion.String(), "*") || strings.HasPrefix(c.DistroVersion.String(), "latest") {
		_, err := aerospikeGetUrl(bv, c.Username, c.Password)
		if err != nil {
			return fmt.Errorf("aerospike Version not found: %s", err)
		}
		c.AerospikeVersion = TypeAerospikeVersion(bv.aerospikeVersion)
		c.DistroName = TypeDistro(bv.distroName)
		c.DistroVersion = TypeDistroVersion(bv.distroVersion)
	}

	machines, err := c.createBase(args, "tools")
	if err != nil {
		return err
	}
	a.opts.Client.Add.Tools.ClientName = c.ClientName
	a.opts.Client.Add.Tools.StartScript = c.StartScript
	a.opts.Client.Add.Tools.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.Tools.Username = c.Username
	a.opts.Client.Add.Tools.Password = c.Password
	a.opts.Client.Add.Tools.AerospikeVersion = c.AerospikeVersion
	a.opts.Client.Add.Tools.DistroName = c.DistroName
	a.opts.Client.Add.Tools.DistroVersion = c.DistroVersion
	a.opts.Client.Add.Tools.ChDir = c.ChDir
	a.opts.Client.Add.Tools.Aws.IsArm = c.Aws.IsArm
	a.opts.Client.Add.Tools.CustomToolsFilePath = c.CustomToolsFilePath
	return a.opts.Client.Add.Tools.addTools(args)
}

func (c *clientAddToolsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.addTools(args)
}

func (c *clientAddToolsCmd) addTools(args []string) error {
	b.WorkOnClients()
	isArm := c.Aws.IsArm
	if a.opts.Config.Backend.Type == "docker" {
		if b.Arch() == TypeArchArm {
			isArm = true
		} else {
			isArm = false
		}
	}
	if string(c.CustomToolsFilePath) != "" {
		if _, err := os.Stat(string(c.CustomToolsFilePath)); os.IsNotExist(err) {
			return logFatal("File %s does not exist", string(c.CustomToolsFilePath))
		}
	}
	a.opts.Installer.Download.AerospikeVersion = c.AerospikeVersion
	a.opts.Installer.Download.ChDir = c.ChDir
	a.opts.Installer.Download.DistroName = c.DistroName
	a.opts.Installer.Download.DistroVersion = c.DistroVersion
	a.opts.Installer.Download.Password = c.Password
	a.opts.Installer.Download.Username = c.Username
	a.opts.Installer.Download.IsArm = isArm
	fn, err := a.opts.Installer.Download.runDownload(args)
	if err != nil {
		return err
	}
	a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
	a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
	a.opts.Files.Upload.Files.Source = flags.Filename(fn)
	a.opts.Files.Upload.Files.Destination = flags.Filename("/opt/installer.tgz")
	a.opts.Files.Upload.IsClient = true
	err = a.opts.Files.Upload.runUpload(args)
	if err != nil {
		return err
	}
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "cd /opt && tar -zxvf installer.tgz && cd aerospike-server-* ; ./asinstall"})
	if err != nil {
		return err
	}

	// add asbench wrapper script
	runasbench := runAsbenchScript()
	f, err := os.CreateTemp(string(a.opts.Config.Backend.TmpDir), "runasbench-")
	if err != nil {
		return fmt.Errorf("could not create a temp file for asbench wrapper: %s", err)
	}
	fName := f.Name()
	defer os.Remove(fName)
	_, err = f.WriteString(runasbench)
	f.Close()
	if err != nil {
		return fmt.Errorf("could not write a temp file for asbench wrapper: %s", err)
	}
	a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
	a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
	a.opts.Files.Upload.Files.Source = flags.Filename(fName)
	a.opts.Files.Upload.Files.Destination = flags.Filename("/usr/bin/run_asbench")
	a.opts.Files.Upload.IsClient = true
	err = a.opts.Files.Upload.runUpload(args)
	if err != nil {
		return err
	}
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "chmod 755 /usr/bin/run_asbench"})
	if err != nil {
		return err
	}

	// upload custom tools
	if string(c.CustomToolsFilePath) != "" {
		a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
		a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
		a.opts.Files.Upload.Files.Source = c.CustomToolsFilePath
		a.opts.Files.Upload.Files.Destination = flags.Filename("/etc/aerospike/astools.conf")
		a.opts.Files.Upload.IsClient = true
		err = a.opts.Files.Upload.runUpload(args)
		if err != nil {
			return err
		}
	}

	// install early/late scripts
	if string(c.StartScript) != "" {
		a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
		a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
		a.opts.Files.Upload.Files.Source = flags.Filename(c.StartScript)
		a.opts.Files.Upload.Files.Destination = flags.Filename("/usr/local/bin/start.sh")
		a.opts.Files.Upload.IsClient = true
		err = a.opts.Files.Upload.runUpload(args)
		if err != nil {
			return err
		}
	}
	log.Print("Done")
	return nil
}

func runAsbenchScript() string {
	return `EXTRAS=""
echo "$@" |grep -- '--latency' >/dev/null 2>&1
[ $? -ne 0 ] && EXTRAS="--latency"
echo "$@" |grep -- '--percentiles' >/dev/null 2>&1
if [ $? -ne 0 ]
then
  EXTRAS="${EXTRAS} --percentiles 50,90,99,99.9,99.99"
else
  echo "$@" |grep ' 50,90,99,99.9,99.99' >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    echo "WARNING: changing the first 5 percentile buckets will cause asbench latency graphs in AMS dashboard to be incorrect"
  fi
fi
NO=$(pidof asbench |sed 's/ /\n/g' |wc -l)
touch /var/log/asbench_${NO}.log
nohup asbench "$@" ${EXTRAS} >>/var/log/asbench_${NO}.log 2>&1 &
pkill promtail >/dev/null 2>&1
if [ -f /opt/autoload/10-promtail ] 
then 
  /opt/autoload/10-promtail
else
  exit 0
fi
`
}
