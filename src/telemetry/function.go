package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("AeroLabTelemetry", AeroLabTelemetry)
}

func AeroLabTelemetry(w http.ResponseWriter, r *http.Request) {
	var d struct {
		// common
		UUID    string   // unique identifier for the telemetry event
		CmdLine []string // actual os.Args[1:]

		//aerolab
		Command         []string    // command name, e.g. "cluster","create"
		Args            []string    // tail args passed to the command's Execute function
		Params          interface{} // actual params struct for the command being executed
		StartTime       int64       // start time of the Execute function
		EndTime         int64       // end time of the Execute function
		Version         string      // version of the telemetry system
		AeroLabVersion  string      // version of the AeroLab binary
		AeroLabCommit   string      // commit hash of the AeroLab binary
		AeroLabEdition  string      // edition of the AeroLab binary
		Error           *string     // error message if the command failed
		Stderr          []string    // capture of the logger output
		StderrTruncated bool        // whether the logger output was truncated (max 1000 lines per event will be logged)
		Defaults        []struct {
			Key   string
			Value string
		}

		//expiry
		Job           string
		Cloud         string
		Zone          string
		ResourceID    string
		ClusterUUID   string
		ResourceType  string
		ResourceName  string
		ClusterName   string
		NodeNo        string
		ExpiryVersion string
		Time          int64
		Tags          map[string]string

		// internal
		TelemetryName    string
		TelemetryVersion string

		// webrun
		WebRun struct {
			Command []string
			Params  map[string]interface{}
		}
	}

	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		switch err {
		case io.EOF:
			fmt.Fprint(w, "EOF")
			return
		default:
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			fmt.Fprintf(w, "json.NewDecoder: %v", err)
			return
		}
	}

	if d.UUID == "" || (d.Time == 0 && d.StartTime == 0) || len(d.CmdLine) == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		fmt.Fprint(w, "EMPTY OR MALFORMED MESSAGE")
		return
	}

	d.TelemetryName = os.Getenv("K_SERVICE")
	d.TelemetryVersion = os.Getenv("K_REVISION")

	telemetryString, err := json.Marshal(d)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}
	log.Printf("%s", string(telemetryString))
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}
	bkt := client.Bucket("aerolab-telemetrics")
	ntime := d.Time
	btime := d.Time
	if ntime == 0 {
		ntime = d.StartTime
		btime = d.StartTime
	}
	if btime > 9999999999 {
		btime = btime / 1000000
	}
	loc := "v8/aerolab/"
	if len(d.CmdLine) > 0 && d.CmdLine[0] == "EXPIRY" {
		loc = "v8/expiry/"
	}
	obj := bkt.Object(loc + time.Unix(btime, 0).Format("2006-01-02") + "/" + d.UUID + "_" + strconv.Itoa(int(ntime)))
	wa := obj.NewWriter(ctx)
	if _, err := fmt.Fprintf(wa, "%s", string(telemetryString)); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}
	if err := wa.Close(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}

	fmt.Fprint(w, "OK")
}
