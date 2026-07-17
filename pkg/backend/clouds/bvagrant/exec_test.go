package bvagrant

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
)

func testInstance(clusterDir, machineName string, state backends.LifeCycleState) *backends.Instance {
	return &backends.Instance{
		InstanceID:    machineName,
		ClusterName:   "c",
		NodeNo:        1,
		Owner:         "owner1",
		InstanceState: state,
		IP:            backends.IP{Private: "10.0.0.5"},
		BackendSpecific: &InstanceDetail{
			ClusterDir:  clusterDir,
			MachineName: machineName,
		},
	}
}

// ---- sshConfigFor ----

func TestSSHConfigFor(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	fr.sshConfigResult = SSHInfo{
		HostName:     "127.0.0.1",
		Port:         2222,
		User:         "vagrant",
		IdentityFile: "/tmp/whatever",
	}

	pubKey, err := s.ensureProjectKeypair()
	if err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}
	if pubKey == "" {
		t.Fatal("expected a non-empty public key")
	}
	wantPrivKey, err := os.ReadFile(filepath.Join(s.sshKeysDir, s.project))
	if err != nil {
		t.Fatalf("reading generated private key: %v", err)
	}

	inst := testInstance("/clusters/c", "aerolab-proj-c-1", backends.LifeCycleStateRunning)

	conf, err := s.sshConfigFor(inst, "root")
	if err != nil {
		t.Fatalf("sshConfigFor: %v", err)
	}
	if conf.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", conf.Host)
	}
	if conf.Port != 2222 {
		t.Errorf("Port = %d, want 2222", conf.Port)
	}
	if conf.Username != "root" {
		t.Errorf("Username = %q, want root (the caller's requested username, not vagrant's)", conf.Username)
	}
	if string(conf.PrivateKey) != string(wantPrivKey) {
		t.Errorf("PrivateKey does not match the project keypair contents")
	}

	// Make sure vagrant's own IdentityFile is not what got used: the project key
	// bytes should differ from whatever's on disk at the fake IdentityFile path
	// (which doesn't even exist), and specifically PrivateKey must equal the
	// project's key, not be empty/derived from IdentityFile.
	if _, err := os.Stat("/tmp/whatever"); err == nil {
		t.Fatal("test fixture assumption violated: /tmp/whatever unexpectedly exists")
	}

	calls := fr.callsSnapshot()
	if len(calls) != 1 || calls[0].method != "SSHConfig" {
		t.Fatalf("expected exactly one SSHConfig call, got %+v", calls)
	}
	wantArgs := "dir=/clusters/c machine=aerolab-proj-c-1"
	if calls[0].args != wantArgs {
		t.Errorf("SSHConfig args = %q, want %q", calls[0].args, wantArgs)
	}
}

// ---- InstancesExec: not-running instances ----

func TestInstancesExec_NotRunning(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	fr.sshConfigResult = SSHInfo{HostName: "127.0.0.1", Port: 2222, User: "vagrant"}
	if _, err := s.ensureProjectKeypair(); err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}

	inst := testInstance("/clusters/c", "aerolab-proj-c-1", backends.LifeCycleStateStopped)
	out := s.InstancesExec(backends.InstanceList{inst}, &backends.ExecInput{
		Username: "root",
	})
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if out[0].Output == nil || out[0].Output.Err == nil {
		t.Fatal("expected an error output for a not-running instance")
	}
	if out[0].Output.Err.Error() != "instance not running" {
		t.Errorf("err = %q, want %q", out[0].Output.Err.Error(), "instance not running")
	}
	if out[0].Instance != inst {
		t.Error("expected the Instance field to be set to the original instance")
	}

	// SSHConfig should never be called for an instance we already know isn't running.
	for _, c := range fr.callsSnapshot() {
		if c.method == "SSHConfig" {
			t.Fatal("SSHConfig must not be called for a not-running instance")
		}
	}
}

func TestInstancesGetSftpConfig_NotRunning(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	if _, err := s.ensureProjectKeypair(); err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}
	inst := testInstance("/clusters/c", "aerolab-proj-c-1", backends.LifeCycleStateStopped)
	_, err := s.InstancesGetSftpConfig(backends.InstanceList{inst}, "root")
	if err == nil {
		t.Fatal("expected an error for a not-running instance")
	}
}

