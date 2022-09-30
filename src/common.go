package main

import (
	"fmt"
	"log"
	"os"
)

func chDir(dir string) error {
	if dir != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("working directory '%s' does not exist", dir)
		}
		err := os.Chdir(dir)
		if err != nil {
			return fmt.Errorf("could not change to working directory '%s'", dir)
		}
	}
	return nil
}

func getLatestVersionForDistro(distro string) string {
	switch distro {
	case "ubuntu":
		return "22.04"
	case "centos":
		return "8"
	case "amazon":
		return "2"
	}
	return ""
}

func checkDistroVersion(distro string, version string) error {
	switch distro {
	case "ubuntu":
		switch version {
		case "22.04", "20.04", "18.04":
			return nil
		}
	case "centos":
		switch version {
		case "8", "7":
			return nil
		}
	case "amazon":
		switch version {
		case "2":
			return nil
		}
	default:
		return fmt.Errorf("distro %s not supported", distro)
	}

	if version == "latest" {
		return nil
	}

	return fmt.Errorf("distro version not supported")
}

func logFatal(format interface{}, values ...interface{}) {
	if len(values) == 0 {
		log.Fatal("ERROR " + fmt.Sprint(format))
	}
	log.Fatalf("ERROR "+fmt.Sprint(format), values...)
}
