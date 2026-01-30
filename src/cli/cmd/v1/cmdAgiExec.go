package cmd

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/agi/notifier"
	"github.com/bestmethod/inslice"
	"gopkg.in/yaml.v3"
)

// Note: The AgiExecCmd struct is defined in cmdAgi.go
// This file implements the Execute methods and helper functions for exec subcommands

// AgiExecSimulateCmd simulates a notification to the AGI monitor for testing
type AgiExecSimulateCmd struct {
	Path       string `long:"path" description:"Path to a JSON file to use for notification" default:"notify.sim.json"`
	Make       bool   `long:"make" description:"Set to make the notification file using resource manager code instead of sending it"`
	AGIName    string `long:"agi-name" description:"Set agiName when making the notification JSON" default:"agi"`
	Help       HelpCmd
	notify     notifier.HTTPSNotify `no-default:"true"`
	deployJson string
}

func (c *AgiExecSimulateCmd) Execute(args []string) error {
	if c.Make {
		isDim := true
		if _, err := os.Stat("/opt/agi/nodim"); err == nil {
			isDim = false
		}
		notifyData, err := GetAgiStatus(true, "/opt/agi/ingest/")
		if err != nil {
			return err
		}
		deploymentjson, _ := os.ReadFile("/opt/agi/deployment.json.gz")
		c.deployJson = base64.StdEncoding.EncodeToString(deploymentjson)
		notifyItem := &ingest.NotifyEvent{
			IsDataInMemory:             isDim,
			IngestStatus:               notifyData,
			Event:                      agi.AgiEventResourceMonitor,
			AGIName:                    c.AGIName,
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
		}
		data, err := json.MarshalIndent(notifyItem, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(c.Path, data, 0644)
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return err
	}
	v := make(map[string]interface{})
	err = json.Unmarshal(data, &v)
	if err != nil {
		return err
	}
	data, err = json.Marshal(v)
	if err != nil {
		return err
	}
	nstring, err := os.ReadFile("/opt/agi/notifier.yaml")
	if err == nil {
		yaml.Unmarshal(nstring, &c.notify)
		c.notify.Init()
		defer c.notify.Close()
	}
	if c.notify.AGIMonitorUrl == "" && c.notify.Endpoint == "" {
		return errors.New("JSON notification is disabled")
	}
	return c.notify.NotifyData(data)
}

// AgiExecIngestStatusCmd retrieves and displays the current ingest status
type AgiExecIngestStatusCmd struct {
	IngestPath string  `long:"ingest-stat-path" default:"/opt/agi/ingest/"`
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecIngestStatusCmd) Execute(args []string) error {
	resp, err := GetAgiStatus(true, c.IngestPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(resp)
	return nil
}

// AgiExecIngestDetailCmd retrieves detailed ingest progress information
type AgiExecIngestDetailCmd struct {
	IngestPath string   `long:"ingest-stat-path" default:"/opt/agi/ingest/"`
	DetailType []string `short:"t" long:"detail-type" description:"File name of the progress detail"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecIngestDetailCmd) Execute(args []string) error {
	files := []string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json", "steps.json"}
	if len(c.DetailType) > 1 {
		fmt.Fprint(os.Stdout, "[\n")
	}
	for fi, fname := range c.DetailType {
		if !inslice.HasString(files, fname) {
			return errors.New("invalid detail type")
		}
		npath := path.Join(c.IngestPath, fname)
		if fname == "steps.json" {
			npath = "/opt/agi/ingest/steps.json"
		}
		gz := false
		if _, err := os.Stat(npath); err != nil {
			npath = npath + ".gz"
			if _, err := os.Stat(npath); err != nil {
				if len(c.DetailType) == 1 {
					return errors.New("file not found")
				} else {
					continue
				}
			}
			gz = true
		}
		f, err := os.Open(npath)
		if err != nil {
			return fmt.Errorf("could not open file: %s", err)
		}
		defer f.Close()
		var reader io.Reader
		reader = f
		if gz {
			fx, err := gzip.NewReader(f)
			if err != nil {
				return fmt.Errorf("could not open gz for reading: %s", err)
			}
			defer fx.Close()
			reader = fx
		}
		io.Copy(os.Stdout, reader)
		if len(c.DetailType) > 1 {
			if fi+1 == len(c.DetailType) {
				fmt.Fprint(os.Stdout, "\n]\n")
			} else {
				fmt.Fprint(os.Stdout, ",\n")
			}
		}
	}
	return nil
}

// GetSSHAuthorizedKeysGzB64 reads and compresses the authorized_keys file for transfer.
// It returns a base64-encoded gzip-compressed version of the file contents.
//
// Returns:
//   - string: Base64-encoded gzip-compressed authorized_keys content, or empty string on error
func GetSSHAuthorizedKeysGzB64() string {
	c, err := os.ReadFile("/root/.ssh/authorized_keys")
	if err != nil {
		return ""
	}
	w := &bytes.Buffer{}
	wr := gzip.NewWriter(w)
	_, err = wr.Write(c)
	if err != nil {
		wr.Close()
		return ""
	}
	wr.Close()
	return base64.StdEncoding.EncodeToString(w.Bytes())
}

// PutSSHAuthorizedKeys appends SSH keys from a compressed base64 string to authorized_keys.
// This is used for monitor recovery to restore SSH access to instances.
//
// Parameters:
//   - ContentsGzB64: Base64-encoded gzip-compressed SSH authorized_keys content
func PutSSHAuthorizedKeys(ContentsGzB64 string) {
	c, err := base64.StdEncoding.DecodeString(ContentsGzB64)
	if err != nil {
		return
	}
	r, err := gzip.NewReader(bytes.NewReader(c))
	if err != nil {
		return
	}
	keys, err := io.ReadAll(r)
	if err != nil {
		return
	}
	kfile, err := os.OpenFile("/root/.ssh/authorized_keys", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer kfile.Close()
	kfile.Write(keys)
}

// cut is a helper function for parsing /proc/meminfo style lines.
// It splits a string by separator and returns the field at the specified index.
//
// Parameters:
//   - s: String to split
//   - field: 1-based index of field to return
//   - sep: Separator string
//
// Returns:
//   - string: The field at the specified index, or empty string if not found
func cut(s string, field int, sep string) string {
	parts := []string{}
	for _, p := range splitMultiple(s, sep) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if field > len(parts) || field < 1 {
		return ""
	}
	return parts[field-1]
}

// splitMultiple splits a string by a separator, handling multiple consecutive separators
func splitMultiple(s string, sep string) []string {
	result := []string{}
	current := ""
	for _, c := range s {
		if string(c) == sep {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

