package aerolabexpire

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

	err := aerolabExpireDo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("Error executing: %s", err)
	} else {
		w.Write([]byte("OK"))
		log.Println("Done")
	}
}

func aerolabExpireDo() error {
	now := time.Now()
	deleteList := make(map[string][]string)
	deleteListForLog := []string{}
	enumCount := 0
	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/project/project-id", nil)
	req.Header.Set("Metadata-Flavor", "Google")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("getProjectId: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("getProjectId statusCode:%d", resp.StatusCode)
	}
	projectX, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("getProjectId read: %s", err)
	}
	projectId := strings.Trim(string(projectX), "\r\t\n ")

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
			}
		}
	}

	log.Printf("Enumerated through %d instances, shutting down %d instances", enumCount, len(deleteList))
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
