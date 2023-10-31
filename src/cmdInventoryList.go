package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/ingest"
	"github.com/bestmethod/inslice"
	isatty "github.com/mattn/go-isatty"
	"golang.org/x/term"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type inventoryListCmd struct {
	Owner      string  `long:"owner" description:"Only show resources tagged with this owner"`
	Pager      bool    `long:"pager" description:"set to enable vertical and horizontal pager"`
	Json       bool    `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool    `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	AWSFull    bool    `long:"aws-full" description:"set to iterate through all regions and provide full output"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *inventoryListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.JsonPretty {
		c.Json = true
	}
	return c.run(true, true, true, true, true, inventoryShowExpirySystem|inventoryShowAGI|inventoryShowVolumes)
}

const inventoryShowExpirySystem = 1
const inventoryShowAGI = 2
const inventoryShowAGIStatus = 4
const inventoryShowVolumes = 8

func (c *inventoryListCmd) run(showClusters bool, showClients bool, showTemplates bool, showFirewalls bool, showSubnets bool, showOthers ...int) error {
	inventoryItems := []int{}
	if showClusters {
		inventoryItems = append(inventoryItems, InventoryItemClusters)
	}
	if showClients {
		inventoryItems = append(inventoryItems, InventoryItemClients)
	}
	if showTemplates {
		inventoryItems = append(inventoryItems, InventoryItemTemplates)
	}
	if showFirewalls || showSubnets {
		inventoryItems = append(inventoryItems, InventoryItemFirewalls)
	}
	for _, showOther := range showOthers {
		if showOther&inventoryShowExpirySystem > 0 {
			inventoryItems = append(inventoryItems, InventoryItemExpirySystem)
		}
		if showOther&inventoryShowVolumes > 0 {
			inventoryItems = append(inventoryItems, InventoryItemVolumes)
		}
		if showOther&inventoryShowAGI > 0 {
			inventoryItems = append(inventoryItems, InventoryItemClusters)
			inventoryItems = append(inventoryItems, InventoryItemAGI)
		}
		if showOther&inventoryShowAGIStatus > 0 && !inslice.HasInt(inventoryItems, InventoryItemVolumes) {
			inventoryItems = append(inventoryItems, InventoryItemVolumes)
		}
	}
	if a.opts.Config.Backend.Type == "aws" && c.AWSFull {
		inventoryItems = append(inventoryItems, InventoryItemAWSAllRegions)
	}

	inv, err := b.Inventory(c.Owner, inventoryItems)
	if err != nil {
		return err
	}

	for vi, v := range inv.Clusters {
		nip := v.PublicIp
		if nip == "" {
			nip = v.PrivateIp
		}
		port := ""
		if a.opts.Config.Backend.Type == "docker" && inv.Clusters[vi].DockerExposePorts != "" {
			nip = "127.0.0.1"
			port = ":" + inv.Clusters[vi].DockerExposePorts
		}
		prot := "http://"
		if v.gcpLabels["aerolab4ssl"] == "true" || v.awsTags["aerolab4ssl"] == "true" || v.DockerInternalPort == "443" {
			prot = "https://"
		}
		if v.Features&ClusterFeatureAGI > 0 {
			inv.Clusters[vi].AccessUrl = prot + nip + port + "/agi/menu"
		}
	}

	for vi, v := range inv.Clients {
		nip := v.PublicIp
		if nip == "" {
			nip = v.PrivateIp
		}
		port := ""
		if a.opts.Config.Backend.Type == "docker" && inv.Clients[vi].DockerExposePorts != "" {
			nip = "127.0.0.1"
			port = ":" + inv.Clients[vi].DockerExposePorts
		}
		switch strings.ToLower(v.ClientType) {
		case "ams":
			if port == "" {
				port = ":3000"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port
			inv.Clients[vi].AccessPort = "3000"
		case "elasticsearch":
			if port == "" {
				port = ":9200"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port + "/NAMESPACE/_search"
			inv.Clients[vi].AccessPort = "9200"
		case "rest-gateway":
			if port == "" {
				port = ":8081"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port
			inv.Clients[vi].AccessPort = "8081"
		case "vscode":
			if port == "" {
				port = ":8080"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port
			inv.Clients[vi].AccessPort = "8080"
		}
	}

	if c.Json {
		enc := json.NewEncoder(os.Stdout)
		if c.JsonPretty {
			enc.SetIndent("", "    ")
		}
		if showClusters && showClients && showTemplates && showFirewalls && showSubnets {
			err = enc.Encode(inv)
			return err
		}
		if showClusters {
			err = enc.Encode(inv.Clusters)
			if err != nil {
				return err
			}
		}
		if showClients {
			err = enc.Encode(inv.Clients)
			if err != nil {
				return err
			}
		}
		if showTemplates {
			err = enc.Encode(inv.Templates)
			if err != nil {
				return err
			}
		}
		if showFirewalls {
			err = enc.Encode(inv.FirewallRules)
			if err != nil {
				return err
			}
		}
		if showSubnets {
			err = enc.Encode(inv.Subnets)
			if err != nil {
				return err
			}
		}
		for _, showOther := range showOthers {
			if showOther&inventoryShowExpirySystem > 0 {
				err = enc.Encode(inv.ExpirySystem)
				if err != nil {
					return err
				}
			}
			if showOther&inventoryShowVolumes > 0 {
				err = enc.Encode(inv.Volumes)
				if err != nil {
					return err
				}
			}
			if showOther&inventoryShowAGI > 0 {
				err = enc.Encode(inv.Clusters)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	sort.Slice(inv.Templates, func(i, j int) bool {
		if inv.Templates[i].AerospikeVersion > inv.Templates[j].AerospikeVersion {
			return true
		} else if inv.Templates[i].AerospikeVersion < inv.Templates[j].AerospikeVersion {
			return false
		} else {
			if inv.Templates[i].Arch < inv.Templates[j].Arch {
				return true
			} else if inv.Templates[i].Arch > inv.Templates[j].Arch {
				return false
			} else {
				if inv.Templates[i].Distribution < inv.Templates[j].Distribution {
					return true
				} else if inv.Templates[i].Distribution > inv.Templates[j].Distribution {
					return false
				} else {
					return inv.Templates[i].OSVersion < inv.Templates[j].OSVersion
				}
			}
		}
	})

	sort.Slice(inv.Clusters, func(i, j int) bool {
		if inv.Clusters[i].ClusterName < inv.Clusters[j].ClusterName {
			return true
		} else if inv.Clusters[i].ClusterName > inv.Clusters[j].ClusterName {
			return false
		} else {
			return inv.Clusters[i].NodeNo < inv.Clusters[j].NodeNo
		}
	})

	sort.Slice(inv.Clients, func(i, j int) bool {
		if inv.Clients[i].ClientName < inv.Clients[j].ClientName {
			return true
		} else if inv.Clients[i].ClientName > inv.Clients[j].ClientName {
			return false
		} else {
			return inv.Clients[i].NodeNo < inv.Clients[j].NodeNo
		}
	})

	sort.Slice(inv.FirewallRules, func(i, j int) bool {
		switch a.opts.Config.Backend.Type {
		case "gcp":
			return inv.FirewallRules[i].GCP.FirewallName < inv.FirewallRules[j].GCP.FirewallName
		case "aws":
			if inv.FirewallRules[i].AWS.VPC < inv.FirewallRules[j].AWS.VPC {
				return true
			} else if inv.FirewallRules[i].AWS.VPC > inv.FirewallRules[j].AWS.VPC {
				return false
			} else {
				return inv.FirewallRules[i].AWS.SecurityGroupName < inv.FirewallRules[j].AWS.SecurityGroupName
			}
		default:
			return inv.FirewallRules[i].Docker.NetworkName < inv.FirewallRules[j].Docker.NetworkName
		}
	})

	colorHiWhite := colorPrint{c: text.Colors{text.FgHiWhite}, enable: true}
	warnExp := colorPrint{c: text.Colors{text.BgHiYellow, text.FgBlack}, enable: true}
	errExp := colorPrint{c: text.Colors{text.BgHiRed, text.FgWhite}, enable: true}
	isColor := true
	if _, ok := os.LookupEnv("NO_COLOR"); ok || os.Getenv("CLICOLOR") == "0" {
		isColor = false
	}
	pipeLess := c.Pager
	isTerminal := false
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		isTerminal = true
	}

	t := table.NewWriter()
	if !isTerminal {
		pipeLess = false
		isColor = false
	}

	if !isColor {
		t.SetStyle(table.StyleDefault)
		colorHiWhite.enable = false
		warnExp.enable = false
		errExp.enable = false
		tstyle := t.Style()
		tstyle.Options.DrawBorder = false
		tstyle.Options.SeparateColumns = false
	} else {
		t.SetStyle(table.StyleColoredBlackOnCyanWhite)
	}

	if !pipeLess && isTerminal {
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width < 1 {
			fmt.Fprintf(os.Stderr, "Couldn't get terminal width (int:%v): %v", width, err)
		} else {
			if width < 40 {
				width = 40
			}
			t.SetAllowedRowLength(width)
		}
	}

	tstyle := t.Style()
	tstyle.Format.Header = text.FormatDefault
	tstyle.Format.Footer = text.FormatDefault

	lessCmd := ""
	lessParams := []string{}
	if pipeLess {
		lessCmd, lessParams = getPagerCommand()
	}
	if lessCmd != "" {
		origStdout := os.Stdout // store original
		origStderr := os.Stderr // store original
		defer func() {          // on exit, last thing we do, we recover stdout/stderr
			os.Stdout = origStdout
			os.Stderr = origStderr
		}()
		less := exec.Command(lessCmd, lessParams...)
		less.Stdout = origStdout // less will write
		less.Stderr = origStderr // less will write
		r, w, err := os.Pipe()   // writer writes, reader reads
		if err == nil {
			os.Stdout = w      // we will write to os.Pipe
			os.Stderr = w      // we will write to os.Pipe
			less.Stdin = r     // less will read from os.Pipe
			err = less.Start() // start less so it can do it's magic
			if err != nil {    // on pagination failure, revert to non-paginated output
				os.Stdout = origStdout
				os.Stderr = origStderr
				log.Printf("Pagination failed, %s returned: %s", lessCmd, err)
			} else {
				defer less.Wait() // after closing w, we should wait for less to finish before exiting
				defer w.Close()   // must close or less will wait for more input
			}
		}
		// close pipes on less exit to actually exit if less is terminated prior to reaching EOF
		go func() {
			less.Wait()
			w.Close()
			r.Close()
		}()
	}

	if showTemplates {
		t.SetTitle(colorHiWhite.Sprint("TEMPLATES"))
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		if c.AWSFull {
			t.AppendHeader(table.Row{"AerospikeVersion", "Arch", "Distribution", "OSVersion", "Region"})
		} else {
			t.AppendHeader(table.Row{"AerospikeVersion", "Arch", "Distribution", "OSVersion"})
		}
		for _, v := range inv.Templates {
			vv := table.Row{
				strings.ReplaceAll(v.AerospikeVersion, "-", "."),
				v.Arch,
				v.Distribution,
				strings.ReplaceAll(v.OSVersion, "-", "."),
			}
			if c.AWSFull {
				vv = append(vv, v.Region)
			}
			t.AppendRow(vv)
		}
		fmt.Println(t.Render())
		fmt.Println()
	}

	if showClusters {
		t.SetTitle(colorHiWhite.Sprint("CLUSTERS"))
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		if a.opts.Config.Backend.Type == "gcp" {
			t.AppendHeader(table.Row{"ClusterName", "NodeNo", "ExpiresIn", "State", "PublicIP", "PrivateIP", "Owner", "AsdVer", "RunningCost", "Firewalls", "Arch", "Distro", "DistroVer", "Zone", "InstanceID"})
		} else if a.opts.Config.Backend.Type == "aws" {
			t.AppendHeader(table.Row{"ClusterName", "NodeNo", "ExpiresIn", "State", "PublicIP", "PrivateIP", "Owner", "AsdVer", "RunningCost", "Firewalls", "Arch", "Distro", "DistroVer", "Region", "InstanceID"})
		} else {
			t.AppendHeader(table.Row{"ClusterName", "NodeNo", "State", "PublicIP", "PrivateIP", "ExposedPort", "Owner", "AsdVer", "Arch", "Distro", "DistroVer", "InstanceID", "ImageID"})
		}
		for _, v := range inv.Clusters {
			if v.Features > ClusterFeatureAerospike {
				continue
			}
			vv := table.Row{
				v.ClusterName,
				v.NodeNo,
			}
			if a.opts.Config.Backend.Type != "docker" {
				if v.Expires == "" {
					vv = append(vv, warnExp.Sprint("WARN: no expiry is set"))
				} else {
					expirationTime, err := time.Parse(time.RFC3339, v.Expires)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error parsing expiration time: %s\n", err)
						return err
					}
					currentTime := time.Now().In(expirationTime.Location())
					expiresIn := expirationTime.Sub(currentTime)
					if expiresIn < 6*time.Hour {
						vv = append(vv, errExp.Sprintf("%s", expiresIn.Round(time.Minute)))
					} else {
						vv = append(vv, expiresIn.Round(time.Minute))
					}
				}
			}
			vv = append(vv, v.State)
			vv = append(vv, v.PublicIp, v.PrivateIp)
			if a.opts.Config.Backend.Type == "docker" {
				vv = append(vv, v.DockerExposePorts)
			}
			vv = append(vv, v.Owner, strings.ReplaceAll(v.AerospikeVersion, "-", "."))
			if a.opts.Config.Backend.Type != "docker" {
				spot := ""
				if v.AwsIsSpot {
					spot = " (spot)"
				}
				vv = append(vv, strconv.FormatFloat(v.InstanceRunningCost, 'f', 4, 64)+spot)
				vv = append(vv, strings.Join(v.Firewalls, "\n"))
			}
			vv = append(vv, v.Arch, v.Distribution, strings.ReplaceAll(v.OSVersion, "-", "."))
			if a.opts.Config.Backend.Type != "docker" {
				vv = append(vv, v.Zone)
			}
			vv = append(vv, v.InstanceId)
			if a.opts.Config.Backend.Type == "docker" {
				vv = append(vv, v.ImageId)
			}
			t.AppendRow(vv)
		}
		fmt.Println(t.Render())
		if a.opts.Config.Backend.Type != "docker" {
			fmt.Fprint(os.Stderr, "* instance Running Cost displays only the cost of owning the instance in a running state for the duration it was running so far. It does not account for taxes, disk, network or transfer costs.\n\n")
		} else {
			fmt.Fprint(os.Stderr, "* to connect directly to the cluster (non-docker-desktop), execute 'aerolab cluster list' and connect to the node IP on the given exposed port (or configured aerospike services port - default 3000)\n")
			fmt.Fprint(os.Stderr, "* to connect to the cluster when using Docker Desktop, execute 'aerolab cluster list` and connect to IP 127.0.0.1:EXPOSED_PORT with a connect policy of `--services-alternate`\n\n")
		}
	}

	if showClients {
		t.SetTitle(colorHiWhite.Sprint("CLIENTS"))
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		if a.opts.Config.Backend.Type == "gcp" {
			t.AppendHeader(table.Row{"ClusterName", "NodeNo", "ExpiresIn", "State", "PublicIP", "PrivateIP", "ClientType", "AccessURL", "AccessPort", "Owner", "AsdVer", "RunningCost", "Firewalls", "Arch", "Distro", "DistroVer", "Zone", "InstanceID"})
		} else if a.opts.Config.Backend.Type == "aws" {
			t.AppendHeader(table.Row{"ClusterName", "NodeNo", "ExpiresIn", "State", "PublicIP", "PrivateIP", "ClientType", "AccessURL", "AccessPort", "Owner", "AsdVer", "RunningCost", "Firewalls", "Arch", "Distro", "DistroVer", "Region", "InstanceID"})
		} else {
			t.AppendHeader(table.Row{"ClusterName", "NodeNo", "State", "PublicIP", "PrivateIP", "ClientType", "AccessURL", "AccessPort", "Owner", "AsdVer", "Arch", "Distro", "DistroVer", "InstanceID", "ImageID"})
		}
		for _, v := range inv.Clients {
			vv := table.Row{
				v.ClientName,
				v.NodeNo,
			}
			if a.opts.Config.Backend.Type != "docker" {
				if v.Expires == "" {
					vv = append(vv, warnExp.Sprint("WARN: no expiry is set"))
				} else {
					expirationTime, err := time.Parse(time.RFC3339, v.Expires)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error parsing expiration time: %s\n", err)
						return err
					}
					currentTime := time.Now().In(expirationTime.Location())
					expiresIn := expirationTime.Sub(currentTime)
					if expiresIn < 6*time.Hour {
						vv = append(vv, errExp.Sprintf("%s", expiresIn.Round(time.Minute)))
					} else {
						vv = append(vv, expiresIn.Round(time.Minute))
					}
				}
			}
			vv = append(vv, v.State)
			vv = append(vv, v.PublicIp, v.PrivateIp, v.ClientType, v.AccessUrl, v.AccessPort)
			vv = append(vv, v.Owner, strings.ReplaceAll(v.AerospikeVersion, "-", "."))
			if a.opts.Config.Backend.Type != "docker" {
				spot := ""
				if v.AwsIsSpot {
					spot = " (spot)"
				}
				vv = append(vv, strconv.FormatFloat(v.InstanceRunningCost, 'f', 4, 64)+spot)
				vv = append(vv, strings.Join(v.Firewalls, "\n"))
			}
			vv = append(vv, v.Arch, v.Distribution, strings.ReplaceAll(v.OSVersion, "-", "."))
			if a.opts.Config.Backend.Type != "docker" {
				vv = append(vv, v.Zone)
			}
			vv = append(vv, v.InstanceId)
			if a.opts.Config.Backend.Type == "docker" {
				vv = append(vv, v.ImageId)
			}
			t.AppendRow(vv)
		}
		fmt.Println(t.Render())
		if a.opts.Config.Backend.Type == "docker" {
			fmt.Fprint(os.Stderr, "* if using Docker Desktop and forwaring ports by exposing them (-e ...), use IP 127.0.0.1 for the Access URL\n\n")
		} else {
			fmt.Fprint(os.Stderr, "* instance Running Cost displays only the cost of owning the instance in a running state for the duration it was running so far. It does not account for taxes, disk, network or transfer costs.\n\n")
		}
	}

	for _, showOther := range showOthers {
		if a.opts.Config.Backend.Type == "aws" && showOther&inventoryShowVolumes > 0 {
			t.SetTitle(colorHiWhite.Sprint("EFS"))
			t.ResetHeaders()
			t.ResetRows()
			t.ResetFooters()
			t.AppendHeader(table.Row{"Name", "VolumeAZ", "FsID", "Created", "Size", "ExpiresIn", "MountTargets", "MountTargetId", "MountTargetAZ", "Owner", "AGILabel"})
			for _, v := range inv.Volumes {
				expiry := ""
				if lastUsed, ok := v.Tags["lastUsed"]; ok {
					if expireDuration, ok := v.Tags["expireDuration"]; ok {
						lu, err := time.Parse(time.RFC3339, lastUsed)
						if err == nil {
							ed, err := time.ParseDuration(expireDuration)
							if err == nil {
								expiresTime := lu.Add(ed)
								expiresIn := expiresTime.Sub(time.Now().In(expiresTime.Location()))
								if expiresIn < 6*time.Hour {
									expiry = errExp.Sprintf("%s", expiresIn.Round(time.Minute))
								} else {
									expiry = expiresIn.Round(time.Minute).String()
								}
							}
						}
					}
				}
				for _, m := range v.MountTargets {
					vv := table.Row{
						v.Name,
						v.AvailabilityZoneName,
						v.FileSystemId,
						v.CreationTime.Format(time.RFC822),
						convSize(int64(v.SizeBytes)),
						expiry,
						strconv.Itoa(v.NumberOfMountTargets),
						m.MountTargetId,
						m.AvailabilityZoneId,
						v.Owner,
						v.Tags["agiLabel"],
					}
					t.AppendRow(vv)
				}
				if len(v.MountTargets) == 0 {
					vv := table.Row{
						v.Name,
						v.AvailabilityZoneName,
						v.FileSystemId,
						v.CreationTime.Format(time.RFC822),
						convSize(int64(v.SizeBytes)),
						expiry,
						strconv.Itoa(v.NumberOfMountTargets),
						"N/A",
						"N/A",
						v.Owner,
						v.Tags["agiLabel"],
					}
					t.AppendRow(vv)
				}
			}
			fmt.Println(t.Render())
			fmt.Println()
		}
		if showOther&inventoryShowAGI > 0 {
			t.SetTitle(colorHiWhite.Sprint("AGI"))
			t.ResetHeaders()
			t.ResetRows()
			t.ResetFooters()
			if showOther&inventoryShowAGIStatus > 0 {
				if a.opts.Config.Backend.Type == "gcp" {
					t.AppendHeader(table.Row{"Name", "State", "Status", "ExpiresIn", "Owner", "Access URL", "AGILabel", "RunningCost", "PublicIP", "PrivateIP", "Firewalls", "Zone", "InstanceID"})
				} else if a.opts.Config.Backend.Type == "aws" {
					t.AppendHeader(table.Row{"Name", "State", "Status", "ExpiresIn", "VolOwner", "Owner", "Access URL", "AGILabel", "VolSize", "VolExpires", "RunningCost", "PublicIP", "PrivateIP", "Firewalls", "Region", "VolID", "InstanceID"})
				} else {
					t.AppendHeader(table.Row{"Name", "State", "Status", "Owner", "Access URL", "AGILabel", "PublicIP", "PrivateIP", "InstanceID", "ImageID"})
				}
				statusWg := new(sync.WaitGroup)
				clusterStatuses := make(map[int]string)
				statusMutex := new(sync.Mutex)
				statusThreads := make(chan int, 5)
				for vi, v := range inv.Clusters {
					if v.Features&ClusterFeatureAGI <= 0 {
						continue
					}
					statusWg.Add(1)
					statusThreads <- 1
					go func(vi int, v inventoryCluster) {
						defer statusWg.Done()
						defer func() {
							<-statusThreads
						}()
						statusMsg := "unknown"
						if (v.PublicIp != "") || (a.opts.Config.Backend.Type == "docker" && v.PrivateIp != "") {
							out, err := b.RunCommands(v.ClusterName, [][]string{{"aerolab", "agi", "exec", "ingest-status"}}, []int{1})
							if err == nil {
								clusterStatus := &ingest.IngestStatusStruct{}
								err = json.Unmarshal(out[0], clusterStatus)
								if err == nil {
									if !clusterStatus.AerospikeRunning {
										statusMsg = errExp.Sprintf("ERR: ASD DOWN")
									} else if !clusterStatus.GrafanaHelperRunning {
										statusMsg = errExp.Sprintf("ERR: GRAFANAFIX DOWN")
									} else if !clusterStatus.PluginRunning {
										statusMsg = errExp.Sprintf("ERR: PLUGIN DOWN")
									} else if !clusterStatus.Ingest.CompleteSteps.Init {
										statusMsg = "(1/6) INIT"
									} else if !clusterStatus.Ingest.CompleteSteps.Download {
										statusMsg = fmt.Sprintf("(2/6) DOWNLOAD %d%%", clusterStatus.Ingest.DownloaderCompletePct)
									} else if !clusterStatus.Ingest.CompleteSteps.Unpack {
										statusMsg = "(3/6) UNPACK"
									} else if !clusterStatus.Ingest.CompleteSteps.PreProcess {
										statusMsg = "(4/6) PRE-PROCESS"
									} else if !clusterStatus.Ingest.CompleteSteps.ProcessLogs {
										statusMsg = fmt.Sprintf("(5/6) PROCESS %d%%", clusterStatus.Ingest.LogProcessorCompletePct)
									} else if !clusterStatus.Ingest.CompleteSteps.ProcessCollectInfo {
										statusMsg = "(6/6) COLLECTINFO"
									} else {
										statusMsg = "READY"
									}
									if statusMsg != "READY" && !clusterStatus.Ingest.Running {
										statusMsg = errExp.Sprintf("ERR: INGEST DOWN")
									}
								}
							}
						} else {
							statusMsg = ""
						}
						statusMutex.Lock()
						clusterStatuses[vi] = statusMsg
						statusMutex.Unlock()
					}(vi, v)
				}
				statusWg.Wait()
				foundVols := []int{}
				for vi, v := range inv.Clusters {
					if v.Features&ClusterFeatureAGI <= 0 {
						continue
					}
					efsOwner := ""
					fsId := ""
					fsSize := ""
					fsexpiry := ""
					if a.opts.Config.Backend.Type == "aws" {
						for voli, vol := range inv.Volumes {
							if vol.Name == v.ClusterName {
								foundVols = append(foundVols, voli)
								fsId = vol.FileSystemId
								fsSize = convSize(int64(vol.SizeBytes))
								efsOwner = vol.Owner
								if lastUsed, ok := vol.Tags["lastUsed"]; ok {
									if expireDuration, ok := vol.Tags["expireDuration"]; ok {
										lu, err := time.Parse(time.RFC3339, lastUsed)
										if err == nil {
											ed, err := time.ParseDuration(expireDuration)
											if err == nil {
												expiresTime := lu.Add(ed)
												expiresIn := expiresTime.Sub(time.Now().In(expiresTime.Location()))
												if expiresIn < 6*time.Hour {
													fsexpiry = errExp.Sprintf("%s", expiresIn.Round(time.Minute))
												} else {
													fsexpiry = expiresIn.Round(time.Minute).String()
												}
											}
										}
									}
								}
								break
							}
						}
					}

					vv := table.Row{v.ClusterName, v.State, clusterStatuses[vi]}
					if a.opts.Config.Backend.Type != "docker" {
						if v.Expires == "" {
							vv = append(vv, warnExp.Sprint("WARN: no expiry is set"))
						} else {
							// Parse the expiration time string
							expirationTime, err := time.Parse(time.RFC3339, v.Expires)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error parsing expiration time: %s\n", err)
								return err
							}
							// Get the current time in the same timezone as the expiration time
							currentTime := time.Now().In(expirationTime.Location())

							// Calculate the duration between the current time and the expiration time
							expiresIn := expirationTime.Sub(currentTime)

							if expiresIn < 6*time.Hour {
								vv = append(vv, errExp.Sprintf("%s", expiresIn.Round(time.Minute)))
							} else {
								vv = append(vv, expiresIn.Round(time.Minute))
							}
						}
					}
					if a.opts.Config.Backend.Type == "aws" {
						vv = append(vv, efsOwner)
					}
					accessUrl := ""
					if (v.PublicIp != "") || (a.opts.Config.Backend.Type == "docker" && v.PrivateIp != "") {
						accessUrl = v.AccessUrl
					}
					vv = append(vv, v.Owner, accessUrl, v.AGILabel)
					/*
						if a.opts.Config.Backend.Type == "aws" {
							vv = append(vv, v.awsTags["agiLabel"])
						} else if a.opts.Config.Backend.Type == "gcp" {
							vv = append(vv, v.gcpMeta["agiLabel"])
						} else {
							vv = append(vv, v.dockerLabels["agiLabel"])
						}
					*/
					if a.opts.Config.Backend.Type == "aws" {
						vv = append(vv, fsSize, fsexpiry)
					}
					if a.opts.Config.Backend.Type != "docker" {
						spot := ""
						if v.AwsIsSpot {
							spot = " (spot)"
						}
						vv = append(vv, strconv.FormatFloat(v.InstanceRunningCost, 'f', 4, 64)+spot)
					}
					vv = append(vv, v.PublicIp, v.PrivateIp)
					if a.opts.Config.Backend.Type != "docker" {
						vv = append(vv, strings.Join(v.Firewalls, "\n"), v.Zone)
					}
					if a.opts.Config.Backend.Type == "aws" {
						vv = append(vv, fsId)
					}
					vv = append(vv, v.InstanceId)
					if a.opts.Config.Backend.Type == "docker" {
						vv = append(vv, v.ImageId)
					}
					t.AppendRow(vv)
				}
				for voli, vol := range inv.Volumes {
					if inslice.HasInt(foundVols, voli) {
						continue
					}
					if _, ok := vol.Tags["agiLabel"]; !ok {
						continue
					}
					expiry := ""
					if lastUsed, ok := vol.Tags["lastUsed"]; ok {
						if expireDuration, ok := vol.Tags["expireDuration"]; ok {
							lu, err := time.Parse(time.RFC3339, lastUsed)
							if err == nil {
								ed, err := time.ParseDuration(expireDuration)
								if err == nil {
									expiresTime := lu.Add(ed)
									expiresIn := expiresTime.Sub(time.Now().In(expiresTime.Location()))
									if expiresIn < 6*time.Hour {
										expiry = errExp.Sprintf("%s", expiresIn.Round(time.Minute))
									} else {
										expiry = expiresIn.Round(time.Minute).String()
									}
								}
							}
						}
					}
					vv := table.Row{vol.Name, "", "", "", vol.Owner, "", "", vol.Tags["agiLabel"], convSize(int64(vol.SizeBytes)), expiry, "", "", "", "", vol.AvailabilityZoneName, vol.FileSystemId, ""}
					t.AppendRow(vv)
				}
			} else {
				if a.opts.Config.Backend.Type == "gcp" {
					t.AppendHeader(table.Row{"Name", "State", "ExpiresIn", "Owner", "Access URL", "AGILabel", "RunningCost", "PublicIP", "PrivateIP", "Firewalls", "Zone", "InstanceID"})
				} else if a.opts.Config.Backend.Type == "aws" {
					t.AppendHeader(table.Row{"Name", "State", "ExpiresIn", "Owner", "Access URL", "AGILabel", "RunningCost", "PublicIP", "PrivateIP", "Firewalls", "Region", "InstanceID"})
				} else {
					t.AppendHeader(table.Row{"Name", "State", "Owner", "Access URL", "AGILabel", "PublicIP", "PrivateIP", "InstanceID", "ImageID"})
				}
				for _, v := range inv.Clusters {
					if v.Features&ClusterFeatureAGI <= 0 {
						continue
					}
					vv := table.Row{v.ClusterName, v.State}
					if a.opts.Config.Backend.Type != "docker" {
						if v.Expires == "" {
							vv = append(vv, warnExp.Sprint("WARN: no expiry is set"))
						} else {
							// Parse the expiration time string
							expirationTime, err := time.Parse(time.RFC3339, v.Expires)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Error parsing expiration time: %s\n", err)
								return err
							}
							// Get the current time in the same timezone as the expiration time
							currentTime := time.Now().In(expirationTime.Location())

							// Calculate the duration between the current time and the expiration time
							expiresIn := expirationTime.Sub(currentTime)

							if expiresIn < 6*time.Hour {
								vv = append(vv, errExp.Sprintf("%s", expiresIn.Round(time.Minute)))
							} else {
								vv = append(vv, expiresIn.Round(time.Minute))
							}
						}
					}
					vv = append(vv, v.Owner, v.AccessUrl, v.AGILabel)
					/*
						if a.opts.Config.Backend.Type == "aws" {
							vv = append(vv, v.awsTags["agiLabel"])
						} else if a.opts.Config.Backend.Type == "gcp" {
							vv = append(vv, v.gcpMeta["agiLabel"])
						} else {
							vv = append(vv, v.dockerLabels["agiLabel"])
						}
					*/
					if a.opts.Config.Backend.Type != "docker" {
						spot := ""
						if v.AwsIsSpot {
							spot = " (spot)"
						}
						vv = append(vv, strconv.FormatFloat(v.InstanceRunningCost, 'f', 4, 64)+spot)
					}
					vv = append(vv, v.PublicIp, v.PrivateIp)
					if a.opts.Config.Backend.Type != "docker" {
						vv = append(vv, strings.Join(v.Firewalls, "\n"), v.Zone)
					}
					vv = append(vv, v.InstanceId)
					if a.opts.Config.Backend.Type == "docker" {
						vv = append(vv, v.ImageId)
					}
					t.AppendRow(vv)
				}
			}
			fmt.Println(t.Render())
			if a.opts.Config.Backend.Type != "docker" {
				fmt.Fprint(os.Stderr, "* instance Running Cost displays only the cost of owning the instance in a running state for the duration it was running so far. It does not account for taxes, disk, network or transfer costs.\n\n")
			} else {
				fmt.Fprint(os.Stderr, "* to connect directly to the cluster (non-docker-desktop), execute 'aerolab cluster list' and connect to the node IP on the given exposed port (or configured aerospike services port - default 3000)\n")
				fmt.Fprint(os.Stderr, "* to connect to the cluster when using Docker Desktop, execute 'aerolab cluster list` and connect to IP 127.0.0.1:EXPOSED_PORT with a connect policy of `--services-alternate`\n\n")
			}
		}
	}

	if showFirewalls {
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		switch a.opts.Config.Backend.Type {
		case "gcp":
			t.SetTitle(colorHiWhite.Sprint("FIREWALL RULES"))
			t.AppendHeader(table.Row{"FirewallName", "TargetTags", "SourceTags", "SourceRanges", "AllowPorts", "DenyPorts"})
			for _, v := range inv.FirewallRules {
				vv := table.Row{
					v.GCP.FirewallName,
					strings.Join(v.GCP.TargetTags, "\n"),
					strings.Join(v.GCP.SourceTags, "\n"),
					strings.Join(v.GCP.SourceRanges, "\n"),
					strings.Join(v.GCP.AllowPorts, "\n"),
					strings.Join(v.GCP.DenyPorts, "\n"),
				}
				t.AppendRow(vv)
			}
		case "aws":
			t.SetTitle(colorHiWhite.Sprint("SECURITY GROUPS"))
			if c.AWSFull {
				t.AppendHeader(table.Row{"VPC", "SecurityGroupName", "SecurityGroupID", "IPs", "Region"})
			} else {
				t.AppendHeader(table.Row{"VPC", "SecurityGroupName", "SecurityGroupID", "IPs"})
			}
			for _, v := range inv.FirewallRules {
				vv := table.Row{
					v.AWS.VPC,
					v.AWS.SecurityGroupName,
					v.AWS.SecurityGroupID,
					strings.Join(v.AWS.IPs, "\n"),
				}
				if c.AWSFull {
					vv = append(vv, v.AWS.Region)
				}
				t.AppendRow(vv)
			}
		case "docker":
			t.SetTitle(colorHiWhite.Sprint("NETWORKS"))
			t.AppendHeader(table.Row{"NetworkName", "NetworkDriver", "Subnets", "MTU"})
			for _, v := range inv.FirewallRules {
				vv := table.Row{
					v.Docker.NetworkName,
					v.Docker.NetworkDriver,
					v.Docker.Subnets,
					v.Docker.MTU,
				}
				t.AppendRow(vv)
			}

		}

		fmt.Println(t.Render())
		fmt.Println()
	}

	if showSubnets {
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		switch a.opts.Config.Backend.Type {
		case "aws":
			t.SetTitle(colorHiWhite.Sprint("SUBNETS"))
			t.AppendHeader(table.Row{"VpcID", "VpcName", "VpcCidr", "Avail.Zone", "SubnetID", "SubnetCidr", "AZDefault", "SubnetName", "Auto-AssignIP"})
			for _, v := range inv.Subnets {
				autoIP := "no (enable to use with aerolab)"
				if v.AWS.AutoPublicIP {
					autoIP = "yes (ok)"
				}
				vv := table.Row{
					v.AWS.VpcId,
					v.AWS.VpcName,
					v.AWS.VpcCidr,
					v.AWS.AvailabilityZone,
					v.AWS.SubnetId,
					v.AWS.SubnetCidr,
					fmt.Sprintf("%t", v.AWS.IsAzDefault),
					v.AWS.SubnetName,
					autoIP,
				}
				t.AppendRow(vv)
			}
			fmt.Println(t.Render())
			fmt.Println()
		}
	}

	for _, showOther := range showOthers {
		if showOther&inventoryShowExpirySystem > 0 {
			t.ResetHeaders()
			t.ResetRows()
			t.ResetFooters()
			t.AppendHeader(table.Row{"#", "Subsystem", "Details"})
			switch a.opts.Config.Backend.Type {
			case "aws":
				t.SetTitle(colorHiWhite.Sprint("EXPIRY_SYSTEM"))
				for i, v := range inv.ExpirySystem {
					t.AppendRow(table.Row{i, "IAM Function Rule", v.IAMFunction})
					t.AppendRow(table.Row{i, "IAM Scheduler Rule", v.IAMScheduler})
					t.AppendRow(table.Row{i, "Function", v.Function})
					t.AppendRow(table.Row{i, "Scheduler", v.Scheduler})
					t.AppendRow(table.Row{i, "Schedule", v.Schedule})
				}
				fmt.Println(t.Render())
				fmt.Println()
			case "gcp":
				t.SetTitle(colorHiWhite.Sprint("EXPIRY_SYSTEM"))
				for i, v := range inv.ExpirySystem {
					t.AppendRow(table.Row{i, "Function", v.Function})
					t.AppendRow(table.Row{i, "Source Bucket", v.SourceBucket})
					t.AppendRow(table.Row{i, "Scheduler", v.Scheduler})
					t.AppendRow(table.Row{i, "Schedule", v.Schedule})
				}
				fmt.Println(t.Render())
				fmt.Println()
			}
		}
	}

	return nil
}

type colorPrint struct {
	c      text.Colors
	enable bool
}

func (c *colorPrint) Sprint(a ...interface{}) string {
	if c.enable {
		return c.c.Sprint(a...)
	}
	return fmt.Sprint(a...)
}

func (c *colorPrint) Sprintf(format string, a ...interface{}) string {
	if c.enable {
		return c.c.Sprintf(format, a...)
	}
	return fmt.Sprintf(format, a...)
}
