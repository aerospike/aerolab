package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type dataInsertNsSetCmd struct {
	Namespace string `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Set       string `short:"s" long:"set" description:"Set name. Either 'name' or 'random:SIZE'" default:"myset"`
}

type dataInsertPkCmd struct {
	PkPrefix      string `short:"p" long:"pk-prefix" description:"Prefix to add to primary key." default:""`
	PkStartNumber int    `short:"a" long:"pk-start-number" description:"The start ID of the unique PK names" default:"1"`
	PkEndNumber   int    `short:"z" long:"pk-end-number" description:"The end ID of the unique PK names" default:"1000"`
}

type dataInsertCommonCmd struct {
	RunDirect        bool              `short:"d" long:"run-direct" description:"If set, will ignore backend, cluster name and node ID and connect to SeedNode directly from running machine"`
	UseMultiThreaded int               `short:"u" long:"multi-thread" description:"If set, will use multithreading. Set to the number of threads you want processing." default:"0"`
	Username         string            `short:"U" long:"username" description:"If set, will use this user to authenticate to aerospike cluster" default:""`
	Password         string            `short:"P" long:"password" description:"If set, will use this pass to authenticate to aerospike cluster" default:""`
	Version          TypeClientVersion `short:"v" long:"version" description:"which aerospike library version to use: 4|5|6" default:"6"`
	AuthExternal     bool              `short:"Q" long:"auth-external" description:"if set, will use external auth method"`
	TlsCaCert        string            `short:"y" long:"tls-ca-cert" description:"Tls CA certificate path" default:""`
	TlsClientCert    string            `short:"w" long:"tls-client-cert" description:"Tls client cerrtificate path" default:""`
	TlsServerName    string            `short:"i" long:"tls-server-name" description:"Tls ServerName" default:""`
}

type dataInsertSelectorCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name of cluster to run aerolab on" default:"mydc"`
	Node            TypeNode        `short:"l" long:"node" description:"Node to run aerolab on to do inserts" default:"1"`
	IsClient        bool            `short:"I" long:"client" description:"set to indicate to run on a client machine instead of server node"`
	SeedNode        string          `short:"g" long:"seed-node" description:"Seed node IP:PORT. Only use if you are inserting data from different node to another one." default:"127.0.0.1:3000"`
	LinuxBinaryPath flags.Filename  `short:"t" long:"path" description:"Path to the linux compiled aerolab binary; this should not be required" default:""`
}

type dataInsertCmd struct {
	dataInsertNsSetCmd
	dataInsertPkCmd
	Bin            string `short:"b" long:"bin" description:"Bin name. Either 'static:NAME' or 'unique:PREFIX' or 'random:LENGTH'" default:"static:mybin"`
	BinContents    string `short:"c" long:"bin-contents" description:"Bin contents. Either 'static:NAME' or 'unique:PREFIX' or 'random:LENGTH'" default:"unique:bin_"`
	ReadAfterWrite bool   `short:"f" long:"read-after-write" description:"Should we read (get) after write"`
	dataInsertCommonCmd
	TTL                   int              `short:"T" long:"ttl" description:"set ttl for records. Set to -1 to use server default, 0=don't expire" default:"-1"`
	InsertToNodes         string           `short:"N" long:"to-nodes" description:"insert to specific node(s); provide comma-separated node IDs" default:""`
	InsertToPartitions    int              `short:"C" long:"to-partitions" description:"insert to X number of partitions at most. to-partitions/to-nodes=partitions-per-node" default:"0"`
	InsertToPartitionList string           `short:"L" long:"to-partition-list" description:"comma-separated list of partition numbers to insert data to. -P and -L  are ignored if this is specified" default:""`
	ExistsAction          TypeExistsAction `short:"E" long:"exists-action" description:"action policy: CREATE_ONLY | REPLACE_ONLY | REPLACE | UPDATE_ONLY | UPDATE" default:""`
	dataInsertSelectorCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *dataInsertCmd) Execute(args []string) error {
	return c.insert(args)
}

func (c *dataInsertCmd) insert(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	log.Print("Running data.insert")
	if c.RunDirect {
		log.Print("Insert start")
		defer log.Print("Insert done")
		switch c.Version {
		case "6":
			return c.insert6(args)
		case "5":
			return c.insert5(args)
		case "4":
			return c.insert4(args)
		default:
			return errors.New("aerospike client version does not exist")
		}
	}
	if b == nil {
		return logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		return logFatal("Could not init backend: %s", err)
	}
	log.Print("Unpacking start")
	if err := c.unpack(args); err != nil {
		return err
	}
	log.Print("Unpacking done")
	return nil
}

