package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
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
	RunDirect        bool   `short:"d" long:"run-direct" description:"If set, will ignore backend, cluster name and node ID and connect to SeedNode directly from running machine"`
	UseMultiThreaded int    `short:"u" long:"multi-thread" description:"If set, will use multithreading. Set to the number of threads you want processing." default:"0"`
	Username         string `short:"U" long:"username" description:"If set, will use this user to authenticate to aerospike cluster" default:""`
	Password         string `short:"P" long:"password" description:"If set, will use this pass to authenticate to aerospike cluster" default:""`
	Version          string `short:"v" long:"version" description:"which aerospike library version to use: 4|5|6" default:"6"`
	AuthExternal     bool   `short:"Q" long:"auth-external" description:"if set, will use external auth method"`
	TlsCaCert        string `short:"y" long:"tls-ca-cert" description:"Tls CA certificate path" default:""`
	TlsClientCert    string `short:"w" long:"tls-client-cert" description:"Tls client cerrtificate path" default:""`
	TlsServerName    string `short:"i" long:"tls-server-name" description:"Tls ServerName" default:""`
}

type dataInsertSelectorCmd struct {
	ClusterName     string `short:"n" long:"name" description:"Cluster name of cluster to run aerolab on" default:"mydc"`
	Node            int    `short:"l" long:"node" description:"Node to run aerolab on to do inserts" default:"1"`
	SeedNode        string `short:"g" long:"seed-node" description:"Seed node IP:PORT. Only use if you are inserting data from different node to another one." default:"127.0.0.1:3000"`
	LinuxBinaryPath string `short:"t" long:"path" description:"Path to the linux compiled aerolab binary; this should not be required" default:""`
}

type dataInsertCmd struct {
	dataInsertNsSetCmd
	dataInsertPkCmd
	Bin            string `short:"b" long:"bin" description:"Bin name. Either 'static:NAME' or 'unique:PREFIX' or 'random:LENGTH'" default:"static:mybin"`
	BinContents    string `short:"c" long:"bin-contents" description:"Bin contents. Either 'static:NAME' or 'unique:PREFIX' or 'random:LENGTH'" default:"unique:bin_"`
	ReadAfterWrite bool   `short:"f" long:"read-after-write" description:"Should we read (get) after write"`
	dataInsertCommonCmd
	TTL                   int    `short:"T" long:"ttl" description:"set ttl for records. Set to -1 to use server default, 0=don't expire" default:"-1"`
	InsertToNodes         string `short:"N" long:"to-nodes" description:"insert to specific node(s); provide comma-separated node IDs" default:""`
	InsertToPartitions    int    `short:"C" long:"to-partitions" description:"insert to X number of partitions at most. to-partitions/to-nodes=partitions-per-node" default:"0"`
	InsertToPartitionList string `short:"L" long:"to-partition-list" description:"comma-separated list of partition numbers to insert data to. -P and -L  are ignored if this is specified" default:""`
	ExistsAction          string `short:"E" long:"exists-action" description:"action policy: CREATE_ONLY | REPLACE_ONLY | REPLACE | UPDATE_ONLY | UPDATE" default:""`
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
		logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		logFatal("Could not init backend: %s", err)
	}
	log.Print("Unpacking start")
	if err := c.unpack(args); err != nil {
		return err
	}
	log.Print("Unpacking done")
	return nil
}

