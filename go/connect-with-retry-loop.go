package main

import (
	"errors"
	"log"
	"time"

	"github.com/aerospike/aerospike-client-go/v7"
)

func main() {
	client, err := connect("10.88.0.2", 3100, -1, time.Second)
	if err != nil {
		log.Fatal(err)
	}
	client.Close()
}

// connect - connect to aerospike cluster
// hostname - seed IP/DNS
// port - seed port
// maxRetries - maximum number of times to retry the connection, set to -1 to keep trying forever
// sleepBetweenRetries - how long to sleep between each connection retry
func connect(hostname string, port int, maxRetries int, sleepBetweenRetries time.Duration) (*aerospike.Client, error) {
	// policy - setting connection timeout of each connection
	cp := aerospike.NewClientPolicy()
	cp.Timeout = 5 * time.Second

	// if we manage to connect to seed, but fail to connect to other nodes, still continue
	cp.FailIfNotConnected = false

	// create new client loop
	var client *aerospike.Client
	var err error
	log.Printf("Connecting to %s:%d", hostname, port)
	retry := -1
	for {
		retry++
		if retry > 0 {
			log.Printf("Retrying connection")
		}
		client, err = aerospike.NewClientWithPolicy(cp, hostname, port)
		if err == nil {
			break
		}
		log.Printf("Connection failure: %s", err)
		if maxRetries != -1 && retry >= maxRetries {
			break
		}
		time.Sleep(sleepBetweenRetries)
	}
	if err != nil {
		return nil, errors.New("all connection attempts failed, aborting")
	}
	log.Print("Connected")

	// warm up 100 connections
	client.WarmUp(100)

	// done
	return client, nil
}
