package bvagrant

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
)

//go:embed scripts/*
var scripts embed.FS

// sshConfigFor resolves an instance's live SSH endpoint (host/port) from vagrant
// via s.runner.SSHConfig, but authenticates with the project's own keypair under
// the caller-supplied username rather than vagrant's own User/IdentityFile: the
// project public key is provisioned into both root and vagrant at boot (see
// writeVagrantfile), so it works for whichever username the caller asks for.
func (s *b) sshConfigFor(instance *backends.Instance, username string) (*sshexec.ClientConf, error) {
	detail, ok := instance.BackendSpecific.(*InstanceDetail)
	if !ok || detail == nil {
		return nil, fmt.Errorf("instance %s has no vagrant backend detail", instance.InstanceID)
	}
	info, err := s.runner.SSHConfig(detail.ClusterDir, detail.MachineName)
	if err != nil {
		return nil, err
	}
	privKey, err := os.ReadFile(path.Join(s.sshKeysDir, s.project))
	if err != nil {
		return nil, fmt.Errorf("required key not found: %w", err)
	}
	return &sshexec.ClientConf{
		Host:       info.HostName,
		Port:       info.Port,
		Username:   username,
		PrivateKey: privKey,
	}, nil
}

// buildExecInput assembles the sshexec.ExecInput for a single instance's exec,
// carrying over conf and e's exec detail/retry settings, then injecting the
// standard AEROLAB_* environment variables (mirrors bdocker/baws).
func (s *b) buildExecInput(conf *sshexec.ClientConf, e *backends.ExecInput, i *backends.Instance) *sshexec.ExecInput {
	conf.ConnectTimeout = e.ConnectTimeout
	conf.MaxRetries = e.MaxRetries
	conf.RetrySleep = e.RetrySleep
	execInput := &sshexec.ExecInput{
		ClientConf: *conf,
		ExecDetail: e.ExecDetail,
	}
	execInput.Env = append(execInput.Env, &sshexec.Env{
		Key:   "AEROLAB_CLUSTER_NAME",
		Value: i.ClusterName,
	})
	execInput.Env = append(execInput.Env, &sshexec.Env{
		Key:   "AEROLAB_NODE_NO",
		Value: strconv.Itoa(i.NodeNo),
	})
	execInput.Env = append(execInput.Env, &sshexec.Env{
		Key:   "AEROLAB_PROJECT_NAME",
		Value: s.project,
	})
	execInput.Env = append(execInput.Env, &sshexec.Env{
		Key:   "AEROLAB_OWNER",
		Value: i.Owner,
	})
	return execInput
}

// InstancesExec runs e against each of instances over SSH, in parallel up to
// e.ParallelThreads (default: len(instances)). Not-running instances are
// reported with an "instance not running" error rather than attempted.
func (s *b) InstancesExec(instances backends.InstanceList, e *backends.ExecInput) []*backends.ExecOutput {
	if len(instances) == 0 {
		return nil
	}
	if e.ParallelThreads == 0 {
		e.ParallelThreads = len(instances)
	}
	out := []*backends.ExecOutput{}
	outl := new(sync.Mutex)
	parallelize.ForEachLimit(instances, e.ParallelThreads, func(i *backends.Instance) {
		if i.InstanceState != backends.LifeCycleStateRunning {
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output:   &sshexec.ExecOutput{Err: errors.New("instance not running")},
				Instance: i,
			})
			outl.Unlock()
			return
		}
		conf, err := s.sshConfigFor(i, e.Username)
		if err != nil {
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output:   &sshexec.ExecOutput{Err: err},
				Instance: i,
			})
			outl.Unlock()
			return
		}
		execInput := s.buildExecInput(conf, e, i)
		o := sshexec.ExecWithRetry(execInput, "ssh-exec-"+i.InstanceID)
		outl.Lock()
		out = append(out, &backends.ExecOutput{
			Output:   o,
			Instance: i,
		})
		outl.Unlock()
	})
	return out
}

// InstancesGetSftpConfig resolves an sshexec.ClientConf per running instance,
// erroring the whole call if any instance isn't running (mirrors bdocker/baws).
func (s *b) InstancesGetSftpConfig(instances backends.InstanceList, username string) ([]*sshexec.ClientConf, error) {
	confs := []*sshexec.ClientConf{}
	for _, i := range instances {
		if i.InstanceState != backends.LifeCycleStateRunning {
			return nil, errors.New("instance not running")
		}
		conf, err := s.sshConfigFor(i, username)
		if err != nil {
			return nil, err
		}
		conf.ConnectTimeout = 30 * time.Second
		confs = append(confs, conf)
	}
	return confs, nil
}

// InstancesGetSSHKeyPath returns the project keypair's private-key path for each
// instance (vagrant always uses the single project key, never a per-instance one).
func (s *b) InstancesGetSSHKeyPath(instances backends.InstanceList) []string {
	out := make([]string, 0, len(instances))
	for range instances {
		out = append(out, path.Join(s.sshKeysDir, s.project))
	}
	return out
}

// renderHostsUpdateScript formats the embedded update-hosts-file.sh template with
// the given pre-built hosts entries (already formatted "ip name # aerolab-managed"
// lines, one per instance, computed by the caller in backends.InstanceList.UpdateHostsFile).
func renderHostsUpdateScript(hostsEntries []string) string {
	tmpl, err := scripts.ReadFile("scripts/update-hosts-file.sh")
	if err != nil {
		// Embedded at build time; only reachable if the file were removed.
		return ""
	}
	return fmt.Sprintf(string(tmpl), strings.Join(hostsEntries, "\n"))
}

// InstancesUpdateHostsFile uploads and runs the /etc/hosts update script on each
// running instance (mirrors baws's InstancesUpdateHostsFile).
func (s *b) InstancesUpdateHostsFile(instances backends.InstanceList, hostsEntries []string, parallelSSHThreads int) error {
	script := renderHostsUpdateScript(hostsEntries)

	sshConfig, err := s.InstancesGetSftpConfig(instances, "root")
	if err != nil {
		return fmt.Errorf("failed to get sftp config: %w", err)
	}

	var retErr error
	retErrLock := new(sync.Mutex)
	wait := new(sync.WaitGroup)
	sem := make(chan struct{}, max(parallelSSHThreads, 1))
	for _, config := range sshConfig {
		wait.Add(1)
		sem <- struct{}{}
		go func(config *sshexec.ClientConf) {
			defer wait.Done()
			defer func() { <-sem }()
			cli, err := sshexec.NewSftp(config)
			if err != nil {
				retErrLock.Lock()
				retErr = errors.Join(retErr, fmt.Errorf("failed to create sftp client for host %s: %w", config.Host, err))
				retErrLock.Unlock()
				return
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/tmp/update-hosts-file.sh",
				Source:      strings.NewReader(script),
				Permissions: 0755,
			})
			if err != nil {
				retErrLock.Lock()
				retErr = errors.Join(retErr, fmt.Errorf("failed to write update-hosts-file.sh for host %s: %w", config.Host, err))
				retErrLock.Unlock()
			}
		}(config)
	}
	wait.Wait()
	if retErr != nil {
		return retErr
	}

	execInput := &backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:  []string{"bash", "/tmp/update-hosts-file.sh"},
			Terminal: true,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: parallelSSHThreads,
	}

	var errs error
	outputs := s.InstancesExec(instances, execInput)
	for _, output := range outputs {
		if output.Output != nil && output.Output.Err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to update hosts file on instance %s: %w", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), output.Output.Err))
		}
	}
	return errs
}
