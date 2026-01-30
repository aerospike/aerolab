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
	scheduler "cloud.google.com/go/scheduler/apiv1"
	"cloud.google.com/go/scheduler/apiv1/schedulerpb"
	serviceusage "cloud.google.com/go/serviceusage/apiv1"
	serviceusagepb "cloud.google.com/go/serviceusage/apiv1/serviceusagepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func (s *b) ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeConfiguration: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	ctx := context.Background()

	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}

	for _, region := range zones {
		getResp, err := s.getFunction(ctx, cli, region)
		if err != nil {
			return fmt.Errorf("failed to get function: %w", err)
		}
		client, err := functions.NewFunctionClient(ctx, option.WithCredentials(cli))
		if err != nil {
			return fmt.Errorf("failed to create function client: %w", err)
		}
		defer client.Close()

		getResp.ServiceConfig.EnvironmentVariables["AEROLAB_LOG_LEVEL"] = strconv.Itoa(logLevel)
		getResp.ServiceConfig.EnvironmentVariables["AEROLAB_CLEANUP_DNS"] = strconv.FormatBool(cleanupDNS)

		updateReq := &functionspb.UpdateFunctionRequest{
			Function: getResp,
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"service_config.environment_variables"},
			},
		}

		op, err := client.UpdateFunction(ctx, updateReq)
		if err != nil {
			return fmt.Errorf("failed to update function: %w", err)
		}

		if _, err := op.Wait(ctx); err != nil {
			return fmt.Errorf("update operation failed: %w", err)
		}
	}
	return nil
}

type ExpirySystemDetail struct {
	LogLevel         int
	CleanupDNS       bool
	Job              *schedulerpb.Job
	Function         *functionspb.Function
	DescriptionField *DescriptionField
}

type DescriptionField struct {
	ExpiryVersion string
	ExpiryToken   string
}

func (s *b) ExpiryList() ([]*backends.ExpirySystem, error) {
	log := s.log.WithPrefix("ExpiryList: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	ctx := context.Background()
	enabledServices, err := s.listEnabledServices()
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled services: %w", err)
	}
	for _, service := range expiryServices {
		if !slices.Contains(enabledServices, service) {
			log.Detail("Service %s is not enabled, expiry system is not installed", service)
			return nil, nil
		}
	}
	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}
	expirySystems := []*backends.ExpirySystem{}
	enabledRegions, err := s.ListEnabledZones()
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled zones: %w", err)
	}
	for _, region := range enabledRegions {
		function, err := s.getFunction(ctx, cli, region)
		if err != nil {
			log.Detail("Function not installed in %s: %s", region, err)
			continue
		}
		description := &DescriptionField{}
		if err := json.Unmarshal([]byte(function.Description), description); err != nil {
			return nil, fmt.Errorf("failed to unmarshal function description: %w", err)
		}
		installSuccess := true
		job, err := s.getScheduler(ctx, cli, region)
		freq := -1
		if err != nil {
			log.Detail("Scheduler not installed in %s: %s", region, err)
			installSuccess = false
		} else {
			freq, err = cronToInterval(job.Schedule)
			if err != nil {
				return nil, fmt.Errorf("failed to convert cron to interval: %w", err)
			}
			jobDescription := &DescriptionField{}
			if err := json.Unmarshal([]byte(job.Description), jobDescription); err != nil {
				return nil, fmt.Errorf("failed to unmarshal job description: %w", err)
			}
			if jobDescription.ExpiryVersion != description.ExpiryVersion {
				log.Detail("Scheduler and function versions differ in %s: %s != %s", region, jobDescription.ExpiryVersion, description.ExpiryVersion)
				installSuccess = false
			}
			if jobDescription.ExpiryToken != description.ExpiryToken {
				log.Detail("Scheduler and function tokens differ in %s: %s != %s", region, jobDescription.ExpiryToken, description.ExpiryToken)
				installSuccess = false
			}
		}
		logLevel, err := strconv.Atoi(function.ServiceConfig.EnvironmentVariables["AEROLAB_LOG_LEVEL"])
		if err != nil {
			return nil, fmt.Errorf("failed to convert log level to int: %w", err)
		}
		cleanupDNS := function.ServiceConfig.EnvironmentVariables["AEROLAB_CLEANUP_DNS"] == "true"
		expirySystems = append(expirySystems, &backends.ExpirySystem{
			BackendType:         backends.BackendTypeGCP,
			Zone:                region,
			Version:             description.ExpiryVersion,
			InstallationSuccess: installSuccess,
			FrequencyMinutes:    freq,
			BackendSpecific: &ExpirySystemDetail{
				LogLevel:         logLevel,
				CleanupDNS:       cleanupDNS,
				Job:              job,
				Function:         function,
				DescriptionField: description,
			},
		})
	}
	return expirySystems, nil
}

