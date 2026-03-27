package expire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

// telemetryTagKey is the tag key used to identify resources that should have telemetry sent on expiry
const telemetryTagKey = "aerolab.telemetry"

// telemetryURL is the endpoint for sending telemetry data
const telemetryURL = "https://aerolab-telemetry-595313549904.us-central1.run.app"

// telemetryExpiryVersion is the version of the expiry telemetry format
const telemetryExpiryVersion = "5"

// expiryTelemetry represents the telemetry data sent when expiring resources
type expiryTelemetry struct {
	UUID          string
	Job           string
	Cloud         string
	Zone          string
	ResourceID    string
	ClusterUUID   string
	ResourceType  string
	ResourceName  string
	ClusterName   string
	NodeNo        string
	ExpiryVersion string
	CmdLine       []string
	Time          int64
	Tags          map[string]string
}

// telemetryWaitGroup is used to wait for all telemetry goroutines to complete
var telemetryWaitGroup = new(sync.WaitGroup)

type ExpiryHandler struct {
	Backend      backends.Backend
	ExpireEksctl bool
	CleanupDNS   bool
	lock         sync.Mutex
	Credentials  *clouds.Credentials
}

func (h *ExpiryHandler) Expire() error {
	log.Print("Expiry start")
	if !h.lock.TryLock() {
		log.Print("Another invocation is already running, skipping")
		return nil
	}
	defer h.lock.Unlock()

	// Ensure all telemetry goroutines complete before returning
	defer telemetryWaitGroup.Wait()

	log.Print("Lock acquired, listing inventory")
	err := h.Backend.ForceRefreshInventory()
	if err != nil {
		return err
	}
	inventory := h.Backend.GetInventory()

	instances := inventory.Instances.WithExpired(true).WithState(backends.LifeCycleStateRunning, backends.LifeCycleStateUnknown, backends.LifeCycleStateStopped, backends.LifeCycleStateFail, backends.LifeCycleStateCreated, backends.LifeCycleStateConfiguring).Describe()

	if len(instances) > 0 {
		var logLine strings.Builder
		fmt.Fprintf(&logLine, "Terminating %d instances: ", len(instances)) //nolint:errcheck
		for _, instance := range instances {
			fmt.Fprintf(&logLine, "clusterName=%s,nodeNo=%d,instanceID=%s;", instance.ClusterName, instance.NodeNo, instance.InstanceID) //nolint:errcheck
		}
		log.Print(logLine.String())

		// Send telemetry for instances with aerolab.telemetry=true tag before termination
		for _, instance := range instances {
			if v, ok := instance.Tags[telemetryTagKey]; ok && v != "" {
				h.shipInstanceTelemetry(instance)
			}
		}

		err := instances.Terminate(10 * time.Minute)
		if err != nil {
			return err
		}
		log.Printf("Terminated %d instances", len(instances))
	} else {
		log.Print("No instances to terminate")
	}

	volumes := inventory.Volumes.WithDeleteOnTermination(false).WithExpired(true).Describe()
	if len(volumes) > 0 {
		var logLine strings.Builder
		fmt.Fprintf(&logLine, "Deleting %d volumes: ", len(volumes)) //nolint:errcheck
		for _, volume := range volumes {
			fmt.Fprintf(&logLine, "volumeID=%s;", volume.FileSystemId) //nolint:errcheck
		}
		log.Print(logLine.String())

		// Send telemetry for volumes with aerolab.telemetry=true tag before deletion
		for _, volume := range volumes {
			if v, ok := volume.Tags[telemetryTagKey]; ok && v != "" {
				h.shipVolumeTelemetry(volume)
			}
		}

		err := volumes.DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
		if err != nil {
			return err
		}
		log.Printf("Deleted %d volumes", len(volumes))
	} else {
		log.Print("No volumes to delete")
	}

	if h.CleanupDNS {
		log.Print("Cleaning up stale DNS")
		err := h.Backend.CleanupDNS()
		if err != nil {
			return err
		}
	}

	if h.ExpireEksctl {
		regions, err := h.Backend.ListEnabledRegions(backends.BackendTypeAWS)
		if err != nil {
			return err
		}
		for _, region := range regions {
			err = h.expireEksctl(region)
			if err != nil {
				return err
			}
		}
	}

	log.Print("Expiry complete, releasing lock")
	return nil
}

