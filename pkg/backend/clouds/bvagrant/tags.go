package bvagrant

// Tag key strings mirror bdocker's exactly (same TAG_* values), so tooling
// that inspects tags is backend-agnostic.
const (
	TAG_NAME            = "aerolab.name"
	TAG_DESCRIPTION     = "aerolab.description"
	TAG_AEROLAB_VERSION = "aerolab.version"
	TAG_AEROLAB_PROJECT = "aerolab.project"
	TAG_OWNER           = "aerolab.owner"
	TAG_EXPIRES         = "aerolab.expires"
	TAG_CLUSTER_NAME    = "aerolab.cluster.name"
	TAG_NODE_NO         = "aerolab.node.no"
	TAG_OS_NAME         = "aerolab.os.name"
	TAG_OS_VERSION      = "aerolab.os.version"
	TAG_ARCHITECTURE    = "aerolab.architecture"
	TAG_DNS_NAME        = "aerolab.dns.name"
	TAG_CLUSTER_UUID    = "aerolab.cluster.uuid"
	TAG_PUBLIC_TEMPLATE = "aerolab.public.template"
	TAG_PUBLIC_NAME     = "aerolab.public.name"
)
