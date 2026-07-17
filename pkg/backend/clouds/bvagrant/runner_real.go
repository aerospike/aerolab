package bvagrant

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	vagrant "github.com/bmatcuk/go-vagrant"
	"github.com/rglonek/logger"
)

// realRunner implements runner using the go-vagrant library for commands it
// supports (up/halt/destroy/status/ssh-config/box list/box add/version) and
// direct os/exec invocations of the vagrant binary for the handful of
// commands go-vagrant does not expose (box remove, package, plugin list).
type realRunner struct {
	// binaryPath is the path to (or name of) the vagrant executable. Empty
	// means "vagrant" resolved from PATH.
	binaryPath string
	log        *logger.Logger
}

// pathMu guards mutation of the process-wide PATH environment variable.
// go-vagrant's NewVagrantClient() resolves the "vagrant" binary internally
// via exec.LookPath and offers no way to inject a specific path, so when a
// custom binaryPath is configured we temporarily prepend its directory to
// PATH for the duration of client construction. This is process-global
// state, hence the mutex.
var pathMu sync.Mutex

func resolveBinaryPath(binaryPath string) string {
	if binaryPath == "" {
		return "vagrant"
	}
	return binaryPath
}

// binaryDirForPATH returns the directory to prepend to PATH so that
// exec.LookPath("vagrant") resolves to binaryPath, or "" if no adjustment is
// needed (binaryPath unset, or it's a bare command name with no directory
// component).
func binaryDirForPATH(binaryPath string) string {
	if binaryPath == "" {
		return ""
	}
	dir := filepath.Dir(binaryPath)
	if dir == "." {
		return ""
	}
	return dir
}

// withAdjustedPATH runs fn with PATH temporarily adjusted (if needed) so that
// go-vagrant's internal exec.LookPath("vagrant") finds binaryPath.
func withAdjustedPATH(binaryPath string, fn func() error) error {
	dir := binaryDirForPATH(binaryPath)
	if dir == "" {
		return fn()
	}
	pathMu.Lock()
	defer pathMu.Unlock()
	old := os.Getenv("PATH")
	defer os.Setenv("PATH", old)
	os.Setenv("PATH", dir+string(os.PathListSeparator)+old)
	return fn()
}

