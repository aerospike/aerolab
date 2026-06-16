package bgcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	functions "cloud.google.com/go/functions/apiv2"
	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	"cloud.google.com/go/iam"
	"cloud.google.com/go/iam/apiv1/iampb"
	run "cloud.google.com/go/run/apiv2"
	scheduler "cloud.google.com/go/scheduler/apiv1"
	"cloud.google.com/go/scheduler/apiv1/schedulerpb"
	"cloud.google.com/go/storage"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
)

// getExpirySystemDetail safely extracts *ExpirySystemDetail from BackendSpecific, initializing it if needed.
// This handles cases where BackendSpecific might be nil, a map (from JSON/YAML deserialization),
// or already the correct type.
func getExpirySystemDetail(expirySystem *backends.ExpirySystem) *ExpirySystemDetail {
	if expirySystem.BackendSpecific == nil {
		expirySystem.BackendSpecific = &ExpirySystemDetail{}
		return expirySystem.BackendSpecific.(*ExpirySystemDetail)
	}
	if esd, ok := expirySystem.BackendSpecific.(*ExpirySystemDetail); ok {
		return esd
	}
	// If it's a map (from JSON/YAML deserialization), try to convert it
	if m, ok := expirySystem.BackendSpecific.(map[string]any); ok {
		jsonBytes, err := json.Marshal(m)
		if err == nil {
			var esd ExpirySystemDetail
			if err := json.Unmarshal(jsonBytes, &esd); err == nil {
				expirySystem.BackendSpecific = &esd
				return &esd
			}
		}
	}
	// If conversion failed or it's something else, create a new ExpirySystemDetail
	expirySystem.BackendSpecific = &ExpirySystemDetail{}
	return expirySystem.BackendSpecific.(*ExpirySystemDetail)
}

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
	// EnableService returning ENABLED does NOT guarantee the per-project
	// service agent SAs (e.g. service-<num>@gcf-admin-robot.iam.gserviceaccount.com)
	// have been provisioned yet -- Google creates those asynchronously. If we
	// proceed straight to bucket-IAM grants below, GCP rejects the binding with
	// `400 Service account ... does not exist., invalid`. Force the agents into
	// existence via the v1beta1 generateServiceIdentity API, which is
	// synchronous (returns once the SA is usable).
	if err := s.generateServiceIdentities(context.Background(),
		"cloudfunctions.googleapis.com",
		"cloudbuild.googleapis.com",
		"pubsub.googleapis.com",
		"run.googleapis.com",
	); err != nil {
		// Non-fatal: gcloud also tolerates partial-failure here, and the
		// retry loop in grantGCFServiceAgentBucketAccess provides a second
		// line of defense.
		log.Warn("generateServiceIdentities reported a problem (will retry IAM grants on dial): %s", err)
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
		newZones := []string{}
		for _, region := range zones {
			found := false
			for _, expirySystem := range expirySystems {
				if expirySystem.Zone == region {
					found = true
					if expirySystem.InstallationSuccess {
						installedVersion, _ := strconv.ParseFloat(strings.Trim(expirySystem.Version, "\r\n\t "), 64)
						latestVersion, _ := strconv.ParseFloat(strings.Trim(backends.ExpiryVersion, "\r\n\t "), 64)
						if installedVersion < latestVersion {
							toRemove = append(toRemove, region)
							newZones = append(newZones, region)
						} else {
							log.Info("Not installing, already installed in %s (version %s)", region, strings.Trim(expirySystem.Version, "\r\n\t "))
						}
					} else {
						toRemove = append(toRemove, region)
						newZones = append(newZones, region)
					}
					break
				}
			}
			if !found {
				newZones = append(newZones, region)
			}
		}
		zones = newZones
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
						if expirySystem.FrequencyMinutes > 0 {
							newCron, err = intervalToCron(expirySystem.FrequencyMinutes)
							if err != nil {
								return err
							}
						}
						esd := getExpirySystemDetail(expirySystem)
						newLogLevel, err = strconv.Atoi(esd.Function.ServiceConfig.EnvironmentVariables["AEROLAB_LOG_LEVEL"])
						if err != nil {
							return err
						}
						newCleanupDNS = esd.Function.ServiceConfig.EnvironmentVariables["AEROLAB_CLEANUP_DNS"] == "true"
						genToken = esd.DescriptionField.ExpiryToken
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
			if slices.Contains(binding.Members, "allUsers") {
				log.Detail("allUsers already has run.invoker role")
				return nil
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

	if err := s.grantGCFServiceAgentBucketAccess(ctx, bucket, projectID); err != nil {
		log.Warn("Failed to grant GCF service agent bucket access (function deployment may still succeed): %s", err)
	}

	return nil
}

func (s *b) grantGCFServiceAgentBucketAccess(ctx context.Context, bucket *storage.BucketHandle, projectID string) error {
	log := s.log.WithPrefix("grantGCFServiceAgentBucketAccess: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	projectNumber, err := s.getProjectNumber(ctx, projectID)
	if err != nil {
		return fmt.Errorf("look up project number: %w", err)
	}

	member := fmt.Sprintf("serviceAccount:service-%s@gcf-admin-robot.iam.gserviceaccount.com", projectNumber)
	role := iam.RoleName("roles/storage.objectViewer")

	// SetPolicy can fail with `400 Service account ... does not exist., invalid`
	// when the GCF service agent has not yet been provisioned (see
	// generateServiceIdentities call site for the eager-provision step). On a
	// brand-new project the agent may take up to a couple of minutes to become
	// usable in IAM bindings even after generateServiceIdentity returns
	// success, so retry with backoff before giving up. This mirrors what
	// gcloud functions deploy does internally.
	const (
		maxAttempts    = 12
		initialBackoff = 2 * time.Second
		maxBackoff     = 20 * time.Second
	)
	backoff := initialBackoff
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		policy, err := bucket.IAM().Policy(ctx)
		if err != nil {
			return fmt.Errorf("get bucket IAM policy: %w", err)
		}
		if policy.HasRole(member, role) {
			log.Detail("GCF service agent already has objectViewer on bucket")
			return nil
		}
		policy.Add(member, role)
		err = bucket.IAM().SetPolicy(ctx, policy)
		if err == nil {
			log.Detail("Granted objectViewer to %s (attempt %d)", member, attempt)
			return nil
		}
		lastErr = err
		if !isServiceAgentNotReadyError(err) {
			return fmt.Errorf("set bucket IAM policy: %w", err)
		}
		log.Detail("GCF service agent not yet visible to IAM (attempt %d/%d), retrying in %s", attempt, maxAttempts, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
	return fmt.Errorf("set bucket IAM policy after %d attempts: %w", maxAttempts, lastErr)
}

// isServiceAgentNotReadyError detects the specific
// `400 Service account ... does not exist., invalid` error that GCP returns
// while a service agent is being provisioned asynchronously. We recognise it
// either via *googleapi.Error (typed) or by string match (defensive --
// different storage clients wrap the error differently across versions).
func isServiceAgentNotReadyError(err error) bool {
	if err == nil {
		return false
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == http.StatusBadRequest &&
			(strings.Contains(gerr.Message, "does not exist") ||
				strings.Contains(strings.ToLower(gerr.Message), "invalid")) {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") &&
		(strings.Contains(msg, "service account") || strings.Contains(msg, "@gcf-admin-robot") ||
			strings.Contains(msg, "@gcp-sa-") || strings.Contains(msg, "iam.gserviceaccount.com"))
}

// generateServiceIdentities forces Google to provision the per-project service
// agents for the supplied APIs. EnableService returning ENABLED is necessary
// but not sufficient: the agent SAs (e.g. service-<num>@gcf-admin-robot...)
// are created lazily, and IAM bindings referencing them fail with HTTP 400
// "does not exist" until that lazy creation completes. The serviceusage
// v1beta1 :generateServiceIdentity endpoint is the supported way to force
// synchronous creation.
//
// Each call returns a long-running operation; we poll until done. We use raw
// HTTP because the v1beta1 client is not vendored. The HTTP client returned
// by connect.GetClient is already authenticated.
func (s *b) generateServiceIdentities(ctx context.Context, services ...string) error {
	log := s.log.WithPrefix("generateServiceIdentities: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return fmt.Errorf("get auth client: %w", err)
	}

	var firstErr error
	for _, svc := range services {
		if err := s.generateServiceIdentity(ctx, cli, log, svc); err != nil {
			log.Warn("generateServiceIdentity(%s) failed: %s", svc, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		log.Detail("Service identity ready: %s", svc)
	}
	return firstErr
}

func (s *b) generateServiceIdentity(ctx context.Context, cli *http.Client, log *logger.Logger, service string) error {
	url := fmt.Sprintf("https://serviceusage.googleapis.com/v1beta1/projects/%s/services/%s:generateServiceIdentity",
		s.credentials.Project, service)

	opName, opDone, opErr, err := postLRO(ctx, cli, url, nil)
	if err != nil {
		return fmt.Errorf("POST generateServiceIdentity: %w", err)
	}
	if opDone {
		if opErr != nil {
			return opErr
		}
		return nil
	}

	// Poll the LRO. Most service-identity operations complete in a few
	// seconds; cap the wait at ~2 minutes to avoid hanging the caller on
	// freak GCP control-plane delays.
	const (
		maxPolls    = 60
		pollBackoff = 2 * time.Second
	)
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollBackoff):
		}
		done, opErr2, err := getLRO(ctx, cli, opName)
		if err != nil {
			log.Detail("poll %s: %s (will retry)", opName, err)
			continue
		}
		if done {
			if opErr2 != nil {
				return opErr2
			}
			return nil
		}
	}
	return fmt.Errorf("operation %s did not complete within %s", opName, time.Duration(maxPolls)*pollBackoff)
}

// lroResponse is the minimal shape of a serviceusage Long-Running Operation.
type lroResponse struct {
	Name  string `json:"name"`
	Done  bool   `json:"done"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// postLRO POSTs to a serviceusage v1beta1 endpoint that returns an LRO. It
// returns (operationName, done, opError, transportError).
func postLRO(ctx context.Context, cli *http.Client, url string, body []byte) (string, bool, error, error) {
	if body == nil {
		body = []byte("{}")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", false, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return "", false, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false, nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var op lroResponse
	if err := json.Unmarshal(raw, &op); err != nil {
		return "", false, nil, fmt.Errorf("decode LRO: %w", err)
	}
	if op.Error != nil {
		return op.Name, op.Done, fmt.Errorf("operation error %d: %s", op.Error.Code, op.Error.Message), nil
	}
	return op.Name, op.Done, nil, nil
}

// getLRO polls a serviceusage v1beta1 operation. Returns (done, opError, transportError).
func getLRO(ctx context.Context, cli *http.Client, opName string) (bool, error, error) {
	url := fmt.Sprintf("https://serviceusage.googleapis.com/v1beta1/%s", opName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, nil, err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var op lroResponse
	if err := json.Unmarshal(raw, &op); err != nil {
		return false, nil, fmt.Errorf("decode LRO: %w", err)
	}
	if op.Error != nil {
		return op.Done, fmt.Errorf("operation error %d: %s", op.Error.Code, op.Error.Message), nil
	}
	return op.Done, nil, nil
}

// getProjectNumber returns the GCP project number for the given project ID.
//
// Note: the Compute Engine v1 Project.Id field is *not* the GCP project
// number -- its proto comment explicitly states it is "the unique identifier
// for the resource ... defined by the server ... *not* the project ID, and is
// just a unique ID used by Compute Engine to identify resources." Using that
// value to construct a service-agent address such as
// `service-<n>@gcf-admin-robot.iam.gserviceaccount.com` yields a 19-digit
// identifier that does not exist in IAM, which is why earlier versions of
// this code produced
// `400 Service account service-<n>@gcf-admin-robot.iam.gserviceaccount.com
// does not exist., invalid` from grantGCFServiceAgentBucketAccess.
//
// The correct number is exposed by Cloud Resource Manager. We use v3 over
// raw HTTP (same pattern as generateServiceIdentity) to avoid pulling in
// another vendored client; v3 returns `name: "projects/<NUMBER>"` where
// NUMBER is the real project number.
func (s *b) getProjectNumber(ctx context.Context, projectID string) (string, error) {
	log := s.log.WithPrefix("getProjectNumber: ")
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return "", fmt.Errorf("get auth client: %w", err)
	}
	url := fmt.Sprintf("https://cloudresourcemanager.googleapis.com/v3/projects/%s", projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build resourcemanager request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("get project from resourcemanager: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resourcemanager HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("decode resourcemanager response: %w", err)
	}
	const prefix = "projects/"
	if !strings.HasPrefix(body.Name, prefix) {
		return "", fmt.Errorf("unexpected resourcemanager name format %q for project %s", body.Name, projectID)
	}
	number := strings.TrimPrefix(body.Name, prefix)
	if number == "" {
		return "", fmt.Errorf("empty project number for project %s", projectID)
	}
	if _, err := strconv.ParseUint(number, 10, 64); err != nil {
		return "", fmt.Errorf("non-numeric project number %q for project %s: %w", number, projectID, err)
	}
	return number, nil
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
			Runtime:    "go126",
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
