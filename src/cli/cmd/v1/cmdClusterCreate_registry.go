package cmd

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"
	"time"
)

type RegistryEntry struct {
	AerospikeVersion string `json:"aerospikeVersion"`
	Flavor           string `json:"flavor"`
	OsName           string `json:"osName"`
	OsVersion        string `json:"osVersion"`
	Architecture     string `json:"architecture"`
	FileName         string `json:"fileName"`
	SHA256           string `json:"sha256"`
}

func fetchRegistryMetadata(baseURL string) ([]RegistryEntry, error) {
	url := strings.TrimRight(baseURL, "/") + "/metadata.json"
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch registry metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry metadata returned HTTP %d", resp.StatusCode)
	}
	var entries []RegistryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode registry metadata: %w", err)
	}
	return entries, nil
}

// findRegistryEntry matches a registry entry for the given aerospike version
// string (format "version-flavor", e.g. "8.0.0.4-enterprise"), OS name,
// OS version list (tried in priority order), and architecture.
func findRegistryEntry(entries []RegistryEntry, av string, osName string, osVersionList []string, arch string) *RegistryEntry {
	parts := strings.SplitN(av, "-", 2)
	if len(parts) != 2 {
		return nil
	}
	version, flavor := parts[0], parts[1]

	for _, osVer := range osVersionList {
		for i := range entries {
			e := &entries[i]
			if e.AerospikeVersion == version && e.Flavor == flavor && e.OsName == osName && e.OsVersion == osVer && e.Architecture == arch {
				return e
			}
		}
	}
	return nil
}

func loadTemplateFromRegistry(system *System, registryURL string, entry *RegistryEntry, region string, projectLabels map[string]string) error {
	url := strings.TrimRight(registryURL, "/") + "/" + entry.FileName
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download template from registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry download returned HTTP %d for %s", resp.StatusCode, entry.FileName)
	}

	var reader io.Reader = resp.Body

	// If SHA256 is specified, wrap in a hash-verifying reader
	var hasher *sha256HashReader
	if entry.SHA256 != "" {
		hasher = &sha256HashReader{
			reader: reader,
			hash:   sha256.New(),
		}
		reader = hasher
	}

	// Decompress if gzip
	if strings.HasSuffix(entry.FileName, ".gz") || strings.HasSuffix(entry.FileName, ".tgz") {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	err = system.Backend.DockerLoadImage(region, reader, projectLabels)
	if err != nil {
		return fmt.Errorf("docker load image: %w", err)
	}

	// Verify SHA256 after successful load
	if hasher != nil {
		actual := fmt.Sprintf("%x", hasher.hash.Sum(nil))
		if !strings.EqualFold(actual, entry.SHA256) {
			return fmt.Errorf("SHA256 mismatch: expected %s, got %s", entry.SHA256, actual)
		}
	}

	return nil
}

// sha256HashReader wraps an io.Reader and computes SHA256 as data passes through.
type sha256HashReader struct {
	reader io.Reader
	hash   hash.Hash
}

func (r *sha256HashReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 {
		r.hash.Write(p[:n]) //nolint:errcheck
	}
	return
}
