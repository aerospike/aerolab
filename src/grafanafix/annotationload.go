package grafanafix

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type annotation struct {
	DashboardUID string   `json:"dashboardUID"`
	PanelId      int      `json:"panelId"`
	Time         int      `json:"time"`
	TimeEnd      int      `json:"timeEnd"`
	Tags         []string `json:"tags"`
	Text         string   `json:"text"`
}

func (g *GrafanaFix) loadAnnotations() error {
	file := g.AnnotationFile
	if _, err := os.Stat(file); err != nil {
		return err
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	annotations := []annotation{}
	err = json.Unmarshal(contents, &annotations)
	if err != nil {
		return err
	}
	for _, a := range annotations {
		res, err := json.Marshal(a)
		if err != nil {
			log.Print(err)
			continue
		}
		err = g.loadAnnotation(res)
		if err != nil {
			log.Print(err)
			continue
		}
	}
	return nil
}

func (g *GrafanaFix) loadAnnotation(a []byte) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(g.GrafanaURL, "/")+"/api/annotations", bytes.NewBuffer(a))
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
