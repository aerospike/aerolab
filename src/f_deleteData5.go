package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerospike-client-go/v5"
)

func (c *config) F_deleteData5() (ret int64, err error) {

	if c.DeleteData.RunDirect == 1 {
		return c.F_deleteData_real5()
	}

	myBinary, _ := os.Executable()
	//myBinary := "/Users/rglonek/Downloads/Aerospike/citrusleaf-tools/opstools/Aero-Lab_Quick_Cluster_Spinup/bin/osx-aio/aerolab"
	if c.DeleteData.LinuxBinaryPath == "" {
		fi, err := os.Stat(myBinary)
		if err != nil {
			myBinary = findExec()
			if myBinary != "" {
				fi, err = os.Stat(myBinary)
				if err != nil {
					return E_BACKEND_ERROR, fmt.Errorf("delete-data: error running stat self: %s", err)
				}
			} else {
				return E_BACKEND_ERROR, fmt.Errorf("delete-data: error running stat self: %s", err)
			}
		}
		size := fi.Size()
		if size > 10*1024*1024 { // if file bigger than 10 MB, this will be aio version
			file, err := os.Open(myBinary)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("delete-data: error opening self for reading: %s", err)
			}

			buf := make([]byte, 19)
			start := size - 19
			_, err = file.ReadAt(buf, start)
			_ = file.Close()
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("delete-data: error reading self: %s", err)
			}
			if string(buf) == "<<<<aerolab-osx-aio" {
				buf = make([]byte, size)
			}
			file, err = os.Open(myBinary)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("delete-data: error reopening self: %s", err)
			}
			nsize, err := file.Read(buf)
			_ = file.Close()
			if nsize != int(size) {
				return E_BACKEND_ERROR, fmt.Errorf("delete-data: could not read the whole aerolab binary. could not read self: %s", err)
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
					return E_BACKEND_ERROR, fmt.Errorf("delete-data: could not open tmp file for writing: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err)
				}
				n, err := nfile.Write(buf[nfound:])
				_ = nfile.Close()
				if err != nil {
					return E_BACKEND_ERROR, fmt.Errorf("delete-data: could not write to tmp file: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err)
				}
				if n != len(buf[nfound:]) {
					return E_BACKEND_ERROR, fmt.Errorf("Could not write binary to temp directory %s fully: size: %d, written: %d", path.Join(os.TempDir(), "aerolab.linux"), len(buf[nfound:]), n)
				}
				c.DeleteData.LinuxBinaryPath = path.Join(os.TempDir(), "aerolab.linux")
				defer func() { _ = os.Remove(path.Join(os.TempDir(), "aerolab.linux")) }()
			}
		}
	}

	if c.DeleteData.LinuxBinaryPath == "" {
		if runtime.GOOS != "linux" {
			return E_BACKEND_ERROR, errors.New("The path to the linux binary when running from MAC cannot be empty, unless you use osx-aio version. LinuxBinaryPath is best set in the aero-lab-common.conf if required :)")
		} else {
			c.DeleteData.LinuxBinaryPath, _ = os.Executable()
		}
	}
	// get backend
	// copy self to the chosen ClusterName_Node
	// run self on the chosen ClusterName_Node with all parameters plus -d 1

	c.log.Info(INFO_SANITY)

	// get backend
	b, err := getBackend(c.DeleteData.DeployOn, c.DeleteData.RemoteHost, c.DeleteData.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("delete-data: getBackend: %s", err)
	}

	contents, err := os.ReadFile(c.DeleteData.LinuxBinaryPath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("delete-data: LinuxBinaryPath read error: %s", err)
	}
	err = b.CopyFilesToCluster(c.DeleteData.ClusterName, []fileList{fileList{"/aerolab.run", contents}}, []int{c.DeleteData.Node})
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("delete-data: backend.CopyFilesToCluster: %s", err)
	}
	err = b.AttachAndRun(c.DeleteData.ClusterName, c.DeleteData.Node, []string{"chmod", "755", "/aerolab.run"})
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("delete-data: backend.AttachAndRun(1): %s", err)
	}
	runCommand := []string{"/aerolab.run"}
	runCommand = append(runCommand, os.Args[1:]...)
	runCommand = append(runCommand, "-d", "1")
	err = b.AttachAndRun(c.DeleteData.ClusterName, c.DeleteData.Node, runCommand)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("delete-data: backend.AttachAndRun(2): %s", err)
	}
	return
}

