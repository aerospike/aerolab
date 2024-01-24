package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type commands struct {
	Config       configCmd       `command:"config" subcommands-optional:"true" description:"Show or change aerolab configuration" webicon:"fas fa-toolbox"`
	Cluster      clusterCmd      `command:"cluster" subcommands-optional:"true" description:"Create and manage Aerospike clusters and nodes" webicon:"fas fa-database"`
	Aerospike    aerospikeCmd    `command:"aerospike" subcommands-optional:"true" description:"Aerospike daemon controls" webicon:"fas fa-a"`
	Client       clientCmd       `command:"client" subcommands-optional:"true" description:"Create and manage Client machine groups" webicon:"fas fa-tv"`
	Inventory    inventoryCmd    `command:"inventory" subcommands-optional:"true" description:"List or operate on all clusters, clients and templates" webicon:"fas fa-warehouse"`
	Attach       attachCmd       `command:"attach" subcommands-optional:"true" description:"Attach to a node and run a command" webicon:"fas fa-plug"`
	Net          netCmd          `command:"net" subcommands-optional:"true" description:"Firewall and latency simulation" webicon:"fas fa-network-wired"`
	Conf         confCmd         `command:"conf" subcommands-optional:"true" description:"Manage Aerospike configuration on running nodes" webicon:"fas fa-wrench"`
	Tls          tlsCmd          `command:"tls" subcommands-optional:"true" description:"Create or copy TLS certificates" webicon:"fas fa-lock"`
	Data         dataCmd         `command:"data" subcommands-optional:"true" description:"Insert/delete Aerospike data" webicon:"fas fa-folder-open"`
	Template     templateCmd     `command:"template" subcommands-optional:"true" description:"Manage or delete template images" webicon:"fas fa-file-image"`
	Installer    installerCmd    `command:"installer" subcommands-optional:"true" description:"List or download Aerospike installer versions" webicon:"fas fa-plus"`
	Logs         logsCmd         `command:"logs" subcommands-optional:"true" description:"show or download logs" webicon:"fas fa-bars-progress"`
	Files        filesCmd        `command:"files" subcommands-optional:"true" description:"Upload/Download files to/from clients or clusters" webicon:"fas fa-file"`
	XDR          xdrCmd          `command:"xdr" subcommands-optional:"true" description:"Mange clusters' xdr configuration" webicon:"fas fa-object-group"`
	Roster       rosterCmd       `command:"roster" subcommands-optional:"true" description:"Show or apply strong-consistency rosters" webicon:"fas fa-sliders"`
	Completion   completionCmd   `command:"completion" subcommands-optional:"true" description:"Install shell completion scripts" webicon:"fas fa-arrows-turn-to-dots"`
	AGI          agiCmd          `command:"agi" subcommands-optional:"true" description:"Launch or manage AGI troubleshooting instances" webicon:"fas fa-chart-line"`
	Volume       volumeCmd       `command:"volume" subcommands-optional:"true" description:"Volume management (AWS EFS/GCP Volume only)" webicon:"fas fa-hard-drive"`
	ShowCommands showcommandsCmd `command:"showcommands" subcommands-optional:"true" description:"Install showsysinfo,showconf,showinterrupts on the current system" webicon:"fas fa-terminal"`
	Rest         restCmd         `command:"rest-api" subcommands-optional:"true" description:"Launch HTTP rest API" webicon:"fas fa-globe"`
	Web          webCmd          `command:"webui" subcommands-optional:"true" description:"Launch AeroLab Web UI" webicon:"fas fa-globe"`
	Version      versionCmd      `command:"version" subcommands-optional:"true" description:"Print AeroLab version" webicon:"fas fa-code-branch"`
	Upgrade      upgradeCmd      `command:"upgrade" subcommands-optional:"true" description:"Upgrade AeroLab binary" webicon:"fas fa-circle-up"`
	WebRun       webRunCmd       `command:"webrun" subcommands-optional:"true" description:"Upgrade AeroLab binary" hidden:"true"`
	commandsDefaults
}

