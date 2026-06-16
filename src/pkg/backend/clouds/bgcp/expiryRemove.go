package bgcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	functions "cloud.google.com/go/functions/apiv2"
	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	scheduler "cloud.google.com/go/scheduler/apiv1"
	"cloud.google.com/go/scheduler/apiv1/schedulerpb"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// deleteFunctionWaitTimeout bounds how long we wait for a Cloud Functions v2
// DeleteFunction LRO to reach done=true. GCP gen2 deletes usually finish in
// 30s-2min; allow generous headroom for slow control planes but never block
// forever (see also: zombie-LRO behavior where the underlying resource is
// already gone but the operation never transitions to done).
const deleteFunctionWaitTimeout = 5 * time.Minute

// deleteServiceWaitTimeout bounds how long we wait for a Cloud Run
// DeleteService LRO. Cloud Run service deletion is typically fast (seconds),
// but we cap it defensively.
const deleteServiceWaitTimeout = 3 * time.Minute

// isNotFound returns true when err represents a gRPC NOT_FOUND or contains a
// "not found" substring (defensive: some googleapi paths wrap differently).
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.NotFound {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

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
		log.Detail("Removing expiry system in region: %s", region)
		log.Detail("Getting credentials for region %s", region)
		cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
		if err != nil {
			return fmt.Errorf("failed to create HTTP client: %w", err)
		}

		// Remove Scheduler Job
		log.Detail("Removing Cloud Scheduler job in region %s", region)
		if err := s.removeSchedulerJob(region, cli); err != nil {
			// Log, but continue to attempt function deletion
			log.Detail("Error removing scheduler in region %s: %v", region, err)
			return err
		}
		log.Detail("Cloud Scheduler job removed in region %s", region)

		// Remove Cloud Function
		log.Detail("Removing Cloud Function in region %s", region)
		if err := s.removeFunction(region, cli); err != nil {
			log.Detail("Error removing function in region %s: %v", region, err)
			return err // Stop here if function removal fails
		}
		log.Detail("Cloud Function removed in region %s", region)

		log.Detail("Removed expiry system in region: %s", region)
	}

	return nil
}

func (s *b) removeSchedulerJob(region string, cli *google.Credentials) error {
	log := s.log.WithPrefix("removeSchedulerJob: job=" + shortuuid.New() + " ")
	log.Detail("Start (region=%s)", region)
	defer log.Detail("End (region=%s)", region)
	ctx := context.Background()
	log.Detail("Creating Cloud Scheduler client")
	client, err := scheduler.NewCloudSchedulerClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create scheduler client: %w", err)
	}
	defer client.Close()

	jobName := fmt.Sprintf("projects/%s/locations/%s/jobs/aerolab-expiry", s.credentials.Project, region)
	log.Detail("Calling DeleteJob %s", jobName)
	err = client.DeleteJob(ctx, &schedulerpb.DeleteJobRequest{Name: jobName})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to delete scheduler job: %w", err)
	}
	if err != nil {
		log.Detail("DeleteJob: scheduler job %s did not exist", jobName)
	} else {
		log.Detail("DeleteJob: scheduler job %s deleted", jobName)
	}
	return nil
}

