package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rglonek/logger"
)

var telemetryVersion = "6" // remember to modify this when changing the telemetry system; remember to update the telemetry structs in cloud function if needed

type telemetryItem struct {
	CmdLine         []string           // actual os.Args[1:]
	Command         []string           // command name, e.g. "cluster","create"
	Args            []string           // tail args passed to the command's Execute function
	Params          interface{}        // actual params struct for the command being executed
	UUID            string             // unique identifier for the telemetry event
	StartTime       int64              // start time of the Execute function
	EndTime         int64              // end time of the Execute function
	Defaults        []telemetryDefault // key:value pairs of changed defaults
	Version         string             // version of the telemetry system
	AeroLabVersion  string             // version of the AeroLab binary
	AeroLabCommit   string             // commit hash of the AeroLab binary
	AeroLabEdition  string             // edition of the AeroLab binary
	Error           *string            // error message if the command failed
	Stderr          []string           // capture of the logger output
	StderrTruncated bool               // whether the logger output was truncated (max 1000 lines per event will be logged)
}

type telemetryDefault struct {
	Key   string
	Value string
}

var telemetryLock = new(sync.Mutex)

func TelemetrySend(logger *logger.Logger) {
	// only one instance of this function can run at a time
	if !telemetryLock.TryLock() {
		logger.Detail("Telemetry lock not acquired, another instance is running")
		return
	}
	defer telemetryLock.Unlock()

	// get telemetry dir
	telemetryDir, err := AerolabRootDir()
	if err != nil {
		logger.Detail("Failed to get telemetry dir: %s", err)
		return
	}
	telemetryDir = path.Join(telemetryDir, "telemetry")
	err = os.MkdirAll(telemetryDir, 0700)
	if err != nil {
		logger.Detail("Failed to create telemetry dir: %s", err)
		return
	}

	// check if telemetry is disabled
	if os.Getenv("AEROLAB_TELEMETRY_DISABLE") != "" {
		logger.Detail("Telemetry is disabled, skipping telemetry send")
		return
	}
	if _, err := os.Stat(path.Join(telemetryDir, "disable")); err == nil {
		logger.Detail("Telemetry is disabled, skipping telemetry send")
		return
	}

	// send items
	err = filepath.WalkDir(telemetryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "item-") {
			return nil
		}
		err = telemetryShipFile(path)
		if err != nil {
			return err
		}
		os.Remove(path)
		return nil
	})
	if err != nil {
		logger.Detail("Failed to send telemetry files: %s", err)
		return
	}
	logger.Detail("Telemetry sent")
}

