package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerospike-client-go/v8"
	"github.com/rglonek/logger"
)

func (c *DataInsertCmd) insert8(args []string, log *logger.Logger) error {
	_ = args

	ipPort := strings.Split(c.SeedNode, ":")
	if len(ipPort) != 2 {
		return fmt.Errorf("failed to process SeedNode, must be IP:PORT: %s", c.SeedNode)
	}

	port, err := strconv.Atoi(ipPort[1])
	if err != nil {
		return fmt.Errorf("error processing SeedNodePort: %s: %w", ipPort[1], err)
	}

	var client *aerospike.Client
	if c.User == "" && c.TlsCaCert == "" && c.TlsClientCert == "" {
		client, err = aerospike.NewClient(ipPort[0], port)
	} else {
		policy := aerospike.NewClientPolicy()
		if c.User != "" {
			policy.User = c.User
			policy.Password = c.Pass
			if c.AuthExternal {
				policy.AuthMode = aerospike.AuthModeExternal
			} else {
				policy.AuthMode = aerospike.AuthModeInternal
			}
		}

		tlsconfig := &tls.Config{}
		if c.TlsCaCert != "" {
			cacertpool := x509.NewCertPool()
			ncertfile, err := os.ReadFile(c.TlsCaCert)
			if err != nil {
				return fmt.Errorf("could not read ca cert: %w", err)
			}
			cacertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.RootCAs = cacertpool
		}
		if c.TlsClientCert != "" {
			clientcertpool := x509.NewCertPool()
			ncertfile, err := os.ReadFile(c.TlsClientCert)
			if err != nil {
				return fmt.Errorf("could not read client cert: %w", err)
			}
			clientcertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.ClientCAs = clientcertpool
		}
		if c.TlsClientCert != "" || c.TlsCaCert != "" {
			tlsconfig.ServerName = c.TlsServerName
			policy.TlsConfig = tlsconfig
		}
		client, err = aerospike.NewClientWithPolicy(policy, ipPort[0], port)
	}
	if err != nil {
		return fmt.Errorf("error connecting to Aerospike: %w", err)
	}

	client.WarmUp(100)
	src := rand.NewSource(time.Now().UnixNano())
	srcLock := new(sync.Mutex)

	total := c.PkEndNumber - c.PkStartNumber + 1
	inserted := 0
	var wg chan int
	if c.UseMultiThreaded > 0 {
		wg = make(chan int, c.UseMultiThreaded)
	}

	startTime := time.Now()
	esc := "                   \r"

	// Progress reporter
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Second):
				if inserted == total {
					return
				}
				nsec := int(time.Since(startTime).Seconds())
				var ttkn int
				if nsec == 0 {
					ttkn = 0
				} else {
					ttkn = inserted / nsec
				}
				fmt.Printf("Total records: %d , Inserted: %d , Subthreads running: %d , Records per second: %d"+esc, total, inserted, len(wg), ttkn)
			}
		}
	}()

	wp := aerospike.NewWritePolicy(0, aerospike.TTLServerDefault)
	if c.TTL > -1 {
		if c.TTL == 0 {
			wp.Expiration = aerospike.TTLDontExpire
		} else {
			wp.Expiration = uint32(c.TTL)
		}
	}
	wp.TotalTimeout = time.Second * 5
	wp.SocketTimeout = 0
	wp.MaxRetries = 2

	switch c.ExistsAction {
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
	if (c.InsertToNodes != "" || c.InsertToPartitions != 0) && c.InsertToPartitionList == "" {
		nodes := strings.Split(c.InsertToNodes, ",")
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

		partitionsPerNode := c.InsertToPartitions / len(nodeList)
		infoPolicy := aerospike.NewInfoPolicy()
		partitionMap := make(map[string][]int)

		for _, node := range nodeList {
			partitionInfo, err := node.RequestInfo(infoPolicy, "partition-info")
			if err != nil {
				return fmt.Errorf("partition-info error: %w", err)
			}
			for _, partition := range strings.Split(partitionInfo["partition-info"], ";") {
				partitionSplit := strings.Split(partition, ":")
				if partitionSplit[0] == c.Namespace && partitionSplit[4] == "0" {
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

	if c.InsertToPartitionList != "" {
		partitionsToInsertToString := strings.Split(c.InsertToPartitionList, ",")
		for _, pTo := range partitionsToInsertToString {
			pToInt, err := strconv.Atoi(pTo)
			if err != nil {
				return fmt.Errorf("partition list not numeric: %w", err)
			}
			partitionsToInsertTo = append(partitionsToInsertTo, pToInt)
		}
	}

	partNoLoop := 0
	for i := c.PkStartNumber; i <= c.PkEndNumber; i++ {
		partNo := -1
		if len(partitionsToInsertTo) > 0 {
			partNo = partitionsToInsertTo[partNoLoop]
			partNoLoop++
			if partNoLoop == len(partitionsToInsertTo) {
				partNoLoop = 0
			}
		}

		if c.UseMultiThreaded == 0 {
			err = c.insert8do(i, client, wp, partNo, src, srcLock, log)
			if err != nil {
				done <- true
				return fmt.Errorf("insertData_perform error: %w", err)
			}
		} else {
			wg <- 1
			go func(i int, client *aerospike.Client) {
				err = c.insert8do(i, client, wp, partNo, src, srcLock, log)
				if err != nil {
					log.Warn("Insert error while multithreading: %s", err)
				}
				<-wg
			}(i, client)
		}

		inserted = inserted + 1
	}

	for len(wg) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	done <- true
	runTime := time.Since(startTime)
	client.Close()

	if int(runTime.Seconds()) > 0 {
		fmt.Printf("Total records: %d , Inserted: %d                                                                \nTime taken: %s , Records per second: %d\n",
			total, inserted, runTime.String(), total/int(runTime.Seconds()))
	} else {
		fmt.Printf("Total records: %d , Inserted: %d                                                                \nTime taken: %s , Records per second: %d\n",
			total, inserted, runTime.String(), total)
	}

	return nil
}

func (c *DataInsertCmd) insert8do(i int, client *aerospike.Client, wp *aerospike.WritePolicy, partitionNumber int, src rand.Source, srcLock *sync.Mutex, log *logger.Logger) error {
	setSplit := strings.Split(c.Set, ":")
	var set string
	if len(setSplit) == 1 {
		set = setSplit[0]
	} else if setSplit[0] == "random" {
		setSizeNo, err := strconv.Atoi(setSplit[1])
		if err != nil {
			return fmt.Errorf("error processing set random: %w", err)
		}
		set = RandStringRunes(setSizeNo, src, srcLock)
	} else {
		return fmt.Errorf("set name error: %s", setSplit)
	}

	binData := strings.Split(c.Bin, ":")
	if len(binData) != 2 {
		return fmt.Errorf("bin data convert error: %s", binData)
	}

	var bin string
	if binData[0] == "static" {
		bin = binData[1]
	} else if binData[0] == "random" {
		binSizeNo, err := strconv.Atoi(binData[1])
		if err != nil {
			return fmt.Errorf("bin size error: %w", err)
		}
		bin = RandStringRunes(binSizeNo, src, srcLock)
	} else if binData[0] == "unique" {
		bin = fmt.Sprintf("%s%d", binData[1], i)
	} else {
		return errors.New("bin name error")
	}

	bincData := strings.Split(c.BinContents, ":")
	if len(bincData) != 2 {
		return errors.New("bin contents invalid")
	}

	var binc string
	if bincData[0] == "static" {
		binc = bincData[1]
	} else if bincData[0] == "random" {
		bincSizeNo, err := strconv.Atoi(bincData[1])
		if err != nil {
			return fmt.Errorf("bin contents size error: %w", err)
		}
		binc = RandStringRunes(bincSizeNo, src, srcLock)
	} else if bincData[0] == "unique" {
		binc = fmt.Sprintf("%s%d", bincData[1], i)
	} else {
		return errors.New("bin contents error")
	}

	var key *aerospike.Key
	var err error
	if partitionNumber < 0 {
		pk := fmt.Sprintf("%s%d", c.PkPrefix, i)
		key, err = aerospike.NewKey(c.Namespace, set, pk)
		if err != nil {
			return fmt.Errorf("aerospike.NewKey error: %w", err)
		}
	} else {
		digest := make([]byte, 20)
		binary.LittleEndian.PutUint32(digest, uint32(partitionNumber))
		binary.LittleEndian.PutUint64(digest[2:], uint64(i))
		key, err = aerospike.NewKeyWithDigest(c.Namespace, set, nil, digest)
		if err != nil {
			return fmt.Errorf("aerospike.NewKeyWithDigest error: %w", err)
		}
	}

	realBin := aerospike.NewBin(bin, binc)
	erra := client.PutBins(wp, key, realBin)

	for erra != nil {
		if strings.Contains(fmt.Sprint(erra), "i/o timeout") || strings.Contains(fmt.Sprint(erra), "command execution timed out on client") {
			time.Sleep(100 * time.Millisecond)
			erra = client.PutBins(wp, key, realBin)
			if erra != nil {
				log.Warn("Client.Put error, giving up: %s", erra)
			}
		} else {
			log.Warn("Client.Put error, giving up: %s", erra)
			return nil
		}
	}

	if c.ReadAfterWrite {
		rp := aerospike.BasePolicy{}
		rp.TotalTimeout = time.Second * 5
		rp.SocketTimeout = 0
		rp.MaxRetries = 2
		_, erra := client.Get(&rp, key)

		for erra != nil {
			if strings.Contains(fmt.Sprint(erra), "i/o timeout") || strings.Contains(fmt.Sprint(erra), "command execution timed out on client") {
				time.Sleep(100 * time.Millisecond)
				_, erra = client.Get(&rp, key)
				if erra != nil {
					log.Warn("Client.Get error, giving up: %s", erra)
				}
			} else {
				log.Warn("Client.Get error, giving up: %s", erra)
				return nil
			}
		}
	}

	return nil
}