// removeFunction tears down the Cloud Functions v2 (gen2) function used by
// the expiry system. It is defensive against three failure modes that GCP's
// gen2 control plane is known to produce:
//
//  1. Function does not exist: skip entirely (idempotent).
//  2. Function exists but is in STATE_DELETING (e.g. a prior run was
//     interrupted): do not start a new DeleteFunction; instead wait, with a
//     bounded timeout, for GCP to finish what it's already doing. If the
//     deletion fails to complete we still treat a subsequent NOT_FOUND as
//     success.
//  3. Function exists in STATE_FAILED / STATE_UNKNOWN (deployment never
//     fully succeeded, leaving an orphan): the standard DeleteFunction LRO
//     can hang indefinitely against these. We still issue DeleteFunction
//     (bounded by deleteFunctionWaitTimeout), and on timeout we fall back to
//     deleting the underlying Cloud Run service directly so a later install
//     can succeed.
func (s *b) removeFunction(region string, cli *google.Credentials) error {
	log := s.log.WithPrefix("removeFunction: job=" + shortuuid.New() + " ")
	log.Detail("Start (region=%s)", region)
	defer log.Detail("End (region=%s)", region)
	ctx := context.Background()
	log.Detail("Creating Cloud Functions client")
	client, err := functions.NewFunctionClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("failed to create functions client: %w", err)
	}
	defer client.Close()

	funcName := fmt.Sprintf("projects/%s/locations/%s/functions/aerolab-expiry", s.credentials.Project, region)

	// Pre-flight: check current state of the function so we can react
	// appropriately instead of blindly issuing DeleteFunction.
	log.Detail("Calling GetFunction %s (pre-flight)", funcName)
	fn, err := client.GetFunction(ctx, &functionspb.GetFunctionRequest{Name: funcName})
	if err != nil {
		if isNotFound(err) {
			log.Detail("GetFunction: function %s does not exist; nothing to delete", funcName)
			return nil
		}
		return fmt.Errorf("get function: %w", err)
	}

	state := fn.GetState()
	log.Detail("Function %s exists in state=%s", funcName, state)

	switch state {
	case functionspb.Function_DELETING:
		// GCP is already deleting this function (typically from a prior
		// interrupted run). Don't start a new operation; just wait for
		// the existing deletion to finish, bounded by our timeout, and
		// then re-check by GetFunction.
		log.Detail("Function %s is already in DELETING; waiting for existing deletion to finish (timeout=%s)", funcName, deleteFunctionWaitTimeout)
		return s.waitForFunctionGone(ctx, client, funcName, log)

	case functionspb.Function_FAILED, functionspb.Function_UNKNOWN:
		// The function is in a broken state; best-effort delete may
		// hang. Try DeleteFunction with a bounded timeout, then if that
		// fails fall back to deleting the underlying Cloud Run service
		// directly so a later install can succeed.
		log.Detail("Function %s is in %s; attempting bounded delete with Cloud Run fallback", funcName, state)
		return s.deleteFunctionWithFallback(ctx, client, cli, funcName, region, log, true)

	default:
		// STATE_ACTIVE, STATE_DEPLOYING or anything else: normal path.
		return s.deleteFunctionWithFallback(ctx, client, cli, funcName, region, log, false)
	}
}

// deleteFunctionWithFallback issues DeleteFunction and waits for the LRO
// with a bounded timeout. If the wait times out (or fails) but a subsequent
// GetFunction returns NOT_FOUND, we treat the operation as successful.
// When allowCloudRunFallback is true, a timed-out wait additionally triggers
// a best-effort deletion of the underlying Cloud Run service so the next
// install can proceed.
func (s *b) deleteFunctionWithFallback(ctx context.Context, client *functions.FunctionClient, cli *google.Credentials, funcName, region string, log *logger.Logger, allowCloudRunFallback bool) error {
	log.Detail("Calling DeleteFunction %s", funcName)
	op, err := client.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{Name: funcName})
	if err != nil {
		if isNotFound(err) {
			log.Detail("DeleteFunction: function %s did not exist", funcName)
			return nil
		}
		return fmt.Errorf("failed to delete function: %w", err)
	}

	log.Detail("Waiting for DeleteFunction operation to complete (timeout=%s)", deleteFunctionWaitTimeout)
	waitCtx, cancel := context.WithTimeout(ctx, deleteFunctionWaitTimeout)
	defer cancel()
	waitErr := op.Wait(waitCtx)
	if waitErr == nil {
		log.Detail("DeleteFunction operation complete for %s", funcName)
		return nil
	}

	// op.Wait returned an error: either timeout (ctx.Err()) or LRO failure.
	// If the function is now gone, the deletion succeeded server-side even
	// though the LRO never reported done; treat as success.
	log.Warn("DeleteFunction wait failed for %s: %s; verifying state", funcName, waitErr)
	if _, gerr := client.GetFunction(ctx, &functionspb.GetFunctionRequest{Name: funcName}); gerr != nil && isNotFound(gerr) {
		log.Detail("Function %s is gone post-wait; treating as success", funcName)
		return nil
	}

	if allowCloudRunFallback && errors.Is(waitErr, context.DeadlineExceeded) {
		log.Warn("DeleteFunction LRO did not complete within %s; attempting Cloud Run service fallback cleanup", deleteFunctionWaitTimeout)
		if crErr := s.deleteCloudRunService(ctx, cli, region, log); crErr != nil {
			return fmt.Errorf("deletion operation timed out and Cloud Run fallback failed: wait=%w, fallback=%v", waitErr, crErr)
		}
		// After deleting Cloud Run, re-check the function.
		if _, gerr := client.GetFunction(ctx, &functionspb.GetFunctionRequest{Name: funcName}); gerr != nil && isNotFound(gerr) {
			log.Detail("Function %s gone after Cloud Run fallback; treating as success", funcName)
			return nil
		}
		log.Warn("Cloud Run fallback completed but function record %s still present; reporting failure", funcName)
	}

	return fmt.Errorf("deletion operation failed: %w", waitErr)
}