func TestInstancesGetSftpConfig_Running(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	fr.sshConfigResult = SSHInfo{HostName: "127.0.0.1", Port: 2222, User: "vagrant"}
	if _, err := s.ensureProjectKeypair(); err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}
	inst := testInstance("/clusters/c", "aerolab-proj-c-1", backends.LifeCycleStateRunning)
	confs, err := s.InstancesGetSftpConfig(backends.InstanceList{inst}, "root")
	if err != nil {
		t.Fatalf("InstancesGetSftpConfig: %v", err)
	}
	if len(confs) != 1 {
		t.Fatalf("len(confs) = %d, want 1", len(confs))
	}
	if confs[0].Host != "127.0.0.1" || confs[0].Port != 2222 || confs[0].Username != "root" {
		t.Errorf("unexpected conf: %+v", confs[0])
	}
}

// ---- InstancesGetSSHKeyPath ----

func TestInstancesGetSSHKeyPath(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	insts := backends.InstanceList{
		testInstance("/clusters/c", "aerolab-proj-c-1", backends.LifeCycleStateRunning),
		testInstance("/clusters/c", "aerolab-proj-c-2", backends.LifeCycleStateRunning),
	}
	paths := s.InstancesGetSSHKeyPath(insts)
	if len(paths) != 2 {
		t.Fatalf("len(paths) = %d, want 2", len(paths))
	}
	want := filepath.Join(s.sshKeysDir, s.project)
	for i, p := range paths {
		if p != want {
			t.Errorf("paths[%d] = %q, want %q", i, p, want)
		}
	}
}

// ---- ExecInput env-var injection ----

func TestBuildExecInput_EnvVars(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	inst := testInstance("/clusters/c", "aerolab-proj-c-1", backends.LifeCycleStateRunning)
	inst.ClusterName = "mycluster"
	inst.NodeNo = 3
	inst.Owner = "jdoty"

	e := &backends.ExecInput{
		Username: "root",
		ExecDetail: sshexec.ExecDetail{
			Command: []string{"ls", "/"},
		},
	}
	conf := &sshexec.ClientConf{Host: "127.0.0.1", Port: 22, Username: "root"}

	execInput := s.buildExecInput(conf, e, inst)

	want := map[string]string{
		"AEROLAB_CLUSTER_NAME": "mycluster",
		"AEROLAB_NODE_NO":      strconv.Itoa(3),
		"AEROLAB_PROJECT_NAME": s.project,
		"AEROLAB_OWNER":        "jdoty",
	}
	got := map[string]string{}
	for _, kv := range execInput.Env {
		got[kv.Key] = kv.Value
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env[%s] = %q, want %q", k, got[k], v)
		}
	}
	if execInput.Host != "127.0.0.1" || execInput.Port != 22 || execInput.Username != "root" {
		t.Errorf("execInput ClientConf not carried through: %+v", execInput.ClientConf)
	}
	if len(execInput.Command) != 2 || execInput.Command[0] != "ls" {
		t.Errorf("execInput.Command = %v, want [ls /]", execInput.Command)
	}
}

// ---- InstancesUpdateHostsFile: script rendering (pure function, no dial) ----

func TestRenderHostsUpdateScript(t *testing.T) {
	entries := []string{
		"10.0.0.5      c-1                            # aerolab-managed",
		"10.0.0.6      c-2                            # aerolab-managed",
	}

	script := renderHostsUpdateScript(entries)
	for _, e := range entries {
		if !containsLine(script, e) {
			t.Errorf("rendered script missing entry %q:\n%s", e, script)
		}
	}
	if !containsLine(script, "mv /etc/hosts.tmp /etc/hosts") {
		t.Errorf("rendered script missing the expected mv step:\n%s", script)
	}
}

// TestInstancesUpdateHostsFile_NotRunning confirms the not-running check is
// enforced before any sftp/ssh attempt is made (via InstancesGetSftpConfig),
// without opening a real network connection.
func TestInstancesUpdateHostsFile_NotRunning(t *testing.T) {
	s, _ := newVagrantTestBackend(t)
	if _, err := s.ensureProjectKeypair(); err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}
	inst := testInstance("/clusters/c", "aerolab-proj-c-1", backends.LifeCycleStateStopped)
	err := s.InstancesUpdateHostsFile(backends.InstanceList{inst}, []string{"10.0.0.5 c-1 # aerolab-managed"}, 1)
	if err == nil {
		t.Fatal("expected an error for a not-running instance")
	}
}

func containsLine(haystack, needle string) bool {
	for _, l := range splitLines(haystack) {
		if l == needle {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
