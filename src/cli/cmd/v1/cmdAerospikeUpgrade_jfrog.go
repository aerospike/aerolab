package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike/jfrog"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/rglonek/logger"
)

// upgradeJFrog drives `aerolab aerospike upgrade` in JFrog mode.
//
// Unlike the public flow which embeds a curl in the script, the JFrog
// flow pre-downloads the package on the operator's laptop, SFTP-uploads
// it to each instance, then runs a small rpm/deb install script. This
// keeps the JFrog auth token off the target instances.
//
// Instances inside a single cluster may run different OS/arch combos,
// so the resolver is keyed per-instance and we cache downloads against
// the local cache directory so repeated targets are free.
func (c *AerospikeUpgradeCmd) upgradeJFrog(cluster backends.InstanceList, system *System, log *logger.Logger, cfg *jfrog.Config) (backends.InstanceList, error) {
	edition, cleanVer := jfrog.EditionFromInput(c.AerospikeVersion, "enterprise")
	build, err := cfg.ResolveBuild(cleanVer)
	if err != nil {
		return nil, err
	}
	log.Info("Querying JFrog build %q number %q", build.Name, build.Number)
	files, err := build.Files(context.Background())
	if err != nil {
		return nil, err
	}

	cacheDir, err := jfrogCacheDir()
	if err != nil {
		return nil, err
	}

	// One artifact lookup per (osName, osVersion, arch). We cache both
	// the match and the local path so the SFTP loop doesn't redo work.
	type resolved struct {
		match    *jfrog.File
		localPkg string
	}
	cache := map[string]*resolved{}
	var cacheMu sync.Mutex

	resolveFor := func(instance *backends.Instance) (*resolved, error) {
		arch := "x86_64"
		switch instance.Architecture {
		case backends.ArchitectureNative:
			if runtime.GOARCH == "arm64" {
				arch = "aarch64"
			}
		case backends.ArchitectureARM64:
			arch = "aarch64"
		}
		osName := instance.OperatingSystem.Name
		if osName == "rocky" {
			osName = "centos"
		}
		osVersion := instance.OperatingSystem.Version
		key := osName + "/" + osVersion + "/" + arch

		cacheMu.Lock()
		hit, ok := cache[key]
		cacheMu.Unlock()
		if ok {
			return hit, nil
		}

		match, err := files.Match(jfrog.MatchCriteria{
			Edition: edition, OSName: osName, OSVersion: osVersion, Arch: arch,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		log.Info("Downloading %s/%s/%s for %s", match.Repo, match.Path, match.Name, key)
		local, err := cfg.Download(context.Background(), match, cacheDir)
		if err != nil {
			return nil, err
		}

		out := &resolved{match: match, localPkg: local}
		cacheMu.Lock()
		cache[key] = out
		cacheMu.Unlock()
		return out, nil
	}

	var hasErr error
	var errMu sync.Mutex
	parallelize.ForEachLimit(cluster.Describe(), c.Threads, func(instance *backends.Instance) {
		r, err := resolveFor(instance)
		if err != nil {
			errMu.Lock()
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", instance.ClusterName, instance.NodeNo, err))
			errMu.Unlock()
			log.Error("Failed to resolve JFrog artifact for %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			return
		}
		if err := c.upgradeInstanceJFrog(instance, r.match, r.localPkg, system, log); err != nil {
			errMu.Lock()
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", instance.ClusterName, instance.NodeNo, err))
			errMu.Unlock()
			log.Error("Failed to upgrade instance %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
		}
	})

	return cluster.Describe(), hasErr
}

// upgradeInstanceJFrog SFTP-uploads the cached package + install script
// onto a single instance and executes it.
func (c *AerospikeUpgradeCmd) upgradeInstanceJFrog(instance *backends.Instance, match *jfrog.File, localPkg string, system *System, log *logger.Logger) error {
	conf, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}
	conf.MaxRetries = c.MaxRetries
	conf.RetrySleep = c.RetrySleep

	cli, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer cli.Close()

	// Upload the package itself.
	f, err := os.Open(localPkg)
	if err != nil {
		return fmt.Errorf("could not open cached package: %w", err)
	}
	remotePkg := jfrog.RemotePackagePath(match)
	log.Info("Uploading %s to %s:%d:%s", localPkg, instance.ClusterName, instance.NodeNo, remotePkg)
	err = cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    remotePkg,
		Source:      f,
		Permissions: 0644,
	})
	f.Close()
	if err != nil {
		return fmt.Errorf("could not upload package: %w", err)
	}

	// Build install script with upgrade=true.
	installScript, err := jfrog.InstallScript(match, system.logLevel >= 5, true)
	if err != nil {
		return fmt.Errorf("could not generate install script: %w", err)
	}
	upgradeScript, err := c.createUpgradeScript(installScript, log)
	if err != nil {
		return fmt.Errorf("could not wrap upgrade script: %w", err)
	}

	scriptPath := "/opt/aerolab/scripts/upgrade-aerospike.sh"
	err = cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    scriptPath,
		Source:      bytes.NewReader(upgradeScript),
		Permissions: 0755,
	})
	if err != nil {
		return fmt.Errorf("failed to upload upgrade script: %w", err)
	}

	log.Info("Running upgrade script on %s:%d", instance.ClusterName, instance.NodeNo)
	var stdout, stderr *os.File
	terminal := false
	if system.logLevel >= 5 {
		stdout, stderr = os.Stdout, os.Stderr
		terminal = true
	}
	detail := sshexec.ExecDetail{
		Command:        []string{"bash", scriptPath},
		Terminal:       terminal,
		SessionTimeout: 15 * time.Minute,
	}
	if stdout != nil {
		detail.Stdout = stdout
	}
	if stderr != nil {
		detail.Stderr = stderr
	}
	output := instance.Exec(&backends.ExecInput{
		ExecDetail:      detail,
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	})
	if output.Output.Err != nil {
		failure := scriptlog.NewScriptFailureWithPath(
			instance.ClusterName,
			instance.NodeNo,
			scriptPath,
			upgradeScript,
			output.Output.Stdout,
			output.Output.Stderr,
			output.Output.Err,
		)
		logPath, saveErr := scriptlog.SaveFailure(failure)
		if saveErr != nil {
			return fmt.Errorf("upgrade script failed: %w\nstdout: %s\nstderr: %s (also failed to save logs: %v)",
				output.Output.Err,
				string(output.Output.Stdout),
				string(output.Output.Stderr),
				saveErr)
		}
		return fmt.Errorf("%s", scriptlog.FormatError(logPath, instance.ClusterName, instance.NodeNo, output.Output.Err))
	}

	log.Info("Successfully upgraded aerospike on %s:%d", instance.ClusterName, instance.NodeNo)
	return nil
}
