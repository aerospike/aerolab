package bvagrant

import (
	"fmt"
	"sync"
)

// fakeCall records a single invocation made against a fakeRunner.
type fakeCall struct {
	method string
	args   string
}

// fakeRunner is a test double implementing the runner interface. It records
// every call it receives (safe for concurrent use since later phases'
// tests exercise concurrency) and returns canned results/errors configured
// by the test. It is intentionally reusable by later phases' test files.
type fakeRunner struct {
	mu    sync.Mutex
	calls []fakeCall

	statusResult     map[string]string
	sshConfigResult  SSHInfo
	boxListResult    []BoxInfo
	versionResult    string
	pluginListResult []string

	// errs, keyed by method name, is returned by that method instead of a
	// canned result.
	errs map[string]error

	// onUp, if set, is invoked synchronously from Up before returning,
	// letting tests detect overlapping/concurrent Up calls.
	onUp func(dir string, machines []string, provider string) error

	// statusFunc, if set, supplies Status results per-call (overriding
	// statusResult), letting a test vary machine states across successive
	// Status calls within a single flow (e.g. ghost VMs that report
	// not_created at reconciliation time but running once re-created).
	statusFunc func(dir string) (map[string]string, error)
}

func (f *fakeRunner) record(method, args string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{method: method, args: args})
}

func (f *fakeRunner) callsSnapshot() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeRunner) errFor(method string) error {
	if f.errs == nil {
		return nil
	}
	return f.errs[method]
}

func (f *fakeRunner) Up(dir string, machines []string, provider string, verbose bool) error {
	f.record("Up", fmt.Sprintf("dir=%s machines=%v provider=%s verbose=%v", dir, machines, provider, verbose))
	if f.onUp != nil {
		if err := f.onUp(dir, machines, provider); err != nil {
			return err
		}
	}
	return f.errFor("Up")
}

func (f *fakeRunner) Halt(dir string, machines []string) error {
	f.record("Halt", fmt.Sprintf("dir=%s machines=%v", dir, machines))
	return f.errFor("Halt")
}

func (f *fakeRunner) Destroy(dir string, machines []string) error {
	f.record("Destroy", fmt.Sprintf("dir=%s machines=%v", dir, machines))
	return f.errFor("Destroy")
}

func (f *fakeRunner) Status(dir string) (map[string]string, error) {
	f.record("Status", fmt.Sprintf("dir=%s", dir))
	if err := f.errFor("Status"); err != nil {
		return nil, err
	}
	if f.statusFunc != nil {
		return f.statusFunc(dir)
	}
	return f.statusResult, nil
}

func (f *fakeRunner) SSHConfig(dir string, machine string) (SSHInfo, error) {
	f.record("SSHConfig", fmt.Sprintf("dir=%s machine=%s", dir, machine))
	if err := f.errFor("SSHConfig"); err != nil {
		return SSHInfo{}, err
	}
	return f.sshConfigResult, nil
}

func (f *fakeRunner) BoxList() ([]BoxInfo, error) {
	f.record("BoxList", "")
	if err := f.errFor("BoxList"); err != nil {
		return nil, err
	}
	return f.boxListResult, nil
}

func (f *fakeRunner) BoxAdd(name, location string) error {
	f.record("BoxAdd", fmt.Sprintf("name=%s location=%s", name, location))
	return f.errFor("BoxAdd")
}

func (f *fakeRunner) BoxRemove(name string) error {
	f.record("BoxRemove", fmt.Sprintf("name=%s", name))
	return f.errFor("BoxRemove")
}

func (f *fakeRunner) Package(dir, machine, outputPath string) error {
	f.record("Package", fmt.Sprintf("dir=%s machine=%s outputPath=%s", dir, machine, outputPath))
	return f.errFor("Package")
}

func (f *fakeRunner) Version() (string, error) {
	f.record("Version", "")
	if err := f.errFor("Version"); err != nil {
		return "", err
	}
	return f.versionResult, nil
}

func (f *fakeRunner) PluginList() ([]string, error) {
	f.record("PluginList", "")
	if err := f.errFor("PluginList"); err != nil {
		return nil, err
	}
	return f.pluginListResult, nil
}