func (c *dataInsertSelectorCmd) unpack(args []string) error {
	if c.IsClient {
		b.WorkOnClients()
	}
	isArm, err := b.IsNodeArm(string(c.ClusterName), int(c.Node))
	if err != nil {
		return err
	}
	if c.LinuxBinaryPath == "" {
		if runtime.GOOS == "linux" && ((runtime.GOARCH == "amd64" && !isArm) || (runtime.GOARCH == "arm64" && isArm)) {
			// the arch of this binary is the same as the arch of the destination system, just point at self
			myBinary, err := findExec()
			if err != nil {
				return fmt.Errorf("failed to find self: %s", err)
			}
			c.LinuxBinaryPath = flags.Filename(myBinary)
		} else {
			nLinuxBinary := nLinuxBinaryX64
			if isArm {
				nLinuxBinary = nLinuxBinaryArm64
			}
			nfile, err := os.OpenFile(path.Join(os.TempDir(), "aerolab.linux"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("insert-data: could not open tmp file for writing: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err)
			}
			defer nfile.Close()
			myBinary, err := base64.StdEncoding.DecodeString(nLinuxBinary)
			if err != nil {
				return fmt.Errorf("failed to unpack embedded linux binary: %s", err)
			}
			_, err = nfile.Write(myBinary)
			if err != nil {
				return fmt.Errorf("failed to unpack-write embedded linux binary: %s", err)
			}
			c.LinuxBinaryPath = flags.Filename(path.Join(os.TempDir(), "aerolab.linux"))
		}
	}

	// copy self to the chosen ClusterName_Node
	// run self on the chosen ClusterName_Node with all parameters plus -d 1
	stat, err := os.Stat(string(c.LinuxBinaryPath))
	pfilelen := 0
	if err != nil {
		return err
	}
	pfilelen = int(stat.Size())
	contents, err := os.Open(string(c.LinuxBinaryPath))
	if err != nil {
		return fmt.Errorf("insert-data: LinuxBinaryPath read error: %s", err)
	}
	defer contents.Close()
	cfgFile, _, err := a.configFileName()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfgFile); err == nil {
		stat, err := os.Stat(cfgFile)
		cfilelen := 0
		if err != nil {
			return err
		}
		cfilelen = int(stat.Size())
		cfgContents, err := os.Open(cfgFile)
		if err != nil {
			return fmt.Errorf("insert-data: cfgFile read error: %s", err)
		}
		defer contents.Close()
		err = b.CopyFilesToCluster(string(c.ClusterName), []fileList{fileList{"/root/.aerolab.conf", cfgContents, cfilelen}}, []int{c.Node.Int()})
		if err != nil {
			return fmt.Errorf("insert-data: cfgFile backend.CopyFilesToCluster: %s", err)
		}
	}
	err = b.CopyFilesToCluster(string(c.ClusterName), []fileList{fileList{"/aerolab.run", contents, pfilelen}}, []int{c.Node.Int()})
	if err != nil {
		return fmt.Errorf("insert-data: backend.CopyFilesToCluster: %s", err)
	}
	err = b.AttachAndRun(string(c.ClusterName), c.Node.Int(), []string{"chmod", "755", "/aerolab.run"})
	if err != nil {
		return fmt.Errorf("insert-data: backend.AttachAndRun(1): %s", err)
	}
	runCommand := []string{"/aerolab.run"}
	runCommand = append(runCommand, os.Args[1:]...)
	runCommand = append(runCommand, "-d", "1")
	err = b.AttachAndRun(string(c.ClusterName), c.Node.Int(), runCommand)
	if err != nil {
		return fmt.Errorf("insert-data: backend.AttachAndRun(2): %s", err)
	}
	return nil
}

func findExec() (string, error) {
	ex, err := os.Executable()
	if err != nil {
		return "", err
	}

	exReal, err := filepath.EvalSymlinks(ex)
	if err != nil {
		return "", err
	}
	return exReal, nil
}

func RandStringRunes(n int, src rand.Source) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const (
		letterIdxBits = 6                    // 6 bits to represent a letter index
		letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
		letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	)
	sb := strings.Builder{}
	sb.Grow(n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}