func (c *config) F_deleteData_real5() (ret int64, err error) {

	ipPort := strings.Split(c.DeleteData.SeedNode, ":")
	if len(ipPort) != 2 {
		return 1, fmt.Errorf("delete-data: Failed to process SeedNode, must be IP:PORT: %s", c.DeleteData.SeedNode)
	}

	port, err := strconv.Atoi(ipPort[1])
	if err != nil {
		return 2, fmt.Errorf("delete-data: Error processing SeedNodePort: %s: %s", ipPort[1], err)
	}

	var client *aerospike.Client
	if c.DeleteData.UserPassword == "" && c.DeleteData.TlsCaCert == "" && c.DeleteData.TlsClientCert == "" {
		client, err = aerospike.NewClient(ipPort[0], port)
	} else {
		policy := aerospike.NewClientPolicy()
		if c.DeleteData.UserPassword != "" {
			up := strings.Split(c.DeleteData.UserPassword, ":")
			if len(up) != 2 {
				err = fmt.Errorf("Username:Password badly formatted, too many or not enough ':'")
				return 3, err
			} else {
				policy.User = up[0]
				policy.Password = up[1]
				if c.DeleteData.AuthType == 1 {
					policy.AuthMode = aerospike.AuthModeExternal
				} else {
					policy.AuthMode = aerospike.AuthModeInternal
				}
			}
		}
		var tlsconfig *tls.Config
		if c.DeleteData.TlsCaCert != "" {
			var cacertpool *x509.CertPool
			ncertfile, err := os.ReadFile(c.DeleteData.TlsCaCert)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("delete-data: could not read ca cert: %s", err)
			}
			cacertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.RootCAs = cacertpool
		}
		if c.DeleteData.TlsClientCert != "" {
			var clientcertpool *x509.CertPool
			ncertfile, err := os.ReadFile(c.DeleteData.TlsClientCert)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("delete-data: could not read client cert: %s", err)
			}
			clientcertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.ClientCAs = clientcertpool
		}
		if c.DeleteData.TlsClientCert != "" || c.DeleteData.TlsCaCert != "" {
			tlsconfig.ServerName = c.DeleteData.TlsServerName
			policy.TlsConfig = tlsconfig
		}
		client, err = aerospike.NewClientWithPolicy(policy, ipPort[0], port)
	}
	if err != nil {
		return 3, fmt.Errorf("delete-data: Error connecting: %s", err)
	}

	client.WarmUp(100)
	rand.Seed(time.Now().UnixNano())

	total := c.DeleteData.PkEndNumber - c.DeleteData.PkStartNumber + 1
	deleted := 0
	var wg chan int
	if c.DeleteData.UseMultiThreaded > 0 {
		wg = make(chan int, c.DeleteData.UseMultiThreaded)
	}
	startTime := time.Now()
	go func() {
		for {
			nsec := int(time.Since(startTime).Seconds())
			var ttkn int
			if nsec == 0 {
				ttkn = 0
			} else {
				ttkn = deleted / nsec
			}
			fmt.Printf("Total records: %d , Deleted: %d , Subthreads running: %d , Records per second: %d                   \r", total, deleted, len(wg), ttkn)
			time.Sleep(time.Second)
		}
	}()
	wp := aerospike.NewWritePolicy(0, aerospike.TTLServerDefault)
	if c.DeleteData.TTL > -1 {
		if c.DeleteData.TTL == 0 {
			wp.Expiration = aerospike.TTLDontExpire
		} else {
			wp.Expiration = uint32(c.DeleteData.TTL)
		}
	}
	wp.TotalTimeout = time.Second * 5
	wp.SocketTimeout = 0
	wp.MaxRetries = 2
	if c.DeleteData.Durable != 0 {
		wp.DurableDelete = true
	}

	for i := c.DeleteData.PkStartNumber; i <= c.DeleteData.PkEndNumber; i++ {

		if c.DeleteData.UseMultiThreaded == 0 {
			ret, err = c.F_deleteData_perform5(i, client, wp)
			if err != nil {
				return ret, fmt.Errorf("delete-data: deleteData_perform: %s", err)
			}
		} else {
			wg <- 1
			go func(i int, client *aerospike.Client) {
				ret, err = c.F_deleteData_perform5(i, client, wp)
				if err != nil {
					c.log.Error("Delete error while multithreading at: deleteData_perform: %s", err)
				}
				<-wg
			}(i, client)
		}

		deleted = deleted + 1
	}
	for len(wg) > 0 {
		time.Sleep(100 * time.Millisecond)
	}
	runTime := time.Since(startTime)
	client.Close()
	if int(runTime.Seconds()) > 0 {
		fmt.Printf("Total records: %d , Deleted: %d                                                                \nTime taken: %s , Records per second: %d\n", total, deleted, runTime.String(), total/int(runTime.Seconds()))
	} else {
		fmt.Printf("Total records: %d , Deleted: %d                                                                \nTime taken: %s , Records per second: %d\n", total, deleted, runTime.String(), total)
	}
	c.log.Info("Done")
	return
}

func (c *config) F_deleteData_perform5(i int, client *aerospike.Client, wp *aerospike.WritePolicy) (ret int64, err error) {
	pk := fmt.Sprintf("%s%d", c.DeleteData.PkPrefix, i)
	key, err := aerospike.NewKey(c.DeleteData.Namespace, c.DeleteData.Set, pk)
	if err != nil {
		return 1, fmt.Errorf("delete-data: aerospike.NewKey error: %s", err)
	}
	_, erra := client.Delete(wp, key)
	for erra != nil {
		if strings.Contains(fmt.Sprint(erra), "i/o timeout") || strings.Contains(fmt.Sprint(erra), "command execution timed out on client") {
			time.Sleep(100 * time.Millisecond)
			_, erra := client.Delete(wp, key)
			if erra != nil {
				c.log.Info("Client.Put error, giving up: %s", erra)
			}
		} else {
			c.log.Info("Client.Put error, giving up: %s", erra)
			return
		}
	}
	return
}
