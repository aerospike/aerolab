package aerolabexpire

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"

	_ "embed"
)

func init() {
	functions.HTTP("AerolabExpire", aerolabExpire)
}

func aerolabExpire(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, "Incorrect input json", http.StatusBadRequest)
		return
	}
	if d.Token != os.Getenv("TOKEN") {
		http.Error(w, "Incorrect input token", http.StatusBadRequest)
		return
	}
	os.Chmod("bootstrap", 0755)
	out, err := exec.Command("./bootstrap").CombinedOutput()
	if err != nil {
		log.Printf("Failed to run bootstrap: %v", err)
		log.Print(string(out))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Bootstrap output: %s", out)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
