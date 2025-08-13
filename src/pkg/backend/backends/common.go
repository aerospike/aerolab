package backends

import (
	"encoding/json"
	"fmt"
	"runtime"

	"gopkg.in/yaml.v3"
)

type LifeCycleState int

const (
	LifeCycleStateCreating    LifeCycleState = iota
	LifeCycleStateCreated     LifeCycleState = iota
	LifeCycleStateStarting    LifeCycleState = iota
	LifeCycleStateRunning     LifeCycleState = iota
	LifeCycleStateStopping    LifeCycleState = iota
	LifeCycleStateStopped     LifeCycleState = iota
	LifeCycleStateTerminating LifeCycleState = iota
	LifeCycleStateTerminated  LifeCycleState = iota
	LifeCycleStateFail        LifeCycleState = iota
	LifeCycleStateConfiguring LifeCycleState = iota
	LifeCycleStateUnknown     LifeCycleState = iota
)

func (a LifeCycleState) String() string {
	switch a {
	case LifeCycleStateCreating:
		return "Creating"
	case LifeCycleStateCreated:
		return "Created"
	case LifeCycleStateStarting:
		return "Starting"
	case LifeCycleStateRunning:
		return "Running"
	case LifeCycleStateStopping:
		return "Stopping"
	case LifeCycleStateStopped:
		return "Stopped"
	case LifeCycleStateTerminating:
		return "Terminating"
	case LifeCycleStateTerminated:
		return "Terminated"
	case LifeCycleStateFail:
		return "Fail"
	case LifeCycleStateConfiguring:
		return "Configuring"
	default:
		return "unknown"
	}
}

func (a *LifeCycleState) FromString(s string) error {
	switch s {
	case "Creating":
		*a = LifeCycleStateCreating
	case "Created":
		*a = LifeCycleStateCreated
	case "Starting":
		*a = LifeCycleStateStarting
	case "Running":
		*a = LifeCycleStateRunning
	case "Stopping":
		*a = LifeCycleStateStopping
	case "Stopped":
		*a = LifeCycleStateStopped
	case "Terminating":
		*a = LifeCycleStateTerminating
	case "Terminated":
		*a = LifeCycleStateTerminated
	case "Fail":
		*a = LifeCycleStateFail
	case "Configuring":
		*a = LifeCycleStateConfiguring
	default:
		return fmt.Errorf("unknown life cycle state: %s", s)
	}
	return nil
}

func (a LifeCycleState) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a LifeCycleState) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

func (a *LifeCycleState) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}

func (a *LifeCycleState) UnmarshalYAML(value *yaml.Node) error {
	var s string
	err := value.Decode(&s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}

type NetworkState int

const (
	NetworkStateUnknown     NetworkState = iota
	NetworkStateAvailable   NetworkState = iota
	NetworkStateConfiguring NetworkState = iota
)

func (a NetworkState) String() string {
	switch a {
	case NetworkStateAvailable:
		return "Available"
	case NetworkStateConfiguring:
		return "Configuring"
	default:
		return "unknown"
	}
}

func (a *NetworkState) FromString(s string) error {
	switch s {
	case "Available":
		*a = NetworkStateAvailable
	case "Configuring":
		*a = NetworkStateConfiguring
	default:
		return fmt.Errorf("unknown network state: %s", s)
	}
	return nil
}

func (a NetworkState) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a NetworkState) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

func (a *NetworkState) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}

func (a *NetworkState) UnmarshalYAML(value *yaml.Node) error {
	var s string
	err := value.Decode(&s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}

type VolumeState int

const (
	VolumeStateCreating    VolumeState = iota
	VolumeStateAvailable   VolumeState = iota
	VolumeStateInUse       VolumeState = iota
	VolumeStateDeleting    VolumeState = iota
	VolumeStateDeleted     VolumeState = iota
	VolumeStateFail        VolumeState = iota
	VolumeStateConfiguring VolumeState = iota
	VolumeStateUnknown     VolumeState = iota
	VolumeStateAttaching   VolumeState = iota
	VolumeStateDetaching   VolumeState = iota
)

func (a VolumeState) String() string {
	switch a {
	case VolumeStateCreating:
		return "Creating"
	case VolumeStateAvailable:
		return "Available"
	case VolumeStateInUse:
		return "InUse"
	case VolumeStateDeleting:
		return "Deleting"
	case VolumeStateDeleted:
		return "Deleted"
	case VolumeStateFail:
		return "Fail"
	case VolumeStateConfiguring:
		return "Configuring"
	case VolumeStateUnknown:
		return "Unknown"
	case VolumeStateAttaching:
		return "Attaching"
	case VolumeStateDetaching:
		return "Detaching"
	default:
		return "unknown"
	}
}

func (a *VolumeState) FromString(s string) error {
	switch s {
	case "Creating":
		*a = VolumeStateCreating
	case "Available":
		*a = VolumeStateAvailable
	case "InUse":
		*a = VolumeStateInUse
	case "Deleting":
		*a = VolumeStateDeleting
	case "Deleted":
		*a = VolumeStateDeleted
	case "Fail":
		*a = VolumeStateFail
	case "Configuring":
		*a = VolumeStateConfiguring
	case "Unknown":
		*a = VolumeStateUnknown
	case "Attaching":
		*a = VolumeStateAttaching
	case "Detaching":
		*a = VolumeStateDetaching
	default:
		return fmt.Errorf("unknown volume state: %s", s)
	}
	return nil
}
func (a VolumeState) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a VolumeState) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

func (a *VolumeState) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}

func (a *VolumeState) UnmarshalYAML(value *yaml.Node) error {
	var s string
	err := value.Decode(&s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}

type StorageSize int64

const (
	StorageKiB StorageSize = 1024
	StorageMiB StorageSize = StorageKiB * 1024
	StorageGiB StorageSize = StorageMiB * 1024
	StorageTiB StorageSize = StorageGiB * 1024
	StorageKB  StorageSize = 1000
	StorageMB  StorageSize = StorageKB * 1000
	StorageGB  StorageSize = StorageMB * 1000
	StorageTB  StorageSize = StorageGB * 1000
)

type Architecture int

const (
	ArchitectureX8664  Architecture = iota
	ArchitectureARM64  Architecture = iota
	ArchitectureNative Architecture = iota
)

func (a Architecture) String() string {
	switch a {
	case ArchitectureX8664:
		return "amd64"
	case ArchitectureARM64:
		return "arm64"
	case ArchitectureNative:
		switch runtime.GOARCH {
		case "amd64":
			return "amd64"
		case "arm64":
			return "arm64"
		default:
			return "unknown"
		}
	default:
		return "unknown"
	}
}

func (a *Architecture) FromString(s string) error {
	switch s {
	case "amd64":
		*a = ArchitectureX8664
	case "arm64":
		*a = ArchitectureARM64
	case "native", "default":
		*a = ArchitectureNative
	default:
		return fmt.Errorf("unknown architecture: %s", s)
	}
	return nil
}

func (a Architecture) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a Architecture) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

func (a *Architecture) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}

func (a *Architecture) UnmarshalYAML(value *yaml.Node) error {
	var s string
	err := value.Decode(&s)
	if err != nil {
		return err
	}
	return a.FromString(s)
}
