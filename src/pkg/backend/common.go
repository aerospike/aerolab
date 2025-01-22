package backend

import "encoding/json"

type LifeCycleState int

const (
	LifeCycleStateCreating    = iota
	LifeCycleStateCreated     = iota
	LifeCycleStateStarting    = iota
	LifeCycleStateRunning     = iota
	LifeCycleStateStopping    = iota
	LifeCycleStateStopped     = iota
	LifeCycleStateTerminating = iota
	LifeCycleStateTerminated  = iota
	LifeCycleStateFail        = iota
	LifeCycleStateConfiguring = iota
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
