package grafanafix

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed dashboards
var dashboards embed.FS

func (g *GrafanaFix) importDashboards() error {
	files := []string{}
	embeddedFiles := []string{}
	folders := make(map[string]string) // [name]uid
	if g.Dashboards.LoadEmbedded {
		err := fs.WalkDir(dashboards, "dashboards", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == "dashboards" {
				return nil
			}
			if d.IsDir() {
				uid := RandStringRunes(12)
				dName := d.Name()
				if strings.Contains(d.Name(), "-") {
					dNames := strings.Split(d.Name(), "-")
					dName = dNames[1]
					uid = dNames[0]
				}
				found := false
				for {
					for _, uu := range folders {
						if uu == uid {
							found = true
							break
						}
					}
					if !found {
						break
					}
					uid = RandStringRunes(12)
				}
				folders[dName] = uid
				log.Printf("Creating folder %s uid %s", dName, uid)
				err = g.createFolder(dName, uid)
				if err != nil {
					log.Printf("ERROR, will retry: %s", err)
					time.Sleep(time.Second)
					err = g.createFolder(dName, uid)
				}
				return err
			}
			if d.Name() == ".gitignore" {
				return nil
			}
			embeddedFiles = append(embeddedFiles, p)
			return nil
		})
		if err != nil {
			return err
		}
	}
	if g.Dashboards.FromDir != "" {
		err := filepath.WalkDir(g.Dashboards.FromDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == g.Dashboards.FromDir {
				return nil
			}
			if d.IsDir() {
				uid := RandStringRunes(12)
				dName := d.Name()
				if strings.Contains(d.Name(), "-") {
					dNames := strings.Split(d.Name(), "-")
					dName = dNames[1]
					uid = dNames[0]
				}
				found := false
				for {
					for _, uu := range folders {
						if uu == uid {
							found = true
							break
						}
					}
					if !found {
						break
					}
					uid = RandStringRunes(12)
				}
				folders[dName] = uid
				log.Printf("Creating folder %s uid %s", dName, uid)
				err = g.createFolder(dName, uid)
				if err != nil {
					log.Printf("ERROR, will retry: %s", err)
					time.Sleep(time.Second)
					err = g.createFolder(dName, uid)
				}
				return err
			}
			if d.Name() == ".gitignore" {
				return nil
			}
			files = append(files, p)
			return nil
		})
		if err != nil {
			return err
		}
	}
	sort.Strings(embeddedFiles)
	sort.Strings(files)
	for _, p := range embeddedFiles {
		if !strings.HasSuffix(p, ".json") {
			continue
		}
		contents, err := dashboards.ReadFile(p)
		if err != nil {
			return err
		}
		folder, _ := path.Split(p)
		folder = strings.TrimRight(folder, "/")
		_, folder = path.Split(folder)
		var folderUid string
		var ok bool
		if strings.Contains(folder, "-") {
			folderUid, ok = folders[strings.Split(folder, "-")[1]]
		} else {
			folderUid, ok = folders[folder]
		}
		if !ok {
			folderUid = ""
		}
		log.Printf("Importing dashboard file %s to folder UID %s", p, folderUid)
		err = g.importDashboard(folderUid, contents)
		if err != nil {
			log.Printf("ERROR, will retry: %s", err)
			time.Sleep(time.Second)
			err = g.importDashboard(folderUid, contents)
		}
		if err != nil {
			return err
		}
	}
	for _, p := range files {
		if !strings.HasSuffix(p, ".json") {
			continue
		}
		contents, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		folder, _ := path.Split(p)
		folder = strings.TrimRight(folder, "/")
		_, folder = path.Split(folder)
		var folderUid string
		var ok bool
		if strings.Contains(folder, "-") {
			folderUid, ok = folders[strings.Split(folder, "-")[1]]
		} else {
			folderUid, ok = folders[folder]
		}
		if !ok {
			folderUid = ""
		}
		log.Printf("Importing dashboard file %s to folder UID %s", p, folderUid)
		err = g.importDashboard(folderUid, contents)
		if err != nil {
			log.Printf("ERROR, will retry: %s", err)
			time.Sleep(time.Second)
			err = g.importDashboard(folderUid, contents)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type folderJson struct {
	Uid   string `json:"uid"`
	Title string `json:"title"`
}

func (g *GrafanaFix) createFolder(name string, uid string) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	jsonx := folderJson{
		Uid:   uid,
		Title: name,
	}
	jsonOut, err := json.Marshal(jsonx)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(g.GrafanaURL, "/")+"/api/folders", bytes.NewBuffer(jsonOut))
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

func (g *GrafanaFix) importDashboard(folderUid string, file []byte) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	json := `{"dashboard":` + string(file) + `,"folderUid":"` + folderUid + `","overwrite":true,"inputs":[{"name":"DS_JSON","type":"datasource","pluginId":"simpod-json-datasource","value":"json"}]}`
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(g.GrafanaURL, "/")+"/api/dashboards/import", bytes.NewBuffer([]byte(json)))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", `application/json`)
	req.Header.Add("x-grafana-org-id", "1")
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
