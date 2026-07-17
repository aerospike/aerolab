package bvagrant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

// ---- GetImages: public templates ----

func TestGetImagesPublicTemplates(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	fr.boxListResult = []BoxInfo{{Name: "bento/ubuntu-22.04", Provider: "virtualbox", Version: "1.0"}}

	imgs, err := s.GetImages()
	if err != nil {
		t.Fatalf("GetImages error: %v", err)
	}

	wantCount := 0
	for _, entry := range boxCatalog {
		wantCount += len(entry.Archs)
	}
	if len(imgs) != wantCount {
		t.Fatalf("expected %d public template images, got %d", wantCount, len(imgs))
	}

	var sawAMD64, sawARM64 bool
	var sawRocky8 bool
	for _, img := range imgs {
		if img.BackendType != backends.BackendTypeVagrant {
			t.Fatalf("expected BackendType vagrant, got %v", img.BackendType)
		}
		if !img.Public {
			t.Fatalf("expected all boxCatalog-derived images to be Public=true, got %+v", img)
		}
		if img.Username != "root" {
			t.Fatalf("expected Username=root, got %q", img.Username)
		}
		if img.Name == "bento/ubuntu-22.04" && img.OSName == "ubuntu" && img.OSVersion == "22.04" {
			if !img.InAccount {
				t.Fatalf("expected InAccount=true for box present in BoxList: %+v", img)
			}
			switch img.Architecture {
			case backends.ArchitectureX8664:
				sawAMD64 = true
			case backends.ArchitectureARM64:
				sawARM64 = true
			}
		}
		if img.OSName == "rocky" && img.OSVersion == "8" {
			sawRocky8 = true
			if img.InAccount {
				t.Fatalf("expected InAccount=false for box not present in BoxList: %+v", img)
			}
		}
	}
	if !sawAMD64 || !sawARM64 {
		t.Fatalf("expected both amd64 and arm64 entries for bento/ubuntu-22.04, amd64=%v arm64=%v", sawAMD64, sawARM64)
	}
	if !sawRocky8 {
		t.Fatalf("expected a rocky 8 template entry to be present")
	}
}

// ---- GetImages: custom images from metadata ----

