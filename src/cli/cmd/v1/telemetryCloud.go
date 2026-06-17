package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
)

// Aerospike Cloud telemetry detection.
//
// Cloud subcommands do not use a feature file, so the existing feature-file
// detection path cannot determine whether the current user is Aerospike
// internal. We instead probe the Aerospike Cloud API after auth:
//
//   1. Parse the access token (JWT) to obtain the calling org's id.
//   2. Look the org up in a local cache at <aerolab-home>/telemetry/cloud-orgs.json.
//   3. On cache miss / TTL expiry, call GET /organization/members?limit=100.
//      - All emails @aerospike.com (and total <= 100) -> internal=true.
//      - Mixed domains or too many members        -> internal=false.
//      - HTTP 403 (no read:org scope)             -> internal=false, reason=permission-denied.
//      - Transient failures                       -> not cached, retried next time.
//   4. On internal -> refresh the one-way marker at aerospike-internal.json.
//
// The probe result is stashed in package-level state so that TelemetryEvent
// can later apply the per-command gate:
//   - cloud commands: telemeter only when the current org is internal.
//   - all other commands: telemeter when the one-way marker exists.

const (
	cloudOrgCacheFileName = "cloud-orgs.json"
	cloudOrgCacheTTL      = 30 * 24 * time.Hour
	cloudOrgMemberLimit   = 100
	cloudOrgInternalDom   = "@aerospike.com"
)

// reasons recorded in cloudOrgCacheEntry.Reason
const (
	cloudOrgReasonChecked          = "checked"
	cloudOrgReasonPermissionDenied = "permission-denied"
)

// cloudOrgCacheEntry is one entry in the local cloud-orgs cache. Each entry
// records the result of a single classification attempt for one org id; this
// is informational/debug surface, with Internal driving the actual gate.
type cloudOrgCacheEntry struct {
	Name          string    `json:"name"`
	Internal      bool      `json:"internal"`
	Reason        string    `json:"reason"`
	CheckedAt     time.Time `json:"checkedAt"`
	TotalMembers  int       `json:"totalMembers,omitempty"`
	MatchedEmails int       `json:"matchedEmails,omitempty"`
}

// expired reports whether this cache entry should be re-probed.
func (e *cloudOrgCacheEntry) expired() bool {
	return time.Since(e.CheckedAt) > cloudOrgCacheTTL
}

// cloudOrgCache is the on-disk shape of cloud-orgs.json. Stored as a map keyed
// by org id for O(1) lookups and easy human inspection.
type cloudOrgCache struct {
	Version int                           `json:"version"`
	Orgs    map[string]cloudOrgCacheEntry `json:"orgs"`
}

const cloudOrgCacheVersion = 1

// currentCloudOrg captures the result of the most recent probe, so that the
// telemetry gate (running later, inside TelemetryEvent / IsTelemetryEnabled)
// can know whether the current invocation is talking to an internal org.
//
// A nil currentCloudOrgInternal means "not probed" or "probe failed
// transiently" - in both cases cloud commands fall back to "do not telemeter
// this command", which is the safe default.
var (
	currentCloudOrgMu       sync.Mutex
	currentCloudOrgID       string
	currentCloudOrgInternal *bool
)

func setCurrentCloudOrg(orgID string, internal *bool) {
	currentCloudOrgMu.Lock()
	defer currentCloudOrgMu.Unlock()
	currentCloudOrgID = orgID
	currentCloudOrgInternal = internal
}

// isCurrentCloudOrgInternal returns (internal, known). known=false means we
// never managed to probe (no client, JWT parse failed, transient API failure).
func isCurrentCloudOrgInternal() (bool, bool) {
	currentCloudOrgMu.Lock()
	defer currentCloudOrgMu.Unlock()
	if currentCloudOrgInternal == nil {
		return false, false
	}
	return *currentCloudOrgInternal, true
}

// newCloudClient is a thin wrapper around cloud.NewClient that also runs the
// Aerospike Cloud telemetry probe once per process. It exists so cloud
// subcommands can opt into the probe via a single line change at every call
// site (cloud.NewClient -> newCloudClient).
func newCloudClient() (*cloud.Client, error) {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return client, err
	}
	probeCloudOrg(client)
	return client, nil
}

