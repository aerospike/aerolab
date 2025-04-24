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
	if ret[0] <= 'a' && ret[0] >= 'z' {
		ret = "a" + ret
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
		ret = ret + "-" + short
	}
	return ret
}