func TestGetImagesCustom(t *testing.T) {
	s, _ := newVagrantTestBackend(t)

	if err := os.MkdirAll(s.imagesDir(), 0755); err != nil {
		t.Fatalf("mkdir images dir: %v", err)
	}
	meta := &imageMeta{
		Name:        "my-custom-image",
		Box:         "aerolab-proj-my-custom-image",
		OSName:      "ubuntu",
		OSVersion:   "22.04",
		Arch:        "amd64",
		Description: "custom test image",
		Owner:       "jdoty",
		Tags:        map[string]string{"foo": "bar"},
		CreatedAt:   time.Now(),
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(s.imageMetaFile(meta.Name), raw, 0644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	imgs, err := s.GetImages()
	if err != nil {
		t.Fatalf("GetImages error: %v", err)
	}

	var found *backends.Image
	for _, img := range imgs {
		if img.Name == "my-custom-image" {
			found = img
		}
	}
	if found == nil {
		t.Fatalf("expected custom image %q in GetImages() output", meta.Name)
	}
	if found.Public {
		t.Fatalf("expected custom image Public=false")
	}
	if !found.InAccount {
		t.Fatalf("expected custom image InAccount=true")
	}
	if found.Username != "root" {
		t.Fatalf("expected custom image Username=root, got %q", found.Username)
	}
	if found.Owner != "jdoty" {
		t.Fatalf("expected owner jdoty, got %q", found.Owner)
	}
	if found.Tags["foo"] != "bar" {
		t.Fatalf("expected tag foo=bar, got %v", found.Tags)
	}
	if found.ImageId != meta.Box {
		t.Fatalf("expected ImageId %q, got %q", meta.Box, found.ImageId)
	}
}

// ---- CreateImage ----

func testRunningInstance(clusterDir, machineName string) *backends.Instance {
	return &backends.Instance{
		ClusterName:   "cl",
		InstanceID:    machineName,
		InstanceState: backends.LifeCycleStateRunning,
		Architecture:  backends.ArchitectureX8664,
		OperatingSystem: backends.OS{
			Name:    "ubuntu",
			Version: "22.04",
		},
		BackendSpecific: &InstanceDetail{
			ClusterDir:  clusterDir,
			MachineName: machineName,
			Provider:    "virtualbox",
		},
	}
}

func TestCreateImageHaltsRunningSourceAndRestartsIt(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	clusterDir := filepath.Join(t.TempDir(), "cl")
	inst := testRunningInstance(clusterDir, "proj-cl-1")

	out, err := s.CreateImage(&backends.CreateImageInput{
		BackendType: backends.BackendTypeVagrant,
		Instance:    inst,
		Name:        "my-image",
		Owner:       "jdoty",
	}, 0)
	if err != nil {
		t.Fatalf("CreateImage error: %v", err)
	}
	if out == nil || out.Image == nil {
		t.Fatalf("expected non-nil CreateImageOutput.Image")
	}
	if out.Image.Name != "my-image" {
		t.Fatalf("expected image name my-image, got %q", out.Image.Name)
	}
	if out.Image.OSName != "ubuntu" || out.Image.OSVersion != "22.04" {
		t.Fatalf("expected OS to default from instance, got %s/%s", out.Image.OSName, out.Image.OSVersion)
	}

	calls := fr.callsSnapshot()
	var haltIdx, packageIdx, boxAddIdx, upIdx = -1, -1, -1, -1
	for i, c := range calls {
		switch c.method {
		case "Halt":
			haltIdx = i
		case "Package":
			packageIdx = i
		case "BoxAdd":
			boxAddIdx = i
		case "Up":
			upIdx = i
		}
	}
	if haltIdx == -1 || packageIdx == -1 || boxAddIdx == -1 || upIdx == -1 {
		t.Fatalf("expected Halt, Package, BoxAdd, and Up calls, got: %+v", calls)
	}
	if !(haltIdx < packageIdx && packageIdx < boxAddIdx && boxAddIdx < upIdx) {
		t.Fatalf("expected call order Halt < Package < BoxAdd < Up, got: %+v", calls)
	}

	// The temp .box file must not be left behind.
	entries, err := os.ReadDir(s.imagesDir())
	if err != nil {
		t.Fatalf("read images dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("expected temp box file to be cleaned up, found %q", e.Name())
		}
	}

	// Metadata file must exist.
	if _, err := os.Stat(s.imageMetaFile("my-image")); err != nil {
		t.Fatalf("expected image metadata file to exist: %v", err)
	}
}

func TestCreateImageDoesNotHaltStoppedSource(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	clusterDir := filepath.Join(t.TempDir(), "cl")
	inst := testRunningInstance(clusterDir, "proj-cl-1")
	inst.InstanceState = backends.LifeCycleStateStopped

	_, err := s.CreateImage(&backends.CreateImageInput{
		BackendType: backends.BackendTypeVagrant,
		Instance:    inst,
		Name:        "my-image-2",
	}, 0)
	if err != nil {
		t.Fatalf("CreateImage error: %v", err)
	}

	for _, c := range fr.callsSnapshot() {
		if c.method == "Halt" || c.method == "Up" {
			t.Fatalf("did not expect Halt/Up calls for an already-stopped source, got: %s", c.method)
		}
	}
}

func TestCreateImagePackageErrorSurfacesActionableHint(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	fr.errs = map[string]error{"Package": errUnsupportedProvider}
	clusterDir := filepath.Join(t.TempDir(), "cl")
	inst := testRunningInstance(clusterDir, "proj-cl-1")
	inst.InstanceState = backends.LifeCycleStateStopped

	_, err := s.CreateImage(&backends.CreateImageInput{
		BackendType: backends.BackendTypeVagrant,
		Instance:    inst,
		Name:        "my-image-3",
	}, 0)
	if err == nil {
		t.Fatalf("expected error from Package failure")
	}
	if !strings.Contains(err.Error(), "virtualbox") {
		t.Fatalf("expected actionable hint mentioning virtualbox-family providers, got: %v", err)
	}
}

// errUnsupportedProvider mimics the error vagrant package returns for providers that
// don't support packaging (e.g. libvirt), used to exercise CreateImage's error wrapping.
var errUnsupportedProvider = &packageProviderError{}

type packageProviderError struct{}

func (e *packageProviderError) Error() string {
	return "the provider for this Vagrant environment does not support the `vagrant package` command"
}

// ---- ImagesDelete ----

func TestImagesDelete(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	if err := os.MkdirAll(s.imagesDir(), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	meta := &imageMeta{Name: "img1", Box: "aerolab-proj-img1"}
	if err := s.saveImageMeta(meta); err != nil {
		t.Fatalf("saveImageMeta: %v", err)
	}

	img := &backends.Image{Name: "img1", ImageId: "aerolab-proj-img1", Public: false}
	if err := s.ImagesDelete(backends.ImageList{img}, 0); err != nil {
		t.Fatalf("ImagesDelete error: %v", err)
	}

	var sawBoxRemove bool
	for _, c := range fr.callsSnapshot() {
		if c.method == "BoxRemove" && strings.Contains(c.args, "aerolab-proj-img1") {
			sawBoxRemove = true
		}
	}
	if !sawBoxRemove {
		t.Fatalf("expected BoxRemove call for aerolab-proj-img1")
	}
	if _, err := os.Stat(s.imageMetaFile("img1")); !os.IsNotExist(err) {
		t.Fatalf("expected metadata file to be removed, stat err: %v", err)
	}
}

// ---- ImagesAddTags / ImagesRemoveTags ----

func TestImagesAddAndRemoveTags(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	meta := &imageMeta{Name: "img2", Box: "aerolab-proj-img2", Tags: map[string]string{"a": "1"}}
	if err := s.saveImageMeta(meta); err != nil {
		t.Fatalf("saveImageMeta: %v", err)
	}

	img := &backends.Image{Name: "img2"}
	if err := s.ImagesAddTags(backends.ImageList{img}, map[string]string{"b": "2"}); err != nil {
		t.Fatalf("ImagesAddTags error: %v", err)
	}
	loaded, err := loadImageMeta(s.imageMetaFile("img2"))
	if err != nil {
		t.Fatalf("loadImageMeta: %v", err)
	}
	if loaded.Tags["a"] != "1" || loaded.Tags["b"] != "2" {
		t.Fatalf("expected merged tags a=1,b=2, got %v", loaded.Tags)
	}

	if err := s.ImagesRemoveTags(backends.ImageList{img}, []string{"a"}); err != nil {
		t.Fatalf("ImagesRemoveTags error: %v", err)
	}
	loaded, err = loadImageMeta(s.imageMetaFile("img2"))
	if err != nil {
		t.Fatalf("loadImageMeta: %v", err)
	}
	if _, ok := loaded.Tags["a"]; ok {
		t.Fatalf("expected tag a to be removed, got %v", loaded.Tags)
	}
	if loaded.Tags["b"] != "2" {
		t.Fatalf("expected tag b=2 to remain, got %v", loaded.Tags)
	}
}