func newVagrantClient(binaryPath, dir string) (*vagrant.VagrantClient, error) {
	var client *vagrant.VagrantClient
	err := withAdjustedPATH(binaryPath, func() error {
		var innerErr error
		client, innerErr = vagrant.NewVagrantClient(dir)
		return innerErr
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

// collapseErr merges the Go/process-level error from cmd.Run() (runErr) and
// the vagrant-level error surfaced in a command's embedded ErrorResponse
// (cmdErr) into a single error, adding context. If both are nil, nil is
// returned. If only one is set, it is wrapped with context. If both are set,
// runErr is preserved as the error chain's cause (via %w, so errors.Is/As
// still work against it) while cmdErr's message is appended for visibility.
func collapseErr(runErr error, cmdErr error, context string) error {
	switch {
	case runErr == nil && cmdErr == nil:
		return nil
	case runErr != nil && cmdErr != nil:
		return fmt.Errorf("%s: %w (vagrant error: %s)", context, runErr, cmdErr.Error())
	case runErr != nil:
		return fmt.Errorf("%s: %w", context, runErr)
	default:
		return fmt.Errorf("%s: %w", context, cmdErr)
	}
}

func (r *realRunner) Up(dir string, machines []string, provider string, verbose bool) error {
	if len(machines) == 0 {
		return r.upWithBootRetry(dir, "", provider, verbose)
	}
	for _, m := range machines {
		if err := r.upWithBootRetry(dir, m, provider, verbose); err != nil {
			return err
		}
	}
	return nil
}

// upWithBootRetry retries a boot-timeout once: VirtualBox guests occasionally
// wedge during boot (RCU stalls / hung systemd jobs, observed empirically even
// on lightly loaded hosts). A forced halt + fresh boot reliably recovers, so
// one retry turns a flaky failure into a slower success.
func (r *realRunner) upWithBootRetry(dir, machine, provider string, verbose bool) error {
	err := r.upOne(dir, machine, provider, verbose)
	if err == nil || !strings.Contains(err.Error(), "VMBootTimeout") {
		return err
	}
	if r.log != nil {
		r.log.Warn("VAGRANT: machine %q timed out booting; forcing halt and retrying once", machine)
	}
	haltArgs := []string{"halt", "--force"}
	if machine != "" {
		haltArgs = append(haltArgs, machine)
	}
	if _, haltErr := r.execVagrant(dir, haltArgs...); haltErr != nil && r.log != nil {
		r.log.Warn("VAGRANT: forced halt of %q failed: %v", machine, haltErr)
	}
	return r.upOne(dir, machine, provider, verbose)
}

func (r *realRunner) upOne(dir, machine, provider string, verbose bool) error {
	client, err := newVagrantClient(r.binaryPath, dir)
	if err != nil {
		return collapseErr(err, nil, upContext(machine))
	}
	cmd := client.Up()
	cmd.MachineName = machine
	cmd.Provider = provider
	cmd.Verbose = verbose
	runErr := cmd.Run()
	return collapseErr(runErr, cmd.Error, upContext(machine))
}

func upContext(machine string) string {
	if machine == "" {
		return "vagrant up"
	}
	return fmt.Sprintf("vagrant up (machine %s)", machine)
}

func (r *realRunner) Halt(dir string, machines []string) error {
	if len(machines) == 0 {
		return r.haltOne(dir, "")
	}
	for _, m := range machines {
		if err := r.haltOne(dir, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *realRunner) haltOne(dir, machine string) error {
	client, err := newVagrantClient(r.binaryPath, dir)
	if err != nil {
		return collapseErr(err, nil, haltContext(machine))
	}
	cmd := client.Halt()
	cmd.MachineName = machine
	runErr := cmd.Run()
	return collapseErr(runErr, cmd.Error, haltContext(machine))
}

func haltContext(machine string) string {
	if machine == "" {
		return "vagrant halt"
	}
	return fmt.Sprintf("vagrant halt (machine %s)", machine)
}

func (r *realRunner) Destroy(dir string, machines []string) error {
	if len(machines) == 0 {
		return r.destroyOne(dir, "")
	}
	for _, m := range machines {
		if err := r.destroyOne(dir, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *realRunner) destroyOne(dir, machine string) error {
	client, err := newVagrantClient(r.binaryPath, dir)
	if err != nil {
		return collapseErr(err, nil, destroyContext(machine))
	}
	cmd := client.Destroy()
	cmd.MachineName = machine
	runErr := cmd.Run()
	return collapseErr(runErr, cmd.Error, destroyContext(machine))
}

func destroyContext(machine string) string {
	if machine == "" {
		return "vagrant destroy"
	}
	return fmt.Sprintf("vagrant destroy (machine %s)", machine)
}

func (r *realRunner) Status(dir string) (map[string]string, error) {
	client, err := newVagrantClient(r.binaryPath, dir)
	if err != nil {
		return nil, collapseErr(err, nil, "vagrant status")
	}
	cmd := client.Status()
	runErr := cmd.Run()
	if err := collapseErr(runErr, cmd.Error, "vagrant status"); err != nil {
		return nil, err
	}
	return cmd.Status, nil
}

func (r *realRunner) SSHConfig(dir string, machine string) (SSHInfo, error) {
	client, err := newVagrantClient(r.binaryPath, dir)
	if err != nil {
		return SSHInfo{}, collapseErr(err, nil, "vagrant ssh-config")
	}
	cmd := client.SSHConfig()
	cmd.MachineName = machine
	runErr := cmd.Run()
	if err := collapseErr(runErr, cmd.Error, "vagrant ssh-config"); err != nil {
		return SSHInfo{}, err
	}
	cfg, ok := cmd.Configs[machine]
	if !ok {
		for _, v := range cmd.Configs {
			cfg = v
			break
		}
	}
	return SSHInfo{
		HostName:     cfg.HostName,
		Port:         cfg.Port,
		User:         cfg.User,
		IdentityFile: cfg.IdentityFile,
	}, nil
}

// BoxList, BoxAdd, and Version are exec-backed rather than go-vagrant-backed.
// go-vagrant's NewVagrantClient(dir) requires <dir>/Vagrantfile to exist
// (vendor/github.com/bmatcuk/go-vagrant/vagrant_client.go), but box/version
// operations are not tied to any particular cluster directory and are
// normally invoked from a working directory that has no Vagrantfile at all.
// We instead run the vagrant binary directly with --machine-readable output
// and parse it ourselves (mirroring go-vagrant's own parsing approach).
func (r *realRunner) BoxList() ([]BoxInfo, error) {
	out, err := r.execVagrant("", boxListArgs()...)
	if err != nil {
		return nil, fmt.Errorf("vagrant box list: %w (output: %s)", err, strings.TrimSpace(out))
	}
	boxes, err := parseBoxList(out)
	if err != nil {
		return nil, fmt.Errorf("vagrant box list: %w", err)
	}
	return boxes, nil
}

func (r *realRunner) BoxAdd(name, location string) error {
	out, err := r.execVagrant("", boxAddArgs(name, location)...)
	if err != nil {
		return fmt.Errorf("vagrant box add: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (r *realRunner) Version() (string, error) {
	out, err := r.execVagrant("", versionArgs()...)
	if err != nil {
		return "", fmt.Errorf("vagrant version: %w (output: %s)", err, strings.TrimSpace(out))
	}
	version, err := parseVersionInstalled(out)
	if err != nil {
		return "", fmt.Errorf("vagrant version: %w", err)
	}
	return version, nil
}

// --- exec-backed commands (not supported by go-vagrant, or not tied to a
// specific Vagrantfile directory) --------------------------------------

func boxRemoveArgs(name string) []string {
	return []string{"box", "remove", "--force", name}
}

func boxListArgs() []string {
	return []string{"box", "list", "--machine-readable"}
}

func boxAddArgs(name, location string) []string {
	return []string{"box", "add", "--name", name, "--force", location}
}

func versionArgs() []string {
	return []string{"version", "--machine-readable"}
}

// machineReadableLine is one parsed line of `vagrant ... --machine-readable`
// output: "timestamp,target,type,data...". Mirrors the parsing done in
// vendor/github.com/bmatcuk/go-vagrant/output_parser.go.
type machineReadableLine struct {
	target string
	key    string
	data   string
}

func parseMachineReadableLines(output string) []machineReadableLine {
	var lines []machineReadableLine
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			continue
		}
		fields := make([]string, len(parts)-3)
		for i, part := range parts[3:] {
			f := strings.ReplaceAll(part, "\\n", "\n")
			f = strings.ReplaceAll(f, "\\r", "\r")
			f = strings.ReplaceAll(f, "%!(VAGRANT_COMMA)", ",")
			fields[i] = f
		}
		lines = append(lines, machineReadableLine{
			target: parts[1],
			key:    parts[2],
			data:   strings.Join(fields, ","),
		})
	}
	return lines
}

// parseBoxList parses the output of `vagrant box list --machine-readable`
// into a list of boxes, mirroring the field-triplet handling in
// vendor/github.com/bmatcuk/go-vagrant/command_box_list_response.go. Output
// with no boxes (e.g. "There are no installed boxes!") yields an empty,
// non-error result.
func parseBoxList(output string) ([]BoxInfo, error) {
	boxes := make([]BoxInfo, 0)
	idx := -1
	for _, line := range parseMachineReadableLines(output) {
		switch line.key {
		case "box-name":
			boxes = append(boxes, BoxInfo{Name: line.data})
			idx++
		case "box-version":
			if idx < 0 {
				return nil, errors.New("assertion broken: no box-name key for box")
			}
			boxes[idx].Version = line.data
		case "box-provider":
			if idx < 0 {
				return nil, errors.New("assertion broken: no box-name key for box")
			}
			boxes[idx].Provider = line.data
		case "error-exit":
			return nil, errors.New(line.data)
		}
	}
	return boxes, nil
}

// parseVersionInstalled parses the output of `vagrant version
// --machine-readable` and returns the installed version.
func parseVersionInstalled(output string) (string, error) {
	for _, line := range parseMachineReadableLines(output) {
		if line.key == "error-exit" {
			return "", errors.New(line.data)
		}
		if line.key == "version-installed" {
			return line.data, nil
		}
	}
	return "", errors.New("no version-installed field found in output")
}

func packageArgs(machine, outputPath string) []string {
	args := []string{"package"}
	if machine != "" {
		args = append(args, machine)
	}
	return append(args, "--output", outputPath)
}

func pluginListArgs() []string {
	return []string{"plugin", "list"}
}

// parsePluginNames extracts plugin names from `vagrant plugin list` output,
// e.g. a line "vagrant-libvirt (0.12.2, global)" yields "vagrant-libvirt".
func parsePluginNames(output string) []string {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, "(")
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name == "" || strings.Contains(name, " ") {
			continue
		}
		names = append(names, name)
	}
	return names
}

func (r *realRunner) execVagrant(dir string, args ...string) (string, error) {
	cmd := exec.Command(resolveBinaryPath(r.binaryPath), args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func (r *realRunner) BoxRemove(name string) error {
	out, err := r.execVagrant("", boxRemoveArgs(name)...)
	if err != nil {
		return fmt.Errorf("vagrant box remove: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (r *realRunner) Package(dir, machine, outputPath string) error {
	out, err := r.execVagrant(dir, packageArgs(machine, outputPath)...)
	if err != nil {
		return fmt.Errorf("vagrant package: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (r *realRunner) PluginList() ([]string, error) {
	out, err := r.execVagrant("", pluginListArgs()...)
	if err != nil {
		return nil, fmt.Errorf("vagrant plugin list: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return parsePluginNames(out), nil
}
