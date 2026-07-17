package bvagrant

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

// --- collapseErr ---------------------------------------------------------

func TestCollapseErr_BothNil(t *testing.T) {
	if err := collapseErr(nil, nil, "vagrant up"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCollapseErr_RunErrorOnly(t *testing.T) {
	runErr := errors.New("exec failed")
	err := collapseErr(runErr, nil, "vagrant up")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, runErr) {
		t.Fatalf("expected wrapped runErr, got %v", err)
	}
}

func TestCollapseErr_CmdErrorOnly(t *testing.T) {
	cmdErr := errors.New("vm not found")
	err := collapseErr(nil, cmdErr, "vagrant up")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, cmdErr) {
		t.Fatalf("expected wrapped cmdErr, got %v", err)
	}
}

func TestCollapseErr_BothSet_RunErrorWins(t *testing.T) {
	runErr := errors.New("exec failed")
	cmdErr := errors.New("vm not found")
	err := collapseErr(runErr, cmdErr, "vagrant up")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, runErr) {
		t.Fatalf("expected runErr to be part of the chain, got %v", err)
	}
	// cmdErr's message should still be visible for debugging even though
	// runErr is the one carried in the error chain.
	if got := err.Error(); !contains(got, cmdErr.Error()) {
		t.Fatalf("expected cmdErr message %q to appear in %q", cmdErr.Error(), got)
	}
}

