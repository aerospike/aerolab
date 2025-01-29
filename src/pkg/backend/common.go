package backend

import "encoding/json"

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

func (a LifeCycleState) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a LifeCycleState) MarshalYAML() (interface{}, error) {
	return a.String(), nil
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
	default:
		return "unknown"
	}
}

func (a VolumeState) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a VolumeState) MarshalYAML() (interface{}, error) {
	return a.String(), nil
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
	ArchitectureX8664 Architecture = iota
	ArchitectureARM64 Architecture = iota
)

func (a Architecture) String() string {
	switch a {
	case ArchitectureX8664:
		return "amd64"
	case ArchitectureARM64:
		return "arm64"
	default:
		return "unknown"
	}
}

func (a Architecture) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a Architecture) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}
