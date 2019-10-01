package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	as "github.com/aerospike/aerospike-client-go"
)

func (c *config) F_insertData() (err error, ret int64) {

	if c.InsertData.RunDirect == 1 {
		return c.F_insertData_real()
	}

	myBinary := os.Args[0]
	//myBinary := "/Users/rglonek/Downloads/Aerospike/citrusleaf-tools/opstools/Aero-Lab_Quick_Cluster_Spinup/bin/osx-aio/aerolab"
	if c.InsertData.LinuxBinaryPath == "" {
		fi, err := os.Stat(myBinary)
		if err != nil {
			return makeError("insert-data: error running stat self: %s", err), E_BACKEND_ERROR
		}
		size := fi.Size()
		if size > 40*1024*1024 { // if file bigger than 40 MB, this will be aio version
			file, err := os.Open(myBinary)
			if err != nil {
				return makeError("insert-data: error opening self for reading: %s", err), E_BACKEND_ERROR
			}

			buf := make([]byte, 19)
			start := size - 19
			_, err = file.ReadAt(buf, start)
			_ = file.Close()
			if err != nil {
				return makeError("insert-data: error reading self: %s", err), E_BACKEND_ERROR
			}
			if string(buf) == "<<<<aerolab-osx-aio" {
				buf = make([]byte, size)
			}
			file, err = os.Open(myBinary)
			if err != nil {
				return makeError("insert-data: error reopening self: %s", err), E_BACKEND_ERROR
			}
			nsize, err := file.Read(buf)
			_ = file.Close()
			if nsize != int(size) {
				return makeError("insert-data: could not read the whole aerolab binary. could not read self: %s", err), E_BACKEND_ERROR
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
					return makeError("insert-data: could not open tmp file for writing: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err), E_BACKEND_ERROR
				}
				n, err := nfile.Write(buf[nfound:])
				_ = nfile.Close()
				if err != nil {
					return makeError("insert-data: could not write to tmp file: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err), E_BACKEND_ERROR
				}
				if n != len(buf[nfound:]) {
					return makeError("Could not write binary to temp directory %s fully: size: %d, written: %d", path.Join(os.TempDir(), "aerolab.linux"), len(buf[nfound:]), n), E_BACKEND_ERROR
				}
				c.InsertData.LinuxBinaryPath = path.Join(os.TempDir(), "aerolab.linux")
				defer func() { _ = os.Remove(path.Join(os.TempDir(), "aerolab.linux")) }()
			}
		}
	}

	if c.InsertData.LinuxBinaryPath == "" {
		if runtime.GOOS != "linux" {
			return errors.New("The path to the linux binary when running from MAC cannot be empty, unless you use osx-aio version. LinuxBinaryPath is best set in the aero-lab-common.conf if required :)"), E_BACKEND_ERROR
		} else {
			c.InsertData.LinuxBinaryPath = os.Args[0]
		}
	}
	// get backend
	// copy self to the chosen ClusterName_Node
	// run self on the chosen ClusterName_Node with all parameters plus -d 1

	c.log.Info(INFO_SANITY)

	// get backend
	b, err := getBackend(c.InsertData.DeployOn, c.InsertData.RemoteHost, c.InsertData.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return makeError("insert-data: getBackend: %s", err), ret
	}

	contents, err := ioutil.ReadFile(c.InsertData.LinuxBinaryPath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return makeError("insert-data: LinuxBinaryPath read error: %s", err), ret
	}
	err = b.CopyFilesToCluster(c.InsertData.ClusterName, []fileList{fileList{"/aerolab.run", contents}}, []int{c.InsertData.Node})
	if err != nil {
		ret = E_BACKEND_ERROR
		return makeError("insert-data: backend.CopyFilesToCluster: %s", err), ret
	}
	err = b.AttachAndRun(c.InsertData.ClusterName, c.InsertData.Node, []string{"chmod", "755", "/aerolab.run"})
	if err != nil {
		ret = E_BACKEND_ERROR
		return makeError("insert-data: backend.AttachAndRun(1): %s", err), ret
	}
	runCommand := []string{"/aerolab.run"}
	runCommand = append(runCommand, os.Args[1:]...)
	runCommand = append(runCommand, "-d", "1")
	err = b.AttachAndRun(c.InsertData.ClusterName, c.InsertData.Node, runCommand)
	if err != nil {
		ret = E_BACKEND_ERROR
		return makeError("insert-data: backend.AttachAndRun(2): %s", err), ret
	}
	return
}

func (c *config) F_insertData_real() (err error, ret int64) {

	ipPort := strings.Split(c.InsertData.SeedNode, ":")
	if len(ipPort) != 2 {
		return makeError("insert-data: Failed to process SeedNode, must be IP:PORT: %s", c.InsertData.SeedNode), 1
	}

	port, err := strconv.Atoi(ipPort[1])
	if err != nil {
		return makeError("insert-data: Error processing SeedNodePort: %s: %s", ipPort[1], err), 2
	}

	var client *as.Client
	if c.InsertData.UserPassword == "" && c.InsertData.TlsCaCert == "" && c.InsertData.TlsClientCert == "" {
		client, err = as.NewClient(ipPort[0], port)
	} else {
		policy := as.NewClientPolicy()
		if c.InsertData.UserPassword != "" {
			up := strings.Split(c.InsertData.UserPassword, ":")
			if len(up) != 2 {
				err = makeError("Username:Password badly formatted, too many or not enough ':'")
				return err, 3
			} else {
				policy.User = up[0]
				policy.Password = up[1]
			}
		}
		var tlsconfig *tls.Config
		if c.InsertData.TlsCaCert != "" {
			var cacertpool *x509.CertPool
			ncertfile, err := ioutil.ReadFile(c.InsertData.TlsCaCert)
			if err != nil {
				return makeError("insert-data: could not read ca cert: %s", err), E_BACKEND_ERROR
			}
			cacertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.RootCAs = cacertpool
		}
		if c.InsertData.TlsClientCert != "" {
			var clientcertpool *x509.CertPool
			ncertfile, err := ioutil.ReadFile(c.InsertData.TlsClientCert)
			if err != nil {
				return makeError("insert-data: could not read client cert: %s", err), E_BACKEND_ERROR
			}
			clientcertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.ClientCAs = clientcertpool
		}
		if c.InsertData.TlsClientCert != "" || c.InsertData.TlsCaCert != "" {
			tlsconfig.ServerName = c.InsertData.TlsServerName
			policy.TlsConfig = tlsconfig
		}
		client, err = as.NewClientWithPolicy(policy, ipPort[0], port)
	}
	if err != nil {
		return errors.New(fmt.Sprintf("insert-data: Error connecting: %s", err)), 3
	}

	rand.Seed(time.Now().UnixNano())

	total := c.InsertData.PkEndNumber - c.InsertData.PkStartNumber + 1
	inserted := 0
	var wg chan int
	if c.InsertData.UseMultiThreaded > 0 {
		wg = make(chan int, c.InsertData.UseMultiThreaded)
	}
	startTime := time.Now()
	go func() {
		for {
			nsec := int(time.Since(startTime).Seconds())
			var ttkn int
			if nsec == 0 {
				ttkn = 0
			} else {
				ttkn = inserted / nsec
			}
			fmt.Printf("Total records: %d , Inserted: %d , Subthreads running: %d , Records per second: %d                   \r", total, inserted, len(wg), ttkn)
			time.Sleep(time.Second)
		}
	}()

	for i := c.InsertData.PkStartNumber; i <= c.InsertData.PkEndNumber; i++ {

		if c.InsertData.UseMultiThreaded == 0 {
			err, ret = c.F_insertData_perform(i, client)
			if err != nil {
				return makeError("insert-data: insertData_perform: %s", err), ret
			}
		} else {
			wg <- 1
			go func(i int, client *as.Client) {
				err, ret = c.F_insertData_perform(i, client)
				if err != nil {
					c.log.Error("Insert error while multithreading at: insertData_perform: %s", err)
				}
				<-wg
			}(i, client)
		}

		inserted = inserted + 1
	}
	for len(wg) > 0 {
		time.Sleep(100 * time.Millisecond)
	}
	runTime := time.Since(startTime)
	client.Close()
	if int(runTime.Seconds()) > 0 {
		fmt.Printf("Total records: %d , Inserted: %d                                                                \nTime taken: %s , Records per second: %d\n", total, inserted, runTime.String(), total/int(runTime.Seconds()))
	} else {
		fmt.Printf("Total records: %d , Inserted: %d                                                                \nTime taken: %s , Records per second: %d\n", total, inserted, runTime.String(), total)
	}
	c.log.Info("Done")
	return
}

func (c *config) F_insertData_perform(i int, client *as.Client) (err error, ret int64) {
	setSplit := strings.Split(c.InsertData.Set, ":")
	var set string
	if len(setSplit) == 1 {
		set = setSplit[0]
	} else if setSplit[0] == "random" {
		setSizeNo, err := strconv.Atoi(setSplit[1])
		if err != nil {
			return errors.New(fmt.Sprintf("insert-data: Error processing set random: %s", err)), 4
		}
		set = RandStringRunes(setSizeNo)
	} else {
		return errors.New(fmt.Sprintf("insert-data: Set name error: %s", setSplit)), 5
	}

	binData := strings.Split(c.InsertData.Bin, ":")
	if len(binData) != 2 {
		return errors.New(fmt.Sprintf("insert-data: Bin data convert error: %s", binData)), 6
	}

	var bin string
	if binData[0] == "static" {
		bin = binData[1]
	} else if binData[0] == "random" {
		binSizeNo, err := strconv.Atoi(binData[1])
		if err != nil {
			return errors.New(fmt.Sprintf("insert-data: Bin Size No error: %s", err)), 7
		}
		bin = RandStringRunes(binSizeNo)
	} else if binData[0] == "unique" {
		bin = fmt.Sprintf("%s%d", binData[1], i)
	} else {
		return errors.New(fmt.Sprintf("insert-data: Bin Name error")), 8
	}

	bincData := strings.Split(c.InsertData.BinContents, ":")
	if len(bincData) != 2 {
		return errors.New(fmt.Sprintf("insert-data: Bin Conents error: %s", err)), 9
	}

	var binc string
	if bincData[0] == "static" {
		binc = bincData[1]
	} else if bincData[0] == "random" {
		bincSizeNo, err := strconv.Atoi(bincData[1])
		if err != nil {
			return errors.New(fmt.Sprintf("insert-data: Bin contents size error: %s", err)), 10
		}
		binc = RandStringRunes(bincSizeNo)
	} else if bincData[0] == "unique" {
		binc = fmt.Sprintf("%s%d", bincData[1], i)
	} else {
		return errors.New(fmt.Sprintf("insert-data: bin contents error")), 11
	}

	var pk string
	pk = fmt.Sprintf("%s%d", c.InsertData.PkPrefix, i)
	key, err := as.NewKey(c.InsertData.Namespace, set, pk)
	if err != nil {
		return errors.New(fmt.Sprintf("insert-data: as.NewKey error: %s", err)), 1
	}
	realBin := as.NewBin(bin, binc)

	nkey := key
	nbin := realBin
	wp := as.WritePolicy{}
	wp.TotalTimeout = time.Second * 5
	wp.SocketTimeout = 0
	wp.MaxRetries = 2
	erra := client.PutBins(&wp, nkey, nbin)
	for erra != nil {
		if strings.Contains(fmt.Sprint(erra), "i/o timeout") || strings.Contains(fmt.Sprint(erra), "command execution timed out on client") {
			time.Sleep(100 * time.Millisecond)
			erra = client.PutBins(&wp, nkey, nbin)
			if erra != nil {
				c.log.Info("Client.Put error, giving up: %s", erra)
			}
		} else {
			c.log.Info("Client.Put error, giving up: %s", erra)
			return
		}
	}
	if c.InsertData.ReadAfterWrite == 1 {
		rp := as.BasePolicy{}
		rp.TotalTimeout = time.Second * 5
		rp.SocketTimeout = 0
		rp.MaxRetries = 2
		_, erra := client.Get(&rp, nkey)
		for erra != nil {
			if strings.Contains(fmt.Sprint(erra), "i/o timeout") || strings.Contains(fmt.Sprint(erra), "command execution timed out on client") {
				time.Sleep(100 * time.Millisecond)
				_, erra = client.Get(&rp, nkey)
				if erra != nil {
					c.log.Info("Client.Get error, giving up: %s", erra)
				}
			} else {
				c.log.Info("Client.Get error, giving up: %s", erra)
				return
			}
		}
	}
	return
}

func RandStringRunes(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
