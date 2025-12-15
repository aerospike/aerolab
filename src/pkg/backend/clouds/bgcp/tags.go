package bgcp

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lithammer/shortuuid"
)

const (
	LABEL_FILTER_AEROLAB = "labels.usedby=\"aerolab\""
)

const (
	TAG_FIREWALL_NAME_PREFIX          = "aerolab-"
	TAG_FIREWALL_NAME_PREFIX_INTERNAL = "aerolab-i-"
)

const (
	TAG_NAME                  = "aerolab-nm"
	TAG_START_TIME            = "aerolab-st"
	TAG_END_TIME              = "aerolab-et"
	TAG_COST_PPH              = "aerolab-cp"
	TAG_COST_SO_FAR           = "aerolab-cs"
	TAG_COST_PER_GB           = "aerolab-cg"
	TAG_CLUSTER_NAME          = "aerolab-cn"
	TAG_NODE_NO               = "aerolab-nn"
	TAG_OS_NAME               = "aerolab-os"
	TAG_OS_VERSION            = "aerolab-ov"
	TAG_DNS_NAME              = "aerolab-dn"
	TAG_DNS_DOMAIN_ID         = "aerolab-di"
	TAG_DNS_DOMAIN_NAME       = "aerolab-dnn"
	TAG_DNS_REGION            = "aerolab-dr"
	TAG_CLUSTER_UUID          = "aerolab-cu"
	TAG_DELETE_ON_TERMINATION = "aerolab-dt"
	TAG_AEROLAB_PROJECT       = "aerolab-p"
	TAG_AEROLAB_VERSION       = "aerolab-v"
	TAG_AEROLAB_OWNER         = "aerolab-o"
	TAG_AEROLAB_EXPIRES       = "aerolab-e"
	TAG_AEROLAB_DESCRIPTION   = "aerolab-d"
)

// V7 migration-related constants
const (
	// LABEL_V7_MIGRATED marks an instance as migrated from v7
	LABEL_V7_MIGRATED = "aerolab-v7-migrated"
	// TAG_SOFT_TYPE identifies the software type (encoded in metadata)
	TAG_SOFT_TYPE = "aerolab.type"
	// TAG_SOFT_VERSION identifies the software version (encoded in metadata)
	TAG_SOFT_VERSION = "aerolab.soft.version"
	// TAG_TELEMETRY is the v8 telemetry label
	TAG_TELEMETRY = "aerolab-telemetry"
	// TAG_CLIENT_TYPE is the v8 client type label
	TAG_CLIENT_TYPE = "aerolab-client-type"
	// TAG_IMAGE_TYPE identifies the image type (e.g., "aerospike")
	TAG_IMAGE_TYPE = "aerolab.image.type"
)

// V7 label names for discovery (GCP uses lowercase with underscores)
const (
	V7_LABEL_USED_BY             = "used_by"
	V7_LABEL_SERVER_MARKER       = "aerolab4"
	V7_LABEL_CLIENT_MARKER       = "aerolab4client"
	V7_LABEL_VOLUME_MARKER       = "aerolab7"
	V7_LABEL_CLUSTER_NAME        = "aerolab4cluster_name"
	V7_LABEL_NODE_NUMBER         = "aerolab4node_number"
	V7_LABEL_OPERATING_SYSTEM    = "aerolab4operating_system"
	V7_LABEL_OPERATING_SYS_VER   = "aerolab4operating_system_version"
	V7_LABEL_AEROSPIKE_VERSION   = "aerolab4aerospike_version"
	V7_LABEL_ARCH                = "arch"
	V7_LABEL_EXPIRES             = "aerolab4expires"
	V7_LABEL_OWNER               = "owner"
	V7_LABEL_COST_PPH            = "aerolab_cost_ph"
	V7_LABEL_COST_SO_FAR         = "aerolab_cost_sofar"
	V7_LABEL_COST_START_TIME     = "aerolab_cost_starttime"
	V7_LABEL_TELEMETRY           = "telemetry"
	V7_LABEL_IS_SPOT             = "isspot"
	V7_LABEL_CLIENT_NAME         = "aerolab4client_name"
	V7_LABEL_CLIENT_NODE_NUMBER  = "aerolab4client_node_number"
	V7_LABEL_CLIENT_OS           = "aerolab4client_operating_system"
	V7_LABEL_CLIENT_OS_VER       = "aerolab4client_operating_system_version"
	V7_LABEL_CLIENT_AS_VER       = "aerolab4client_aerospike_version"
	V7_LABEL_CLIENT_TYPE         = "aerolab4client_type"
	V7_LABEL_VOLUME_USED_BY      = "usedby"
	V7_LABEL_VOLUME_LAST_USED    = "lastused"
	V7_LABEL_VOLUME_EXPIRE_DUR   = "expireduration"
	V7_LABEL_VOLUME_OWNER        = "aerolab7owner"
	V7_LABEL_AGI_INSTANCE        = "agiinstance"
	V7_LABEL_AGI_NODIM           = "aginodim"
	V7_LABEL_TERM_ON_POW         = "termonpow"
)

// volumes uses labels
// firewalls uses description
// network doesn't do custom metadata as we do not have network creation and management at this time
// images use XXX
// instances use XXX
// expiry uses XXX

func encodeToDescriptionField(m map[string]string) string {
	json, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(json)
}

