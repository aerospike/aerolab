package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerospike-client-go/v5"
)

func (c *dataInsertCmd) insert5(args []string) error {
	ipPort := strings.Split(c.SeedNode, ":")
	if len(ipPort) != 2 {
		return fmt.Errorf("insert-data: Failed to process SeedNode, must be IP:PORT: %s", c.SeedNode)
	}

	port, err := strconv.Atoi(ipPort[1])
	if err != nil {
		return fmt.Errorf("insert-data: Error processing SeedNodePort: %s: %s", ipPort[1], err)
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
				return fmt.Errorf("insert-data: could not read ca cert: %s", err)
			}
			cacertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.RootCAs = cacertpool
		}
		if c.TlsClientCert != "" {
			clientcertpool := x509.NewCertPool()
			ncertfile, err := os.ReadFile(c.TlsClientCert)
			if err != nil {
				return fmt.Errorf("insert-data: could not read client cert: %s", err)
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
		return fmt.Errorf("insert-data: Error connecting: %s", err)
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
				return fmt.Errorf("insert-data: partition-info: %s", err)
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
				if err != nil {
					return fmt.Errorf("insert-data: partition list not numeric: %s", err)
				}
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
			err = c.insert5do(i, client, wp, partNo, src, srcLock)
			if err != nil {
				return fmt.Errorf("insert-data: insertData_perform: %s", err)
			}
		} else {
			wg <- 1
			go func(i int, client *aerospike.Client) {
				err = c.insert5do(i, client, wp, partNo, src, srcLock)
				if err != nil {
					log.Printf("WARN Insert error while multithreading at: insertData_perform: %s", err)
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
	return nil
}

func (c *dataInsertCmd) insert5do(i int, client *aerospike.Client, wp *aerospike.WritePolicy, partitionNumber int, src rand.Source, srcLock *sync.Mutex) error {
	setSplit := strings.Split(c.Set, ":")
	var set string
	if len(setSplit) == 1 {
		set = setSplit[0]
	} else if setSplit[0] == "random" {
		setSizeNo, err := strconv.Atoi(setSplit[1])
		if err != nil {
			return fmt.Errorf("insert-data: Error processing set random: %s", err)
		}
		set = RandStringRunes(setSizeNo, src, srcLock)
	} else {
		return fmt.Errorf("insert-data: Set name error: %s", setSplit)
	}

	binData := strings.Split(c.Bin, ":")
	if len(binData) != 2 {
		return fmt.Errorf("insert-data: Bin data convert error: %s", binData)
	}

	var bin string
	if binData[0] == "static" {
		bin = binData[1]
	} else if binData[0] == "random" {
		binSizeNo, err := strconv.Atoi(binData[1])
		if err != nil {
			return fmt.Errorf("insert-data: Bin Size No error: %s", err)
		}
		bin = RandStringRunes(binSizeNo, src, srcLock)
	} else if binData[0] == "unique" {
		bin = fmt.Sprintf("%s%d", binData[1], i)
	} else {
		return fmt.Errorf("insert-data: Bin Name error")
	}

	bincData := strings.Split(c.BinContents, ":")
	if len(bincData) != 2 {
		return errors.New("insert-data: Bin Conents invalid")
	}

	var binc string
	if bincData[0] == "static" {
		binc = bincData[1]
	} else if bincData[0] == "random" {
		bincSizeNo, err := strconv.Atoi(bincData[1])
		if err != nil {
			return fmt.Errorf("insert-data: Bin contents size error: %s", err)
		}
		binc = RandStringRunes(bincSizeNo, src, srcLock)
	} else if bincData[0] == "unique" {
		binc = fmt.Sprintf("%s%d", bincData[1], i)
	} else {
		return fmt.Errorf("insert-data: bin contents error")
	}

	var key *aerospike.Key
	var err error
	if partitionNumber < 0 {
		pk := fmt.Sprintf("%s%d", c.PkPrefix, i)
		key, err = aerospike.NewKey(c.Namespace, set, pk)
		if err != nil {
			return fmt.Errorf("insert-data: aerospike.NewKey error: %s", err)
		}
	} else {
		digest := make([]byte, 20)
		binary.LittleEndian.PutUint32(digest, uint32(partitionNumber))
		binary.LittleEndian.PutUint64(digest[2:], uint64(i))
		key, err = aerospike.NewKeyWithDigest(c.Namespace, set, nil, digest)
		if err != nil {
			return fmt.Errorf("insert-data: aerospike.NewKey error: %s", err)
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
				log.Printf("WARN Client.Put error, giving up: %s", erra)
			}
		} else {
			log.Printf("WARN Client.Put error, giving up: %s", erra)
			return nil
		}
	}
	if c.ReadAfterWrite {
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
					log.Printf("WARN Client.Get error, giving up: %s", erra)
				}
			} else {
				log.Printf("WARN Client.Get error, giving up: %s", erra)
				return nil
			}
		}
	}
	return nil
}
