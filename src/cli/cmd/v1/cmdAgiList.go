package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

// AgiListCmd lists AGI instances from the inventory.
// This command filters instances by the "agi" type tag and displays
// relevant AGI-specific information such as labels, access URLs, and state.
//
// Usage:
//
//	aerolab agi list
//	aerolab agi list -o json
//	aerolab agi list --pager
type AgiListCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Owner      string   `short:"O" long:"owner" description:"Filter by owner"`
	Name       string   `short:"n" long:"name" description:"Filter by AGI name"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiListOutput represents the output structure for AGI list command.
type AgiListOutput struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	State       string `json:"state"`
	PublicIP    string `json:"publicIP,omitempty"`
	PrivateIP   string `json:"privateIP,omitempty"`
	AccessURL   string `json:"accessURL"`
	Owner       string `json:"owner"`
	Backend     string `json:"backend"`
	Zone        string `json:"zone,omitempty"`
	Instance    string `json:"instance,omitempty"`
	Expires     string `json:"expires,omitempty"`
	SourceLocal string `json:"sourceLocal,omitempty"`
	SourceSftp  string `json:"sourceSftp,omitempty"`
	SourceS3    string `json:"sourceS3,omitempty"`
	Spot        bool   `json:"spot,omitempty"`
	SSL         bool   `json:"ssl"`
	CreatedAt   string `json:"createdAt"`
	// Details contains the full instance information from the backend inventory.
	// This provides comprehensive instance details similar to cluster list output.
	Details *backends.Instance `json:"details,omitempty"`
}

// AgiVolumeOutput represents the output structure for AGI volumes.
type AgiVolumeOutput struct {
	Name             string `json:"name"`
	Label            string `json:"label,omitempty"`
	Type             string `json:"type"`
	SizeGiB          int64  `json:"sizeGiB"`
	Zone             string `json:"zone,omitempty"`
	State            string `json:"state"`
	AttachedTo       string `json:"attachedTo,omitempty"`
	Owner            string `json:"owner"`
	Backend          string `json:"backend"`
	Expires          string `json:"expires,omitempty"`
	SourceLocal      string `json:"sourceLocal,omitempty"`
	SourceSftp       string `json:"sourceSftp,omitempty"`
	SourceS3         string `json:"sourceS3,omitempty"`
	AerospikeVersion string `json:"aerospikeVersion,omitempty"`
	CreatedAt        string `json:"createdAt"`
	// Details contains the full volume information from the backend inventory.
	Details *backends.Volume `json:"details,omitempty"`
}

// AgiListFullOutput combines both instances and volumes for JSON output.
type AgiListFullOutput struct {
	Instances []AgiListOutput   `json:"instances"`
	Volumes   []AgiVolumeOutput `json:"volumes"`
}

// Execute implements the command execution for agi list.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiListCmd) Execute(args []string) error {
	cmd := []string{"agi", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListAGI(system, system.Backend.GetInventory(), args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ListAGI lists AGI instances from the inventory.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - args: Additional command arguments
//   - out: Output writer
//   - page: Optional pager
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiListCmd) ListAGI(system *System, inventory *backends.Inventory, args []string, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Filter for AGI instances
	instances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "agi",
	}).WithNotState(backends.LifeCycleStateTerminated).Describe()

	// Apply additional filters
	if c.Owner != "" {
		instances = instances.WithOwner(c.Owner).Describe()
	}
	if c.Name != "" {
		instances = instances.WithClusterName(c.Name).Describe()
	}

	// Filter for AGI volumes (volumes with aerolab7agiav tag and DeleteOnTermination=false)
	// These are persistent AGI volumes that survive instance termination
	agiVolumes := inventory.Volumes.WithTags(map[string]string{
		"aerolab7agiav": "",
	}).WithDeleteOnTermination(false).Describe()

	// Apply additional filters for volumes
	if c.Owner != "" {
		agiVolumes = agiVolumes.WithOwner(c.Owner).Describe()
	}
	if c.Name != "" {
		agiVolumes = agiVolumes.WithName(c.Name).Describe()
	}

	// Setup pager if requested
	if c.Pager && page == nil {
		var err error
		page, err = pager.New(out)
		if err != nil {
			return err
		}
		err = page.Start()
		if err != nil {
			return err
		}
		defer page.Close()
		out = page
	}

	// Build instance output data
	outputData := make([]AgiListOutput, 0, len(instances))
	for _, inst := range instances {
		output := AgiListOutput{
			Name:      inst.ClusterName,
			Label:     decodeBase64Tag(inst.Tags["agiLabel"]),
			State:     inst.InstanceState.String(),
			PublicIP:  inst.IP.Public,
			PrivateIP: inst.IP.Private,
			Owner:     inst.Owner,
			Backend:   string(inst.BackendType),
			Zone:      inst.ZoneName,
			Instance:  inst.InstanceType,
			Spot:      inst.SpotInstance,
			SSL:       inst.Tags["aerolab4ssl"] == "true",
			CreatedAt: inst.CreationTime.Format(time.RFC3339),
			Details:   inst,
		}

		// Decode source tags
		output.SourceLocal = decodeBase64Tag(inst.Tags["agiSrcLocal"])
		output.SourceSftp = decodeBase64Tag(inst.Tags["agiSrcSftp"])
		output.SourceS3 = decodeBase64Tag(inst.Tags["agiSrcS3"])

		// Build access URL
		protocol := "https"
		if inst.Tags["aerolab4ssl"] != "true" {
			protocol = "http"
		}

		if inst.BackendType == backends.BackendTypeDocker {
			// For Docker, use localhost with the mapped host port
			// Firewall format: host=0.0.0.0:8443,container=443
			output.AccessURL = computeDockerAgiAccessURL(inst.Firewalls, protocol)
		} else {
			// For cloud backends, use the IP
			ip := inst.IP.Public
			if ip == "" {
				ip = inst.IP.Private
			}
			if ip == "" {
				ip = inst.Name
			}
			output.AccessURL = fmt.Sprintf("%s://%s", protocol, ip)
		}

		// Handle expiry
		if !inst.Expires.IsZero() {
			if inst.Expires.After(time.Now()) {
				output.Expires = time.Until(inst.Expires).Truncate(time.Second).String()
			} else {
				output.Expires = "expired"
			}
		}

		outputData = append(outputData, output)
	}

	// Build volume output data
	volumeData := make([]AgiVolumeOutput, 0, len(agiVolumes))
	for _, vol := range agiVolumes {
		output := AgiVolumeOutput{
			Name:             vol.Name,
			Label:            decodeBase64Tag(vol.Tags["agiLabel"]),
			Type:             vol.VolumeType.String(),
			SizeGiB:          int64(vol.Size / backends.StorageGiB),
			Zone:             vol.ZoneName,
			State:            vol.State.String(),
			AttachedTo:       strings.Join(vol.AttachedTo, ", "),
			Owner:            vol.Owner,
			Backend:          string(vol.BackendType),
			SourceLocal:      decodeBase64Tag(vol.Tags["agiSrcLocal"]),
			SourceSftp:       decodeBase64Tag(vol.Tags["agiSrcSftp"]),
			SourceS3:         decodeBase64Tag(vol.Tags["agiSrcS3"]),
			AerospikeVersion: vol.Tags["aerolab7agiav"],
			CreatedAt:        vol.CreationTime.Format(time.RFC3339),
			Details:          vol,
		}

		// Handle expiry
		if !vol.Expires.IsZero() {
			if vol.Expires.After(time.Now()) {
				output.Expires = time.Until(vol.Expires).Truncate(time.Second).String()
			} else {
				output.Expires = "expired"
			}
		}

		volumeData = append(volumeData, output)
	}

	// Combine data for JSON output
	fullOutput := AgiListFullOutput{
		Instances: outputData,
		Volumes:   volumeData,
	}

	// Render output based on format
	switch c.Output {
	case "jq":
		params := []string{}
		if page != nil && page.HasColors() {
			params = append(params, "-C")
		}
		cmd := exec.Command("jq", params...)
		cmd.Stdout = out
		cmd.Stderr = out
		w, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(fullOutput)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(fullOutput)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(fullOutput)
	case "text":
		fmt.Fprintln(out, "AGI INSTANCES:")
		for _, agi := range outputData {
			fmt.Fprintf(out, "Name: %s, Label: %s, State: %s, PublicIP: %s, PrivateIP: %s, AccessURL: %s, Owner: %s, Backend: %s, Zone: %s, Instance: %s, Expires: %s, SSL: %t, Spot: %t, CreatedAt: %s\n",
				agi.Name, agi.Label, agi.State, agi.PublicIP, agi.PrivateIP, agi.AccessURL, agi.Owner, agi.Backend, agi.Zone, agi.Instance, agi.Expires, agi.SSL, agi.Spot, agi.CreatedAt)
		}
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "AGI VOLUMES:")
		for _, vol := range volumeData {
			// Build source string
			source := ""
			if vol.SourceLocal != "" {
				source = vol.SourceLocal
			} else if vol.SourceSftp != "" {
				source = "sftp:" + vol.SourceSftp
			} else if vol.SourceS3 != "" {
				source = "s3:" + vol.SourceS3
			}
			fmt.Fprintf(out, "Name: %s, Label: %s, Type: %s, SizeGiB: %d, Zone: %s, State: %s, AttachedTo: %s, Owner: %s, Backend: %s, Expires: %s, Source: %s, AerospikeVersion: %s, CreatedAt: %s\n",
				vol.Name, vol.Label, vol.Type, vol.SizeGiB, vol.Zone, vol.State, vol.AttachedTo, vol.Owner, vol.Backend, vol.Expires, source, vol.AerospikeVersion, vol.CreatedAt)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Name:asc", "State:asc"}
		}
		header := table.Row{"Name", "Label", "State", "AccessURL", "Owner", "Backend", "Zone", "Instance", "Expires", "SSL", "Spot", "Source", "CreatedAt"}
		if system.Opts.Config.Backend.Type == "docker" {
			header = table.Row{"Name", "Label", "State", "AccessURL", "Owner", "Source", "CreatedAt"}
		}
		rows := []table.Row{}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}

		for _, agi := range outputData {
			// Build source string
			source := ""
			if agi.SourceLocal != "" {
				source = agi.SourceLocal
			} else if agi.SourceSftp != "" {
				source = "sftp:" + agi.SourceSftp
			} else if agi.SourceS3 != "" {
				source = "s3:" + agi.SourceS3
			}
			if len(source) > 40 {
				source = source[:37] + "..."
			}

			// Color state
			state := agi.State
			if t != nil {
				if agi.State == "running" {
					state = t.ColorHiWhite.Sprint(agi.State)
				} else if agi.State == "stopped" {
					state = t.ColorWarn.Sprint(agi.State)
				}
			}

			// Color expiry
			expires := agi.Expires
			if t != nil && agi.Expires != "" {
				if agi.Expires == "expired" {
					expires = t.ColorErr.Sprint(agi.Expires)
				} else if strings.Contains(agi.Expires, "h") {
					// Less than a day left
					parts := strings.Split(agi.Expires, "h")
					if len(parts) > 0 {
						hours := 0
						fmt.Sscanf(parts[0], "%d", &hours)
						if hours < 6 {
							expires = t.ColorWarn.Sprint(agi.Expires)
						}
					}
				}
			}

			if system.Opts.Config.Backend.Type == "docker" {
				rows = append(rows, table.Row{
					agi.Name,
					agi.Label,
					state,
					agi.AccessURL,
					agi.Owner,
					source,
					agi.CreatedAt,
				})
			} else {
				rows = append(rows, table.Row{
					agi.Name,
					agi.Label,
					state,
					agi.AccessURL,
					agi.Owner,
					agi.Backend,
					agi.Zone,
					agi.Instance,
					expires,
					agi.SSL,
					agi.Spot,
					source,
					agi.CreatedAt,
				})
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("AGI INSTANCES"), header, rows))
		fmt.Fprintln(out, "")

		// Render volumes table (only for non-Docker backends since Docker volumes don't persist separately)
		if system.Opts.Config.Backend.Type != "docker" && len(volumeData) > 0 {
			volHeader := table.Row{"Name", "Label", "Type", "SizeGiB", "Zone", "State", "AttachedTo", "Owner", "Backend", "Expires", "Source", "AerospikeVersion", "CreatedAt"}
			volRows := []table.Row{}
			t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
			if err != nil {
				if err == printer.ErrTerminalWidthUnknown {
					system.Logger.Warn("Couldn't get terminal width, using default width")
				} else {
					return err
				}
			}
			for _, vol := range volumeData {
				// Build source string
				source := ""
				if vol.SourceLocal != "" {
					source = vol.SourceLocal
				} else if vol.SourceSftp != "" {
					source = "sftp:" + vol.SourceSftp
				} else if vol.SourceS3 != "" {
					source = "s3:" + vol.SourceS3
				}
				if len(source) > 40 {
					source = source[:37] + "..."
				}

				// Color state
				state := vol.State
				if t != nil {
					if vol.AttachedTo != "" {
						state = t.ColorHiWhite.Sprint(vol.State)
					}
				}

				// Color expiry
				expires := vol.Expires
				if t != nil && vol.Expires != "" {
					if vol.Expires == "expired" {
						expires = t.ColorErr.Sprint(vol.Expires)
					} else if strings.Contains(vol.Expires, "h") {
						// Less than a day left
						parts := strings.Split(vol.Expires, "h")
						if len(parts) > 0 {
							hours := 0
							fmt.Sscanf(parts[0], "%d", &hours)
							if hours < 6 {
								expires = t.ColorWarn.Sprint(vol.Expires)
							}
						}
					}
				}

				volRows = append(volRows, table.Row{
					vol.Name,
					vol.Label,
					vol.Type,
					vol.SizeGiB,
					vol.Zone,
					state,
					vol.AttachedTo,
					vol.Owner,
					vol.Backend,
					expires,
					source,
					vol.AerospikeVersion,
					vol.CreatedAt,
				})
			}
			fmt.Fprintln(out, t.RenderTable(printer.String("AGI VOLUMES"), volHeader, volRows))
			fmt.Fprintln(out, "")
		}
	}
	return nil
}

// decodeBase64Tag decodes a base64-encoded tag value.
func decodeBase64Tag(encoded string) string {
	if encoded == "" {
		return ""
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded // Return as-is if decoding fails
	}
	return string(decoded)
}

// computeDockerAgiAccessURL computes the access URL for an AGI instance running on Docker.
// It parses the firewall port mappings and returns localhost with the appropriate host port.
// Format: host=0.0.0.0:8443,container=443
//
// Parameters:
//   - firewalls: list of firewall/port mapping strings
//   - protocol: "http" or "https" (used as fallback only)
//
// Returns:
//   - string: the computed access URL (e.g., "https://localhost:8443")
func computeDockerAgiAccessURL(firewalls []string, protocol string) string {
	var port443, port80 int

	for _, fw := range firewalls {
		// Format: host=0.0.0.0:8443,container=443
		parts := strings.Split(fw, ",")
		if len(parts) != 2 {
			continue
		}

		var hostPort, containerPort string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "host=") {
				hostPart := strings.TrimPrefix(part, "host=")
				// Extract port from IP:PORT
				if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx >= 0 {
					hostPort = hostPart[colonIdx+1:]
				}
			} else if strings.HasPrefix(part, "container=") {
				containerPort = strings.TrimPrefix(part, "container=")
			}
		}

		if hostPort != "" {
			if port, err := strconv.Atoi(hostPort); err == nil {
				if containerPort == "443" {
					port443 = port
				} else if containerPort == "80" {
					port80 = port
				}
			}
		}
	}

	// Prioritize 443 (HTTPS) over 80 (HTTP)
	if port443 > 0 {
		return fmt.Sprintf("https://localhost:%d", port443)
	}
	if port80 > 0 {
		return fmt.Sprintf("http://localhost:%d", port80)
	}

	// Fallback
	return fmt.Sprintf("%s://localhost", protocol)
}
