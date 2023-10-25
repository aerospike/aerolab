package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/aerospike/aerolab/ingest"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
		b := &ingest.NotifyEvent{
			IngestStatus: &ingest.IngestStatusStruct{},
		}
		err = json.Unmarshal(body, b)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
		body, _ = json.MarshalIndent(b, "", "    ")
		log.Printf("%s: %s\n", r.RemoteAddr, string(body))
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})
	cmd := exec.Command("ip", "addr", "sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Run()
	log.Println("Starting")
	http.ListenAndServe("0.0.0.0:8080", nil)
}