// probeCloudOrg classifies the org identified by the access token, updates
// the cache and the one-way marker as appropriate, and stashes the result for
// later consumption by TelemetryEvent. It never returns an error: this is a
// best-effort observation path that must not break the user's command.
func probeCloudOrg(client *cloud.Client) {
	orgID := client.OrgID()
	if orgID == "" {
		return
	}

	cache, _ := readCloudOrgCache()
	if cache == nil {
		cache = &cloudOrgCache{Version: cloudOrgCacheVersion, Orgs: map[string]cloudOrgCacheEntry{}}
	}

	// Cache hit and still fresh: use the cached verdict without an API call.
	if entry, ok := cache.Orgs[orgID]; ok && !entry.expired() {
		applyCloudOrgVerdict(orgID, entry.Internal)
		return
	}

	// Cache miss or TTL expired: probe live and update the cache.
	entry, ok := classifyCloudOrgByMembers(client)
	if !ok {
		// transient failure - leave the cache untouched so we retry next time.
		return
	}
	cache.Orgs[orgID] = entry
	_ = writeCloudOrgCache(cache)
	applyCloudOrgVerdict(orgID, entry.Internal)
}

// applyCloudOrgVerdict records the verdict in package state and, when the org
// is internal, also writes the one-way machine-wide marker so subsequent
// non-cloud commands telemeter without re-probing.
func applyCloudOrgVerdict(orgID string, internal bool) {
	v := internal
	setCurrentCloudOrg(orgID, &v)
	if !internal {
		return
	}
	// We do not know an expiry for an Aerospike Cloud org, so leave it zero;
	// the marker is informational only on that field.
	_ = refreshAerospikeInternalMarker("aerospike-cloud://"+orgID, time.Time{})
}

// cloudOrgMembersResponse is the subset of GET /organization/members we need
// to classify membership. Additional fields returned by the API are ignored.
type cloudOrgMembersResponse struct {
	Count   int `json:"count"`
	Members []struct {
		Email string `json:"email"`
	} `json:"members"`
}

// classifyCloudOrgByMembers fetches the org's member list and classifies the
// org as internal or external. The boolean return reports whether the result
// is authoritative enough to cache:
//
//   - 200 OK                     -> entry returned, cacheable
//   - 401 / 403                  -> permission-denied entry, cacheable
//   - anything else / network err-> empty entry, NOT cacheable
//
// Caching 401/403 ("permission denied") avoids hammering the API with a key
// that will never get read:org scope. Other failures are treated as transient.
func classifyCloudOrgByMembers(client *cloud.Client) (cloudOrgCacheEntry, bool) {
	status, body, err := client.GetStatus(fmt.Sprintf("/organization/members?limit=%d", cloudOrgMemberLimit))
	if err != nil {
		return cloudOrgCacheEntry{}, false
	}

	now := time.Now().UTC()

	switch {
	case status == http.StatusOK:
		var resp cloudOrgMembersResponse
		if jerr := json.Unmarshal(body, &resp); jerr != nil {
			return cloudOrgCacheEntry{}, false
		}
		matched := 0
		for _, m := range resp.Members {
			if isAerospikeInternalEmail(m.Email) {
				matched++
			}
		}
		// internal = at least one member AND every fetched member is internal
		// AND the org is small enough that "all members" is meaningful (i.e.
		// our limit=100 page returned the whole list).
		internal := resp.Count > 0 &&
			resp.Count <= cloudOrgMemberLimit &&
			len(resp.Members) == resp.Count &&
			matched == len(resp.Members)
		return cloudOrgCacheEntry{
			Internal:      internal,
			Reason:        cloudOrgReasonChecked,
			CheckedAt:     now,
			TotalMembers:  resp.Count,
			MatchedEmails: matched,
		}, true

	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		// User cannot list members - cache as not-internal so we stop
		// hammering the API on every subsequent invocation.
		return cloudOrgCacheEntry{
			Internal:  false,
			Reason:    cloudOrgReasonPermissionDenied,
			CheckedAt: now,
		}, true

	default:
		// 5xx, 404, unexpected codes - treat as transient.
		return cloudOrgCacheEntry{}, false
	}
}

func isAerospikeInternalEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(email)), cloudOrgInternalDom)
}

func cloudOrgCachePath() (string, error) {
	rootDir, err := AerolabRootDir()
	if err != nil {
		return "", err
	}
	return path.Join(rootDir, "telemetry", cloudOrgCacheFileName), nil
}

func readCloudOrgCache() (*cloudOrgCache, error) {
	p, err := cloudOrgCachePath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var c cloudOrgCache
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if c.Orgs == nil {
		c.Orgs = map[string]cloudOrgCacheEntry{}
	}
	return &c, nil
}

func writeCloudOrgCache(c *cloudOrgCache) error {
	if c.Version == 0 {
		c.Version = cloudOrgCacheVersion
	}
	p, err := cloudOrgCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path.Dir(p), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0600)
}
