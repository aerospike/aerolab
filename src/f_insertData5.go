package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
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

	"github.com/aerospike/aerospike-client-go/v5"
)

func (c *config) F_insertData5() (ret int64, err error) {

	if c.InsertData.RunDirect == 1 {
		return c.F_insertData_real5()
	}

	myBinary, _ := os.Executable()
	//myBinary := "/Users/rglonek/Downloads/Aerospike/citrusleaf-tools/opstools/Aero-Lab_Quick_Cluster_Spinup/bin/osx-aio/aerolab"
	if c.InsertData.LinuxBinaryPath == "" {
		fi, err := os.Stat(myBinary)
		if err != nil {
			myBinary = findExec()
			if myBinary != "" {
				fi, err = os.Stat(myBinary)
				if err != nil {
					return E_BACKEND_ERROR, fmt.Errorf("insert-data: error running stat self: %s", err)
				}
			} else {
				return E_BACKEND_ERROR, fmt.Errorf("insert-data: error running stat self: %s", err)
			}
		}
		size := fi.Size()
		if size > 10*1024*1024 { // if file bigger than 10 MB, this will be aio version
			file, err := os.Open(myBinary)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("insert-data: error opening self for reading: %s", err)
			}

			buf := make([]byte, 19)
			start := size - 19
			_, err = file.ReadAt(buf, start)
			_ = file.Close()
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("insert-data: error reading self: %s", err)
			}
			if string(buf) == "<<<<aerolab-osx-aio" {
				buf = make([]byte, size)
			}
			file, err = os.Open(myBinary)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("insert-data: error reopening self: %s", err)
			}
			nsize, err := file.Read(buf)
			_ = file.Close()
			if nsize != int(size) {
				return E_BACKEND_ERROR, fmt.Errorf("insert-data: could not read the whole aerolab binary. could not read self: %s", err)
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
					return E_BACKEND_ERROR, fmt.Errorf("insert-data: could not open tmp file for writing: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err)
				}
				n, err := nfile.Write(buf[nfound:])
				_ = nfile.Close()
				if err != nil {
					return E_BACKEND_ERROR, fmt.Errorf("insert-data: could not write to tmp file: %s: %s", path.Join(os.TempDir(), "aerolab.linux"), err)
				}
				if n != len(buf[nfound:]) {
					return E_BACKEND_ERROR, fmt.Errorf("Could not write binary to temp directory %s fully: size: %d, written: %d", path.Join(os.TempDir(), "aerolab.linux"), len(buf[nfound:]), n)
				}
				c.InsertData.LinuxBinaryPath = path.Join(os.TempDir(), "aerolab.linux")
				defer func() { _ = os.Remove(path.Join(os.TempDir(), "aerolab.linux")) }()
			}
		}
	}

	if c.InsertData.LinuxBinaryPath == "" {
		if runtime.GOOS != "linux" {
			return E_BACKEND_ERROR, errors.New("The path to the linux binary when running from MAC cannot be empty, unless you use osx-aio version. LinuxBinaryPath is best set in the aero-lab-common.conf if required :)")
		} else {
			c.InsertData.LinuxBinaryPath, _ = os.Executable()
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
		return ret, fmt.Errorf("insert-data: getBackend: %s", err)
	}

	contents, err := ioutil.ReadFile(c.InsertData.LinuxBinaryPath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("insert-data: LinuxBinaryPath read error: %s", err)
	}
	err = b.CopyFilesToCluster(c.InsertData.ClusterName, []fileList{fileList{"/aerolab.run", contents}}, []int{c.InsertData.Node})
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("insert-data: backend.CopyFilesToCluster: %s", err)
	}
	err = b.AttachAndRun(c.InsertData.ClusterName, c.InsertData.Node, []string{"chmod", "755", "/aerolab.run"})
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("insert-data: backend.AttachAndRun(1): %s", err)
	}
	runCommand := []string{"/aerolab.run"}
	runCommand = append(runCommand, os.Args[1:]...)
	runCommand = append(runCommand, "-d", "1")
	err = b.AttachAndRun(c.InsertData.ClusterName, c.InsertData.Node, runCommand)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, fmt.Errorf("insert-data: backend.AttachAndRun(2): %s", err)
	}
	return
}

