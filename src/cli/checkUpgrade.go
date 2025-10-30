package main

import (
	"log"
	"os"
	"path"
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
	if os.Getenv("AEROLAB_TEST") == "1" {
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
	if oldHomeExists {
		if newHomeExists {
			return
		} else {
			if hasOldGot79Marker(oldHome) {
				if err := cmd.MigrateAerolabConfig(oldHome, newHome); err != nil {
					log.Printf("Could not migrate AeroLab configuration: %s", err)
					os.Exit(1)
				}
				return
			} else {
				rollbackTo79(newHome) // remember to mkdir new home as part of rollback, print info about downgrade
				os.Exit(1)
			}
		}
	} else {
		rollbackTo79(newHome) // remember to mkdir new home as part of rollback, print info about downgrade
		os.Exit(1)
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
		os.MkdirAll(customHome, 0700)
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

func rollbackTo79(home string) {
	// rename self to aerolab8, preserving the path and extension
	cur, err := cmd.GetSelfPath()
	if err != nil {
		log.Printf("Could not determine path of self: %s", err)
		os.Exit(1)
	}
	ext := path.Ext(cur)
	os.Rename(cur, strings.TrimSuffix(cur, ext)+"8"+ext)
	// using cmd, run the upgrade command with the latest 7
	upgrade := cmd.UpgradeCmd{Version: "7.", Edge: true, Force: true}
	err = upgrade.UpgradeAerolab(logger.NewLogger())
	if err != nil {
		log.Printf("Could not downgrade to AeroLab v7.9.0: %s", err)
		os.Exit(1)
	}
	os.MkdirAll(home, 0700)
	os.WriteFile(path.Join(home, "v8"), []byte(""), 0644)
	log.Println("WARNING: AeroLab v8 has been installed, though it is still in development. The following actions are being performed:")
	log.Println("1. Renaming self to `aerolab8`")
	log.Println("2. Installing the latest AeroLab v7 available")
	log.Println("Note that this fix is a one-off action and you will need to perform the `--major` flag next time you run `aerolab upgrade` to upgrade to AeroLab v8.")
	os.Exit(1)
}
