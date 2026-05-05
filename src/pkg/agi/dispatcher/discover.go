package dispatcher

import (
	"errors"
	"fmt"
	"strings"

	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

// Source describes a single log destination resolved either from
// aerospike.conf or from a manual override on the dispatcher Config.
// Exactly one of File/Journal is non-empty.
type Source struct {
	// File is the absolute path of the file source. Empty when the
	// source is journald.
	File string

	// Journal is the systemd unit name to follow with `journalctl -u`.
	// Empty when the source is a file.
	Journal string
}

// IsFile reports whether s describes a file source.
func (s Source) IsFile() bool { return s.File != "" }

// IsJournal reports whether s describes a journald source.
func (s Source) IsJournal() bool { return s.Journal != "" }

// Label returns a short, stable string describing the source for
// logging/labeling purposes. Used as the ?source= query param when
// posting to the AGI listener.
func (s Source) Label() string {
	if s.IsFile() {
		return "file:" + s.File
	}
	if s.IsJournal() {
		return "journal:" + s.Journal
	}
	return "unknown"
}

// DiscoverSourceFromConf inspects an aerospike.conf at confPath and
// returns the first usable log destination it finds in the `logging`
// stanza. The lookup prefers a file destination over console because
// a file destination is always preferable for our purposes — it
// gives us byte-offset based resume which the journald path cannot.
//
// When the conf file lists both file and console, file wins.
//
// Errors:
//   - returned when confPath cannot be read or parsed.
//   - if the `logging` stanza exists but is empty or has no
//     recognized destinations, returns errNoLogging so the caller
//     can fall back to a sensible default (file path of
//     /var/log/aerospike/aerospike.log) rather than aborting.
func DiscoverSourceFromConf(confPath string) (Source, error) {
	cfg, err := aeroconf.ParseFile(confPath)
	if err != nil {
		return Source{}, fmt.Errorf("parse %s: %w", confPath, err)
	}
	logging := cfg.Stanza("logging")
	if logging == nil {
		return Source{}, errNoLogging
	}
	keys := logging.ListKeys()
	var (
		fileDest    string
		consoleSeen bool
	)
	for _, k := range keys {
		switch {
		case strings.HasPrefix(k, "file "):
			// "file <path>" — the path is everything after "file ".
			path := strings.TrimSpace(strings.TrimPrefix(k, "file "))
			if path != "" && fileDest == "" {
				fileDest = path
			}
		case strings.HasPrefix(k, "file"):
			// Defensive: some parsers might surface "file" with a
			// nested stanza/value rather than as part of the key
			// itself. Try to read the value.
			vals, _ := logging.GetValues(k)
			for _, v := range vals {
				if v == nil {
					continue
				}
				if path := strings.TrimSpace(*v); path != "" && fileDest == "" {
					fileDest = path
				}
			}
		case k == "console" || strings.HasPrefix(k, "console"):
			consoleSeen = true
		}
	}
	if fileDest != "" {
		return Source{File: fileDest}, nil
	}
	if consoleSeen {
		return Source{Journal: defaultJournalUnit}, nil
	}
	return Source{}, errNoLogging
}

// errNoLogging is a sentinel returned when the parsed conf file has
// no recognizable file/console logging destination. Callers should
// fall back to defaults (defaultLogPath / defaultJournalUnit).
var errNoLogging = errors.New("no logging destination found in aerospike.conf")

const (
	// defaultLogPath is used when --source-file is unset and the
	// aerospike.conf cannot be parsed. Matches the package-default
	// path on Aerospike server installs.
	defaultLogPath = "/var/log/aerospike/aerospike.log"

	// defaultJournalUnit is used when the conf says console-logging
	// but no unit override is provided.
	defaultJournalUnit = "aerospike.service"
)

// ResolveSource picks the source the dispatcher should follow, given
// (in priority order): explicit overrides > conf-file discovery >
// hardcoded default. This is the single entry point used by Dispatcher
// at startup.
func ResolveSource(cfg Config) Source {
	if cfg.SourceFile != "" {
		return Source{File: cfg.SourceFile}
	}
	if cfg.SourceJournal != "" {
		return Source{Journal: cfg.SourceJournal}
	}
	if cfg.AerospikeConf != "" {
		if s, err := DiscoverSourceFromConf(cfg.AerospikeConf); err == nil {
			return s
		}
	}
	return Source{File: defaultLogPath}
}