func TelemetryEvent(command []string, params interface{}, args []string, system *System, err error) {
	// get err value
	var errString *string
	if err != nil {
		d := err.Error()
		errString = &d
	}

	id := uuid.New().String()
	logger := system.Logger.WithPrefix("TELEMETRY-EVENT " + id + ": ")
	// basics
	rootDir, err := AerolabRootDir()
	if err != nil {
		logger.Detail("Failed to get root dir: %s", err)
		return
	}
	telemetryDir := path.Join(rootDir, "telemetry")
	err = os.MkdirAll(telemetryDir, 0700)
	if err != nil {
		logger.Detail("Failed to create telemetry dir: %s", err)
		return
	}

	// check if telemetry is disabled
	if os.Getenv("AEROLAB_TELEMETRY_DISABLE") != "" {
		logger.Detail("Telemetry is disabled, skipping telemetry event")
		return
	}
	if _, err := os.Stat(path.Join(telemetryDir, "disable")); err == nil {
		logger.Detail("Telemetry is disabled, skipping telemetry event")
		return
	}

	// test if telemetry is disabled by feature key file
	enabled := telemetryFeatureKeyFileCheck(system, logger)
	if !enabled {
		return
	}

	logger.Detail("Recording telemetry event")
	// if more than 1000 files in telemetry dir, do NOT write the new file
	telemetryFileCount := 0
	err = filepath.WalkDir(telemetryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "item-") {
			return nil
		}
		telemetryFileCount++
		return nil
	})
	if err != nil {
		logger.Detail("Failed to count telemetry files: %s", err)
		return
	}
	if telemetryFileCount > 1000 {
		logger.Detail("Telemetry file count is greater than 1000, skipping telemetry event")
		return
	}

	// read the log buffer
	logBuffer := []string{}
	func() {
		for {
			select {
			case line := <-system.LogBuffer:
				logBuffer = append(logBuffer, line)
			default:
				return
			}
		}
	}()

	// fill item
	version, commit, edition, _ := GetAerolabVersion()
	item := telemetryItem{
		CmdLine:         os.Args[1:],
		Command:         command,
		Args:            args,
		Params:          params,
		UUID:            id,
		StartTime:       system.InitTime.UnixMicro(),
		EndTime:         time.Now().UnixMicro(),
		Defaults:        getDefaults(system),
		Version:         telemetryVersion,
		AeroLabVersion:  version,
		AeroLabCommit:   commit,
		AeroLabEdition:  edition,
		Error:           errString,
		Stderr:          logBuffer,
		StderrTruncated: system.LogBufferTruncated,
	}

	// write item to file
	newFile := path.Join(rootDir, "telemetry", "item-"+strconv.Itoa(int(item.StartTime)))
	f, err := os.OpenFile(newFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		logger.Detail("Failed to open telemetry file: %s", err)
		return
	}
	err = json.NewEncoder(f).Encode(item)
	if err != nil {
		f.Close()
		os.Remove(newFile)
		logger.Detail("Failed to write telemetry file: %s", err)
		return
	}
	f.Close()
	logger.Detail("Telemetry event recorded")
}

func getDefaults(system *System) []telemetryDefault {
	out := []telemetryDefault{}
	// add changed default values to the item
	ret := make(chan ConfigValueCmd, 1)
	originalOnlyChanged := system.Opts.Config.Defaults.OnlyChanged
	system.Opts.Config.Defaults.OnlyChanged = true
	keyField := reflect.ValueOf(system.Opts).Elem()
	go system.Opts.Config.Defaults.getValues(keyField, "", ret, "", system.Opts.Config.Defaults.OnlyChanged)
	for {
		val, ok := <-ret
		if !ok {
			break
		}
		if strings.HasSuffix(val.Key, ".Password") || strings.HasSuffix(val.Key, ".Pass") || strings.HasSuffix(val.Key, ".User") || strings.HasSuffix(val.Key, ".Username") {
			continue
		}
		out = append(out, telemetryDefault(val))
	}
	system.Opts.Config.Defaults.OnlyChanged = originalOnlyChanged
	return out
}

func telemetryShipFile(file string) error {
	contents, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	url := "https://us-central1-aerospike-gaia.cloudfunctions.net/aerolab-telemetrics"
	ret, err := http.Post(url, "application/json", bytes.NewReader(contents))
	if err != nil {
		return err
	}
	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("returned ret code: %d:%s", ret.StatusCode, ret.Status)
	}
	return nil
}

func telemetryFeatureKeyFileCheck(system *System, logger *logger.Logger) bool {
	// only enable if a feature file is present and belongs to Aerospike internal users
	if system.Opts.Cluster.Create.FeaturesFilePath == "" {
		logger.Detail("No feature file path provided, skipping telemetry")
		return false
	}
	enableTelemetry := false
	err := filepath.WalkDir(string(system.Opts.Cluster.Create.FeaturesFilePath), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if err = scanner.Err(); err != nil {
				return err
			}
			line := strings.ToLower(strings.Trim(scanner.Text(), "\r\n\t "))
			if strings.HasPrefix(line, "account-name") && (strings.HasSuffix(line, "aerospike") || strings.Contains(line, " aerospike") || strings.Contains(line, "aerospike_test") || strings.Contains(line, "\taerospike")) {
				enableTelemetry = true
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.Detail("Failed to check feature key file, skipping telemetry: %s", err)
		return false
	}
	if !enableTelemetry {
		logger.Detail("Telemetry is disabled by feature key file, skipping telemetry event")
	}
	return enableTelemetry
}
