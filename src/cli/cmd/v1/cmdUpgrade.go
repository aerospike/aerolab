package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/github"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/aerospike/aerolab/pkg/utils/versions"
	"github.com/rglonek/logger"
)

type UpgradeCmd struct {
	Edge   bool    `long:"edge" description:"Include pre-releases when discovering versions"`
	DryRun bool    `long:"dry-run" description:"Set to show the upgrade source URL and destination path, do not upgrade"`
	Force  bool    `long:"force" description:"Force upgrade, even if the available version is the same as, or older than, the currently installed one"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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
	latest, err = aerolab.GetLatestVersion(!c.Edge)
	if err != nil {
		return false, "", nil, err
	}
	latestVersion := strings.TrimPrefix(latest.TagName, "v")
	latestCommit := latest.TargetCommitish[0:8]
	latestVersionString = "v" + latestVersion + "-" + latestCommit
	if latest.Prerelease {
		latestVersionString = "v" + latestVersion + "-prerelease"
	}

	// Determine if we need to install the latest version
	install = false
	if c.Edge {
		if c.Force || vEdition == "-unofficial" || versions.Compare(latestVersion, vBranch) > 0 || (versions.Compare(latestVersion, vBranch) == 0 && latestCommit != vCommit) {
			install = true
		}
	} else {
		if c.Force || vEdition == "-unofficial" || vEdition == "-prerelease" || versions.Compare(latestVersion, vBranch) > 0 {
			install = true
		}
	}
	return
}

func (c *UpgradeCmd) UpgradeAerolab(log *logger.Logger) error {
	_, _, _, versionString := GetAerolabVersion()
	// Get the latest version
	log.Info("Checking latest version...")

	install, latestVersionString, latest, err := c.CheckForUpgrade()
	if err != nil {
		return err
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
