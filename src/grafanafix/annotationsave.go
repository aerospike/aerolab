package grafanafix

import (
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func (g *GrafanaFix) saveAnnotations() error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(g.GrafanaURL, "/")+"/api/annotations", nil)
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
	return os.WriteFile(g.AnnotationFile, body, 0644)
}
