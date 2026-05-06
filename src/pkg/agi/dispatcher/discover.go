package dispatcher

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

type SourceKind string

const (
	SourceKindFile    SourceKind = "file"
	SourceKindJournal SourceKind = "journal"
)

type Source struct {
	Kind SourceKind
	Path string
	Unit string
}

func (s Source) Name() string {
	switch s.Kind {
	case SourceKindFile:
		return s.Path
	case SourceKindJournal:
		return "journal:" + s.Unit
	default:
		return "live"
	}
}

func (d *Dispatcher) resolveSource() (Source, error) {
	if d.cfg.SourceFile != "" {
		return Source{Kind: SourceKindFile, Path: d.cfg.SourceFile}, nil
	}
	if d.cfg.SourceJournal != "" {
		return Source{Kind: SourceKindJournal, Unit: d.cfg.SourceJournal}, nil
	}
	conf, err := os.ReadFile(d.cfg.AerospikeConf)
	if err != nil {
		return Source{}, fmt.Errorf("read aerospike conf: %w", err)
	}
	cfg, err := aeroconf.Parse(bytes.NewReader(conf))
	if err != nil {
		return Source{}, fmt.Errorf("parse aerospike conf: %w", err)
	}
	keys := cfg.Stanza("logging").ListKeys()
	for _, key := range keys {
		if strings.HasPrefix(key, "file ") {
			return Source{Kind: SourceKindFile, Path: strings.TrimSpace(strings.TrimPrefix(key, "file "))}, nil
		}
	}
	for _, key := range keys {
		if strings.HasPrefix(key, "console") {
			return Source{Kind: SourceKindJournal, Unit: "aerospike.service"}, nil
		}
	}
	return Source{}, fmt.Errorf("no file or console logging destination found in %s", d.cfg.AerospikeConf)
}

func (d *Dispatcher) resolveIdentity(source Source) (string, string, error) {
	cluster := strings.TrimSpace(d.cfg.ClusterName)
	node := strings.TrimSpace(d.cfg.NodeID)
	if node == "" {
		if v, err := asinfo("node"); err == nil {
			node = strings.TrimSpace(v)
		}
	}
	if cluster == "" {
		if v, err := asinfo("cluster-name"); err == nil {
			cluster = strings.TrimSpace(v)
		}
	}
	if (cluster == "" || node == "") && source.Kind == SourceKindFile {
		lc, ln := scanIdentityFromLog(source.Path, 30*time.Second)
		if cluster == "" {
			cluster = lc
		}
		if node == "" {
			node = ln
		}
	}
	if cluster == "" {
		return "", "", fmt.Errorf("cluster name could not be detected; pass --cluster")
	}
	if node == "" {
		return "", "", fmt.Errorf("node id could not be detected; pass --node-id")
	}
	return cluster, node, nil
}

func scanIdentityFromLog(path string, wait time.Duration) (cluster, node string) {
	re := regexp.MustCompile(`NODE-ID ([^ ]+) CLUSTER-SIZE \d+(?: CLUSTER-NAME ([^$]+))*`)
	deadline := time.Now().Add(wait)
	for {
		b, err := os.ReadFile(path)
		if err == nil {
			m := re.FindStringSubmatch(string(b))
			if len(m) > 0 {
				node = strings.TrimSpace(m[1])
				if len(m) > 2 {
					cluster = strings.TrimSpace(m[2])
				}
				return cluster, node
			}
		}
		if time.Now().After(deadline) {
			return "", ""
		}
		time.Sleep(time.Second)
	}
}
