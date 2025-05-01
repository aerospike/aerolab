package bgcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	functions "cloud.google.com/go/functions/apiv2"
	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	"cloud.google.com/go/iam/apiv1/iampb"
	run "cloud.google.com/go/run/apiv2"
	scheduler "cloud.google.com/go/scheduler/apiv1"
	"cloud.google.com/go/scheduler/apiv1/schedulerpb"
	"cloud.google.com/go/storage"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
)

// force true means remove previous expiry systems and install new ones
// force false means install only if previous installation was failed or version is different
// onUpdateKeepOriginalSettings true means keep original settings on update, and only apply specified settings on reinstall
func (s *b) ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryInstall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	enabledServices, err := s.listEnabledServices()
	if err != nil {
		return err
	}
	for _, service := range expiryServices {
		if !slices.Contains(enabledServices, service) {
			log.Detail("Service %s is not enabled, enabling services", service)
			err = s.enableExpiryServices()
			if err != nil {
				return err
			}
			break
		}
	}
	newToken := shortuuid.New() + "-" + shortuuid.New()
	cron, err := intervalToCron(intervalMinutes)
	if err != nil {
		return err
	}
	expirySystems, err := s.ExpiryList()
	if err != nil {
		return err
	}
	toRemove := []string{}
	if force {
		// if force is true, first remove all existing expiry systems, we will be reinstalling them
		for _, region := range zones {
			for _, expirySystem := range expirySystems {
				if expirySystem.Zone == region {
					toRemove = append(toRemove, region)
				}
			}
		}
		if len(toRemove) > 0 {
			err = s.ExpiryRemove(toRemove...)
			if err != nil {
				return err
			}
		}
	} else {
		// if force is false, only remove if the version is different or installation failed
		for _, region := range zones {
			for _, expirySystem := range expirySystems {
				if expirySystem.Zone == region {
					if expirySystem.InstallationSuccess {
						installedVersion, _ := strconv.ParseFloat(strings.Trim(expirySystem.Version, "\r\n\t "), 64)
						latestVersion, _ := strconv.ParseFloat(strings.Trim(backends.ExpiryVersion, "\r\n\t "), 64)
						if installedVersion < latestVersion {
							toRemove = append(toRemove, region)
						}
					} else {
						toRemove = append(toRemove, region)
					}
				}
			}
		}
		if len(toRemove) > 0 {
			err = s.ExpiryRemove(toRemove...)
			if err != nil {
				return err
			}
		}
	}
	for _, region := range zones {
		// any region that exists in toRemove is being reinstalled; if onUpdateKeepOriginalSettings is set, copy settings from previous installation instead of deploying new ones
		newLogLevel := logLevel
		newCleanupDNS := cleanupDNS
		genToken := newToken
		newCron := cron
		if onUpdateKeepOriginalSettings {
			if slices.Contains(toRemove, region) {
				for _, expirySystem := range expirySystems {
					if expirySystem.Zone == region {
						if expirySystem.FrequencyMinutes != -1 {
							newCron, err = intervalToCron(expirySystem.FrequencyMinutes)
							if err != nil {
								return err
							}
						}
						newLogLevel, err = strconv.Atoi(expirySystem.BackendSpecific.(*ExpirySystemDetail).Function.ServiceConfig.EnvironmentVariables["AEROLAB_LOG_LEVEL"])
						if err != nil {
							return err
						}
						newCleanupDNS = expirySystem.BackendSpecific.(*ExpirySystemDetail).Function.ServiceConfig.EnvironmentVariables["AEROLAB_CLEANUP_DNS"] == "true"
						genToken = expirySystem.BackendSpecific.(*ExpirySystemDetail).DescriptionField.ExpiryToken
					}
				}
			}
		}
		ctx := context.Background()
		err = s.deployFunctionBucketCode(ctx, s.credentials.Project, region)
		if err != nil {
			return err
		}
		err = s.deployFunction(ctx, s.credentials.Project, region, genToken, newLogLevel, newCleanupDNS)
		if err != nil {
			return err
		}
		err = s.allowUnauthenticated(ctx, s.credentials.Project, region)
		if err != nil {
			return err
		}
		err = s.createSchedulerJob(ctx, s.credentials.Project, region, genToken, newCron)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) allowUnauthenticated(ctx context.Context, projectID, region string) error {
	log := s.log.WithPrefix("allowUnauthenticated: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	client, err := run.NewServicesClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create functions client: %w", err)
	}
	defer client.Close()
	// Cloud Run service name that backs the function
	serviceName := fmt.Sprintf("projects/%s/locations/%s/services/aerolab-expiry", projectID, region)

	// Fetch current IAM policy
	policyResp, err := client.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{
		Resource: serviceName,
	})
	if err != nil {
		return fmt.Errorf("failed to get Cloud Run IAM policy: %w", err)
	}

	// Check if "allUsers" already has "roles/run.invoker"
	for _, binding := range policyResp.Bindings {
		if binding.Role == "roles/run.invoker" {
			for _, member := range binding.Members {
				if member == "allUsers" {
					log.Detail("allUsers already has run.invoker role")
					return nil
				}
			}
		}
	}

	// Add the allUsers binding to run.invoker
	policyResp.Bindings = append(policyResp.Bindings, &iampb.Binding{
		Role:    "roles/run.invoker",
		Members: []string{"allUsers"},
	})

	// Set updated policy
	_, err = client.SetIamPolicy(ctx, &iampb.SetIamPolicyRequest{
		Resource: serviceName,
		Policy:   policyResp,
	})
	if err != nil {
		return fmt.Errorf("failed to set Cloud Run IAM policy: %w", err)
	}

	log.Detail("Successfully granted unauthenticated access to Cloud Run service")
	return nil
}