type showcommandsCmd struct {
	DestDir string  `short:"d" long:"destination" default:"/usr/local/bin/"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *showcommandsCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	cur, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get absolute path os self: %s", err)
	}
	log.Printf("Discovered absolute path: %s", cur)
	for _, dest := range []string{"showconf", "showsysinfo", "showinterrupts"} {
		d := filepath.Join(c.DestDir, dest)
		log.Printf("> ln -s %s %s", cur, d)
		if _, err := os.Stat(d); err == nil {
			os.Remove(d)
		}
		err = os.Symlink(cur, d)
		if err != nil {
			log.Printf("ERROR symlinking %s->%s : %s", cur, d, err)
		}
	}
	log.Println("Done")
	return nil
}

type upgradeCmd struct {
	Edge   bool    `long:"edge" description:"Include pre-releases when discovering latest version"`
	DryRun bool    `long:"dryrun" description:"Set to show the upgrade source URL and destination path, do not upgrade"`
	Help   helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *upgradeCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	log.Println("Checking latest version...")
	v := &GitHubRelease{}
	var pre *GitHubRelease
	err := a.isLatestVersionQuery(v, "")
	if err != nil {
		return err
	}
	latestVersion := ""
	if c.Edge {
		pre, err = a.isLatestVersionQueryPrerelease()
		if err != nil {
			if err.Error() == "NOT FOUND" {
				c.Edge = false
			} else {
				return err
			}
		}
	}
	if !c.Edge {
		ved := strings.Trim(vEdition, "\r\n\t ")
		if len(v.CommitHash) > 8 {
			v.CommitHash = v.CommitHash[0:7]
		}
		if ved == "-unofficial" || ved == "-prerelease" || VersionCheck(v.CurrentVersion, v.LatestVersion) > 0 {
			latestVersion = "v" + v.LatestVersion + "-" + v.CommitHash + "-stable"
		} else {
			log.Println("Already on latest stable")
			return nil
		}
	} else {
		if pre.LatestVersion+"-prerelease" != pre.CurrentVersion+"-"+strings.Trim(vCommit, "\r\t\n ")+strings.Trim(vEdition, "\r\t\n ") {
			latestVersion = "v" + pre.LatestVersion + "-prerelease"
			v = pre
		} else {
			log.Println("Already on latest edge")
			return nil
		}
	}
	cur, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get absolute path os self: %s", err)
	}
	for {
		if st, err := os.Stat(cur); err != nil {
			return fmt.Errorf("failed to stat self: %s", err)
		} else {
			if st.Mode()&os.ModeSymlink > 0 {
				cur, err = filepath.EvalSymlinks(cur)
				if err != nil {
					return fmt.Errorf("error resolving symlink source to self: %s", err)
				}
			} else {
				break
			}
		}
	}
	from := ""
	fn := "aerolab-"
	switch runtime.GOOS {
	case "windows", "linux":
		fn = fn + runtime.GOOS + "-"
	case "darwin", "macos":
		fn = fn + "macos-"
	default:
		return fmt.Errorf("operating system %s not supported", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "x86_64", "amd64":
		fn = fn + "amd64-"
	case "arm64", "aarch64":
		fn = fn + "arm64-"
	default:
		return fmt.Errorf("cpu architecture %s not supported", runtime.GOARCH)
	}
	fn = fn + strings.Split(v.LatestVersion, "-")[0] + ".zip"
	for _, pre := range v.Assets {
		if pre.ContentType != "application/zip" {
			continue
		}
		if pre.FileName != fn {
			continue
		}
		from = pre.DownloadUrl
		break
	}
	if from == "" {
		return fmt.Errorf("asset (%s) not found in releases page", fn)
	}
	log.Printf("Upgrading %s => %s", version, latestVersion)
	if c.DryRun {
		log.Printf("DryRun: %s => %s", from, cur)
		return nil
	}
	dest, err := os.OpenFile(cur+"-upgrade", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary file '%s': %s", cur+"-upgrade", err)
	}
	defer dest.Close()
	client := &http.Client{}
	client.Timeout = 10 * time.Minute
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", from, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		err = fmt.Errorf("GET '%s': exit code (%d), message: %s", from, response.StatusCode, string(body))
		return err
	}
	buf, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %s", err)
	}
	zipc, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return fmt.Errorf("failed to open body as zip file: %s", err)
	}
	f, err := zipc.Open("aerolab")
	if err != nil {
		return fmt.Errorf("failed to open file 'aerolab' inside zip: %s", err)
	}
	defer f.Close()
	_, err = io.Copy(dest, f)
	if err != nil {
		return fmt.Errorf("failed to unzip file: %s", err)
	}
	err = dest.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync temp file to storage: %s", err)
	}
	err = os.Rename(cur+"-upgrade", cur)
	if err != nil {
		return fmt.Errorf("failed to rename temp file '%s' to final destination '%s': %s", cur+"-upgrade", cur, err)
	}
	log.Println("Done")
	return nil
}