func (s *b) ExpiryChangeFrequency(intervalMinutes int, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeFrequency: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	ctx := context.Background()

	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	client, err := scheduler.NewCloudSchedulerClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	cron, err := intervalToCron(intervalMinutes)
	if err != nil {
		return err
	}
	for _, zone := range zones {
		job, err := s.getScheduler(ctx, cli, zone)
		if err != nil {
			return err
		}
		job.Schedule = cron
		_, err = client.UpdateJob(ctx, &schedulerpb.UpdateJobRequest{
			Job: job,
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"schedule"},
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) InstancesChangeExpiry(instances backends.InstanceList, expiry time.Time) error {
	log := s.log.WithPrefix("InstancesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// If expiry is zero, remove the tag to indicate no expiry
	if expiry.IsZero() {
		return instances.RemoveTags([]string{TAG_AEROLAB_EXPIRES})
	}
	return instances.AddTags(map[string]string{TAG_AEROLAB_EXPIRES: expiry.Format(time.RFC3339)})
}

func (s *b) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	log := s.log.WithPrefix("VolumesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// If expiry is zero, remove the tag to indicate no expiry
	if expiry.IsZero() {
		return volumes.RemoveTags([]string{TAG_AEROLAB_EXPIRES}, 2*time.Minute)
	}
	return volumes.AddTags(map[string]string{TAG_AEROLAB_EXPIRES: expiry.Format(time.RFC3339)}, 2*time.Minute)
}

func (s *b) getScheduler(ctx context.Context, cli *google.Credentials, region string) (*schedulerpb.Job, error) {
	client, err := scheduler.NewCloudSchedulerClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()
	jobName := fmt.Sprintf("projects/%s/locations/%s/jobs/aerolab-expiry", s.credentials.Project, region)
	req := &schedulerpb.GetJobRequest{
		Name: jobName,
	}
	job, err := client.GetJob(ctx, req)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *b) getFunction(ctx context.Context, cli *google.Credentials, region string) (*functionspb.Function, error) {
	client, err := functions.NewFunctionClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()
	fullName := fmt.Sprintf("projects/%s/locations/%s/functions/aerolab-expiry", s.credentials.Project, region)
	req := &functionspb.GetFunctionRequest{
		Name: fullName,
	}
	function, err := client.GetFunction(ctx, req)
	if err != nil {
		return nil, err
	}
	return function, nil
}

var expiryServices = []string{"logging.googleapis.com", "cloudfunctions.googleapis.com", "cloudbuild.googleapis.com", "pubsub.googleapis.com", "cloudscheduler.googleapis.com", "compute.googleapis.com", "run.googleapis.com", "artifactregistry.googleapis.com", "storage.googleapis.com"}

func (s *b) enableExpiryServices() error {
	log := s.log.WithPrefix("enableExpiryServices: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return s.enableService(expiryServices...)
}

func (s *b) enableService(names ...string) error {
	log := s.log.WithPrefix("enableService: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	ctx := context.Background()

	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	client, err := serviceusage.NewClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	var ops []*serviceusage.EnableServiceOperation
	for _, name := range names {
		log.Detail("Enabling service: %s", name)
		name := fmt.Sprintf("projects/%s/services/%s", s.credentials.Project, name)
		op, err := client.EnableService(ctx, &serviceusagepb.EnableServiceRequest{
			Name: name,
		})
		if err != nil {
			return fmt.Errorf("failed to enable cloud-billing API: %w", err)
		}
		ops = append(ops, op)
	}
	for _, op := range ops {
		log.Detail("Waiting for %s API to be enabled", op.Name())
		_, err = op.Wait(ctx)
		if err != nil {
			return fmt.Errorf("failed to wait for %s API to be enabled: %w", op.Name(), err)
		}
	}
	log.Detail("All services enabled")
	return nil
}

func (s *b) listEnabledServices() (names []string, err error) {
	log := s.log.WithPrefix("listEnabledServices: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	ctx := context.Background()

	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}
	client, err := serviceusage.NewClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return nil, fmt.Errorf("failed to create serviceusage client: %w", err)
	}
	defer client.Close()

	it := client.ListServices(ctx, &serviceusagepb.ListServicesRequest{
		Parent: fmt.Sprintf("projects/%s", s.credentials.Project),
		Filter: "state:ENABLED",
	})
	maxRetries := 10
	retryDelay := 2 * time.Second
	for {
		var resp *serviceusagepb.Service
		var innerErr error
		for retry := 0; retry <= maxRetries; retry++ {
			resp, innerErr = it.Next()
			if innerErr == nil || innerErr == iterator.Done {
				break
			}
			st, ok := status.FromError(innerErr)
			if ok && st.Code() == codes.ResourceExhausted {
				log.Detail("Quota exhausted, retrying after delay...")
				time.Sleep(retryDelay)
				continue
			}
			break
		}

		if innerErr == iterator.Done {
			break
		}
		if innerErr != nil {
			return nil, fmt.Errorf("failed to get next service: %w", innerErr)
		}
		if resp.State == serviceusagepb.State_ENABLED {
			parts := strings.Split(resp.Name, "/")
			names = append(names, parts[len(parts)-1])
		}
	}
	return names, nil
}

func intervalToCron(intervalMinutes int) (string, error) {
	cron := "*/" + strconv.Itoa(intervalMinutes) + " * * * *"
	if intervalMinutes >= 60 {
		if intervalMinutes%60 != 0 || intervalMinutes > 1440 {
			return "", errors.New("frequency can be 0-60 in 1-minute increments, or 60-1440 at 60-minute increments")
		}
		if intervalMinutes == 1440 {
			cron = "0 1 * * *"
		} else {
			if intervalMinutes == 60 {
				cron = "0 * * * *"
			} else {
				cron = "0 */" + strconv.Itoa(intervalMinutes/60) + " * * *"
			}
		}
	}
	return cron, nil
}

func cronToInterval(cron string) (int, error) {
	parts := strings.Fields(cron)
	if len(parts) != 5 {
		return 0, errors.New("invalid cron format")
	}

	minuteField := parts[0]
	hourField := parts[1]

	switch {
	case strings.HasPrefix(minuteField, "*/"):
		// Case: "*/5 * * * *" → every N minutes
		n, err := strconv.Atoi(strings.TrimPrefix(minuteField, "*/"))
		if err != nil || n <= 0 {
			return 0, errors.New("invalid minute interval")
		}
		return n, nil

	case minuteField == "0" && strings.HasPrefix(hourField, "*/"):
		// Case: "0 */2 * * *" → every N hours
		n, err := strconv.Atoi(strings.TrimPrefix(hourField, "*/"))
		if err != nil || n <= 0 {
			return 0, errors.New("invalid hour interval")
		}
		return n * 60, nil

	case minuteField == "0" && hourField == "1":
		// Special case: "0 1 * * *" → 1440 minutes (once daily)
		return 1440, nil

	default:
		return 0, errors.New("unsupported cron pattern")
	}
}

// ExpiryV7Check checks if the v7 expiry system is still installed.
// Returns true if v7 expiry system is detected, along with a list of regions where it was found.
// V7 used Cloud Function and Scheduler named "aerolab-expiries" (with 's'), whereas v8 uses "aerolab-expiry".
func (s *b) ExpiryV7Check() (bool, []string, error) {
	log := s.log.WithPrefix("ExpiryV7Check: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	ctx := context.Background()

	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return false, nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	foundRegions := []string{}
	enabledRegions, err := s.ListEnabledZones()
	if err != nil {
		return false, nil, fmt.Errorf("failed to list enabled zones: %w", err)
	}

	for _, region := range enabledRegions {
		// Check for v7 Cloud Function "aerolab-expiries" (with 's')
		v7FunctionFound := false
		funcClient, err := functions.NewFunctionClient(ctx, option.WithCredentials(cli))
		if err != nil {
			log.Detail("Failed to create function client for region %s: %s", region, err)
			continue
		}
		v7FunctionName := fmt.Sprintf("projects/%s/locations/%s/functions/aerolab-expiries", s.credentials.Project, region)
		_, err = funcClient.GetFunction(ctx, &functionspb.GetFunctionRequest{
			Name: v7FunctionName,
		})
		funcClient.Close()
		if err == nil {
			log.Detail("Found v7 Cloud Function 'aerolab-expiries' in region %s", region)
			v7FunctionFound = true
		}

		// Check for v7 Cloud Scheduler Job "aerolab-expiries" (with 's')
		v7SchedulerFound := false
		schedClient, err := scheduler.NewCloudSchedulerClient(ctx, option.WithCredentials(cli))
		if err != nil {
			log.Detail("Failed to create scheduler client for region %s: %s", region, err)
			continue
		}
		v7JobName := fmt.Sprintf("projects/%s/locations/%s/jobs/aerolab-expiries", s.credentials.Project, region)
		_, err = schedClient.GetJob(ctx, &schedulerpb.GetJobRequest{
			Name: v7JobName,
		})
		schedClient.Close()
		if err == nil {
			log.Detail("Found v7 Cloud Scheduler 'aerolab-expiries' in region %s", region)
			v7SchedulerFound = true
		}

		if v7FunctionFound || v7SchedulerFound {
			if !slices.Contains(foundRegions, region) {
				foundRegions = append(foundRegions, region)
			}
		}
	}

	return len(foundRegions) > 0, foundRegions, nil
}
