package baws

import (
	"bytes"
	"fmt"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
)

// EfsInstallInput contains parameters for EFS installation
type EfsInstallInput struct {
	Instance *backends.Instance
}

// EfsInstall installs EFS utilities on the instance without mounting
// This is intended to be called during template creation
func EfsInstall(input *EfsInstallInput) error {
	if input.Instance == nil {
		return fmt.Errorf("instance is required")
	}

	// Upload install script
	sshconf, err := input.Instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}
	sftp, err := sshexec.NewSftp(sshconf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftp.Close()

	data, err := scripts.ReadFile("scripts/efs_install.sh")
	if err != nil {
		return fmt.Errorf("failed to read efs_install.sh: %w", err)
	}
	err = sftp.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/opt/aerolab/scripts/efs_install.sh",
		Permissions: 0755,
		Source:      bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to upload efs_install.sh: %w", err)
	}

	// Run install script
	execOut := input.Instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:  []string{"/bin/bash", "-c", "bash /opt/aerolab/scripts/efs_install.sh"},
			Terminal: true,
		},
		Username: "root",
	})
	if execOut.Output.Err != nil {
		return fmt.Errorf("EFS install failed: %s\n=== stdout ===\n%s\n=== stderr ===\n%s", execOut.Output.Err, string(execOut.Output.Stdout), string(execOut.Output.Stderr))
	}

	return nil
}

// EfsMountInput contains parameters for EFS mounting
type EfsMountInput struct {
	Instance             *backends.Instance
	MountTargetIP        string // Mount target IP address
	FileSystemId         string // EFS filesystem ID
	Region               string // AWS region
	MountTargetDirectory string // Where to mount the filesystem
	FIPS                 bool   // Enable FIPS mode
	IAM                  bool   // Enable IAM authentication
	IAMProfile           string // IAM profile name (requires IAM=true)
	MaxAttempts          int    // Max mount attempts (default 15)
}

// EfsMount mounts an EFS filesystem on the instance
// This assumes EFS utilities are already installed (via EfsInstall or template)
// This is intended to be called during AGI create
func EfsMount(input *EfsMountInput) error {
	if input.Instance == nil {
		return fmt.Errorf("instance is required")
	}
	if input.MountTargetIP == "" {
		return fmt.Errorf("mount target IP is required")
	}
	if input.FileSystemId == "" {
		return fmt.Errorf("filesystem ID is required")
	}
	if input.Region == "" {
		return fmt.Errorf("region is required")
	}
	if input.MountTargetDirectory == "" {
		return fmt.Errorf("mount target directory is required")
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = 15
	}

	// Upload mount script
	sshconf, err := input.Instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}
	sftp, err := sshexec.NewSftp(sshconf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftp.Close()

	data, err := scripts.ReadFile("scripts/efs_mount.sh")
	if err != nil {
		return fmt.Errorf("failed to read efs_mount.sh: %w", err)
	}
	err = sftp.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/opt/aerolab/scripts/efs_mount.sh",
		Permissions: 0755,
		Source:      bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to upload efs_mount.sh: %w", err)
	}

	// Build mount command: efs_mount.sh ATTEMPTS IP FSID REGION DEST_PATH [fips] [on [PROFILENAME]]
	cmd := fmt.Sprintf("bash /opt/aerolab/scripts/efs_mount.sh %d %s %s %s %s",
		input.MaxAttempts, input.MountTargetIP, input.FileSystemId, input.Region, input.MountTargetDirectory)
	if input.FIPS {
		cmd += " fips"
	}
	if input.IAM {
		cmd += " on"
		if input.IAMProfile != "" {
			cmd += " " + input.IAMProfile
		}
	}

	// Run mount script
	execOut := input.Instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:  []string{"/bin/bash", "-c", cmd},
			Terminal: true,
		},
		Username: "root",
	})
	if execOut.Output.Err != nil {
		return fmt.Errorf("EFS mount failed: %s\n=== stdout ===\n%s\n=== stderr ===\n%s", execOut.Output.Err, string(execOut.Output.Stdout), string(execOut.Output.Stderr))
	}

	return nil
}

// EfsInstallAndMount combines install and mount operations
// This is a convenience function that calls both EfsInstall and EfsMount
func EfsInstallAndMount(installInput *EfsInstallInput, mountInput *EfsMountInput) error {
	if err := EfsInstall(installInput); err != nil {
		return fmt.Errorf("install step failed: %w", err)
	}
	if err := EfsMount(mountInput); err != nil {
		return fmt.Errorf("mount step failed: %w", err)
	}
	return nil
}
