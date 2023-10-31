package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/efs"
)

type MyEvent struct{}

type MyResponse struct {
	Status  string
	Message string `json:"Message:"`
}

func HandleLambdaEvent(event MyEvent) (MyResponse, error) {
	// connect
	sess, err := session.NewSession()
	if err != nil {
		return makeErrorResponse("Could not create AWS session: ", err)
	}
	svc := ec2.New(sess)

	// list instances
	instances, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: aws.StringSlice([]string{"pending", "running", "stopping", "stopped"}),
			},
			{
				Name:   aws.String("tag-key"),
				Values: aws.StringSlice([]string{"aerolab4expires"}),
			},
		},
	})
	if err != nil {
		return makeErrorResponse("Could not list instances: ", err)
	}

	// check each instance for expiry
	now := time.Now()
	deleteList := []string{}
	deleteListForLog := []string{}
	enumCount := 0
	defer telemetryLock.Wait()
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			enumCount++
			tags := make(map[string]string)
			for _, tag := range instance.Tags {
				tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
			}
			if expires, ok := tags["aerolab4expires"]; ok {
				expiry, err := time.Parse(time.RFC3339, expires)
				if err != nil {
					log.Printf("Could not handle expiry for instance %s: %s: %s", aws.StringValue(instance.InstanceId), expires, err)
					continue
				}
				if expiry.Before(now) && expiry.After(time.Date(1985, time.April, 29, 0, 0, 0, 0, time.UTC)) {
					// test for termination protection
					attr, err := svc.DescribeInstanceAttribute(&ec2.DescribeInstanceAttributeInput{
						Attribute:  aws.String(ec2.InstanceAttributeNameDisableApiTermination),
						InstanceId: instance.InstanceId,
					})
					if err == nil && aws.BoolValue(attr.DisableApiTermination.Value) {
						log.Printf("Not terminating %s, ApiTermination protection is enabled", *instance.InstanceId)
						continue
					}
					deleteList = append(deleteList, aws.StringValue(instance.InstanceId))
					name := tags["Aerolab4ClusterName"]
					node := tags["Aerolab4NodeNumber"]
					if name == "" || node == "" {
						name = tags["Aerolab4clientClusterName"]
						node = tags["Aerolab4clientNodeNumber"]
					}
					deleteListForLog = append(deleteListForLog, fmt.Sprintf("instanceId=%s clusterName=%s nodeNo=%s", aws.StringValue(instance.InstanceId), name, node))
					if _, ok := tags["telemetry"]; ok {
						telemetryShip(tags["telemetry"], aws.StringValue(instance.Placement.AvailabilityZone), aws.StringValue(instance.InstanceId), name, node)
					}
				}
			}
		}
	}

	// expire if found
	log.Printf("Enumerated through %d instances, shutting down %d instances", enumCount, len(deleteList))
	if len(deleteList) > 0 {
		// expire instances
		for _, del := range deleteListForLog {
			log.Printf("Removing: %s", del)
		}
		_, err = svc.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: aws.StringSlice(deleteList),
		})
		if err != nil {
			return makeErrorResponse("Could not terminate instances: ", err)
		}
	}

	//// EXPIRE EFS
	fsList := []string{}
	mtList := []string{}
	fsCount := 0
	efsSvc := efs.New(sess)
	err = efsSvc.DescribeFileSystemsPages(&efs.DescribeFileSystemsInput{}, func(vol *efs.DescribeFileSystemsOutput, lastPage bool) bool {
		for _, fs := range vol.FileSystems {
			fsCount++
			allTags := make(map[string]string)
			for _, tag := range fs.Tags {
				allTags[*tag.Key] = *tag.Value
			}
			if lastUsed, ok := allTags["lastUsed"]; ok {
				if expireDuration, ok := allTags["expireDuration"]; ok {
					lu, err := time.Parse(time.RFC3339, lastUsed)
					if err == nil && !lu.IsZero() {
						ed, err := time.ParseDuration(expireDuration)
						if err == nil && ed > 0 {
							expiresTime := lu.Add(ed)
							if expiresTime.Before(time.Now()) {
								fsList = append(fsList, aws.StringValue(fs.FileSystemId))
								if aws.Int64Value(fs.NumberOfMountTargets) > 0 {
									mts, err := efsSvc.DescribeMountTargets(&efs.DescribeMountTargetsInput{
										FileSystemId: aws.String(*fs.FileSystemId),
									})
									if err != nil {
										continue
									}
									for _, mt := range mts.MountTargets {
										mtList = append(mtList, aws.StringValue(mt.MountTargetId))
									}
								}
							}
						}
					}
				}
			}
		}
		return true
	})
	if err != nil {
		return makeErrorResponse("Could not list EFS: ", err)
	}

	log.Printf("Enumerated through %d EFS, shutting down %d EFS", fsCount, len(fsList))
	if len(fsList) > 0 {
		// expire volumes
		for _, del := range mtList {
			log.Printf("Removing: %s", del)
			_, err = efsSvc.DeleteMountTarget(&efs.DeleteMountTargetInput{
				MountTargetId: aws.String(del),
			})
			if err != nil {
				return makeErrorResponse("Could not delete EFS mount target: ", err)
			}
		}
		for _, del := range fsList {
			for {
				time.Sleep(5 * time.Second)
				targets, err := efsSvc.DescribeMountTargets(&efs.DescribeMountTargetsInput{
					FileSystemId: aws.String(del),
				})
				if err != nil {
					return makeErrorResponse("error waiting on mount targets to be deleted: ", err)
				}
				if len(targets.MountTargets) == 0 {
					break
				}
			}
		}
		for _, del := range fsList {
			log.Printf("Removing: %s", del)
			_, err = efsSvc.DeleteFileSystem(&efs.DeleteFileSystemInput{
				FileSystemId: aws.String(del),
			})
			if err != nil {
				return makeErrorResponse("Could not delete EFS volume: ", err)
			}
		}
	}

	return MyResponse{Message: "Completed", Status: "OK"}, nil
}

func main() {
	lambda.Start(HandleLambdaEvent)
}

func makeErrorResponse(prefix string, err error) (MyResponse, error) {
	return MyResponse{Message: prefix + err.Error(), Status: "ERROR"}, errors.New(prefix + err.Error())
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
		Cloud:         "AWS",
		Zone:          zone,
		Instance:      instance,
		ClusterName:   clusterName,
		NodeNo:        nodeNo,
		ExpiryVersion: "2",
		Time:          time.Now().Unix(),
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
