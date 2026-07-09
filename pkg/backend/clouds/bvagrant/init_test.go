package bvagrant

import (
	"testing"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/rglonek/logger"
)

// TestRegistration verifies that the vagrant backend is registered.
func TestRegistration(t *testing.T) {
	backend, ok := backends.LookupBackend(backends.BackendTypeVagrant)
	if !ok {
		t.Fatalf("vagrant backend not found in registry")
	}
	if backend == nil {
		t.Fatalf("vagrant backend is nil")
	}
}

// TestZones verifies that vagrant has a single fixed "local" zone.
func TestZones(t *testing.T) {
	instance := &b{}

	// Test ListEnabledZones
	zones, err := instance.ListEnabledZones()
	if err != nil {
		t.Fatalf("ListEnabledZones() returned error: %v", err)
	}
	if len(zones) != 1 || zones[0] != "local" {
		t.Fatalf("ListEnabledZones() expected [local], got %v", zones)
	}

	// Test ListAvailableZones
	zones, err = instance.ListAvailableZones()
	if err != nil {
		t.Fatalf("ListAvailableZones() returned error: %v", err)
	}
	if len(zones) != 1 || zones[0] != "local" {
		t.Fatalf("ListAvailableZones() expected [local], got %v", zones)
	}

	// Test EnableZones should fail
	err = instance.EnableZones("x")
	if err == nil {
		t.Fatalf("EnableZones() should return error for non-local zone")
	}

	// Test DisableZones should fail for "local"
	err = instance.DisableZones("local")
	if err == nil {
		t.Fatalf("DisableZones() should return error for local zone")
	}
}

// TestSetConfig verifies SetConfig populates fields and handles Subnet defaulting.
func TestSetConfig(t *testing.T) {
	instance := &b{}
	log := logger.NewLogger()
	configDir := "/tmp/test-config"
	project := "test-project"
	sshKeyDir := "/tmp/ssh-keys"
	aerolabVersion := "8.0.0"
	workDir := "/tmp/work"

	// Test with empty Subnet - should default to "192.168.56.0/24"
	creds := &clouds.Credentials{
		VAGRANT: clouds.VAGRANT{
			Subnet: "",
		},
	}

	err := instance.SetConfig(configDir, creds, project, sshKeyDir, log, aerolabVersion, workDir, nil, false)
	if err != nil {
		t.Fatalf("SetConfig() returned error: %v", err)
	}

	// Verify fields are set
	if instance.configDir != configDir {
		t.Fatalf("configDir mismatch: expected %s, got %s", configDir, instance.configDir)
	}
	if instance.credentials == nil {
		t.Fatalf("credentials is nil")
	}
	if instance.project != project {
		t.Fatalf("project mismatch: expected %s, got %s", project, instance.project)
	}
	if instance.sshKeysDir != sshKeyDir {
		t.Fatalf("sshKeysDir mismatch: expected %s, got %s", sshKeyDir, instance.sshKeysDir)
	}
	if instance.workDir != workDir {
		t.Fatalf("workDir mismatch: expected %s, got %s", workDir, instance.workDir)
	}
	if instance.aerolabVersion != aerolabVersion {
		t.Fatalf("aerolabVersion mismatch: expected %s, got %s", aerolabVersion, instance.aerolabVersion)
	}
	if instance.listAllProjects != false {
		t.Fatalf("listAllProjects should be false")
	}

	// Verify Subnet was defaulted
	if instance.credentials.Subnet != "192.168.56.0/24" {
		t.Fatalf("Subnet not defaulted: expected 192.168.56.0/24, got %s", instance.credentials.Subnet)
	}

	// Test with explicit Subnet - should be preserved
	instance2 := &b{}
	creds2 := &clouds.Credentials{
		VAGRANT: clouds.VAGRANT{
			Subnet: "10.99.0.0/16",
		},
	}

	err = instance2.SetConfig(configDir, creds2, project, sshKeyDir, log, aerolabVersion, workDir, nil, false)
	if err != nil {
		t.Fatalf("SetConfig() returned error: %v", err)
	}

	if instance2.credentials.Subnet != "10.99.0.0/16" {
		t.Fatalf("Subnet not preserved: expected 10.99.0.0/16, got %s", instance2.credentials.Subnet)
	}
}

// TestStubs verifies that stubbed methods return not-implemented errors.
func TestStubs(t *testing.T) {
	instance := &b{}

	// Test AcceptVPCPeering
	err := instance.AcceptVPCPeering("x")
	if err == nil {
		t.Fatalf("AcceptVPCPeering() should return error")
	}
	if err.Error() != "function AcceptVPCPeering not implemented for backend type: vagrant" {
		t.Fatalf("AcceptVPCPeering() returned wrong error: %v", err)
	}

	// Test CreateFirewall
	_, err = instance.CreateFirewall(nil, 0)
	if err == nil {
		t.Fatalf("CreateFirewall() should return error")
	}
	if err.Error() != "function CreateFirewall not implemented for backend type: vagrant" {
		t.Fatalf("CreateFirewall() returned wrong error: %v", err)
	}
}

// Compile-time assertion that b satisfies backends.Cloud.
var _ backends.Cloud = (*b)(nil)
