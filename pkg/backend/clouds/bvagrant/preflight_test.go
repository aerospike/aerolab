package bvagrant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

// fakeLookPath builds a lookPath func from a set of names that should resolve
// successfully; anything else returns an error. It also counts total invocations
// per name so tests can assert on call counts.
type fakeLookPath struct {
	found map[string]string
	calls map[string]int
}

func newFakeLookPath(found ...string) *fakeLookPath {
	f := &fakeLookPath{found: map[string]string{}, calls: map[string]int{}}
	for _, name := range found {
		f.found[name] = "/usr/bin/" + name
	}
	return f
}

func (f *fakeLookPath) lookPath(name string) (string, error) {
	f.calls[name]++
	if path, ok := f.found[name]; ok {
		return path, nil
	}
	return "", os.ErrNotExist
}

func TestPreflightBinaryMissing(t *testing.T) {
	flp := newFakeLookPath() // nothing resolves
	s := &b{
		configDir: t.TempDir(),
		runner:    &fakeRunner{versionResult: "2.4.0"},
		lookPath:  flp.lookPath,
	}
	res, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if res.OK {
		t.Fatalf("expected OK=false when vagrant binary is missing")
	}
	found := false
	for _, issue := range res.Issues {
		if strings.Contains(issue, "vagrant") && strings.Contains(issue, "https://developer.hashicorp.com/vagrant/install") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an issue mentioning the vagrant install URL, got: %v", res.Issues)
	}
}

func TestPreflightBinaryMissingCustomPath(t *testing.T) {
	flp := newFakeLookPath() // nothing resolves
	s := &b{
		configDir:   t.TempDir(),
		credentials: &clouds.VAGRANT{BinaryPath: "/opt/vagrant/bin/vagrant"},
		runner:      &fakeRunner{versionResult: "2.4.0"},
		lookPath:    flp.lookPath,
	}
	res, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if res.OK {
		t.Fatalf("expected OK=false when configured vagrant binary is missing")
	}
	if flp.calls["/opt/vagrant/bin/vagrant"] == 0 {
		t.Fatalf("expected lookPath to be called with the configured BinaryPath")
	}
}

func TestPreflightVersionTooOld(t *testing.T) {
	flp := newFakeLookPath("vagrant")
	s := &b{
		configDir: t.TempDir(),
		runner:    &fakeRunner{versionResult: "2.3.4"},
		lookPath:  flp.lookPath,
	}
	res, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if res.OK {
		t.Fatalf("expected OK=false for vagrant version below minimum")
	}
	found := false
	for _, issue := range res.Issues {
		if strings.Contains(issue, "2.3.4") && strings.Contains(issue, minVagrantVersion) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an issue naming found version 2.3.4 and required %s, got: %v", minVagrantVersion, res.Issues)
	}
}

func TestPreflightCheckMirrorsPreflight(t *testing.T) {
	flp := newFakeLookPath("vagrant", "VBoxManage")
	s := &b{
		configDir: t.TempDir(),
		runner:    &fakeRunner{versionResult: "2.4.1", pluginListResult: []string{}},
		lookPath:  flp.lookPath,
	}
	info, err := s.PreflightCheck(true)
	if err != nil {
		t.Fatalf("PreflightCheck returned error: %v", err)
	}
	if !info.OK {
		t.Fatalf("expected OK=true, got issues: %v", info.Issues)
	}
	if info.VagrantVersion != "2.4.1" {
		t.Fatalf("expected VagrantVersion=2.4.1, got %q", info.VagrantVersion)
	}
	if len(info.Providers) != 1 || info.Providers[0] != "virtualbox" {
		t.Fatalf("expected providers=[virtualbox], got %v", info.Providers)
	}
}

func TestPreflightCheckPropagatesFailure(t *testing.T) {
	flp := newFakeLookPath() // nothing resolves
	s := &b{
		configDir: t.TempDir(),
		runner:    &fakeRunner{versionResult: "2.4.0"},
		lookPath:  flp.lookPath,
	}
	info, err := s.PreflightCheck(true)
	if err != nil {
		t.Fatalf("PreflightCheck returned error: %v", err)
	}
	if info.OK {
		t.Fatalf("expected OK=false when vagrant binary is missing")
	}
	if len(info.Issues) == 0 {
		t.Fatalf("expected at least one issue")
	}
}

func TestPreflightProviderDetection(t *testing.T) {
	cases := []struct {
		name      string
		lookNames []string
		plugins   []string
		want      []string
		notWant   []string
	}{
		{
			name:      "virtualbox only",
			lookNames: []string{"vagrant", "VBoxManage"},
			plugins:   nil,
			want:      []string{"virtualbox"},
			notWant:   []string{"libvirt", "vmware_desktop", "hyperv"},
		},
		{
			name:      "libvirt requires binary and plugin",
			lookNames: []string{"vagrant", "virsh"},
			plugins:   []string{"vagrant-libvirt"},
			want:      []string{"libvirt"},
			notWant:   []string{"virtualbox", "vmware_desktop", "hyperv"},
		},
		{
			name:      "virsh present but plugin missing does not count as libvirt",
			lookNames: []string{"vagrant", "virsh"},
			plugins:   nil,
			want:      nil,
			notWant:   []string{"libvirt"},
		},
		{
			name:      "vmware_desktop via plugin only",
			lookNames: []string{"vagrant"},
			plugins:   []string{"vagrant-vmware-desktop"},
			want:      []string{"vmware_desktop"},
			notWant:   []string{"virtualbox", "libvirt", "hyperv"},
		},
		{
			name:      "hyperv never detected on linux",
			lookNames: []string{"vagrant"},
			plugins:   nil,
			want:      nil,
			notWant:   []string{"hyperv"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			flp := newFakeLookPath(c.lookNames...)
			s := &b{
				configDir: t.TempDir(),
				runner:    &fakeRunner{versionResult: "2.4.0", pluginListResult: c.plugins},
				lookPath:  flp.lookPath,
			}
			res, err := s.Preflight(false)
			if err != nil {
				t.Fatalf("Preflight returned error: %v", err)
			}
			for _, w := range c.want {
				if !containsStr(res.Providers, w) {
					t.Fatalf("expected provider %q detected, got %v", w, res.Providers)
				}
			}
			for _, nw := range c.notWant {
				if containsStr(res.Providers, nw) {
					t.Fatalf("expected provider %q NOT detected, got %v", nw, res.Providers)
				}
			}
		})
	}
}

