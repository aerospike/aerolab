package main

import (
	"log"

	"github.com/aerospike/aerospike-client-go/v6"
)

func main() {
	// connection seed node
	hostname := "172.17.0.2"
	port := 3000

	// create new client
	client, err := aerospike.NewClient(hostname, port)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// warm up 100 connections
	_, err = client.WarmUp(100)
	if err != nil {
		log.Fatal(err)
	}

	// TODO code here
}
