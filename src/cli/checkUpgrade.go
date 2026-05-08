package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	cmd "github.com/aerospike/aerolab/cli/cmd/v1"
	"github.com/aerospike/aerolab/pkg/utils/versions"
	"github.com/rglonek/logger"
)

/*
Purpose of the code: aerolab v8 is still unfinished and we want to do a dev build. But upgrade --prerelease command in versions piror to aerolab v7.9 will actually install v8. The idea is that if a user accidentally installs v8 without having aerolab before, or upgrades from a version older than v7.9, they will get auto-downgraded to v7.9. With v7.9 user needs to perform --major on purpose, or it won't auto-upgrade to v8, so we are safe.

Below logic explains the code logic to be put in place while v8 is still in development to handle this process (to be adjusted later once v8 is released, but also kept - just adjusted).

-- LOGIC --
is AEROLAB_HOME set:
* Y: does it exist:
  * Y: is it v8 HOME?
    * Y: continue() // all good
    * N: inform user it is not v8 home, tell them to pick a new one and use the migrate command to migrate to it()
  * N: initialize+continue() // user wants a new home, we are good
* N: does old_home exist:
  * Y: does new_home exist:
    * Y: continue() // everything was done and both are initialized
    * N: does old_home have v7.9 marker:
      * Y: migrate_conf()+continue() // user on purpose wanted aerolab v8
      * N: rename self to aerolab8, download and install aerolab v7.9() // accidental upgrade, rollback
  * N: inform user that this version is unfinished and what we are doing, then rename self, install v7.9
       * also initialize the new_home, this downgrade thing was a one-off
       // user probably wanted a stable version, let them know aerolab8 is available for non-stable testing
*/

func checkUpgrade() {
	_, nerr := os.Stat("/opt/aerolab-agi-exec")
	if os.Getenv("AEROLAB_TEST") == "1" || os.Getenv("AEROLAB_SKIP_DOWNGRADE") == "1" || nerr == nil {
		return
	}
	if os.Getenv("AEROLAB_HOME") == "" {
		defaultHomeLogic()
	} else {
		customHomeLogic()
	}
}

func defaultHomeLogic() {
	oldHome, err := cmd.AerolabRootDirOld()
	if err != nil {
		log.Printf("Could not determine user's old home directory: %s", err)
		os.Exit(1)
	}
	newHome, err := cmd.AerolabRootDir()
	if err != nil {
		log.Printf("Could not determine user's new home directory: %s", err)
		os.Exit(1)
	}
	oldHomeExists := false
	newHomeExists := false
	if _, err := os.Stat(oldHome); err == nil {
		oldHomeExists = true
	}
	if _, err := os.Stat(newHome); err == nil {
		newHomeExists = true
	}

	// Check if we're running as aerolab8 (user wants to use v8)
	isAerolab8 := isRunningAsAerolab8()

	if oldHomeExists {
		if newHomeExists {
			return
		} else {
			// Old home exists, new home doesn't
			if hasOldGot79Marker(oldHome) || isAerolab8 {
				// User intentionally wants v8, migrate config
				if err := cmd.MigrateAerolabConfig(oldHome, newHome); err != nil {
					log.Printf("Could not migrate AeroLab configuration: %s", err)
					os.Exit(1)
				}
				fixHomeOwnership(newHome)
				return
			} else {
				// Accidental upgrade, perform rollback
				if rollbackTo79(newHome) {
					os.Exit(1)
				}
				return
			}
		}
	} else {
		return // no old home exists, new fresh install, we are good
	}
}

func customHomeLogic() {
	customHome, err := cmd.AerolabRootDir()
	if err != nil {
		log.Printf("Could not determine user's custom home directory: %s", err)
		os.Exit(1)
	}
	exists := false
	if _, err := os.Stat(customHome); err == nil {
		exists = true
	}
	if !exists {
		if err := os.MkdirAll(customHome, 0700); err != nil {
			log.Printf("Could not create home directory %s: %s", customHome, err)
			os.Exit(1)
		}
		fixHomeOwnership(customHome)
		return
	}
	if isV8Home(customHome) {
		return
	}
	log.Println("The $AEROLAB_HOME directory is pointing at an old version of AeroLab. Please pick a new directory and use the `aerolab config migrate` command to migrate to it.")
	os.Exit(1)
}

func isV8Home(home string) bool {
	if _, err := os.Stat(path.Join(home, "v8")); err == nil {
		return true
	}
	return false
}

