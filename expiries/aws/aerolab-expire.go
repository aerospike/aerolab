package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
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
	instances, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{})
	if err != nil {
		return makeErrorResponse("Could not list instances: ", err)
	}

	// check each instance for expiry
	now := time.Now()
	deleteList := []string{}
	deleteListForLog := []string{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			tags := make(map[string]string)
			for _, tag := range instance.Tags {
				tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
			}
			if expires, ok := tags["aerolab4expires"]; ok {
				expiry, err := time.Parse(time.RFC3339, expires)
				if err != nil {
					log.Printf("Could not handle expiry for instance %s: %s: %s", aws.StringValue(instance.InstanceId), expires, err)
				}
				if expiry.Before(now) || expiry.After(time.Date(1985, time.April, 29, 0, 0, 0, 0, time.UTC)) {
					deleteList = append(deleteList, aws.StringValue(instance.InstanceId))
					name := tags["Aerolab4ClusterName"]
					node := tags["Aerolab4NodeNumber"]
					if name == "" || node == "" {
						name = tags["Aerolab4clientClusterName"]
						node = tags["Aerolab4clientNodeNumber"]
					}
					deleteListForLog = append(deleteListForLog, fmt.Sprintf("instanceId=%s clusterName=%s nodeNo=%s", aws.StringValue(instance.InstanceId), name, node))
				}
			}
		}
	}

	// exit if no instances found to delete
	if len(deleteList) == 0 {
		return MyResponse{Message: "Completed", Status: "OK"}, nil
	}

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
	return MyResponse{Message: "Completed", Status: "OK"}, nil
}

func main() {
	lambda.Start(HandleLambdaEvent)
}

func makeErrorResponse(prefix string, err error) (MyResponse, error) {
	return MyResponse{Message: prefix + err.Error(), Status: "ERROR"}, errors.New(prefix + err.Error())
}
