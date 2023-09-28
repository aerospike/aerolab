package ingest

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aerospike/aerospike-client-go/v6"
	"github.com/bestmethod/logger"
)

func (i *Ingest) dbConnect() error {
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
	nerr := ""
	retries := 0
	for err != nil {
		i.db, err = aerospike.NewClientWithPolicy(connectPolicy, i.config.Aerospike.Host, i.config.Aerospike.Port)
		if err != nil {
			logger.Debug("Failed to connect: %s", err)
			retries++
			nerr = nerr + "\n" + err.Error()
			if i.config.Aerospike.Retries.Connect > -1 && retries > i.config.Aerospike.Retries.Connect {
				return fmt.Errorf("failed to connect, aborting: %s", nerr)
			}
			time.Sleep(i.config.Aerospike.Retries.ConnectSleep)
		}
	}

	i.wp = aerospike.NewWritePolicy(0, 0)
	i.wp.SocketTimeout = i.config.Aerospike.Timeouts.Socket
	i.wp.TotalTimeout = i.config.Aerospike.Timeouts.Total
	i.wp.MaxRetries = i.config.Aerospike.Retries.Write
	logger.Debug("DB: WarmUp")
	i.db.WarmUp(i.config.Aerospike.MaxPutThreads)
	logger.Debug("DB: Create indexes")
	return i.dbSindex(i.config.Aerospike.WaitForSindexes)
}

func (i *Ingest) dbSindex(wait bool) error {
	i.createSindex(i.config.Aerospike.DefaultSetName, i.config.Aerospike.TimestampIndexName, wait)
	i.createSindex(i.config.Aerospike.LogFileRagesSetName, fmt.Sprintf("%s_%s", i.config.Aerospike.TimestampIndexName, i.config.Aerospike.LogFileRagesSetName), wait)
	for _, pattern := range i.patterns.Patterns {
		if pattern.Name != "" {
			i.createSindex(pattern.Name, fmt.Sprintf("%s_%s", i.config.Aerospike.TimestampIndexName, pattern.Name), wait)
		}
		for _, adv := range pattern.RegexAdvanced {
			if adv.SetName != "" {
				i.createSindex(adv.SetName, fmt.Sprintf("%s_%s", i.config.Aerospike.TimestampIndexName, adv.SetName), wait)
			}
		}
	}
	return nil
}

func (i *Ingest) createSindex(setName string, indexName string, wait bool) {
	logger.Detail("Creating sindex (set:%s) (idxName:%s)", setName, indexName)
	indexCreateTask, err := i.db.CreateIndex(nil, i.config.Aerospike.Namespace, setName, indexName, i.config.Aerospike.TimestampBinName, aerospike.NUMERIC)
	if err != nil {
		logger.Warn("index create error: %s", err)
	} else if wait {
		for {
			res, err := indexCreateTask.IsDone()
			if err != nil {
				logger.Warn("index create error: %s", err)
			}
			if res {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}
}
