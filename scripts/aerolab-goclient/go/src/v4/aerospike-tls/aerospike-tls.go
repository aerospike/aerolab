// Copyright 2014-2021 Aerospike, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	as "github.com/aerospike/aerospike-client-go"
)

// client connect code
func connect() (*as.Client, error) {
	connectPolicy := as.NewClientPolicy()
	connectPolicy.Timeout = 10 * time.Second
	connectPolicy.AuthMode = as.AuthModeExternal // ldap, as.AuthModeInternal is internal aerospike auth
	connectPolicy.User = "badwan"
	connectPolicy.Password = "blastoff"
	var err error
	connectPolicy.TlsConfig, err = buildTLSConfig()
	if err != nil {
		return nil, err
	}
	client, err := as.NewClientWithPolicy(connectPolicy, "CLUSTERIP", 3000)
	return client, err
}

func buildTLSConfig() (*tls.Config, error) {
	nTLS := new(tls.Config)
	nTLS.InsecureSkipVerify = true                                 // skip certificate verification, WARN: not safe in production
	nTLS.ServerName = "server1"                                    // server certificate common name
	caCert, err := ioutil.ReadFile("/root/certs/local/rootCA.pem") // path to the shared root ca
	if err != nil {
		return nil, fmt.Errorf("tls: loadca: %s", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	nTLS.RootCAs = caCertPool
	// mutual auth, specify the certFile and keyFile of the client certificate:
	/*
		certFile := ""
		keyFile := ""
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("tls: loadkeys: %s", err)
		}
		nTLS.Certificates = []tls.Certificate{cert}
		nTLS.BuildNameToCertificate()
	*/
	return nTLS, nil
}

func main() {
	// connect to the client
	client, err := connect()
	if err != nil {
		log.Fatal(err)
	}

	defer client.Close()

	// define primary key
	namespace := "test"
	setName := "bar"
	keyName := "mykey"
	key, err := as.NewKey(namespace, setName, keyName) // user key can be of any supported type
	if err != nil {
		log.Fatal(err)
	}

	// define some bins
	bins := as.BinMap{
		"bin1": 42, // you can pass any supported type as bin value
		"bin2": "An elephant is a mouse with an operating system",
		"bin3": []interface{}{"Go", 17981},
	}

	// write the bins
	writePolicy := as.NewWritePolicy(0, 0)
	err = client.Put(writePolicy, key, bins)
	if err != nil {
		log.Fatal(err)
	}

	// read it back!
	readPolicy := as.NewPolicy()
	rec, err := client.Get(readPolicy, key)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%#v\n", *rec)

	// Add to bin1
	err = client.Add(writePolicy, key, as.BinMap{"bin1": 1})
	if err != nil {
		log.Fatal(err)
	}

	rec2, err := client.Get(readPolicy, key)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("value of %s: %v\n", "bin1", rec2.Bins["bin1"])

	// prepend and append to bin2
	err = client.Prepend(writePolicy, key, as.BinMap{"bin2": "Frankly:  "})
	if err != nil {
		log.Fatal(err)
	}
	err = client.Append(writePolicy, key, as.BinMap{"bin2": "."})
	if err != nil {
		log.Fatal(err)
	}

	rec3, err := client.Get(readPolicy, key)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("value of %s: %v\n", "bin2", rec3.Bins["bin2"])

	// delete bin3
	err = client.Put(writePolicy, key, as.BinMap{"bin3": nil})
	if err != nil {
		log.Fatal(err)
	}
	rec4, err := client.Get(readPolicy, key)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("bin3 does not exist anymore: %#v\n", *rec4)

	// check if key exists
	exists, err := client.Exists(readPolicy, key)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("key exists in the database: %#v\n", exists)

	// delete the key, and check if key exists
	existed, err := client.Delete(writePolicy, key)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("did key exist before delete: %#v\n", existed)

	exists, err = client.Exists(readPolicy, key)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("key exists: %#v\n", exists)
}