func TestCollapseErr_ContextIncluded(t *testing.T) {
	err := collapseErr(errors.New("boom"), nil, "vagrant up (machine foo)")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !contains(err.Error(), "vagrant up (machine foo)") {
		t.Fatalf("expected context in error, got %q", err.Error())
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// --- fakeRunner sanity (so later phases can trust it) --------------------

func TestFakeRunner_RecordsCallsAndReturnsCanned(t *testing.T) {
	f := &fakeRunner{
		statusResult:     map[string]string{"default": "running"},
		sshConfigResult:  SSHInfo{HostName: "127.0.0.1", Port: 2222, User: "vagrant", IdentityFile: "/tmp/key"},
		boxListResult:    []BoxInfo{{Name: "aerolab/box", Provider: "virtualbox", Version: "1.0"}},
		versionResult:    "2.4.1",
		pluginListResult: []string{"vagrant-libvirt"},
	}

	if err := f.Up("/dir", []string{"a", "b"}, "virtualbox", true); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := f.Halt("/dir", nil); err != nil {
		t.Fatalf("Halt: %v", err)
	}
	if err := f.Destroy("/dir", []string{"a"}); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	status, err := f.Status("/dir")
	if err != nil || !reflect.DeepEqual(status, f.statusResult) {
		t.Fatalf("Status: got %v, %v", status, err)
	}
	ssh, err := f.SSHConfig("/dir", "a")
	if err != nil || ssh != f.sshConfigResult {
		t.Fatalf("SSHConfig: got %v, %v", ssh, err)
	}
	boxes, err := f.BoxList()
	if err != nil || !reflect.DeepEqual(boxes, f.boxListResult) {
		t.Fatalf("BoxList: got %v, %v", boxes, err)
	}
	if err := f.BoxAdd("name", "location"); err != nil {
		t.Fatalf("BoxAdd: %v", err)
	}
	if err := f.BoxRemove("name"); err != nil {
		t.Fatalf("BoxRemove: %v", err)
	}
	if err := f.Package("/dir", "a", "/out.box"); err != nil {
		t.Fatalf("Package: %v", err)
	}
	ver, err := f.Version()
	if err != nil || ver != f.versionResult {
		t.Fatalf("Version: got %v, %v", ver, err)
	}
	plugins, err := f.PluginList()
	if err != nil || !reflect.DeepEqual(plugins, f.pluginListResult) {
		t.Fatalf("PluginList: got %v, %v", plugins, err)
	}

	calls := f.callsSnapshot()
	wantMethods := []string{"Up", "Halt", "Destroy", "Status", "SSHConfig", "BoxList", "BoxAdd", "BoxRemove", "Package", "Version", "PluginList"}
	if len(calls) != len(wantMethods) {
		t.Fatalf("expected %d calls, got %d: %+v", len(wantMethods), len(calls), calls)
	}
	for i, m := range wantMethods {
		if calls[i].method != m {
			t.Fatalf("call %d: expected method %s, got %s", i, m, calls[i].method)
		}
	}
}

func TestFakeRunner_ErrsMap(t *testing.T) {
	wantErr := errors.New("boom")
	f := &fakeRunner{errs: map[string]error{"Up": wantErr}}
	if err := f.Up("/dir", nil, "", false); !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestFakeRunner_OnUpHook(t *testing.T) {
	called := false
	f := &fakeRunner{onUp: func(dir string, machines []string, provider string) error {
		called = true
		if dir != "/dir" || provider != "virtualbox" {
			t.Fatalf("unexpected onUp args: dir=%s provider=%s", dir, provider)
		}
		return nil
	}}
	if err := f.Up("/dir", []string{"a"}, "virtualbox", false); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !called {
		t.Fatal("expected onUp to be invoked")
	}
}

// --- realRunner pure helpers (no vagrant binary required) -----------------

func TestBoxRemoveArgs(t *testing.T) {
	got := boxRemoveArgs("aerolab/box")
	want := []string{"box", "remove", "--force", "aerolab/box"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPackageArgs(t *testing.T) {
	got := packageArgs("mymachine", "/tmp/out.box")
	want := []string{"package", "mymachine", "--output", "/tmp/out.box"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPackageArgs_NoMachine(t *testing.T) {
	got := packageArgs("", "/tmp/out.box")
	want := []string{"package", "--output", "/tmp/out.box"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPluginListArgs(t *testing.T) {
	got := pluginListArgs()
	want := []string{"plugin", "list"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBoxListArgs(t *testing.T) {
	got := boxListArgs()
	want := []string{"box", "list", "--machine-readable"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBoxAddArgs(t *testing.T) {
	got := boxAddArgs("aerolab/box", "/tmp/box.box")
	want := []string{"box", "add", "--name", "aerolab/box", "--force", "/tmp/box.box"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestVersionArgs(t *testing.T) {
	got := versionArgs()
	want := []string{"version", "--machine-readable"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseBoxList_MultipleBoxes(t *testing.T) {
	output := "" +
		"1657000000,,box-name,aerolab/ubuntu-focal\n" +
		"1657000000,,box-provider,virtualbox\n" +
		"1657000000,,box-version,20220615.0.0\n" +
		"1657000001,,box-name,aerolab/ubuntu-jammy\n" +
		"1657000001,,box-provider,libvirt\n" +
		"1657000001,,box-version,20230101.0.0\n"
	got, err := parseBoxList(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []BoxInfo{
		{Name: "aerolab/ubuntu-focal", Provider: "virtualbox", Version: "20220615.0.0"},
		{Name: "aerolab/ubuntu-jammy", Provider: "libvirt", Version: "20230101.0.0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseBoxList_Empty(t *testing.T) {
	got, err := parseBoxList("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no boxes, got %+v", got)
	}
}

func TestParseBoxList_NoInstalledBoxesMessage(t *testing.T) {
	// vagrant emits ui/info lines (fewer than 4 comma-separated fields won't
	// happen in practice, but a ui line with no box-name key should not
	// produce any boxes).
	output := "1657000000,,ui,There are no installed boxes! Use `vagrant box add` to add some.\n"
	got, err := parseBoxList(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no boxes, got %+v", got)
	}
}

func TestParseBoxList_ErrorExit(t *testing.T) {
	output := "1657000000,,error-exit,something went wrong\n"
	_, err := parseBoxList(output)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "something went wrong") {
		t.Fatalf("expected error message to contain vagrant error text, got %v", err)
	}
}

func TestParseBoxList_VersionOrProviderBeforeName(t *testing.T) {
	output := "1657000000,,box-version,1.0.0\n"
	_, err := parseBoxList(output)
	if err == nil {
		t.Fatal("expected error for box-version with no preceding box-name")
	}
}

func TestParseVersionInstalled(t *testing.T) {
	output := "1657000000,,version-installed,2.4.1\n1657000000,,version-latest,2.4.3\n"
	got, err := parseVersionInstalled(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2.4.1" {
		t.Fatalf("got %q, want %q", got, "2.4.1")
	}
}

func TestParseVersionInstalled_ErrorExit(t *testing.T) {
	output := "1657000000,,error-exit,boom\n"
	_, err := parseVersionInstalled(output)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "boom") {
		t.Fatalf("expected error to contain 'boom', got %v", err)
	}
}

func TestParseVersionInstalled_Missing(t *testing.T) {
	_, err := parseVersionInstalled("1657000000,,ui,hello\n")
	if err == nil {
		t.Fatal("expected error when version-installed is absent")
	}
}

func TestParsePluginNames(t *testing.T) {
	output := `vagrant-libvirt (0.12.2, global)
vagrant-vbguest (0.31.0, global)

`
	got := parsePluginNames(output)
	want := []string{"vagrant-libvirt", "vagrant-vbguest"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParsePluginNames_Empty(t *testing.T) {
	got := parsePluginNames("")
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestParsePluginNames_IgnoresMalformedLines(t *testing.T) {
	output := "Installed Plugins:\n\nvagrant-libvirt (0.12.2, global)\nsomething weird with no parens\n"
	got := parsePluginNames(output)
	want := []string{"vagrant-libvirt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveBinaryPath_Empty(t *testing.T) {
	if got := resolveBinaryPath(""); got != "vagrant" {
		t.Fatalf("expected 'vagrant', got %q", got)
	}
}

func TestResolveBinaryPath_Explicit(t *testing.T) {
	if got := resolveBinaryPath("/opt/vagrant/bin/vagrant"); got != "/opt/vagrant/bin/vagrant" {
		t.Fatalf("expected explicit path, got %q", got)
	}
}

func TestBinaryDirForPATH_EmptyBinaryPath(t *testing.T) {
	if got := binaryDirForPATH(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestBinaryDirForPATH_BareCommand(t *testing.T) {
	if got := binaryDirForPATH("vagrant"); got != "" {
		t.Fatalf("expected empty (no dir to prepend), got %q", got)
	}
}

func TestBinaryDirForPATH_AbsolutePath(t *testing.T) {
	if got := binaryDirForPATH("/opt/vagrant/bin/vagrant"); got != "/opt/vagrant/bin" {
		t.Fatalf("expected /opt/vagrant/bin, got %q", got)
	}
}

// TestNoVagrantBinaryRequired guards that none of the pure-helper tests in
// this file need a real vagrant binary on PATH.
func TestNoVagrantBinaryRequired(t *testing.T) {
	old := os.Getenv("PATH")
	defer os.Setenv("PATH", old)
	os.Setenv("PATH", "/nonexistent-bin-dir-for-test")

	if err := collapseErr(nil, nil, "ctx"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resolveBinaryPath(""); got != "vagrant" {
		t.Fatalf("unexpected: %v", got)
	}
}
