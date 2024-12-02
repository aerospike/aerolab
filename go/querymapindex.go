package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/aerospike/aerospike-client-go/v7"
)

func main() {
	// connect
	hostname := "127.0.0.1"
	port := 3100
	client, err := aerospike.NewClient(hostname, port)
	if err != nil {
		log.Fatal(err)
	}

	// defer closure
	defer client.Close()

	// cause we need to...
	client.WarmUp(100)

	// new write policy
	wp := aerospike.NewWritePolicy(0, 0)

	// create sindex if it doesn't exist - for top-level map search
	task, err := client.CreateComplexIndex(wp, "test", "testset", "BOBVALX", "testbin", aerospike.STRING, aerospike.ICT_DEFAULT, aerospike.CtxMapKey(aerospike.NewStringValue("bob")))
	if err != nil && !strings.Contains(err.Error(), "Index already exists") {
		log.Fatal(err)
	} else if err == nil {
		err = <-task.OnComplete()
		if err != nil {
			log.Fatal(err)
		}
	}

	// create sindex if it doesn't exist - for map-in-map search
	task, err = client.CreateComplexIndex(wp, "test", "testset", "BOBVALXY", "testbin", aerospike.STRING, aerospike.ICT_DEFAULT, aerospike.CtxMapKey(aerospike.NewStringValue("robert")), aerospike.CtxMapKey(aerospike.NewStringValue("depth")))
	if err != nil && !strings.Contains(err.Error(), "Index already exists") {
		log.Fatal(err)
	} else if err == nil {
		err = <-task.OnComplete()
		if err != nil {
			log.Fatal(err)
		}
	}
	/*
		the above indexes can be create using asadm instead:
		asadm
		> enable
		> manage sindex create string BOBVALX ns test set testset bin testbin ctx map_key(bob)
		> manage sindex create string BOBVALXY ns test set testset bin testbin ctx map_key(robert) map_key(depth)
	*/

	// create key
	key, err := aerospike.NewKey("test", "testset", "testkey")
	if err != nil {
		log.Fatal(err)
	}

	// create map value variable
	binValue := make(map[string]interface{})
	insideValue := make(map[string]interface{})
	insideValue["depth"] = "boom"
	binValue["robert"] = insideValue
	binValue["bob"] = "5"

	// print the map as json
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	enc.Encode(binValue)

	// create bin variable
	bin := aerospike.NewBin("testbin", binValue)

	// store record
	err = client.PutBins(wp, key, bin)
	if err != nil {
		log.Fatal(err)
	}

	// get record
	rp := aerospike.NewPolicy()
	rec, err := client.Get(rp, key)
	if err != nil {
		log.Fatal(err)
	}
	log.Print(rec.Bins["testbin"])

	// query based on map data - top-level
	qp := aerospike.NewQueryPolicy()
	statement := aerospike.NewStatement("test", "testset", "testbin")
	filter := aerospike.NewEqualFilter("testbin", "5", aerospike.CtxMapKey(aerospike.NewStringValue("bob")))
	statement.Filter = filter
	res, err := client.Query(qp, statement)
	if err != nil {
		log.Fatal(err)
	}
	for rec := range res.Results() {
		if rec.Err != nil {
			log.Fatal(err)
		}
		log.Print(rec.Record)
	}

	// query based on map data - map-in-map
	qp = aerospike.NewQueryPolicy()
	statement = aerospike.NewStatement("test", "testset", "testbin")
	filter = aerospike.NewEqualFilter("testbin", "boom", aerospike.CtxMapKey(aerospike.NewStringValue("robert")), aerospike.CtxMapKey(aerospike.NewStringValue("depth")))
	statement.Filter = filter
	res, err = client.Query(qp, statement)
	if err != nil {
		log.Fatal(err)
	}
	for rec := range res.Results() {
		if rec.Err != nil {
			log.Fatal(err)
		}
		log.Print(rec.Record)
	}

	// done
	log.Print("Done")
}