// shipInstanceTelemetry sends telemetry data for an expiring instance asynchronously
func (h *ExpiryHandler) shipInstanceTelemetry(instance *backends.Instance) {
	telemetryWaitGroup.Go(func() {
		err := h.shipInstanceTelemetrySync(instance)
		if err != nil {
			log.Printf("Telemetry: failed to send telemetry for instance %s: %s", instance.InstanceID, err)
		}
	})
}

// shipInstanceTelemetrySync sends telemetry data for an expiring instance synchronously
func (h *ExpiryHandler) shipInstanceTelemetrySync(instance *backends.Instance) error {
	cloud := "unknown"
	switch instance.BackendType {
	case backends.BackendTypeAWS:
		cloud = "AWS"
	case backends.BackendTypeGCP:
		cloud = "GCP"
	case backends.BackendTypeDocker:
		cloud = "Docker"
	}

	t := &expiryTelemetry{
		UUID:          instance.Tags[telemetryTagKey],
		Job:           "expire",
		Cloud:         cloud,
		Zone:          instance.ZoneName,
		ResourceID:    instance.InstanceID,
		ClusterUUID:   instance.ClusterUUID,
		ResourceType:  "instance",
		ClusterName:   instance.ClusterName,
		NodeNo:        strconv.Itoa(instance.NodeNo),
		ResourceName:  instance.Name,
		ExpiryVersion: telemetryExpiryVersion,
		Time:          time.Now().UnixMicro(),
		CmdLine:       []string{"EXPIRY"},
		Tags:          instance.Tags,
	}

	contents, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry: %w", err)
	}

	ret, err := http.Post(telemetryURL, "application/json", bytes.NewReader(contents))
	if err != nil {
		return fmt.Errorf("failed to send telemetry: %w", err)
	}
	defer ret.Body.Close()

	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("telemetry returned status code: %d:%s", ret.StatusCode, ret.Status)
	}

	log.Printf("Telemetry: sent for instance %s (cluster=%s, node=%d)", instance.InstanceID, instance.ClusterName, instance.NodeNo)
	return nil
}

// shipVolumeTelemetry sends telemetry data for an expiring volume asynchronously
func (h *ExpiryHandler) shipVolumeTelemetry(volume *backends.Volume) {
	telemetryWaitGroup.Go(func() {
		err := h.shipVolumeTelemetrySync(volume)
		if err != nil {
			log.Printf("Telemetry: failed to send telemetry for volume %s: %s", volume.FileSystemId, err)
		}
	})
}

// shipVolumeTelemetrySync sends telemetry data for an expiring volume synchronously
func (h *ExpiryHandler) shipVolumeTelemetrySync(volume *backends.Volume) error {
	cloud := "unknown"
	switch volume.BackendType {
	case backends.BackendTypeAWS:
		cloud = "AWS"
	case backends.BackendTypeGCP:
		cloud = "GCP"
	case backends.BackendTypeDocker:
		cloud = "Docker"
	}

	t := &expiryTelemetry{
		UUID:          volume.Tags[telemetryTagKey],
		Job:           "expire",
		Cloud:         cloud,
		Zone:          volume.ZoneName,
		ResourceID:    volume.FileSystemId,
		ResourceType:  "volume",
		ResourceName:  volume.Name,
		ClusterName:   "",
		NodeNo:        "",
		ExpiryVersion: telemetryExpiryVersion,
		Time:          time.Now().UnixMicro(),
		CmdLine:       []string{"EXPIRY"},
		Tags:          volume.Tags,
	}

	contents, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry: %w", err)
	}

	ret, err := http.Post(telemetryURL, "application/json", bytes.NewReader(contents))
	if err != nil {
		return fmt.Errorf("failed to send telemetry: %w", err)
	}
	defer ret.Body.Close()

	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("telemetry returned status code: %d:%s", ret.StatusCode, ret.Status)
	}

	log.Printf("Telemetry: sent for volume %s (name=%s)", volume.FileSystemId, volume.Name)
	return nil
}