func decodeFromDescriptionField(description string) (map[string]string, error) {
	var ret map[string]string
	err := json.Unmarshal([]byte(description), &ret)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func encodeToLabels(m map[string]string) map[string]string {
	// tags to also encode as native labels (after sanitizing the character set of the values)
	nativeLabels := map[string]string{
		"usedby":            "aerolab",
		TAG_AEROLAB_PROJECT: "aerolab-project",
		TAG_AEROLAB_OWNER:   "aerolab-owner",
		TAG_NAME:            "aerolab-name",
		TAG_CLUSTER_NAME:    "aerolab-cluster-name",
		TAG_NODE_NO:         "aerolab-node-no",
		TAG_CLUSTER_UUID:    "aerolab-cluster-uuid",
		TAG_AEROLAB_EXPIRES: "aerolab-expires",
		TAG_AEROLAB_VERSION: "aerolab-version",
	}
	ret := make(map[string]string)
	json, err := json.Marshal(m)
	if err != nil {
		return ret
	}
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(json))
	// split encoded into 63 character chunks
	for i := 0; i < len(encoded); i += 63 {
		end := i + 63
		if end > len(encoded) {
			end = len(encoded)
		}
		ret[fmt.Sprintf("aerolab-metadata-%d", i)] = encoded[i:end]
	}
	for k, v := range nativeLabels {
		if len(ret) >= 64 {
			break
		}
		if _, ok := m[k]; ok {
			ret[v] = sanitize(m[k], false)
		}
	}
	return ret
}

func decodeFromLabels(labels map[string]string) (map[string]string, error) {
	// Create a map to store chunks in order
	chunks := make(map[int]string)

	// Extract chunk number and value from labels
	maxIndex := 0
	for k, v := range labels {
		if strings.HasPrefix(k, "aerolab-metadata-") {
			// Parse chunk index from key
			var index int
			fmt.Sscanf(k, "aerolab-metadata-%d", &index)
			chunks[index] = v
			if index > maxIndex {
				maxIndex = index
			}
		}
	}

	// Reassemble chunks in order
	var b32string strings.Builder
	for i := 0; i <= maxIndex; i += 63 {
		if chunk, ok := chunks[i]; ok {
			b32string.WriteString(chunk)
		}
	}
	if b32string.Len() == 0 {
		return nil, nil
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(b32string.String()))
	if err != nil {
		return nil, err
	}
	var m map[string]string
	err = json.Unmarshal(decoded, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// sanitize converts a string to comply with GCP resource naming requirements.
// GCP requires resource names to:
//   - Contain only lowercase letters (a-z), numbers (0-9), and hyphens (-)
//   - Start with a lowercase letter [a-z]
//   - End with a lowercase letter or number [a-z0-9]
//   - Not contain consecutive hyphens
//   - Be at most 63 characters long
//
// Parameters:
//   - s: The input string to sanitize. Can contain any characters including uppercase letters,
//     spaces, dots, underscores, and special characters. All invalid characters will be removed
//     or converted to valid ones.
//   - withUUID: When true, appends a short UUID to the sanitized string to ensure uniqueness.
//     The UUID is separated by a hyphen and the total length is kept within 63 characters.
//     When false, simply returns the sanitized version of the input string.
//
// Returns:
//   - A sanitized string that complies with all GCP naming requirements.
//
// Examples:
//   - sanitize("My_Cluster.Name", false) -> "my-cluster-name"
//   - sanitize("Test123", true) -> "test123-abc123xyz" (with UUID appended)
//   - sanitize("___CAPS___", false) -> "a-caps-a" (starts and ends properly)
func sanitize(s string, withUUID bool) string {
	ret := ""
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			ret += string(c)
			continue
		}
		if c >= 'A' && c <= 'Z' {
			ret += strings.ToLower(string(c))
			continue
		}
		if c == ' ' || c == '.' || c == '_' {
			ret += "-"
			continue
		}
	}
	for strings.Contains(ret, "--") {
		ret = strings.ReplaceAll(ret, "--", "-")
	}
	if withUUID {
		short := strings.ToLower(shortuuid.New())
		maxRetSize := 62 - len(short) // 63 - len(short) - 1 for the hyphen
		if len(ret) > maxRetSize {
			ret = ret[:maxRetSize]
		}
		// Trim trailing hyphens before appending UUID to avoid double hyphens
		ret = strings.TrimRight(ret, "-")
		// If ret is empty after truncation, ensure we have a prefix to avoid UUID starting with digit
		if len(ret) == 0 {
			ret = "i"
		}
		ret = ret + "-" + short
	}
	// GCP requires names to start with a lowercase letter: [a-z]
	// Also ensure it doesn't start with a hyphen
	ret = strings.TrimLeft(ret, "-")
	if len(ret) == 0 || ret[0] < 'a' || ret[0] > 'z' {
		ret = "a" + ret
	}
	// GCP requires names to end with [a-z0-9] (lowercase letter or number)
	// Remove trailing hyphens
	ret = strings.TrimRight(ret, "-")
	if len(ret) == 0 {
		ret = "a"
	}
	// Final check: ensure it ends with [a-z0-9]
	if len(ret) > 0 {
		lastChar := ret[len(ret)-1]
		if !((lastChar >= 'a' && lastChar <= 'z') || (lastChar >= '0' && lastChar <= '9')) {
			ret = ret + "a"
		}
	}
	// Final length check: ensure the string doesn't exceed 63 characters
	if len(ret) > 63 {
		ret = ret[:63]
		ret = strings.TrimRight(ret, "-")
	}
	return ret
}
