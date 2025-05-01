package bgcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	functions "cloud.google.com/go/functions/apiv2"
	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	scheduler "cloud.google.com/go/scheduler/apiv1"
	"cloud.google.com/go/scheduler/apiv1/schedulerpb"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

func (s *b) ExpiryRemove(zones ...string) error {
	log := s.log.WithPrefix("ExpiryRemove: job=" + shortuuid.New() + " ")
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

	for _, region := range zones {
		cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
		if err != nil {
			return fmt.Errorf("failed to create HTTP client: %w", err)
		}

		// Remove Scheduler Job
		if err := s.removeSchedulerJob(region, cli); err != nil {
			// Log, but continue to attempt function deletion
			log.Detail("Error removing scheduler in region %s: %v", region, err)
			return err
		}

		// Remove Cloud Function
		if err := s.removeFunction(region, cli); err != nil {
			log.Detail("Error removing function in region %s: %v", region, err)
			return err // Stop here if function removal fails
		}

		log.Detail("Removed expiry system in region: %s", region)
	}

	return nil
}

func (s *b) removeSchedulerJob(region string, cli *google.Credentials) error {
	ctx := context.Background()
	client, err := scheduler.NewCloudSchedulerClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create scheduler client: %w", err)
	}
	defer client.Close()

	jobName := fmt.Sprintf("projects/%s/locations/%s/jobs/aerolab-expiry", s.credentials.Project, region)
	err = client.DeleteJob(ctx, &schedulerpb.DeleteJobRequest{Name: jobName})
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to delete scheduler job: %w", err)
	}
	return nil
}

func (s *b) removeFunction(region string, cli *google.Credentials) error {
	ctx := context.Background()
	client, err := functions.NewFunctionClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create functions client: %w", err)
	}
	defer client.Close()

	funcName := fmt.Sprintf("projects/%s/locations/%s/functions/aerolab-expiry", s.credentials.Project, region)
	op, err := client.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{Name: funcName})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete function: %w", err)
	}

	// Wait for deletion to complete
	if err := op.Wait(ctx); err != nil {
		return fmt.Errorf("deletion operation failed: %w", err)
	}

	return nil
}
