package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike/jfrog"
	"github.com/rglonek/go-wget"
	"github.com/rglonek/logger"
)

type InstallerDownloadCmd struct {
	aerospikeVersionSelectorCmd
	IsArm  bool    `long:"arm" description:"indicate installing on an arm instance"`
	DryRun bool    `long:"dry-run" description:"do not download the installer, just print the URL"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstallerDownloadCmd) Execute(args []string) error {
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, []string{"installer", "download"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"installer", "download"}, c, args)
	}
	system.Logger.Info("Running installer.download")

	err = c.FindAndDownloadAerospikeServerInstaller(system.Logger)
	if err != nil {
		return Error(err, system, []string{"installer", "download"}, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, []string{"installer", "download"}, c, args)
}

func (c *InstallerDownloadCmd) FindAndDownloadAerospikeServerInstaller(log *logger.Logger) (err error) {
	if cfg := jfrog.FromEnv(); cfg != nil {
		return c.findAndDownloadJFrog(log, cfg)
	}

	fileName, installURL, err := c.FindAerospikeInstaller(log)
	if err != nil {
		return err
	}

	arch := aerospike.ArchitectureTypeX86_64
	if c.IsArm {
		arch = aerospike.ArchitectureTypeAARCH64
	}
	log.Info("Distro = %s:%s ; AerospikeVersion = %s ; arch = %s", c.DistroName, c.DistroVersion, c.AerospikeVersion, arch)
	log.Info("Downloading from %s to %s", installURL, fileName)

	err = c.DownloadAerospikeInstaller(log, ".", fileName, installURL, func(p *wget.Progress) {
		log.Info("%d%% complete @ %s / second (%s elapsed)", p.PctComplete, wget.SizeToString(p.BytesPerSecond), p.TimeElapsed.Round(time.Second))
	})
	if err != nil {
		return err
	}
	return nil
}

// findAndDownloadJFrog resolves the requested build via JFrog AQL,
// matches one artifact against the requested OS/arch/edition and
// streams it into the current working directory. Unlike the
// public-flow download, this writes a .deb or .rpm rather than a tgz.
func (c *InstallerDownloadCmd) findAndDownloadJFrog(log *logger.Logger, cfg *jfrog.Config) error {
	if c.DistroVersion.String() == "latest" {
		return fmt.Errorf("JFrog mode requires --distro-version to be explicit (not 'latest')")
	}
	edition, cleanVer := jfrog.EditionFromInput(c.AerospikeVersion.String(), "enterprise")
	build, err := cfg.ResolveBuild(cleanVer)
	if err != nil {
		return err
	}
	log.Info("Querying JFrog build %q number %q", build.Name, build.Number)
	files, err := build.Files(context.Background())
	if err != nil {
		return err
	}

	arch := "x86_64"
	if c.IsArm {
		arch = "aarch64"
	}
	osName := c.DistroName.String()
	if osName == "rocky" {
		osName = "centos"
	}
	match, err := files.Match(jfrog.MatchCriteria{
		Edition:   edition,
		OSName:    osName,
		OSVersion: c.DistroVersion.String(),
		Arch:      arch,
	})
	if err != nil {
		return err
	}
	log.Info("Selected %s/%s/%s (%d bytes)", match.Repo, match.Path, match.Name, match.Size)
	if c.DryRun {
		log.Info("Dry run: download URL is %s", match.DownloadURL)
		return nil
	}
	if err := os.MkdirAll(".", 0o755); err != nil {
		return err
	}
	dest := filepath.Join(".", match.Name)
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("could not create %s: %w", tmp, err)
	}
	written, getErr := cfg.Get(context.Background(), match.DownloadURL, out)
	closeErr := out.Close()
	if getErr != nil {
		_ = os.Remove(tmp)
		return getErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if match.Size > 0 && written != match.Size {
		_ = os.Remove(tmp)
		return fmt.Errorf("download size mismatch: got %d, want %d", written, match.Size)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	log.Info("Downloaded %s (%d bytes)", dest, written)
	return nil
}

func (c *InstallerDownloadCmd) FindAerospikeInstaller(log *logger.Logger) (fileName string, installURL string, err error) {
	edition := "enterprise"
	if strings.HasSuffix(c.AerospikeVersion.String(), "c") {
		edition = "community"
		c.AerospikeVersion = TypeAerospikeVersion(strings.TrimSuffix(c.AerospikeVersion.String(), "c"))
	} else if strings.HasSuffix(c.AerospikeVersion.String(), "f") {
		edition = "federal"
		c.AerospikeVersion = TypeAerospikeVersion(strings.TrimSuffix(c.AerospikeVersion.String(), "f"))
	}

	products, err := aerospike.GetProducts(time.Second * 10)
	if err != nil {
		return "", "", fmt.Errorf("could not get products: %s", err)
	}

	product := products.WithName("aerospike-server-" + edition)
	if product == nil {
		return "", "", fmt.Errorf("product not found")
	}
	versions, err := aerospike.GetVersions(time.Second*10, product[0])
	if err != nil {
		return "", "", fmt.Errorf("could not get versions: %s", err)
	}

	if c.AerospikeVersion.String() != "latest" {
		versions = versions.WithName(c.AerospikeVersion.String())
	}

	if len(versions) == 0 {
		return "", "", fmt.Errorf("version %s not found", c.AerospikeVersion.String())
	}

	version := versions.Latest()
	if version == nil {
		return "", "", fmt.Errorf("version not found")
	}
	c.AerospikeVersion = TypeAerospikeVersion(version.Name)

	files, err := aerospike.GetFiles(time.Second*10, *version)
	if err != nil {
		return "", "", fmt.Errorf("could not get assets for version %s: %s", version.Name, err)
	}

	arch := aerospike.ArchitectureTypeX86_64
	if c.IsArm {
		arch = aerospike.ArchitectureTypeAARCH64
	}
	osName := aerospike.OSName(c.DistroName.String())
	if osName == "rocky" {
		osName = "centos"
	}
	return files.GetServerInstallerURL(arch, osName, c.DistroVersion.String())
}

func (c *InstallerDownloadCmd) DownloadAerospikeInstaller(log *logger.Logger, destDir string, destFileName string, installURL string, progressFunc func(p *wget.Progress)) error {
	if c.DryRun {
		log.Info("Dry run, skipping download")
		return nil
	}

	destFile := filepath.Join(destDir, destFileName)

	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		err = os.MkdirAll(destDir, 0755)
		if err != nil {
			return fmt.Errorf("could not create directory: %s", err)
		}
	}

	f, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("could not create file: %s", err)
	}
	defer f.Close()

	if progressFunc == nil {
		_, err = wget.Get(&wget.GetInput{
			Url:    installURL,
			Writer: f,
		})
	} else {
		_, err = wget.GetWithProgress(&wget.GetInput{
			Url:               installURL,
			Writer:            f,
			CallbackFrequency: time.Second,
			CallbackFunc:      progressFunc,
		})
	}
	if err != nil {
		return fmt.Errorf("could not download file: %s", err)
	}
	return nil
}
