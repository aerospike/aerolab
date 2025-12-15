package baws

const (
	TAG_NAME                 = "Name"
	TAG_DESCRIPTION          = "Description"
	TAG_AEROLAB_VERSION      = "AEROLAB_VERSION"
	TAG_AEROLAB_PROJECT      = "AEROLAB_PROJECT"
	TAG_OWNER                = "AEROLAB_OWNER"
	TAG_EXPIRES              = "AEROLAB_EXPIRES"
	TAG_CLUSTER_NAME         = "AEROLAB_CLUSTER_NAME"
	TAG_NODE_NO              = "AEROLAB_NODE_NO"
	TAG_OS_NAME              = "AEROLAB_OS_NAME"
	TAG_OS_VERSION           = "AEROLAB_OS_VERSION"
	TAG_COST_PPH             = "AEROLAB_COST_PPH"
	TAG_COST_SO_FAR          = "AEROLAB_COST_SO_FAR"
	TAG_COST_GB              = "AEROLAB_COST_PER_GB"
	TAG_START_TIME           = "AEROLAB_LAST_START_TIME"
	TAG_DNS_NAME             = "AEROLAB_DNS_NAME"
	TAG_DNS_DOMAIN_ID        = "AEROLAB_DNS_DOMAIN_ID"
	TAG_DNS_DOMAIN_NAME      = "AEROLAB_DNS_DOMAIN_NAME"
	TAG_DNS_REGION           = "AEROLAB_DNS_REGION"
	TAG_CLUSTER_UUID         = "AEROLAB_CLUSTER_UUID"
	TAG_FIREWALL_NAME_PREFIX = "AEROLAB_DEFAULT_"
)

// V7 migration-related constants
const (
	// TAG_V7_MIGRATED marks an instance as migrated from v7
	TAG_V7_MIGRATED = "AEROLAB_V7_MIGRATED"
	// TAG_SOFT_TYPE identifies the software type (e.g., "aerospike", "tools", "graph")
	TAG_SOFT_TYPE = "aerolab.type"
	// TAG_SOFT_VERSION identifies the software version
	TAG_SOFT_VERSION = "aerolab.soft.version"
	// TAG_TELEMETRY is the v8 telemetry tag
	TAG_TELEMETRY = "aerolab.telemetry"
	// TAG_CLIENT_TYPE is the v8 client type tag
	TAG_CLIENT_TYPE = "aerolab.client.type"
	// TAG_IMAGE_TYPE identifies the image type (e.g., "aerospike")
	TAG_IMAGE_TYPE = "aerolab.image.type"
)

// V7 tag names for discovery
const (
	V7_TAG_USED_BY              = "UsedBy"
	V7_TAG_SERVER_MARKER        = "aerolab4"
	V7_TAG_CLIENT_MARKER        = "aerolab4client"
	V7_TAG_VOLUME_MARKER        = "aerolab7"
	V7_TAG_CLUSTER_NAME         = "Aerolab4ClusterName"
	V7_TAG_NODE_NUMBER          = "Aerolab4NodeNumber"
	V7_TAG_OPERATING_SYSTEM     = "Aerolab4OperatingSystem"
	V7_TAG_OPERATING_SYS_VER    = "Aerolab4OperatingSystemVersion"
	V7_TAG_AEROSPIKE_VERSION    = "Aerolab4AerospikeVersion"
	V7_TAG_ARCH                 = "Arch"
	V7_TAG_EXPIRES              = "aerolab4expires"
	V7_TAG_OWNER                = "owner"
	V7_TAG_COST_PPH             = "Aerolab4CostPerHour"
	V7_TAG_COST_SO_FAR          = "Aerolab4CostSoFar"
	V7_TAG_COST_START_TIME      = "Aerolab4CostStartTime"
	V7_TAG_TELEMETRY            = "telemetry"
	V7_TAG_SPOT                 = "aerolab7spot"
	V7_TAG_CLIENT_CLUSTER_NAME  = "Aerolab4clientClusterName"
	V7_TAG_CLIENT_NODE_NUMBER   = "Aerolab4clientNodeNumber"
	V7_TAG_CLIENT_OS            = "Aerolab4clientOperatingSystem"
	V7_TAG_CLIENT_OS_VER        = "Aerolab4clientOperatingSystemVersion"
	V7_TAG_CLIENT_AS_VER        = "Aerolab4clientAerospikeVersion"
	V7_TAG_CLIENT_TYPE          = "Aerolab4clientType"
	V7_TAG_AGI_AV               = "aerolab7agiav"
	V7_TAG_FEATURES             = "aerolab4features"
	V7_TAG_SSL                  = "aerolab4ssl"
	V7_TAG_AGI_LABEL            = "agiLabel"
	V7_TAG_AGI_INSTANCE         = "agiinstance"
	V7_TAG_AGI_NODIM            = "aginodim"
	V7_TAG_TERM_ON_POW          = "termonpow"
	V7_TAG_IS_SPOT              = "isspot"
	V7_TAG_AGI_SRC_LOCAL        = "agiSrcLocal"
	V7_TAG_AGI_SRC_SFTP         = "agiSrcSftp"
	V7_TAG_AGI_SRC_S3           = "agiSrcS3"
	V7_TAG_AGI_DOMAIN           = "agiDomain"
	V7_TAG_VOLUME_NAME          = "Name"
	V7_TAG_VOLUME_LAST_USED     = "lastUsed"
	V7_TAG_VOLUME_EXPIRE_DUR    = "expireDuration"
	V7_TAG_VOLUME_OWNER         = "aerolab7owner"
)
