package bgcp

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	LABEL_FILTER_AEROLAB = "labels.usedby=aerolab"
)

const (
	TAG_FIREWALL_NAME_PREFIX = "aerolab-default-"
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
