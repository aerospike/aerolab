package bvagrant

import (
	"fmt"
	"net/netip"
)

// allocateIPs returns count free IP addresses from the configured (or default) subnet,
// skipping the network address, the first host address (reserved as the gateway), and
// the broadcast address. Addresses already recorded against any node in any cluster's
// metadata are treated as used.
func (s *b) allocateIPs(count int) ([]string, error) {
	subnetStr := defaultSubnet
	if s.credentials != nil && s.credentials.Subnet != "" {
		subnetStr = s.credentials.Subnet
	}

	prefix, err := netip.ParsePrefix(subnetStr)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet %q: %w", subnetStr, err)
	}
	prefix = prefix.Masked()

	used, err := s.usedIPs()
	if err != nil {
		return nil, err
	}

	network := prefix.Addr()
	broadcast := lastAddr(prefix)
	gateway := network.Next()

	ips := make([]string, 0, count)
	for addr := network; addr.IsValid() && prefix.Contains(addr); addr = addr.Next() {
		if addr == network || addr == broadcast || addr == gateway {
			continue
		}
		if used[addr.String()] {
			continue
		}
		ips = append(ips, addr.String())
		if len(ips) == count {
			return ips, nil
		}
	}

	return nil, fmt.Errorf("subnet %s exhausted: could not allocate %d ip(s), only found %d free", subnetStr, count, len(ips))
}

// usedIPs scans all cluster metadata files and returns the set of IP addresses already
// assigned to a node.
func (s *b) usedIPs() (map[string]bool, error) {
	metas, err := s.listClusterMetas()
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, meta := range metas {
		for _, node := range meta.Nodes {
			if node == nil || node.IP == "" {
				continue
			}
			used[node.IP] = true
		}
	}
	return used, nil
}

// lastAddr computes the broadcast (last) address of a masked prefix.
func lastAddr(prefix netip.Prefix) netip.Addr {
	addr := prefix.Addr()
	if addr.Is4() {
		bytes := addr.As4()
		bits := prefix.Bits()
		hostBits := 32 - bits
		mask := uint32(0)
		if hostBits > 0 {
			mask = uint32(1)<<uint(hostBits) - 1
		}
		val := uint32(bytes[0])<<24 | uint32(bytes[1])<<16 | uint32(bytes[2])<<8 | uint32(bytes[3])
		val |= mask
		var out [4]byte
		out[0] = byte(val >> 24)
		out[1] = byte(val >> 16)
		out[2] = byte(val >> 8)
		out[3] = byte(val)
		return netip.AddrFrom4(out)
	}
	// IPv6 not supported for this backend's use case; return the address unchanged.
	return addr
}