// waitForFunctionGone polls GetFunction until it returns NOT_FOUND or the
// timeout elapses. Used when a function is already in STATE_DELETING and we
// don't want to spawn a new DeleteFunction LRO.
func (s *b) waitForFunctionGone(ctx context.Context, client *functions.FunctionClient, funcName string, log *logger.Logger) error {
	waitCtx, cancel := context.WithTimeout(ctx, deleteFunctionWaitTimeout)
	defer cancel()
	const pollInterval = 10 * time.Second
	for {
		_, err := client.GetFunction(waitCtx, &functionspb.GetFunctionRequest{Name: funcName})
		if err != nil && isNotFound(err) {
			log.Detail("Function %s confirmed deleted", funcName)
			return nil
		}
		if err != nil && !isNotFound(err) {
			// Transient or auth error; log and keep polling until timeout.
			log.Detail("GetFunction poll error (will retry): %s", err)
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("function %s still present after %s while waiting for in-progress deletion to finish", funcName, deleteFunctionWaitTimeout)
		case <-time.After(pollInterval):
		}
	}
}

// deleteCloudRunService best-effort deletes the Cloud Run service that backs
// a gen2 Cloud Function. This is the escape hatch when DeleteFunction itself
// is stuck against a FAILED/UNKNOWN function.
func (s *b) deleteCloudRunService(ctx context.Context, cli *google.Credentials, region string, log *logger.Logger) error {
	log = log.WithPrefix("deleteCloudRunService: ")
	log.Detail("Start (region=%s)", region)
	defer log.Detail("End (region=%s)", region)

	client, err := run.NewServicesClient(ctx, option.WithCredentials(cli))
	if err != nil {
		return fmt.Errorf("create Cloud Run client: %w", err)
	}
	defer client.Close()

	serviceName := fmt.Sprintf("projects/%s/locations/%s/services/aerolab-expiry", s.credentials.Project, region)
	log.Detail("Calling DeleteService %s", serviceName)
	op, err := client.DeleteService(ctx, &runpb.DeleteServiceRequest{Name: serviceName})
	if err != nil {
		if isNotFound(err) {
			log.Detail("Cloud Run service %s did not exist", serviceName)
			return nil
		}
		return fmt.Errorf("DeleteService: %w", err)
	}

	log.Detail("Waiting for Cloud Run DeleteService operation (timeout=%s)", deleteServiceWaitTimeout)
	waitCtx, cancel := context.WithTimeout(ctx, deleteServiceWaitTimeout)
	defer cancel()
	if _, werr := op.Wait(waitCtx); werr != nil {
		if isNotFound(werr) {
			log.Detail("Cloud Run service %s already gone", serviceName)
			return nil
		}
		return fmt.Errorf("DeleteService wait: %w", werr)
	}
	log.Detail("Cloud Run service %s deleted", serviceName)
	return nil
}