func (c *dataInsertSelectorCmd) unpack(args []string) error {
	myBinary, _ := os.Executable()
	if c.LinuxBinaryPath == "" {
		fi, err := os.Stat(myBinary)
		if err != nil {
			myBinary = findExec()
			if myBinary != "" {
				fi, err = os.Stat(myBinary)
				if err != nil {
					return fmt.Errorf("insert-data: error running stat self: %s", err)
				}
			} else {
				return fmt.Errorf("insert-data: error running stat self: %s", err)
			}
		}
		size := fi.Size()
		file, err := os.Open(myBinary)
		if err != nil {
			return fmt.Errorf("insert-data: error opening self for reading: %s", err)
		}

		buf := make([]byte, 19)
		start := size - 19
		_, err = file.ReadAt(buf, start)
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("insert-data: error reading self: %s", err)
		}
		if string(buf) == "<<<<aerolab-osx-aio" {
			buf = make([]byte, size)
		}
		file, err = os.Open(myBinary)
		if err != nil {
			return fmt.Errorf("insert-data: error reopening self: %s", err)
		}
		nsize, err := file.Read(buf)
		_ = file.Close()
		if err != nil || nsize != int(size) {
			return fmt.Errorf("insert-data: could not read the whole aerolab binary. could not read self: %s", err)
		}
		comp := make([]byte, 23)
		comp[0] = '>'
		comp[1] = '>'
		comp[2] = '>'
		comp[3] = '>'
		comp[4] = 'a'
		comp[5] = 'e'
		comp[6] = 'r'
		comp[7] = 'o'
		comp[8] = 'l'
		comp[9] = 'a'
		comp[10] = 'b'
		comp[11] = '-'
		comp[12] = 'o'
		comp[13] = 's'
		comp[14] = 'x'
		comp[15] = '-'
		comp[16] = 'a'
		comp[17] = 'i'
		comp[18] = 'o'
		comp[19] = '>'
		comp[20] = '>'
		comp[21] = '>'
		comp[22] = '>'
		nfound := -1
		for offs := range buf {
			found := offs
			for offc := range comp {
				if buf[offs+offc] != comp[offc] {
					found = -1
					break
				}
			}
			if found != -1 {
				nfound = found + 23
				break
			}
		}
		if nfound != -1 {
			// buf[nfound] is the start of the buffer location where we have the linux file from!
			nfile, err := os.OpenFile(path.Join(os.TempDir(), "aerolab.linux"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("insert-data: could not open tmp file for writing: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err)
			}
			n, err := nfile.Write(buf[nfound:])
			_ = nfile.Close()
			if err != nil {
				return fmt.Errorf("insert-data: could not write to tmp file: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err)
			}
			if n != len(buf[nfound:]) {
				return fmt.Errorf("could not write binary to temp directory %s fully: size: %d, written: %d", path.Join(os.TempDir(), "aerolab.linux"), len(buf[nfound:]), n)
			}
			c.LinuxBinaryPath = path.Join(os.TempDir(), "aerolab.linux")
			defer func() { _ = os.Remove(path.Join(os.TempDir(), "aerolab.linux")) }()
		}
	}

	if c.LinuxBinaryPath == "" {
		if runtime.GOOS != "linux" {
			return errors.New("the path to the linux binary when running from MAC cannot be empty")
		} else {
			c.LinuxBinaryPath, _ = os.Executable()
		}
	}
	// copy self to the chosen ClusterName_Node
	// run self on the chosen ClusterName_Node with all parameters plus -d 1
	stat, err := os.Stat(c.LinuxBinaryPath)
	pfilelen := 0
	if err != nil {
		return err
	}
	pfilelen = int(stat.Size())
	contents, err := os.Open(c.LinuxBinaryPath)
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
		err = b.CopyFilesToCluster(c.ClusterName, []fileList{fileList{"/root/.aerolab.conf", cfgContents, cfilelen}}, []int{c.Node})
		if err != nil {
			return fmt.Errorf("insert-data: cfgFile backend.CopyFilesToCluster: %s", err)
		}
	}
	err = b.CopyFilesToCluster(c.ClusterName, []fileList{fileList{"/aerolab.run", contents, pfilelen}}, []int{c.Node})
	if err != nil {
		return fmt.Errorf("insert-data: backend.CopyFilesToCluster: %s", err)
	}
	err = b.AttachAndRun(c.ClusterName, c.Node, []string{"chmod", "755", "/aerolab.run"})
	if err != nil {
		return fmt.Errorf("insert-data: backend.AttachAndRun(1): %s", err)
	}
	runCommand := []string{"/aerolab.run"}
	runCommand = append(runCommand, os.Args[1:]...)
	runCommand = append(runCommand, "-d", "1")
	err = b.AttachAndRun(c.ClusterName, c.Node, runCommand)
	if err != nil {
		return fmt.Errorf("insert-data: backend.AttachAndRun(2): %s", err)
	}
	return nil
}

func findExec() string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}

	exReal, err := filepath.EvalSymlinks(ex)
	if err != nil {
		return ""
	}
	return exReal
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
