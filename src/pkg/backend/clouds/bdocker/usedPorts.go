package bdocker

import (
	"net"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type usedPorts struct {
	ports       []int
	stickyPorts []int
	mu          sync.Mutex
}

func (u *usedPorts) release(ports []int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stickyPorts = slices.DeleteFunc(u.stickyPorts, func(p int) bool {
		return slices.Contains(ports, p)
	})
}

// get given port as used
// return false if port is already used
func (u *usedPorts) get(port int) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u._get(port)
}

// get given port as used
// return false if port is already used
func (u *usedPorts) _get(port int) bool {
	if slices.Contains(u.ports, port) {
		return false
	}
	if slices.Contains(u.stickyPorts, port) {
		return false
	}
	if !isPortAvailable(port) {
		return false
	}
	u.stickyPorts = append(u.stickyPorts, port)
	u.ports = append(u.ports, port)
	sort.Ints(u.stickyPorts)
	sort.Ints(u.ports)
	return true
}

// get next free port starting from start
// return -1 if no free port is found
func (u *usedPorts) getNextFree(start int) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	for {
		if u._get(start) {
			return start
		}
		start++
		if start > 65535 {
			return -1
		}
	}
}

// reset used ports to a given list
func (u *usedPorts) reset(used backends.InstanceList) {
	usedPorts := []int{}
	for _, inst := range used.Describe() {
		for _, port := range inst.BackendSpecific.(*InstanceDetail).Docker.Ports {
			usedPorts = append(usedPorts, int(port.PublicPort))
		}
	}
	sort.Ints(usedPorts)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.ports = usedPorts
}

// check if we can bind to a port using net.Listen, so we know it's really available
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
