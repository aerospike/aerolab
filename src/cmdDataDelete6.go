package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerospike-client-go/v6"
)

func (c *dataDeleteCmd) delete6(args []string) error {

	ipPort := strings.Split(c.SeedNode, ":")
	if len(ipPort) != 2 {
		return fmt.Errorf("delete-data: Failed to process SeedNode, must be IP:PORT: %s", c.SeedNode)
	}

	port, err := strconv.Atoi(ipPort[1])
	if err != nil {
		return fmt.Errorf("delete-data: Error processing SeedNodePort: %s: %s", ipPort[1], err)
	}

	var client *aerospike.Client
	if c.Username == "" && c.TlsCaCert == "" && c.TlsClientCert == "" {
		client, err = aerospike.NewClient(ipPort[0], port)
	} else {
		policy := aerospike.NewClientPolicy()
		if c.Username != "" {
			policy.User = c.Username
			policy.Password = c.Password
			if c.AuthExternal {
				policy.AuthMode = aerospike.AuthModeExternal
			} else {
				policy.AuthMode = aerospike.AuthModeInternal
			}
		}
		var tlsconfig *tls.Config
		if c.TlsCaCert != "" {
			var cacertpool *x509.CertPool
			ncertfile, err := os.ReadFile(c.TlsCaCert)
			if err != nil {
				return fmt.Errorf("delete-data: could not read ca cert: %s", err)
			}
			cacertpool.AppendCertsFromPEM(ncertfile)
			tlsconfig.RootCAs = cacertpool
		}
		if c.TlsClientCert != "" {
			var clientcertpool *x509.CertPool
			ncertfile, err := os.ReadFile(c.TlsClientCert)
			if err != nil {
				return fmt.Errorf("delete-data: could not read client cert: %s", err)
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
		return fmt.Errorf("delete-data: Error connecting: %s", err)
	}

	client.WarmUp(100)

	total := c.PkEndNumber - c.PkStartNumber + 1
	deleted := 0
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
				ttkn = deleted / nsec
			}
			fmt.Printf("Total records: %d , Deleted: %d , Subthreads running: %d , Records per second: %d                   \r", total, deleted, len(wg), ttkn)
			time.Sleep(time.Second)
		}
	}()
	wp := aerospike.NewWritePolicy(0, aerospike.TTLServerDefault)
	wp.TotalTimeout = time.Second * 5
	wp.SocketTimeout = 0
	wp.MaxRetries = 2
	wp.DurableDelete = c.Durable

	for i := c.PkStartNumber; i <= c.PkEndNumber; i++ {

		if c.UseMultiThreaded == 0 {
			err = c.delete6do(i, client, wp)
			if err != nil {
				return fmt.Errorf("WARN delete-data: do: %s", err)
			}
		} else {
			wg <- 1
			go func(i int, client *aerospike.Client) {
				err = c.delete6do(i, client, wp)
				if err != nil {
					log.Printf("WARN delete error while multithreading at: do: %s", err)
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
	return nil
}

func (c *dataDeleteCmd) delete6do(i int, client *aerospike.Client, wp *aerospike.WritePolicy) error {
	pk := fmt.Sprintf("%s%d", c.PkPrefix, i)
	key, err := aerospike.NewKey(c.Namespace, c.Set, pk)
	if err != nil {
		return fmt.Errorf("delete-data: aerospike.NewKey error: %s", err)
	}
	_, erra := client.Delete(wp, key)
	for erra != nil {
		if strings.Contains(fmt.Sprint(erra), "i/o timeout") || strings.Contains(fmt.Sprint(erra), "command execution timed out on client") {
			time.Sleep(100 * time.Millisecond)
			_, erra := client.Delete(wp, key)
			if erra != nil {
				log.Printf("WARN Client.Put error, giving up: %s", erra)
			}
		} else {
			log.Printf("WARN Client.Put error, giving up: %s", erra)
			return nil
		}
	}
	return nil
}
