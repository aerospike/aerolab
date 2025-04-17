package bgcp

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	LABEL_FILTER_AEROLAB = "labels.usedby=\"aerolab\""
)

const (
	TAG_FIREWALL_NAME_PREFIX = "aerolab-default-"
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
	TAG_COST_GB               = "aerolab-cg"
	TAG_AEROLAB_PROJECT       = "aerolab-p"
	TAG_AEROLAB_VERSION       = "aerolab-v"
	TAG_AEROLAB_OWNER         = "aerolab-o"
	TAG_AEROLAB_EXPIRES       = "aerolab-e"
	TAG_AEROLAB_DESCRIPTION   = "aerolab-d"
	TAG_AEROLAB_CUSTOM        = "aerolab-c"
)

// volumes uses labels
// firewalls uses description
// network doesn't do custom metadata as we do not have network creation and management at this time
// images use XXX
// instances use XXX
// expiry uses XXX

type metadata struct {
	Name                string            `json:"n,omitempty"`
	Description         string            `json:"d,omitempty"`
	AerolabVersion      string            `json:"v,omitempty"`
	AerolabProject      string            `json:"p,omitempty"`
	Owner               string            `json:"o,omitempty"`
	Expires             string            `json:"e,omitempty"`
	ClusterName         string            `json:"cn,omitempty"`
	NodeNo              int               `json:"nn,omitempty"`
	OsName              string            `json:"os,omitempty"`
	OsVersion           string            `json:"ov,omitempty"`
	CostPph             float64           `json:"cp,omitempty"`
	CostSoFar           float64           `json:"cs,omitempty"`
	CostPerGb           float64           `json:"cg,omitempty"`
	StartTime           time.Time         `json:"st,omitempty"`
	DnsName             string            `json:"dn,omitempty"`
	DnsDomainId         string            `json:"di,omitempty"`
	DnsDomainName       string            `json:"dnn,omitempty"`
	DnsRegion           string            `json:"dr,omitempty"`
	ClusterUuid         string            `json:"cu,omitempty"`
	DeleteOnTermination bool              `json:"dt,omitempty"`
	Custom              map[string]string `json:"c,omitempty"`
}

func (m *metadata) encodeToDescriptionField() string {
	json, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(json)
}

func (m *metadata) decodeFromDescriptionField(description string) error {
	return json.Unmarshal([]byte(description), m)
}

func (m *metadata) encodeToLabels() map[string]string {
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

func (m *metadata) decodeFromLabels(labels map[string]string) error {
	// Create a map to store chunks in order
	chunks := make(map[int]string)

	// Extract chunk number and value from labels
	for k, v := range labels {
		if strings.HasPrefix(k, "aerolab-metadata-") {
			// Parse chunk index from key
			var index int
			fmt.Sscanf(k, "aerolab-metadata-%d", &index)
			chunks[index] = v
		}
	}

	// Reassemble chunks in order
	var b32string strings.Builder
	for i := 0; i < len(chunks); i += 63 {
		if chunk, ok := chunks[i]; ok {
			b32string.WriteString(chunk)
		}
	}
	if b32string.Len() == 0 {
		return nil
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(b32string.String()))
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, m)
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
	for k, v := range labels {
		if strings.HasPrefix(k, "aerolab-metadata-") {
			// Parse chunk index from key
			var index int
			fmt.Sscanf(k, "aerolab-metadata-%d", &index)
			chunks[index] = v
		}
	}

	// Reassemble chunks in order
	var b32string strings.Builder
	for i := 0; i < len(chunks); i += 63 {
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

func sanitize(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, ".", "-"), "_", "-"), " ", "-"))
}
