package ingest

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aerospike/aerospike-client-go/v6"
)

func (i *Ingest) DBConnect() error {
	connectPolicy := aerospike.NewClientPolicy()
	connectPolicy.FailIfNotConnected = true
	connectPolicy.IdleTimeout = i.config.Aerospike.Timeouts.Idle
	connectPolicy.Timeout = time.Second * i.config.Aerospike.Timeouts.Connect
	if i.config.Aerospike.Security.Username != "" || i.config.Aerospike.Security.Password != "" {
		if i.config.Aerospike.Security.Username != "" {
			connectPolicy.User = i.config.Aerospike.Security.Username
		}
		if i.config.Aerospike.Security.Password != "" {
			connectPolicy.Password = i.config.Aerospike.Security.Password
		}
		if i.config.Aerospike.Security.AuthModeExternal {
			connectPolicy.AuthMode = aerospike.AuthModeExternal
		} else {
			connectPolicy.AuthMode = aerospike.AuthModeInternal
		}
	}
	if i.config.Aerospike.TLS.CaFile != "" || i.config.Aerospike.TLS.CertFile != "" || i.config.Aerospike.TLS.KeyFile != "" || i.config.Aerospike.TLS.ServerName != "" {
		nTLS := new(tls.Config)
		nTLS.InsecureSkipVerify = true
		if i.config.Aerospike.TLS.ServerName != "" {
			nTLS.ServerName = i.config.Aerospike.TLS.ServerName
		}
		if i.config.Aerospike.TLS.CaFile != "" {
			caCert, err := os.ReadFile(i.config.Aerospike.TLS.CaFile)
			if err != nil {
				return fmt.Errorf("tls: loadca: %s", err)
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			nTLS.RootCAs = caCertPool
		}
		if i.config.Aerospike.TLS.CertFile != "" || i.config.Aerospike.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(i.config.Aerospike.TLS.CertFile, i.config.Aerospike.TLS.KeyFile)
			if err != nil {
				return fmt.Errorf("tls: loadkeys: %s", err)
			}
			nTLS.Certificates = []tls.Certificate{cert}
		}
		connectPolicy.TlsConfig = nTLS
	}

	connectPolicy.ConnectionQueueSize = i.config.Aerospike.MaxPutThreads

	err := errors.New("non-null")
	retries := 0
	for err != nil {
		i.db, err = aerospike.NewClientWithPolicy(connectPolicy, i.config.Aerospike.Host, i.config.Aerospike.Port)
		if err != nil {
			log.Printf("Failed to connect: %s", err)
			retries++
			if i.config.Aerospike.Retries.Connect > -1 && retries > i.config.Aerospike.Retries.Connect {
				return errors.New("failed to connect, aborting")
			}
			time.Sleep(i.config.Aerospike.Retries.ConnectSleep)
		}
	}

	i.wp = aerospike.NewWritePolicy(0, 0)
	i.wp.SocketTimeout = i.config.Aerospike.Timeouts.Socket
	i.wp.TotalTimeout = i.config.Aerospike.Timeouts.Total
	i.wp.MaxRetries = i.config.Aerospike.Retries.Write
	return nil
}
