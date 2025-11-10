package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/github"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/aerospike/aerolab/pkg/utils/versions"
	"github.com/rglonek/logger"
)

type UpgradeCmd struct {
	Edge    bool    `long:"edge" description:"Include pre-releases when discovering versions"`
	Major   bool    `long:"major" description:"Upgrade to the next major version prerelease if available (v8); WARN: this may break things"`
	DryRun  bool    `long:"dry-run" description:"Set to show the upgrade source URL and destination path, do not upgrade"`
	Force   bool    `long:"force" description:"Force upgrade, even if the available version is the same as, or older than, the currently installed one"`
	Version string  `short:"v" long:"version" description:"Version to upgrade to" hidden:"true"`
	Help    HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *UpgradeCmd) Execute(args []string) error {
	system, err := Initialize(&Init{InitBackend: false}, []string{"upgrade"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"upgrade"}, c, args)
	}
	system.Logger.Info("Running upgrade")
	err = c.UpgradeAerolab(system.Logger)
	if err != nil {
		return Error(err, system, []string{"upgrade"}, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, []string{"upgrade"}, c, args)
}

func (c *UpgradeCmd) CheckForUpgrade() (install bool, latestVersionString string, latest *github.Release, err error) {
	vBranch, vCommit, vEdition, _ := GetAerolabVersion()

	// If Edge mode, we need to handle Major flag filtering
	if c.Edge {
		// Get all releases to filter prereleases
		releases, err := github.GetReleases(30*time.Second, "aerospike", "aerolab")
		if err != nil {
			return false, "", nil, err
		}

		// Filter to only prereleases
		prereleases := releases.WithPrerelease(true)

		// If Major flag is not set, filter out prereleases from different major versions
		if !c.Major {
			currentMajor, err := strconv.Atoi(strings.Split(vBranch, ".")[0])
			if err != nil {
				return false, "", nil, err
			}

			// Filter prereleases to only those matching current major version
			filtered := make(github.Releases, 0)
			for _, pre := range prereleases {
				preVersion := strings.TrimPrefix(pre.TagName, "v")
				preMajor, err := strconv.Atoi(strings.Split(preVersion, ".")[0])
				if err != nil {
					continue
				}
				if preMajor == currentMajor {
					filtered = append(filtered, pre)
				}
			}
			prereleases = filtered
		}

		if len(prereleases) == 0 {
			// No matching prereleases found, fall back to stable
			latest, err = aerolab.GetLatestVersion(true)
			if err != nil {
				return false, "", nil, err
			}
		} else {
			// Get the latest matching prerelease
			latest = prereleases.Latest()
			if latest == nil {
				return false, "", nil, fmt.Errorf("no prerelease found")
			}
		}
	} else {
		// Stable mode - get latest stable release
		latest, err = aerolab.GetLatestVersion(true)
		if err != nil {
			return false, "", nil, err
		}
	}

	latestVersion := strings.TrimPrefix(latest.TagName, "v")

	// Extract base version without commit hash for comparison
	// For tags like "8.0.0-4816232", extract just "8.0.0"
	latestVersionBase := latestVersion
	if idx := strings.Index(latestVersion, "-"); idx > 0 {
		// Check if what follows the dash looks like a commit hash (7+ hex digits)
		suffix := latestVersion[idx+1:]
		isCommitHash := len(suffix) >= 7
		if isCommitHash {
			for _, r := range suffix {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
					isCommitHash = false
					break
				}
			}
		}
		if isCommitHash {
			latestVersionBase = latestVersion[:idx]
		}
	}

	latestCommit := latest.TargetCommitish[0:7]
	latestVersionString = "v" + latestVersion + "-" + latestCommit
	if latest.Prerelease {
		latestVersionString = "v" + latestVersion + "-prerelease"
	}

	// Determine if we need to install the latest version
	install = false
	if c.Edge {
		if c.Force || vEdition == "-unofficial" || versions.Compare(latestVersionBase, vBranch) > 0 || (versions.Compare(latestVersionBase, vBranch) == 0 && latestCommit != vCommit) {
			install = true
		}
	} else {
		if c.Force || vEdition == "-unofficial" || vEdition == "-prerelease" || versions.Compare(latestVersionBase, vBranch) > 0 {
			install = true
		}
	}
	return
}

func (c *UpgradeCmd) UpgradeAerolab(log *logger.Logger) error {
	_, _, _, versionString := GetAerolabVersion()
	// Get the latest version
	log.Info("Checking latest version...")

	var install bool
	var latestVersionString string
	var latest *github.Release
	var err error
	if c.Version == "" {
		install, latestVersionString, latest, err = c.CheckForUpgrade()
		if err != nil {
			return err
		}
	} else {
		latestVersionString = c.Version
		latest, err = aerolab.GetRelease(c.Version)
		if err != nil {
			return err
		}
		if latest == nil {
			return fmt.Errorf("version %s not found", c.Version)
		}
		install = true
	}

	// If we don't need to install the latest version, exit
	if !install {
		log.Info("Already on latest version")
		return nil
	}

	// Print the upgrade message
	log.Info("Upgrading %s => %s", versionString, latestVersionString)

	// Get the absolute path of the current executable
	cur, err := GetSelfPath()
	if err != nil {
		return err
	}

	// build the filename
	fn := "aerolab-"
	inzip := "aerolab"
	switch runtime.GOOS {
	case "windows":
		inzip = "aerolab.exe"
		fn = fn + runtime.GOOS + "-"
	case "linux":
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

	// get the asset with the correct prefix
	assets := latest.Assets.WithNamePrefix(fn)
	if assets == nil {
		return fmt.Errorf("asset with prefix (%s) not found in releases page", fn)
	}

	// get the asset with the correct suffix
	assets = assets.WithNameSuffix(".zip")
	if assets == nil {
		return fmt.Errorf("asset with prefix (%s) and suffix (.zip) not found in releases page", fn)
	}

	// get the asset
	asset := assets.First()

	log.Debug("Downloading URL=%s Size=%d CreatedAt=%s", asset.BrowserDownloadURL, asset.Size, asset.CreatedAt)

	// if dry run, exit
	if c.DryRun {
		log.Info("DryRun: exiting")
		return nil
	}

	// create the temporary file
	dest, err := os.OpenFile(cur+"-upgrade", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary file '%s': %s", cur+"-upgrade", err)
	}
	defer dest.Close()

	// get the asset
	client := &http.Client{}
	client.Timeout = 10 * time.Minute
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", asset.BrowserDownloadURL, nil)
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
		err = fmt.Errorf("GET '%s': exit code (%d), message: %s", asset.BrowserDownloadURL, response.StatusCode, string(body))
		return err
	}
	buf, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %s", err)
	}

	// unzip the file
	zipc, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return fmt.Errorf("failed to open body as zip file: %s", err)
	}
	f, err := zipc.Open(inzip)
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

	// cause windows is weird: rename the current executable
	if runtime.GOOS == "windows" {
		dest.Close()
		os.Remove(cur + "-old")
		err = os.Rename(cur, cur+"-old")
		if err != nil {
			return fmt.Errorf("failed to rename current executable '%s' to destination '%s': %s", cur, cur+"-old", err)
		}
	}

	// replace the current executable with the new one
	err = os.Rename(cur+"-upgrade", cur)
	if err != nil {
		return fmt.Errorf("failed to rename temp file '%s' to final destination '%s': %s", cur+"-upgrade", cur, err)
	}

	return nil
}