func TestPreflightDefaultProviderNotDetected(t *testing.T) {
	flp := newFakeLookPath("vagrant", "VBoxManage")
	s := &b{
		configDir:   t.TempDir(),
		credentials: &clouds.VAGRANT{DefaultProvider: "libvirt"},
		runner:      &fakeRunner{versionResult: "2.4.0"},
		lookPath:    flp.lookPath,
	}
	res, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if res.OK {
		t.Fatalf("expected OK=false when configured default provider is not detected")
	}
	found := false
	for _, issue := range res.Issues {
		if strings.Contains(issue, "libvirt") && strings.Contains(issue, "virtualbox") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected issue naming configured provider and detected providers, got: %v", res.Issues)
	}
}

func TestPreflightDefaultProviderDetectedIsFine(t *testing.T) {
	flp := newFakeLookPath("vagrant", "VBoxManage")
	s := &b{
		configDir:   t.TempDir(),
		credentials: &clouds.VAGRANT{DefaultProvider: "virtualbox"},
		runner:      &fakeRunner{versionResult: "2.4.0"},
		lookPath:    flp.lookPath,
	}
	res, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got issues: %v", res.Issues)
	}
}

func TestPreflightCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	flp := newFakeLookPath("vagrant", "VBoxManage")
	fr := &fakeRunner{versionResult: "2.4.0"}
	s := &b{configDir: dir, runner: fr, lookPath: flp.lookPath}

	res1, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("first Preflight returned error: %v", err)
	}
	if !res1.OK {
		t.Fatalf("expected first Preflight OK, got issues: %v", res1.Issues)
	}
	if _, err := os.Stat(filepath.Join(dir, "preflight.json")); err != nil {
		t.Fatalf("expected preflight.json to be written: %v", err)
	}

	callsAfterFirst := len(fr.callsSnapshot())

	res2, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("second Preflight (cached) returned error: %v", err)
	}
	if !res2.OK {
		t.Fatalf("expected cached Preflight OK")
	}
	if len(fr.callsSnapshot()) != callsAfterFirst {
		t.Fatalf("expected cached Preflight to make zero new runner calls, calls went from %d to %d", callsAfterFirst, len(fr.callsSnapshot()))
	}

	res3, err := s.Preflight(true)
	if err != nil {
		t.Fatalf("forced Preflight returned error: %v", err)
	}
	if !res3.OK {
		t.Fatalf("expected forced Preflight OK")
	}
	if len(fr.callsSnapshot()) <= callsAfterFirst {
		t.Fatalf("expected force=true to bypass cache and make new runner calls")
	}
}

func TestPreflightMissingCacheForceFalseRunsFresh(t *testing.T) {
	dir := t.TempDir()
	flp := newFakeLookPath("vagrant", "VBoxManage")
	fr := &fakeRunner{versionResult: "2.4.0"}
	s := &b{configDir: dir, runner: fr, lookPath: flp.lookPath}

	if _, err := os.Stat(filepath.Join(dir, "preflight.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no preflight.json to exist yet")
	}

	res, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK on fresh run, got issues: %v", res.Issues)
	}
	if len(fr.callsSnapshot()) == 0 {
		t.Fatalf("expected runner calls to have been made on a fresh (uncached) check")
	}
}

// A failed (non-OK) result must not be cached as a usable success: a subsequent
// force=false call must re-run the check rather than returning stale bad data
// (or worse, stale bad data reported as OK).
func TestPreflightFailedResultNotCachedAsSuccess(t *testing.T) {
	dir := t.TempDir()
	flp := newFakeLookPath() // vagrant missing -> not OK
	fr := &fakeRunner{versionResult: "2.4.0"}
	s := &b{configDir: dir, runner: fr, lookPath: flp.lookPath}

	res1, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if res1.OK {
		t.Fatalf("expected first Preflight to not be OK")
	}

	callsAfterFirst := len(fr.callsSnapshot())

	// Now fix the environment and check again without force: since the failed
	// result was never cached, this must re-run the check fresh and now succeed.
	flp.found["vagrant"] = "/usr/bin/vagrant"
	flp.found["VBoxManage"] = "/usr/bin/VBoxManage"

	res2, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("second Preflight returned error: %v", err)
	}
	if !res2.OK {
		t.Fatalf("expected second Preflight to succeed now that environment is fixed, got issues: %v", res2.Issues)
	}
	if len(fr.callsSnapshot()) <= callsAfterFirst {
		t.Fatalf("expected the failed result to not have been cached, forcing a fresh re-check")
	}
}

func TestPreflightNilSafeLookPath(t *testing.T) {
	// A bare &b{} (nil lookPath) must not panic; it should fall back to exec.LookPath.
	s := &b{configDir: t.TempDir(), runner: &fakeRunner{versionResult: "2.4.0"}}
	res, err := s.Preflight(false)
	if err != nil {
		t.Fatalf("Preflight returned error on bare &b{}: %v", err)
	}
	if res == nil {
		t.Fatalf("expected a non-nil result")
	}
}