func hasOldGot79Marker(home string) bool {
	if _, err := os.Stat(path.Join(home, "current-version.txt")); err == nil {
		content, err := os.ReadFile(path.Join(home, "current-version.txt"))
		if err != nil {
			return false
		}
		v := strings.TrimPrefix(strings.Split(string(content), "-")[0], "v")
		return versions.Compare(v, "7.9.0") >= 0
	}
	return false
}

func isRunningAsAerolab8() bool {
	cur, err := cmd.GetSelfPath()
	if err != nil {
		return false
	}
	ext := path.Ext(cur)
	baseName := strings.TrimSuffix(path.Base(cur), ext)
	return baseName == "aerolab8"
}

// fixHomeOwnership corrects ownership of the aerolab home directory when
// running under sudo. Without this, 'sudo aerolab' creates ~/.config/aerolab
// owned by root, breaking subsequent non-sudo runs.
func fixHomeOwnership(homePath string) {
	if runtime.GOOS == "windows" {
		return
	}
	sudoUID := os.Getenv("SUDO_UID")
	sudoGID := os.Getenv("SUDO_GID")
	if sudoUID == "" || sudoGID == "" {
		return
	}
	uid, err := strconv.Atoi(sudoUID)
	if err != nil {
		return
	}
	gid, err := strconv.Atoi(sudoGID)
	if err != nil {
		return
	}
	// Also fix the parent directory (e.g. ~/.config) in case MkdirAll created it
	parent := path.Dir(homePath)
	if parent != "" && parent != "." && parent != "/" {
		//nolint:errcheck
		os.Chown(parent, uid, gid)
	}
	//nolint:errcheck
	filepath.WalkDir(homePath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		//nolint:errcheck
		os.Chown(p, uid, gid)
		return nil
	})
}

func rollbackTo79(home string) bool {
	// if we're already running as aerolab8, skip the downgrade
	if isRunningAsAerolab8() {
		log.Println("Running as aerolab8, skipping downgrade process")
		//nolint:errcheck
		os.MkdirAll(home, 0700)
		//nolint:errcheck
		os.WriteFile(path.Join(home, "v8"), []byte(""), 0644)
		fixHomeOwnership(home)
		return false
	}

	if err := cmd.CheckBinaryDirWritable(); err != nil {
		// Try to re-run the original command under sudo so the user gets a
		// password prompt instead of a hard failure. On success this never
		// returns (the sudo'd child takes over and os.Exit is called).
		wrapped := fmt.Errorf("AeroLab needs to perform an auto-downgrade to v7, but %s", err)
		if rerr := cmd.ReExecWithSudo(logger.NewLogger(), wrapped); rerr != nil {
			log.Printf("%s", rerr)
			os.Exit(1)
		}
	}

	// get current binary path
	cur, err := cmd.GetSelfPath()
	if err != nil {
		log.Printf("Could not determine path of self: %s", err)
		os.Exit(1)
	}

	ext := path.Ext(cur)

	destPath := strings.TrimSuffix(cur, ext) + "8" + ext

	// copy the file instead of renaming so upgrade can still find the original
	source, err := os.ReadFile(cur)
	if err != nil {
		log.Printf("Could not read current binary: %s", err)
		os.Exit(1)
	}
	err = os.WriteFile(destPath, source, 0755)
	if err != nil {
		log.Printf("Could not write aerolab8 binary: %s", err)
		os.Exit(1)
	}

	// using cmd, run the upgrade command with the latest 7 (this will replace the current binary)
	upgrade := cmd.UpgradeCmd{Version: "7.", Edge: true, Force: true}
	err = upgrade.UpgradeAerolab(logger.NewLogger())
	if err != nil {
		log.Printf("Could not downgrade to AeroLab v7.9.0: %s", err)
		os.Exit(1)
	}
	//nolint:errcheck
	os.MkdirAll(home, 0700)
	//nolint:errcheck
	os.WriteFile(path.Join(home, "v8"), []byte(""), 0644)
	fixHomeOwnership(home)
	log.Println("WARNING: AeroLab v8 has been installed, though it is still in development. The following actions are being performed:")
	log.Println("1. Copying self to `aerolab8`")
	log.Println("2. Installing the latest AeroLab v7 available (replaces current binary)")
	log.Println("Note that this fix is a one-off action and you will need to perform the `--major` flag next time you run `aerolab upgrade` to upgrade to AeroLab v8.")
	return true
}
