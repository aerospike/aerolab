package bvagrant

import (
	"embed"
)

//go:embed scripts/*
var scripts embed.FS

// Tag constants for Vagrant VM labels/metadata
const (
	TAG_AEROLAB_VERSION = "aerolab.version"
	TAG_AEROLAB_PROJECT = "aerolab.project"
	TAG_CLUSTER_NAME    = "aerolab.cluster.name"
	TAG_CLUSTER_UUID    = "aerolab.cluster.uuid"
	TAG_NODE_NO         = "aerolab.node.no"
	TAG_NAME            = "aerolab.name"
	TAG_OWNER           = "aerolab.owner"
	TAG_DESCRIPTION     = "aerolab.description"
	TAG_EXPIRES         = "aerolab.expires"
	TAG_OS_NAME         = "aerolab.os.name"
	TAG_OS_VERSION      = "aerolab.os.version"
	TAG_ARCHITECTURE    = "aerolab.architecture"
	TAG_PUBLIC_NAME     = "aerolab.public.name"
	TAG_PUBLIC_TEMPLATE = "aerolab.public.template"
)
