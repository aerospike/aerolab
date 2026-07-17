package bvagrant

import (
	"testing"
)

func TestGetVolumePrices(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	prices, err := s.GetVolumePrices()
	if err != nil {
		t.Fatalf("GetVolumePrices error: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected exactly 1 volume price entry, got %d", len(prices))
	}
	p := prices[0]
	if p.Type != "vagrant-volume" {
		t.Fatalf("expected type vagrant-volume, got %q", p.Type)
	}
	if p.PricePerGBHour != 0 {
		t.Fatalf("expected zero cost, got %v", p.PricePerGBHour)
	}
}

func TestGetInstanceTypes(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	types, err := s.GetInstanceTypes()
	if err != nil {
		t.Fatalf("GetInstanceTypes error: %v", err)
	}
	if len(types) != 1 {
		t.Fatalf("expected exactly 1 instance type entry, got %d", len(types))
	}
	it := types[0]
	if it.Name != "vagrant-instance" {
		t.Fatalf("expected name vagrant-instance, got %q", it.Name)
	}
	if it.PricePerHour.OnDemand != 0 || it.PricePerHour.Spot != 0 {
		t.Fatalf("expected zero cost, got %+v", it.PricePerHour)
	}
}

func TestCreateInstancesGetPrice(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	pph, gb, err := s.CreateInstancesGetPrice(nil)
	if err != nil {
		t.Fatalf("CreateInstancesGetPrice error: %v", err)
	}
	if pph != 0 || gb != 0 {
		t.Fatalf("expected (0,0), got (%v,%v)", pph, gb)
	}
}

func TestCreateVolumeGetPrice(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	gb, err := s.CreateVolumeGetPrice(nil)
	if err != nil {
		t.Fatalf("CreateVolumeGetPrice error: %v", err)
	}
	if gb != 0 {
		t.Fatalf("expected 0, got %v", gb)
	}
}
