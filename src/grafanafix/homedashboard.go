package grafanafix

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func (g *GrafanaFix) homeDashboard() error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	json := "{\"homeDashboardUID\": \"dashList\"}"
	req, err := http.NewRequest(http.MethodPatch, strings.TrimRight(g.GrafanaURL, "/")+"/api/org/preferences", bytes.NewBuffer([]byte(json)))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", `application/json`)
	req.SetBasicAuth("admin", "admin")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	log.Println("STATUS:", resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Println("BODY: ", string(body))
	return nil
}
