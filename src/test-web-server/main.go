package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
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
