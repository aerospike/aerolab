package main

import (
	"errors"
	"fmt"
	"log"
	"os"
)

func debug(format string, values ...interface{}) {
	if os.Getenv("AEROLAB_BACKEND_DEBUG") == "on" {
		log.Printf(format, values...)
	}
}

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
		return "24.04"
	case "centos":
		return "9"
	case "rocky":
		return "9"
	case "amazon":
		return "2023"
	case "debian":
		return "12"
	}
	return ""
}

func checkDistroVersion(distro string, version string) error {
	switch distro {
	case "ubuntu":
		switch version {
		case "24.04", "22.04", "20.04", "18.04":
			return nil
		}
	case "centos":
		switch version {
		case "10", "9", "7":
			return nil
		case "8":
			return nil
		}
	case "rocky":
		switch version {
		case "10", "9", "8":
			return nil
		}
	case "amazon":
		switch version {
		case "2023", "2":
			return nil
		}
	case "debian":
		switch version {
		case "13", "12", "11", "10", "9", "8":
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

/*
func logFatal(format interface{}, values ...interface{}) {
	if len(values) == 0 {
		log.Fatal("ERROR " + fmt.Sprint(format))
	}
	log.Fatalf("ERROR "+fmt.Sprint(format), values...)
}
*/

func logFatal(format interface{}, values ...interface{}) error {
	if len(values) == 0 {
		return errors.New("ERROR " + fmt.Sprint(format))
	}
	return fmt.Errorf("ERROR "+fmt.Sprint(format), values...)
}

func logExit(format interface{}, values ...interface{}) {
	if len(values) == 0 {
		log.Print(format)
	} else {
		log.Printf(format.(string), values...)
	}
	defer handleExit()
	panic(Exit{1})
}

func beepExit(code int) {
	defer handleExit()
	panic(Exit{code})
}
