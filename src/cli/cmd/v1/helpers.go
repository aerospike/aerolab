package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
)

func GetSelfPath() (string, error) {
	// Get the absolute path of the current executable
	cur, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of self: %s", err)
	}

	// Resolve the symlink to the current executable
	for {
		if st, err := os.Stat(cur); err != nil {
			return "", fmt.Errorf("failed to stat self: %s", err)
		} else {
			if st.Mode()&os.ModeSymlink > 0 {
				cur, err = filepath.EvalSymlinks(cur)
				if err != nil {
					return "", fmt.Errorf("error resolving symlink source to self: %s", err)
				}
			} else {
				break
			}
		}
	}
	return cur, nil
}

type osSelectorCmd struct {
	DistroName    TypeDistro        `short:"d" long:"distro" description:"Linux distro, one of: debian|ubuntu|centos|rocky|amazon" default:"ubuntu" webchoice:"debian,ubuntu,rocky,centos,amazon"`
	DistroVersion TypeDistroVersion `short:"i" long:"distro-version" description:"ubuntu:24.04|22.04|20.04|18.04 rocky:9,8 centos:9,7 amazon:2|2023 debian:13|12|11|10|9|8" default:"latest" webchoice:"latest,24.04,22.04,20.04,18.04,2023,2,12,11,10,9,8,7"`
}

type aerospikeVersionCmd struct {
	AerospikeVersion TypeAerospikeVersion `short:"v" long:"aerospike-version" description:"Aerospike server version; add 'c' to the end for community edition, or 'f' for federal edition" default:"latest"`
}

type aerospikeVersionSelectorCmd struct {
	osSelectorCmd
	aerospikeVersionCmd
}

// string format: [protocol:]from[-to]
func parsePortRange(port string) (string, int, int, error) {
	protocol := "tcp"
	parts := strings.Split(port, ":")
	if len(parts) > 1 {
		protocol = parts[0]
		port = parts[1]
	}
	parts = strings.Split(port, "-")
	if len(parts) == 1 {
		port, err := strconv.Atoi(parts[0])
		return protocol, port, port, err
	}
	from, err := strconv.Atoi(parts[0])
	if err != nil {
		return protocol, 0, 0, err
	}
	to, err := strconv.Atoi(parts[1])
	if err != nil {
		return protocol, 0, 0, err
	}
	if from > to {
		return protocol, 0, 0, errors.New("from port must be less than to port")
	}
	return protocol, from, to, nil
}

/*
func getip2_old() string {
	type IP struct {
		Query string
	}
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return err.Error()
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err.Error()
	}

	var ip IP
	json.Unmarshal(body, &ip)

	return ip.Query
}
*/

func getip2() string {
	req, err := http.Get("https://api.ipify.org?format=json")
	if err != nil {
		return err.Error()
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err.Error()
	}

	type ret struct {
		IP string `json:"ip"`
	}

	var ip ret
	json.Unmarshal(body, &ip)

	return ip.IP
}

func IsInteractive() bool {
	return os.Getenv("AEROLAB_NONINTERACTIVE") == "" && (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()))
}

func AskForString(prompt string) (string, error) {
	if IsInteractive() {
		fmt.Printf("%s: ", prompt)
		reader := bufio.NewReader(os.Stdin)
		return reader.ReadString('\n')
	}
	return "", errors.New("not interactive")
}

func AskForInt(prompt string) (int, error) {
	s, err := AskForString(prompt)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(s))
}

func AskForFloat(prompt string) (float64, error) {
	s, err := AskForString(prompt)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func UpdateDiskCache(system *System) {
	if !system.Opts.Config.Backend.InventoryCache {
		return
	}
	system.Logger.Info("Updating disk cache")
	err := system.Backend.RefreshChangedInventory()
	if err != nil {
		system.Logger.Error("Failed to update disk cache: %s", err)
	}
}
