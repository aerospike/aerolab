package backends

// MigrateV7Input contains parameters for migrating v7 resources to v8 format.
//
// This structure is used by the MigrateV7Resources method in each cloud backend
// to control the migration behavior.
type MigrateV7Input struct {
	// Project name for tagging
	Project string
	// DryRun when true performs discovery only without making changes
	DryRun bool
	// Force when true re-migrates already migrated resources
	Force bool
	// SSHKeyInfo contains paths for SSH key migration
	SSHKeyInfo *SSHKeyPathInfo
	// AerolabVersion is the current version of aerolab for tagging
	AerolabVersion string
}

// SSHKeyPathInfo contains information about SSH key paths for migration.
type SSHKeyPathInfo struct {
	// KeysDir is the directory containing old key files (default: ~/aerolab-keys/)
	KeysDir string
	// SharedKeyPath is the path to shared key if exists (empty if none)
	// When set, per-cluster key migration is skipped
	SharedKeyPath string
	// IsAerolabHome is true if KeysDir was derived from an aerolab home directory
	IsAerolabHome bool
}

// MigrationResult contains the results of a v7 to v8 migration operation.
type MigrationResult struct {
	// DryRun indicates if this was a dry-run (no changes made)
	DryRun bool
	// InstancesMigrated is the count of successfully migrated instances
	InstancesMigrated int
	// VolumesMigrated is the count of successfully migrated volumes
	VolumesMigrated int
	// ImagesMigrated is the count of successfully migrated images
	ImagesMigrated int
	// FirewallsMigrated is the count of successfully migrated firewalls
	FirewallsMigrated int
	// SSHKeysMigrated is the count of successfully copied SSH keys
	SSHKeysMigrated int
	// Errors contains all errors encountered during migration
	Errors []error

	// DryRunInstances contains details of instances that WOULD be migrated (dry-run)
	DryRunInstances []MigrationInstanceDetail
	// DryRunVolumes contains details of volumes that WOULD be migrated (dry-run)
	DryRunVolumes []MigrationVolumeDetail
	// DryRunImages contains details of images that WOULD be migrated (dry-run)
	DryRunImages []MigrationImageDetail
	// DryRunFirewalls contains details of firewalls that WOULD be migrated (dry-run)
	DryRunFirewalls []MigrationFirewallDetail
	// DryRunSSHKeys contains details of SSH keys that WOULD be copied (dry-run)
	DryRunSSHKeys []MigrationSSHKeyDetail

	// MigratedInstances contains details of instances that WERE migrated
	MigratedInstances []MigrationInstanceDetail
	// MigratedVolumes contains details of volumes that WERE migrated
	MigratedVolumes []MigrationVolumeDetail
	// MigratedImages contains details of images that WERE migrated
	MigratedImages []MigrationImageDetail
	// MigratedFirewalls contains details of firewalls that WERE migrated
	MigratedFirewalls []MigrationFirewallDetail
	// MigratedSSHKeys contains details of SSH keys that WERE copied
	MigratedSSHKeys []MigrationSSHKeyDetail
}

// MigrationInstanceDetail contains details about an instance migration.
type MigrationInstanceDetail struct {
	// InstanceID is the cloud provider's instance ID
	InstanceID string
	// Name is the instance name
	Name string
	// ClusterName is the aerolab cluster name
	ClusterName string
	// NodeNo is the node number within the cluster
	NodeNo int
	// Zone is the availability zone or region
	Zone string
	// IsClient indicates if this is a client instance (vs server)
	IsClient bool

	// TagsToAdd contains new v8 tags that will be added
	TagsToAdd map[string]string
	// TagsAdded contains tags that were actually added (after migration)
	TagsAdded map[string]string
	// TagsToPrefix contains old tags that will be preserved with v7- prefix
	TagsToPrefix map[string]string
	// TagsToRemove contains labels that must be removed (GCP only, for 64-label limit)
	TagsToRemove []string

	// SSHKeyFrom is the source path of the SSH key
	SSHKeyFrom string
	// SSHKeyTo is the destination path for the SSH key
	SSHKeyTo string
	// SSHKeyMigrated indicates if the SSH key was successfully copied
	SSHKeyMigrated bool

	// LabelLimitInfo is GCP-only info about label count (e.g., "current=45, adding=12")
	LabelLimitInfo string

	// MigrationStatus is "success", "failed", or "skipped"
	MigrationStatus string
	// MigrationError contains the error message if failed
	MigrationError string
}

