package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aerospike/aerospike-client-go/v7"
)

// build TLS configuration
func buildTLSConfig(tlsName string, caFile string, certFile string, keyFile string) (*tls.Config, error) {
	nTLS := new(tls.Config)
	nTLS.InsecureSkipVerify = true     // skip certificate verification, WARN: not safe in production
	nTLS.ServerName = tlsName          // server certificate common name
	caCert, err := os.ReadFile(caFile) // path to the shared root ca
	if err != nil {
		return nil, fmt.Errorf("tls: loadca: %s", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	nTLS.RootCAs = caCertPool
	if certFile != "" && keyFile != "" { // mTLS only if certificate and key are provided; otherwise skip this step
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("tls: loadkeys: %s", err)
		}
		nTLS.Certificates = []tls.Certificate{cert}
	}
	return nTLS, nil
}

func main() {
	// basics
	hostname := "172.17.0.2"
	port := 4333

	// create connect policy
	var err error
	connectPolicy := aerospike.NewClientPolicy()
	connectPolicy.Timeout = 10 * time.Second
	connectPolicy.AuthMode = aerospike.AuthModeInternal // change to External for ldap, or PKI for mTLS-based user auth
	connectPolicy.User = "badwan"
	connectPolicy.Password = "blastoff"
	connectPolicy.TlsConfig, err = buildTLSConfig("tls1", "/path/to/rootca.pem", "", "") // specify certFile and keyFile if using mTLS
	if err != nil {
		log.Fatal(err)
	}

	// connect
	client, err := aerospike.NewClientWithPolicy(connectPolicy, hostname, port)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// open 100 initial connections
	_, err = client.WarmUp(100)
	if err != nil {
		log.Fatal(err)
	}

	// TODO code here
}
