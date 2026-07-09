package bvagrant

import (
	"testing"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

func TestGetNetworksDefaultSubnet(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	s.credentials = nil // force fallback to defaultSubnet

	nets, err := s.GetNetworks()
	if err != nil {
		t.Fatalf("GetNetworks error: %v", err)
	}
	if len(nets) != 1 {
		t.Fatalf("expected exactly 1 synthetic network, got %d", len(nets))
	}
	n := nets[0]
	if n.BackendType != backends.BackendTypeVagrant {
		t.Fatalf("expected BackendType vagrant, got %v", n.BackendType)
	}
	if n.ZoneName != "local" || n.ZoneID != "local" {
		t.Fatalf("expected ZoneName/ZoneID local, got %s/%s", n.ZoneName, n.ZoneID)
	}
	if n.Cidr != defaultSubnet {
		t.Fatalf("expected Cidr %q, got %q", defaultSubnet, n.Cidr)
	}
}

func TestGetNetworksConfiguredSubnet(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	s.credentials = &clouds.VAGRANT{Subnet: "10.20.0.0/24"}

	nets, err := s.GetNetworks()
	if err != nil {
		t.Fatalf("GetNetworks error: %v", err)
	}
	if len(nets) != 1 {
		t.Fatalf("expected exactly 1 synthetic network, got %d", len(nets))
	}
	if nets[0].Cidr != "10.20.0.0/24" {
		t.Fatalf("expected Cidr 10.20.0.0/24, got %q", nets[0].Cidr)
	}
}