func (s *b) deployFunctionBucketCode(ctx context.Context, projectID, region string) error {
	log := s.log.WithPrefix("deployFunctionBucketCode: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Get credentials
	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}

	// Create Storage client
	client, err := storage.NewClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer client.Close()

	// Bucket and object names
	bucketName := "aerolab-expiry-code-" + region
	objectName := "expiry.zip"
	bucket := client.Bucket(bucketName)

	// Check if bucket exists
	_, err = bucket.Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrBucketNotExist) {
			log.Detail("Bucket does not exist, creating...")
			if err := bucket.Create(ctx, projectID, &storage.BucketAttrs{
				Location: region,
			}); err != nil {
				return fmt.Errorf("failed to create bucket: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check bucket attributes: %w", err)
		}
	}

	// Upload the binary as "expiry.zip"
	log.Detail("Uploading expiry.zip...")
	obj := bucket.Object(objectName)
	writer := obj.NewWriter(ctx)
	defer writer.Close()

	// Write the expiry binary content
	if _, err := writer.Write(backends.ExpiryBinary); err != nil {
		return fmt.Errorf("failed to write to object: %w", err)
	}

	log.Detail("Upload complete.")
	return nil
}

func (s *b) deployFunction(ctx context.Context, projectID, region, token string, logLevel int, cleanupDNS bool) error {
	log := s.log.WithPrefix("deployFunction: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	client, err := functions.NewFunctionClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create functions v2 client: %w", err)
	}
	defer client.Close()
	functionName := "aerolab-expiry"
	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, region)
	fullName := fmt.Sprintf("%s/functions/%s", parent, functionName)

	descriptionField := &DescriptionField{
		ExpiryVersion: backends.ExpiryVersion,
		ExpiryToken:   token,
	}
	description, err := json.Marshal(descriptionField)
	if err != nil {
		return fmt.Errorf("failed to marshal description: %w", err)
	}
	function := &functionspb.Function{
		Name:        fullName,
		Description: string(description),
		BuildConfig: &functionspb.BuildConfig{
			Runtime:    "go123",
			EntryPoint: "aerolabExpire",
			Source: &functionspb.Source{
				Source: &functionspb.Source_StorageSource{
					StorageSource: &functionspb.StorageSource{
						Bucket: "aerolab-expiry-code-" + region,
						Object: "expiry.zip",
					},
				},
			},
		},
		ServiceConfig: &functionspb.ServiceConfig{
			AvailableMemory:  "512Mi",
			AvailableCpu:     "1",
			TimeoutSeconds:   60,
			MaxInstanceCount: 2,
			IngressSettings:  functionspb.ServiceConfig_ALLOW_ALL,
			EnvironmentVariables: map[string]string{
				"TOKEN":               token,
				"AEROLAB_LOG_LEVEL":   strconv.Itoa(logLevel),
				"AEROLAB_VERSION":     s.aerolabVersion,
				"AEROLAB_CLEANUP_DNS": strconv.FormatBool(cleanupDNS),
				"GCP_PROJECT":         projectID,
				"GCP_REGION":          region,
			},
		},
	}

	// Try to create the function
	createReq := &functionspb.CreateFunctionRequest{
		Parent:     parent,
		Function:   function,
		FunctionId: functionName,
	}

	op, err := client.CreateFunction(ctx, createReq)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			log.Detail("Function already exists. Updating...")
			updateReq := &functionspb.UpdateFunctionRequest{
				Function: function,
			}
			updateOp, err := client.UpdateFunction(ctx, updateReq)
			if err != nil {
				return fmt.Errorf("update function failed: %w", err)
			}
			if _, err := updateOp.Wait(ctx); err != nil {
				return fmt.Errorf("update operation failed: %w", err)
			}
			return nil
		}
		return fmt.Errorf("create function failed: %w", err)
	}

	if _, err := op.Wait(ctx); err != nil {
		return fmt.Errorf("create operation failed: %w", err)
	}

	return nil
}

func (s *b) createSchedulerJob(ctx context.Context, projectID, region, token string, cron string) error {
	log := s.log.WithPrefix("createSchedulerJob: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	client, err := scheduler.NewCloudSchedulerClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create scheduler client: %w", err)
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, region)
	jobName := "aerolab-expiry"
	uri := fmt.Sprintf("https://%s-%s.cloudfunctions.net/%s", region, projectID, jobName)

	messageBody := fmt.Sprintf(`{"token":"%s"}`, token)

	descriptionField := &DescriptionField{
		ExpiryVersion: backends.ExpiryVersion,
		ExpiryToken:   token,
	}
	description, err := json.Marshal(descriptionField)
	if err != nil {
		return fmt.Errorf("failed to marshal description: %w", err)
	}
	job := &schedulerpb.Job{
		Name:        fmt.Sprintf("%s/jobs/%s", parent, jobName),
		Description: string(description),
		Schedule:    cron,
		TimeZone:    "Etc/UTC",
		RetryConfig: &schedulerpb.RetryConfig{
			MaxBackoffDuration: durationpb.New(15 * time.Second),
			MinBackoffDuration: durationpb.New(5 * time.Second),
			MaxDoublings:       2,
			RetryCount:         0, // Max retry attempts = 0
		},
		Target: &schedulerpb.Job_HttpTarget{
			HttpTarget: &schedulerpb.HttpTarget{
				Uri:        uri,
				HttpMethod: schedulerpb.HttpMethod_POST,
				Body:       []byte(messageBody),
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				// Optionally set OAuth token here if needed
			},
		},
	}

	req := &schedulerpb.CreateJobRequest{
		Parent: parent,
		Job:    job,
	}

	_, err = client.CreateJob(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create scheduler job: %w", err)
	}

	return nil
}
