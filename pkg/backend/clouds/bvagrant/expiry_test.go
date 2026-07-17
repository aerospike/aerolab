package bvagrant

import (
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

func TestExpiryDaemonMethodsAreNoOps(t *testing.T) {
	s, _ := newVagrantTestBackend(t)

	if err := s.ExpiryInstall(60, 0, false, false, false, false, "local"); err != nil {
		t.Fatalf("ExpiryInstall error: %v", err)
	}
	if err := s.ExpiryChangeConfiguration(0, false, false, "local"); err != nil {
		t.Fatalf("ExpiryChangeConfiguration error: %v", err)
	}
	if err := s.ExpiryChangeFrequency(60, "local"); err != nil {
		t.Fatalf("ExpiryChangeFrequency error: %v", err)
	}
	list, err := s.ExpiryList()
	if err != nil {
		t.Fatalf("ExpiryList error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty ExpiryList, got %v", list)
	}
	if err := s.ExpiryRemove("local"); err != nil {
		t.Fatalf("ExpiryRemove error: %v", err)
	}
}

func TestExpiryV7CheckAlwaysFalse(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	found, regions, err := s.ExpiryV7Check()
	if err != nil {
		t.Fatalf("ExpiryV7Check error: %v", err)
	}
	if found {
		t.Fatalf("expected found=false, got true")
	}
	if len(regions) != 0 {
		t.Fatalf("expected no regions, got %v", regions)
	}
}

// TestVolumesChangeExpiryReflectedInGetVolumes exercises the end-to-end path: create a
// volume, change its expiry, and confirm GetVolumes() reflects it (and that a zero-time
// clears it again), mirroring InstancesChangeExpiry's semantics from Phase 3. It lives
// here (rather than volumes_test.go) since it's primarily exercising VolumesChangeExpiry.
func TestVolumesChangeExpiryReflectedInGetVolumes(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	if _, err := s.CreateVolume(&backends.CreateVolumeInput{
		BackendType: backends.BackendTypeVagrant,
		VolumeType:  backends.VolumeTypeSharedDisk,
		Name:        "expiryvol",
	}); err != nil {
		t.Fatalf("CreateVolume error: %v", err)
	}

	vols, err := s.GetVolumes()
	if err != nil {
		t.Fatalf("GetVolumes error: %v", err)
	}
	var target *backends.Volume
	for _, v := range vols {
		if v.Name == "expiryvol" {
			target = v
		}
	}
	if target == nil {
		t.Fatalf("expected to find volume expiryvol in GetVolumes() output")
	}
	if !target.Expires.IsZero() {
		t.Fatalf("expected zero Expires initially, got %v", target.Expires)
	}

	expiry := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	if err := s.VolumesChangeExpiry(backends.VolumeList{target}, expiry); err != nil {
		t.Fatalf("VolumesChangeExpiry error: %v", err)
	}

	vols, err = s.GetVolumes()
	if err != nil {
		t.Fatalf("GetVolumes error: %v", err)
	}
	target = nil
	for _, v := range vols {
		if v.Name == "expiryvol" {
			target = v
		}
	}
	if target == nil {
		t.Fatalf("expected to find volume expiryvol in GetVolumes() output")
	}
	if !target.Expires.Equal(expiry) {
		t.Fatalf("expected Expires=%v, got %v", expiry, target.Expires)
	}

	if err := s.VolumesChangeExpiry(backends.VolumeList{target}, time.Time{}); err != nil {
		t.Fatalf("VolumesChangeExpiry (clear) error: %v", err)
	}
	vols, err = s.GetVolumes()
	if err != nil {
		t.Fatalf("GetVolumes error: %v", err)
	}
	target = nil
	for _, v := range vols {
		if v.Name == "expiryvol" {
			target = v
		}
	}
	if target == nil {
		t.Fatalf("expected to find volume expiryvol in GetVolumes() output")
	}
	if !target.Expires.IsZero() {
		t.Fatalf("expected Expires cleared to zero, got %v", target.Expires)
	}
}
