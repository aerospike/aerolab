package bvagrant

import (
	"strings"
	"testing"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

func TestIPAMAllocatesSequentialFreeIPs(t *testing.T) {
	s := newTestBackend(t)
	s.credentials = &clouds.VAGRANT{Subnet: "192.168.56.0/24"}

	ips, err := s.allocateIPs(3)
	if err != nil {
		t.Fatalf("allocateIPs: %v", err)
	}
	want := []string{"192.168.56.2", "192.168.56.3", "192.168.56.4"}
	if len(ips) != len(want) {
		t.Fatalf("expected %d ips, got %v", len(want), ips)
	}
	for i := range want {
		if ips[i] != want[i] {
			t.Fatalf("ip[%d] mismatch: got %s want %s", i, ips[i], want[i])
		}
	}
}

func TestIPAMDefaultSubnetWhenNoCredentials(t *testing.T) {
	s := newTestBackend(t)
	// s.credentials is nil

	ips, err := s.allocateIPs(1)
	if err != nil {
		t.Fatalf("allocateIPs: %v", err)
	}
	if ips[0] != "192.168.56.2" {
		t.Fatalf("expected default subnet allocation 192.168.56.2, got %s", ips[0])
	}
}

func TestIPAMSkipsUsedIPs(t *testing.T) {
	s := newTestBackend(t)
	s.credentials = &clouds.VAGRANT{Subnet: "192.168.56.0/24"}

	meta := &clusterMeta{
		ClusterName: "used",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "used-1", IP: "192.168.56.2"},
			2: {MachineName: "used-2", IP: "192.168.56.3"},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}

	ips, err := s.allocateIPs(1)
	if err != nil {
		t.Fatalf("allocateIPs: %v", err)
	}
	if ips[0] != "192.168.56.4" {
		t.Fatalf("expected next free ip 192.168.56.4, got %s", ips[0])
	}
}

func TestIPAMScansAllClusterMetadataFiles(t *testing.T) {
	s := newTestBackend(t)
	s.credentials = &clouds.VAGRANT{Subnet: "192.168.56.0/24"}

	if err := s.saveClusterMeta(&clusterMeta{
		ClusterName: "clusterA",
		Nodes:       map[int]*nodeMeta{1: {IP: "192.168.56.2"}},
	}); err != nil {
		t.Fatalf("saveClusterMeta A: %v", err)
	}
	if err := s.saveClusterMeta(&clusterMeta{
		ClusterName: "clusterB",
		Nodes:       map[int]*nodeMeta{1: {IP: "192.168.56.3"}, 2: {IP: "192.168.56.4"}},
	}); err != nil {
		t.Fatalf("saveClusterMeta B: %v", err)
	}

	ips, err := s.allocateIPs(1)
	if err != nil {
		t.Fatalf("allocateIPs: %v", err)
	}
	if ips[0] != "192.168.56.5" {
		t.Fatalf("expected 192.168.56.5 (after scanning both clusters), got %s", ips[0])
	}
}

func TestIPAMExhaustion(t *testing.T) {
	s := newTestBackend(t)
	// /30 gives 4 addresses: network, host1(gw), host2, broadcast -> effectively 1 usable host beyond gateway
	s.credentials = &clouds.VAGRANT{Subnet: "192.168.99.0/30"}

	_, err := s.allocateIPs(1)
	if err != nil {
		t.Fatalf("allocateIPs(1): %v", err)
	}

	_, err = s.allocateIPs(2)
	if err == nil {
		t.Fatalf("expected exhaustion error requesting 2 ips from /30 subnet")
	}
	if !strings.Contains(err.Error(), "192.168.99.0/30") {
		t.Fatalf("expected error to mention subnet, got: %v", err)
	}
}

func TestIPAMCustomSubnet(t *testing.T) {
	s := newTestBackend(t)
	s.credentials = &clouds.VAGRANT{Subnet: "10.99.0.0/28"}

	ips, err := s.allocateIPs(2)
	if err != nil {
		t.Fatalf("allocateIPs: %v", err)
	}
	want := []string{"10.99.0.2", "10.99.0.3"}
	for i := range want {
		if ips[i] != want[i] {
			t.Fatalf("ip[%d] mismatch: got %s want %s", i, ips[i], want[i])
		}
	}
}

func TestIPAMNonSlash24Mask(t *testing.T) {
	s := newTestBackend(t)
	s.credentials = &clouds.VAGRANT{Subnet: "172.16.0.0/22"}

	ips, err := s.allocateIPs(5)
	if err != nil {
		t.Fatalf("allocateIPs: %v", err)
	}
	if len(ips) != 5 {
		t.Fatalf("expected 5 ips, got %d", len(ips))
	}
	if ips[0] != "172.16.0.2" {
		t.Fatalf("expected first ip 172.16.0.2, got %s", ips[0])
	}
	// last usable in /22 is 172.16.3.254 (broadcast is 172.16.3.255); just check we're not returning it early
	for _, ip := range ips {
		if strings.HasSuffix(ip, ".255") || strings.HasSuffix(ip, ".0") {
			t.Fatalf("allocated network/broadcast address: %s", ip)
		}
	}
}
