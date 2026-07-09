package bvagrant

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

func TestCreateVolume(t *testing.T) {
	s, _ := newVagrantTestBackend(t)

	out, err := s.CreateVolume(&backends.CreateVolumeInput{
		BackendType: backends.BackendTypeVagrant,
		VolumeType:  backends.VolumeTypeSharedDisk,
		Name:        "data",
		Owner:       "jdoty",
		Tags:        map[string]string{"env": "test"},
	})
	if err != nil {
		t.Fatalf("CreateVolume error: %v", err)
	}
	if out.Volume.Name != "data" {
		t.Fatalf("expected volume name data, got %q", out.Volume.Name)
	}
	if out.Volume.VolumeType != backends.VolumeTypeSharedDisk {
		t.Fatalf("expected VolumeTypeSharedDisk, got %v", out.Volume.VolumeType)
	}
	if _, err := os.Stat(s.volumeDir("data")); err != nil {
		t.Fatalf("expected volume directory to exist: %v", err)
	}
	if _, err := os.Stat(s.volumeMetaFile("data")); err != nil {
		t.Fatalf("expected volume.json to exist: %v", err)
	}
}

func TestCreateVolumeDuplicateNameErrors(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	input := &backends.CreateVolumeInput{
		BackendType: backends.BackendTypeVagrant,
		VolumeType:  backends.VolumeTypeSharedDisk,
		Name:        "dup",
	}
	if _, err := s.CreateVolume(input); err != nil {
		t.Fatalf("first CreateVolume error: %v", err)
	}
	if _, err := s.CreateVolume(input); err == nil {
		t.Fatalf("expected error creating duplicate volume name")
	}
}

func TestGetVolumes(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	if _, err := s.CreateVolume(&backends.CreateVolumeInput{
		BackendType: backends.BackendTypeVagrant,
		VolumeType:  backends.VolumeTypeSharedDisk,
		Name:        "vol1",
		Owner:       "jdoty",
		Tags:        map[string]string{"a": "b"},
	}); err != nil {
		t.Fatalf("CreateVolume error: %v", err)
	}

	vols, err := s.GetVolumes()
	if err != nil {
		t.Fatalf("GetVolumes error: %v", err)
	}
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	v := vols[0]
	if v.Name != "vol1" {
		t.Fatalf("expected name vol1, got %q", v.Name)
	}
	if v.VolumeType != backends.VolumeTypeSharedDisk {
		t.Fatalf("expected VolumeTypeSharedDisk, got %v", v.VolumeType)
	}
	if v.Size != 0 {
		t.Fatalf("expected zero cost/size, got %v", v.Size)
	}
	if v.State != backends.VolumeStateAvailable {
		t.Fatalf("expected VolumeStateAvailable, got %v", v.State)
	}
	if v.ZoneName != "local" || v.ZoneID != "local" {
		t.Fatalf("expected ZoneName/ZoneID local, got %s/%s", v.ZoneName, v.ZoneID)
	}
	if v.BackendType != backends.BackendTypeVagrant {
		t.Fatalf("expected BackendType vagrant, got %v", v.BackendType)
	}
	if v.Owner != "jdoty" {
		t.Fatalf("expected owner jdoty, got %q", v.Owner)
	}
	if v.Tags["a"] != "b" {
		t.Fatalf("expected tag a=b, got %v", v.Tags)
	}
}

func TestDeleteVolumes(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	if _, err := s.CreateVolume(&backends.CreateVolumeInput{
		BackendType: backends.BackendTypeVagrant,
		VolumeType:  backends.VolumeTypeSharedDisk,
		Name:        "todelete",
	}); err != nil {
		t.Fatalf("CreateVolume error: %v", err)
	}

	vol := &backends.Volume{Name: "todelete"}
	if err := s.DeleteVolumes(backends.VolumeList{vol}, nil, 0); err != nil {
		t.Fatalf("DeleteVolumes error: %v", err)
	}
	if _, err := os.Stat(s.volumeDir("todelete")); !os.IsNotExist(err) {
		t.Fatalf("expected volume directory to be removed, stat err: %v", err)
	}
}

func TestVolumesAddAndRemoveTags(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	if _, err := s.CreateVolume(&backends.CreateVolumeInput{
		BackendType: backends.BackendTypeVagrant,
		VolumeType:  backends.VolumeTypeSharedDisk,
		Name:        "tagvol",
		Tags:        map[string]string{"a": "1"},
	}); err != nil {
		t.Fatalf("CreateVolume error: %v", err)
	}
	vol := &backends.Volume{Name: "tagvol"}

	if err := s.VolumesAddTags(backends.VolumeList{vol}, map[string]string{"b": "2"}, 0); err != nil {
		t.Fatalf("VolumesAddTags error: %v", err)
	}
	vols, err := s.GetVolumes()
	if err != nil {
		t.Fatalf("GetVolumes error: %v", err)
	}
	if vols[0].Tags["a"] != "1" || vols[0].Tags["b"] != "2" {
		t.Fatalf("expected merged tags a=1,b=2, got %v", vols[0].Tags)
	}

	if err := s.VolumesRemoveTags(backends.VolumeList{vol}, []string{"a"}, 0); err != nil {
		t.Fatalf("VolumesRemoveTags error: %v", err)
	}
	vols, err = s.GetVolumes()
	if err != nil {
		t.Fatalf("GetVolumes error: %v", err)
	}
	if _, ok := vols[0].Tags["a"]; ok {
		t.Fatalf("expected tag a removed, got %v", vols[0].Tags)
	}
	if vols[0].Tags["b"] != "2" {
		t.Fatalf("expected tag b=2 to remain, got %v", vols[0].Tags)
	}
}

// TestVolumeDiskMountsCreatedVolumeDir is the volume-attach-at-create integration test:
// it confirms that vagrantfile.go's parseSyncedFolders resolves a Disks entry naming a
// volume to the exact same host directory CreateVolume creates for it.
func TestVolumeDiskMountsCreatedVolumeDir(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	if _, err := s.CreateVolume(&backends.CreateVolumeInput{
		BackendType: backends.BackendTypeVagrant,
		VolumeType:  backends.VolumeTypeSharedDisk,
		Name:        "mounted",
	}); err != nil {
		t.Fatalf("CreateVolume error: %v", err)
	}

	folders, err := s.parseSyncedFolders([]string{"mounted:/mnt/mounted"})
	if err != nil {
		t.Fatalf("parseSyncedFolders error: %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("expected 1 synced folder, got %d", len(folders))
	}
	wantDir := s.volumeDir("mounted")
	if folders[0].Host != wantDir {
		t.Fatalf("expected synced folder host %q to equal created volume dir %q", folders[0].Host, wantDir)
	}
	if _, err := os.Stat(wantDir); err != nil {
		t.Fatalf("expected the created volume directory to actually exist on disk: %v", err)
	}
}

func TestVolumeDirPath(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	got := s.volumeDir("myvol")
	want := filepath.Join(s.configDir, "volumes", "myvol")
	if got != want {
		t.Fatalf("expected volumeDir %q, got %q", want, got)
	}
}
