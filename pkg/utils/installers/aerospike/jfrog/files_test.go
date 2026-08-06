package jfrog

import (
	"strings"
	"testing"
)

func file(repo, path, name string) File {
	return File{Repo: repo, Path: path, Name: name, Parts: ParseFileName(name)}
}

func TestMatch_PrefersNativePackageOverBundle(t *testing.T) {
	fs := Files{
		file("d", "pool/noble", "aerospike-server-enterprise_8.1.3.0_ubuntu24.04_x86_64.tgz"),
		file("d", "pool/noble", "aerospike-server-enterprise_8.1.3.0-70ubuntu24.04_amd64.deb"),
	}
	got, err := fs.Match(MatchCriteria{Edition: "enterprise", OSName: "ubuntu", OSVersion: "24.04", Arch: "x86_64"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if !strings.HasSuffix(got.Name, ".deb") {
		t.Fatalf("expected the deb to win over the tgz bundle, got %s", got.Name)
	}
}

func TestMatch_FallsBackToBundle(t *testing.T) {
	fs := Files{
		file("d", "x", "aerospike-server-enterprise_8.1.3.0_ubuntu24.04_x86_64.tgz"),
		file("d", "x", "aerospike-server-enterprise_8.1.3.0_ubuntu24.04_aarch64.tgz"),
	}
	got, err := fs.Match(MatchCriteria{Edition: "enterprise", OSName: "ubuntu", OSVersion: "24.04", Arch: "x86_64"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Name != "aerospike-server-enterprise_8.1.3.0_ubuntu24.04_x86_64.tgz" {
		t.Fatalf("wrong artifact: %s", got.Name)
	}
	if got.Parts.Format != "tgz" {
		t.Fatalf("format = %q, want tgz", got.Parts.Format)
	}
}

func TestMatch_ErrorListsCandidates(t *testing.T) {
	fs := Files{
		file("d", "x", "aerospike-server-enterprise_8.1.3.0-70ubuntu22.04_amd64.deb"),
	}
	_, err := fs.Match(MatchCriteria{Edition: "enterprise", OSName: "ubuntu", OSVersion: "24.04", Arch: "x86_64"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "ubuntu22.04") {
		t.Fatalf("error should list the candidates it saw: %v", err)
	}
}

// The build carries artifacts, but none of them parse as a server package.
// This is the case that used to dead-end with "no enterprise deb package
// found" and nothing else to go on.
func TestMatch_ErrorSamplesUnparsedArtifacts(t *testing.T) {
	fs := Files{
		file("d", "pool/noble", "aerospike-server-enterprise_8.1.3.0-70_amd64.deb"), // no OS tag in the name
		file("d", "container", "aerospike-server-enterprise-docker_8.1.3.0_x86_64.tgz"),
		file("d", "sig", "aerospike-server-enterprise_8.1.3.0-70ubuntu24.04_amd64.deb.asc"),
	}
	_, err := fs.Match(MatchCriteria{Edition: "enterprise", OSName: "ubuntu", OSVersion: "24.04", Arch: "x86_64"})
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{
		"none of the 3 artifacts",
		"pool/noble/aerospike-server-enterprise_8.1.3.0-70_amd64.deb",
		"aerospike-server-enterprise_<version>-<release>ubuntu24.04_amd64.deb",
		"aerospike-server-enterprise_<version>_ubuntu24.04_x86_64.tgz",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestSample(t *testing.T) {
	if got := sample(nil, 3); got != "(none)" {
		t.Errorf("empty sample = %q", got)
	}
	if got := sample([]string{"a", "b"}, 3); got != "a, b" {
		t.Errorf("short sample = %q", got)
	}
	if got := sample([]string{"a", "b", "c", "d"}, 2); got != "a, b, ... (+2 more)" {
		t.Errorf("truncated sample = %q", got)
	}
}

func TestInstallScript_TGZBundle(t *testing.T) {
	f := file("d", "x", "aerospike-server-enterprise_8.1.3.0_ubuntu24.04_x86_64.tgz")
	script, err := InstallScript(&f, false, false)
	if err != nil {
		t.Fatalf("InstallScript: %v", err)
	}
	s := string(script)
	for _, want := range []string{"/opt/aerolab/files/" + f.Name, "./asinstall", "command -v asd"} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q", want)
		}
	}
}
