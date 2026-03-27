//go:build !nogcp

package cmd

import "github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"

func firewallGcpPortDetails(backendSpecific any) (targetTags []string, destRanges []string) {
	if v, ok := backendSpecific.(*bgcp.PortDetail); ok {
		return v.TargetTags, v.DestinationRanges
	}
	return nil, nil
}
