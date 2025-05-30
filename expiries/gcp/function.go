package aerolabexpire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

func init() {
	functions.HTTP("AerolabExpire", aerolabExpire)
}

func aerolabExpire(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, "Incorrect input json", http.StatusBadRequest)
		return
	}
	if d.Token != os.Getenv("TOKEN") {
		http.Error(w, "Incorrect input token", http.StatusBadRequest)
		return
	}

	err := aerolabExpireDoInstances()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("Error executing: %s", err)
	} else {
		err = aerolabExpireDoVolumes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("Error executing: %s", err)
		} else {
			w.Write([]byte("OK"))
			log.Println("Done")
		}
	}
}

var zones []string

func listZones() ([]string, error) {
	if len(zones) > 0 {
		return zones, nil
	}
	ctx := context.Background()
	client, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	it := client.List(ctx, &computepb.ListZonesRequest{
		Project: projectId,
	})
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		zones = append(zones, pair.GetName())
	}
	return zones, nil
}

func aerolabExpireDoVolumes() error {
	_, err := getProjectId()
	if err != nil {
		return err
	}
	zones, err := listZones()
	if err != nil {
		return err
	}
	wait := new(sync.WaitGroup)
	lock := new(sync.Mutex)
	vls := [][]string{} // []->0-zone,1-name
	for _, zone := range zones {
		wait.Add(1)
		go func(zone string) {
			defer wait.Done()
			ctx := context.Background()
			client, err := compute.NewDisksRESTClient(ctx)
			if err != nil {
				log.Printf("NewDisksRESTClient: %s", err)
				return
			}
			defer client.Close()
			it := client.List(ctx, &computepb.ListDisksRequest{
				Project: projectId,
				Filter:  proto.String("labels.usedby=" + "\"aerolab7\""),
				Zone:    zone,
			})
			for {
				pair, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					log.Printf("zone=%s iterator: %s", zone, err)
					return
				}
				nzone := strings.Split(pair.GetZone(), "/")
				if len(pair.Users) > 0 {
					continue
				}
				pairZone := nzone[len(nzone)-1]
				pairName := *pair.Name
				if lastUsed, ok := pair.Labels["lastused"]; ok {
					lastUsed = strings.ToUpper(strings.ReplaceAll(lastUsed, "_", ":"))
					if expireDuration, ok := pair.Labels["expireduration"]; ok {
						expireDuration = strings.ReplaceAll(expireDuration, "_", ".")
						lu, err := time.Parse(time.RFC3339, lastUsed)
						if err == nil {
							ed, err := time.ParseDuration(expireDuration)
							if err == nil {
								expiresTime := lu.Add(ed)
								expiresIn := expiresTime.Sub(time.Now().In(expiresTime.Location()))
								if expiresIn < 0 {
									lock.Lock()
									vls = append(vls, []string{pairZone, pairName, expiresIn.String(), lastUsed, expireDuration})
									lock.Unlock()
								}
							}
						}
					}
				}
			}
		}(zone)
	}
	wait.Wait()
	for _, item := range vls {
		zone := item[0]
		name := item[1]
		log.Printf("VOLUME EXPIRE: %s -> %s; expiresIn: %s lastUsed: %s expireDuration: %s", zone, name, item[2], item[3], item[4])
		ctx := context.Background()
		client, err := compute.NewDisksRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewDisksRESTClient: %w", err)
		}
		defer client.Close()
		_, err = client.Delete(ctx, &computepb.DeleteDiskRequest{
			Zone:    zone,
			Project: projectId,
			Disk:    name,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

var projectId = ""

func getProjectId() (string, error) {
	if projectId != "" {
		return projectId, nil
	}
	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/project/project-id", nil)
	req.Header.Set("Metadata-Flavor", "Google")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return projectId, fmt.Errorf("getProjectId: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return projectId, fmt.Errorf("getProjectId statusCode:%d", resp.StatusCode)
	}
	projectX, err := io.ReadAll(resp.Body)
	if err != nil {
		return projectId, fmt.Errorf("getProjectId read: %s", err)
	}
	projectId = strings.Trim(string(projectX), "\r\t\n ")
	return projectId, nil
}

func aerolabExpireDoInstances() error {
	now := time.Now()
	deleteList := make(map[string][]string)
	deleteListForLog := []string{}
	enumCount := 0
	_, err := getProjectId()
	if err != nil {
		return err
	}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	reqi := &computepb.AggregatedListInstancesRequest{
		Filter:  proto.String("labels.aerolab4expires:*"),
		Project: projectId,
	}
	iti := instancesClient.AggregatedList(ctx, reqi)
	defer telemetryLock.Wait()
	for {
		pair, err := iti.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("aggregatedListIterator: %s", err)
		}
		instances := pair.Value.Instances
		for _, instance := range instances {
			if strings.ToUpper(*instance.Status) != "RUNNING" && strings.ToUpper(*instance.Status) != "TERMINATED" && strings.ToUpper(*instance.Status) != "SUSPENDED" {
				continue
			}
			expire, ok := instance.Labels["aerolab4expires"]
			if !ok {
				continue
			}
			enumCount++
			expiry, err := time.Parse(time.RFC3339, strings.ToUpper(strings.ReplaceAll(expire, "_", ":")))
			if err != nil {
				return fmt.Errorf("could not handle expiry for instance %s: %s: %s", *instance.Name, expire, err)
			}
			if expiry.Before(now) && expiry.After(time.Date(1985, time.April, 29, 0, 0, 0, 0, time.UTC)) {
				if _, ok := deleteList[*instance.Zone]; !ok {
					deleteList[*instance.Zone] = []string{}
				}
				deleteList[*instance.Zone] = append(deleteList[*instance.Zone], *instance.Name)
				name := instance.Labels["aerolab4cluster_name"]
				node := instance.Labels["aerolab4node_number"]
				if name == "" || node == "" {
					name = instance.Labels["aerolab4client_name"]
					node = instance.Labels["aerolab4client_node_number"]
				}
				deleteListForLog = append(deleteListForLog, fmt.Sprintf("instanceId=%s zone=%s clusterName=%s nodeNo=%s", *instance.Name, *instance.Zone, name, node))
				if _, ok := instance.Labels["telemetry"]; ok {
					telemetryShip(instance.Labels["telemetry"], *instance.Zone, *instance.Name, name, node)
				}
			}
		}
	}

	log.Printf("Enumerated through %d instances, shutting down %d instances", enumCount, len(deleteListForLog))
	if len(deleteList) == 0 {
		return nil
	}
	for _, del := range deleteListForLog {
		log.Printf("Removing: %s", del)
	}
	// expire deleteList
	for zone, names := range deleteList {
		ss := strings.Split(zone, "/")
		zone = ss[len(ss)-1]
		for _, name := range names {
			req := &computepb.DeleteInstanceRequest{
				Instance: name,
				Project:  projectId,
				Zone:     zone,
			}
			_, err = instancesClient.Delete(ctx, req)
			if err != nil {
				return fmt.Errorf("unable to delete instance: %w", err)
			}
		}
	}
	return nil
}

type telemetry struct {
	UUID          string
	Job           string
	Cloud         string
	Zone          string
	Instance      string
	ClusterName   string
	NodeNo        string
	ExpiryVersion string
	CmdLine       []string
	Time          int64
}

var telemetryLock = new(sync.WaitGroup)

func telemetryShip(uuid string, zone string, instance string, clusterName string, nodeNo string) {
	telemetryLock.Add(1)
	go func() {
		defer telemetryLock.Done()
		err := telemetryShipDo(uuid, zone, instance, clusterName, nodeNo)
		if err != nil {
			log.Println(err)
		}
	}()
}

func telemetryShipDo(uuid string, zone string, instance string, clusterName string, nodeNo string) error {
	t := &telemetry{
		UUID:          uuid,
		Job:           "expire",
		Cloud:         "GCP",
		Zone:          zone,
		Instance:      instance,
		ClusterName:   clusterName,
		NodeNo:        nodeNo,
		ExpiryVersion: "2",
		Time:          time.Now().UnixMicro(),
		CmdLine:       []string{"EXPIRY"},
	}
	contents, err := json.Marshal(t)
	if err != nil {
		return err
	}
	url := "https://us-central1-aerospike-gaia.cloudfunctions.net/aerolab-telemetrics"
	ret, err := http.Post(url, "application/json", bytes.NewReader(contents))
	if err != nil {
		return err
	}
	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("returned ret code: %d:%s", ret.StatusCode, ret.Status)
	}
	return nil
}
