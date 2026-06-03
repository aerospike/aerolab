package bgcp

import (
	"fmt"
	"strings"
)

const gcpHyperdiskBalancedDefault = "type=hyperdisk-balanced,size=20"

// gcpRequiresHyperdiskBootDisk reports machine types that cannot use persistent
// disk (pd-*) for the boot volume and require hyperdisk-balanced instead.
func gcpRequiresHyperdiskBootDisk(instanceType string) bool {
	return strings.HasPrefix(instanceType, "n4-") || strings.HasPrefix(instanceType, "c4a-")
}

func gcpDiskDefType(diskDef string) string {
	for _, part := range strings.Split(diskDef, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), "type") {
			return kv[1]
		}
	}
	return ""
}

// applyGCPInstanceDiskDefaults picks a compatible root disk for n4/c4a families and
// rewrites an incompatible pd-* root disk to hyperdisk-balanced.
func applyGCPInstanceDiskDefaults(disks []string, instanceType string) []string {
	if !gcpRequiresHyperdiskBootDisk(instanceType) {
		return disks
	}
	if len(disks) == 0 {
		return []string{gcpHyperdiskBalancedDefault}
	}
	if !strings.HasPrefix(gcpDiskDefType(disks[0]), "pd-") {
		return disks
	}
	size := "20"
	for _, part := range strings.Split(disks[0], ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), "size") {
			size = kv[1]
		}
	}
	out := make([]string, len(disks))
	copy(out, disks)
	out[0] = fmt.Sprintf("type=hyperdisk-balanced,size=%s", size)
	return out
}