func (c *config) F_insertData_real5() (ret int64, err error) {

	ipPort := strings.Split(c.InsertData.SeedNode, ":")
	if len(ipPort) != 2 {
		return 1, fmt.Errorf("insert-data: Failed to process SeedNode, must be IP:PORT: %s", c.InsertData.SeedNode)
	}

	port, err := strconv.Atoi(ipPort[1])
	if err != nil {
		return 2, fmt.Errorf("insert-data: Error processing SeedNodePort: %s: %s", ipPort[1], err)
	}

	var client *aerospike.Client
	if c.InsertData.UserPassword == "" && c.InsertData.TlsCaCert == "" && c.InsertData.TlsClientCert == "" {
		client, err = aerospike.NewClient(ipPort[0], port)
	} else {
		policy := aerospike.NewClientPolicy()
		if c.InsertData.UserPassword != "" {
			up := strings.Split(c.InsertData.UserPassword, ":")
			if len(up) != 2 {
				err = fmt.Errorf("Username:Password badly formatted, too many or not enough ':'")
				return 3, err
			} else {
				policy.User = up[0]
				policy.Password = up[1]
				if c.InsertData.AuthType == 1 {
					policy.AuthMode = aerospike.AuthModeExternal
				} else {
					policy.AuthMode = aerospike.AuthModeInternal
				}
			}
		}
		tlsconfig := &tls.Config{}
		if c.InsertData.TlsCaCert != "" {
			cacertpool := x509.NewCertPool()
			ncertfile, err := ioutil.ReadFile(c.InsertData.TlsCaCert)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("insert-data: could not read ca cert: %s", err)
			}
			cacertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.RootCAs = cacertpool
		}
		if c.InsertData.TlsClientCert != "" {
			clientcertpool := x509.NewCertPool()
			ncertfile, err := ioutil.ReadFile(c.InsertData.TlsClientCert)
			if err != nil {
				return E_BACKEND_ERROR, fmt.Errorf("insert-data: could not read client cert: %s", err)
			}
			clientcertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.ClientCAs = clientcertpool
		}
		if c.InsertData.TlsClientCert != "" || c.InsertData.TlsCaCert != "" {
			tlsconfig.ServerName = c.InsertData.TlsServerName
			policy.TlsConfig = tlsconfig
		}
		client, err = aerospike.NewClientWithPolicy(policy, ipPort[0], port)
	}
	if err != nil {
		return 3, fmt.Errorf("insert-data: Error connecting: %s", err)
	}

	client.WarmUp(100)
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

	wp := aerospike.NewWritePolicy(0, aerospike.TTLServerDefault)
	if c.InsertData.TTL > -1 {
		if c.InsertData.TTL == 0 {
			wp.Expiration = aerospike.TTLDontExpire
		} else {
			wp.Expiration = uint32(c.InsertData.TTL)
		}
	}
	wp.TotalTimeout = time.Second * 5
	wp.SocketTimeout = 0
	wp.MaxRetries = 2
	switch c.InsertData.ExistsAction {
	case "UPDATE":
		wp.RecordExistsAction = aerospike.UPDATE
	case "UPDATE_ONLY":
		wp.RecordExistsAction = aerospike.UPDATE_ONLY
	case "REPLACE":
		wp.RecordExistsAction = aerospike.REPLACE
	case "REPLACE_ONLY":
		wp.RecordExistsAction = aerospike.REPLACE_ONLY
	case "CREATE_ONLY":
		wp.RecordExistsAction = aerospike.CREATE_ONLY
	}

	var partitionsToInsertTo []int
	if (c.InsertData.InsertToNodes != "" || c.InsertData.InsertToPartitions != 0) && c.InsertData.InsertToPartitionList == "" {
		nodes := strings.Split(c.InsertData.InsertToNodes, ",")
		nodeList := []*aerospike.Node{}
		for _, node := range nodes {
			node = strings.ToLower(strings.Trim(node, "\n\t\r ,"))
			if node != "" {
				for _, realNode := range client.GetNodes() {
					realName := strings.ToLower(realNode.GetName())
					if realName == node {
						nodeList = append(nodeList, realNode)
					}
				}
			}
		}
		if len(nodeList) == 0 {
			nodeList = client.GetNodes()
		}
		partitionsPerNode := c.InsertData.InsertToPartitions / len(nodeList)
		infoPolicy := aerospike.NewInfoPolicy()
		partitionMap := make(map[string][]int)
		for _, node := range nodeList {
			partitionInfo, err := node.RequestInfo(infoPolicy, "partition-info")
			if err != nil {
				return ret, fmt.Errorf("insert-data: partition-info: %s", err)
			}
			for _, partition := range strings.Split(partitionInfo["partition-info"], ";") {
				partitionSplit := strings.Split(partition, ":")
				if partitionSplit[0] == c.InsertData.Namespace && partitionSplit[4] == "0" {
					if strings.EqualFold(node.GetName(), partitionSplit[6]) {
						pNo, err := strconv.Atoi(partitionSplit[1])
						if err == nil {
							if len(partitionMap[strings.ToLower(node.GetName())]) < partitionsPerNode || partitionsPerNode == 0 {
								partitionMap[strings.ToLower(node.GetName())] = append(partitionMap[strings.ToLower(node.GetName())], pNo)
							}
						}
					}
				}
			}
		}
		for _, partitions := range partitionMap {
			partitionsToInsertTo = append(partitionsToInsertTo, partitions...)
		}
	}

	if c.InsertData.InsertToPartitionList != "" {
		partitionsToInsertToString := strings.Split(c.InsertData.InsertToPartitionList, ",")
		for _, pTo := range partitionsToInsertToString {
			pToInt, err := strconv.Atoi(pTo)
			if err != nil {
				if err != nil {
					return ret, fmt.Errorf("insert-data: partition list not numeric: %s", err)
				}
			}
			partitionsToInsertTo = append(partitionsToInsertTo, pToInt)
		}
	}

	partNoLoop := 0
	for i := c.InsertData.PkStartNumber; i <= c.InsertData.PkEndNumber; i++ {
		partNo := -1
		if len(partitionsToInsertTo) > 0 {
			partNo = partitionsToInsertTo[partNoLoop]
			partNoLoop++
			if partNoLoop == len(partitionsToInsertTo) {
				partNoLoop = 0
			}
		}
		if c.InsertData.UseMultiThreaded == 0 {
			ret, err = c.F_insertData_perform5(i, client, wp, partNo)
			if err != nil {
				return ret, fmt.Errorf("insert-data: insertData_perform: %s", err)
			}
		} else {
			wg <- 1
			go func(i int, client *aerospike.Client) {
				ret, err = c.F_insertData_perform5(i, client, wp, partNo)
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

func (c *config) F_insertData_perform5(i int, client *aerospike.Client, wp *aerospike.WritePolicy, partitionNumber int) (ret int64, err error) {
	setSplit := strings.Split(c.InsertData.Set, ":")
	var set string
	if len(setSplit) == 1 {
		set = setSplit[0]
	} else if setSplit[0] == "random" {
		setSizeNo, err := strconv.Atoi(setSplit[1])
		if err != nil {
			return 4, fmt.Errorf("insert-data: Error processing set random: %s", err)
		}
		set = RandStringRunes(setSizeNo)
	} else {
		return 5, fmt.Errorf("insert-data: Set name error: %s", setSplit)
	}

	binData := strings.Split(c.InsertData.Bin, ":")
	if len(binData) != 2 {
		return 6, fmt.Errorf("insert-data: Bin data convert error: %s", binData)
	}

	var bin string
	if binData[0] == "static" {
		bin = binData[1]
	} else if binData[0] == "random" {
		binSizeNo, err := strconv.Atoi(binData[1])
		if err != nil {
			return 7, fmt.Errorf("insert-data: Bin Size No error: %s", err)
		}
		bin = RandStringRunes(binSizeNo)
	} else if binData[0] == "unique" {
		bin = fmt.Sprintf("%s%d", binData[1], i)
	} else {
		return 8, fmt.Errorf("insert-data: Bin Name error")
	}

	bincData := strings.Split(c.InsertData.BinContents, ":")
	if len(bincData) != 2 {
		return 9, fmt.Errorf("insert-data: Bin Conents error: %s", err)
	}

	var binc string
	if bincData[0] == "static" {
		binc = bincData[1]
	} else if bincData[0] == "random" {
		bincSizeNo, err := strconv.Atoi(bincData[1])
		if err != nil {
			return 10, fmt.Errorf("insert-data: Bin contents size error: %s", err)
		}
		binc = RandStringRunes(bincSizeNo)
	} else if bincData[0] == "unique" {
		binc = fmt.Sprintf("%s%d", bincData[1], i)
	} else {
		return 11, fmt.Errorf("insert-data: bin contents error")
	}

	var key *aerospike.Key
	if partitionNumber < 0 {
		pk := fmt.Sprintf("%s%d", c.InsertData.PkPrefix, i)
		key, err = aerospike.NewKey(c.InsertData.Namespace, set, pk)
		if err != nil {
			return 1, fmt.Errorf("insert-data: aerospike.NewKey error: %s", err)
		}
	} else {
		digest := make([]byte, 20)
		binary.LittleEndian.PutUint32(digest, uint32(partitionNumber))
		binary.LittleEndian.PutUint64(digest[2:], uint64(i))
		key, err = aerospike.NewKeyWithDigest(c.InsertData.Namespace, set, nil, digest)
		if err != nil {
			return 1, fmt.Errorf("insert-data: aerospike.NewKey error: %s", err)
		}
	}
	realBin := aerospike.NewBin(bin, binc)
	nkey := key
	nbin := realBin
	erra := client.PutBins(wp, nkey, nbin)
	for erra != nil {
		if strings.Contains(fmt.Sprint(erra), "i/o timeout") || strings.Contains(fmt.Sprint(erra), "command execution timed out on client") {
			time.Sleep(100 * time.Millisecond)
			erra = client.PutBins(wp, nkey, nbin)
			if erra != nil {
				c.log.Info("Client.Put error, giving up: %s", erra)
			}
		} else {
			c.log.Info("Client.Put error, giving up: %s", erra)
			return
		}
	}
	if c.InsertData.ReadAfterWrite == 1 {
		rp := aerospike.BasePolicy{}
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