// MigrationVolumeDetail contains details about a volume migration.
type MigrationVolumeDetail struct {
	// VolumeID is the cloud provider's volume ID
	VolumeID string
	// VolumeType is the type (e.g., "efs", "ebs", "pd-ssd")
	VolumeType string
	// Name is the volume name
	Name string
	// Zone is the availability zone or region
	Zone string
	// AttachedToInstance is the instance ID this volume is attached to (empty if standalone)
	AttachedToInstance string
	// DeleteOnTermination indicates if the volume is deleted when the instance is terminated
	DeleteOnTermination bool
	// TagsToAdd contains new v8 tags that will be added
	TagsToAdd map[string]string
	// TagsAdded contains tags that were actually added
	TagsAdded map[string]string
	// MigrationStatus is "success", "failed", or "skipped"
	MigrationStatus string
	// MigrationError contains the error message if failed
	MigrationError string
}

// MigrationImageDetail contains details about an image migration.
type MigrationImageDetail struct {
	// ImageID is the cloud provider's image ID (AMI or GCP image)
	ImageID string
	// Name is the image name
	Name string
	// Zone is the region
	Zone string
	// OSName is parsed from image name (e.g., "ubuntu")
	OSName string
	// OSVersion is parsed from image name (e.g., "22.04")
	OSVersion string
	// AerospikeVersion is parsed from image name (e.g., "7.0.0")
	AerospikeVersion string
	// Architecture is parsed from image name (e.g., "amd", "arm")
	Architecture string
	// TagsToAdd contains new v8 tags that will be added
	TagsToAdd map[string]string
	// TagsAdded contains tags that were actually added
	TagsAdded map[string]string
	// MigrationStatus is "success", "failed", or "skipped"
	MigrationStatus string
	// MigrationError contains the error message if failed
	MigrationError string
}

// MigrationFirewallDetail contains details about a firewall migration.
type MigrationFirewallDetail struct {
	// FirewallID is the cloud provider's firewall/security group ID
	FirewallID string
	// Name is the firewall name
	Name string
	// Zone is the region
	Zone string
	// VPCID is the VPC/Network ID the firewall belongs to
	VPCID string
	// TagsToAdd contains new v8 tags that will be added
	TagsToAdd map[string]string
	// TagsAdded contains tags that were actually added
	TagsAdded map[string]string
	// MigrationStatus is "success", "failed", or "skipped"
	MigrationStatus string
	// MigrationError contains the error message if failed
	MigrationError string
}

// MigrationSSHKeyDetail contains details about an SSH key migration.
type MigrationSSHKeyDetail struct {
	// ClusterName is the cluster this key belongs to
	ClusterName string
	// Region is AWS-only: the region component of the key name
	Region string
	// FromPath is the source path of the key
	FromPath string
	// ToPath is the destination path for the key
	ToPath string
	// Copied indicates if the copy was successful
	Copied bool
	// Error contains the error message if copy failed
	Error string
}

// OldInstance represents a discovered v7 instance for migration.
// This is used internally by the migration discovery process.
type OldInstance struct {
	// InstanceID is the cloud provider's instance ID
	InstanceID string
	// Name is the instance name
	Name string
	// ClusterName is the v7 cluster name
	ClusterName string
	// NodeNo is the node number
	NodeNo int
	// Zone is the availability zone or region
	Zone string
	// IsClient indicates if this is a client instance
	IsClient bool
	// Tags contains all v7 tags (AWS) or labels (GCP)
	Tags map[string]string
}

// OldVolume represents a discovered v7 volume for migration.
type OldVolume struct {
	// VolumeID is the cloud provider's volume ID
	VolumeID string
	// Name is the volume name
	Name string
	// VolumeType is the type (e.g., "efs", "ebs", "pd-ssd")
	VolumeType string
	// Zone is the availability zone or region
	Zone string
	// Tags contains all v7 tags or labels
	Tags map[string]string
}

// OldImage represents a discovered v7 image for migration.
type OldImage struct {
	// ImageID is the cloud provider's image ID
	ImageID string
	// Name is the image name
	Name string
	// Zone is the region
	Zone string
	// Tags contains all v7 tags or labels
	Tags map[string]string
}

// OldAttachedVolume represents a volume attached to an instance for migration.
type OldAttachedVolume struct {
	// VolumeID is the cloud provider's volume ID
	VolumeID string
	// Name is the volume name (from tags)
	Name string
	// VolumeType is the volume type (e.g., "gp3", "pd-ssd")
	VolumeType string
	// Zone is the availability zone
	Zone string
	// DeleteOnTermination indicates if the volume is deleted when instance terminates
	DeleteOnTermination bool
	// DeviceName is the device path (e.g., "/dev/sda1")
	DeviceName string
	// Tags contains existing tags/labels on the volume
	Tags map[string]string
}

// OldFirewall represents a discovered v7 firewall/security group for migration.
type OldFirewall struct {
	// FirewallID is the cloud provider's firewall/security group ID
	FirewallID string
	// Name is the firewall name
	Name string
	// Zone is the region
	Zone string
	// VPCID is the VPC/Network ID
	VPCID string
	// Tags contains existing tags/labels on the firewall
	Tags map[string]string
}
