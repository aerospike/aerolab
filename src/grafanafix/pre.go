package grafanafix

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
)

//go:embed simpod-json-datasource
var datasource embed.FS

var dataSourceYaml = `# config file version
apiVersion: 1

datasources:
  # <string, required> name of the datasource. Required
  - name: JSON
    type: simpod-json-datasource
    access: proxy
    orgId: 1
    uid: json
    url: %s
    isDefault: true
    version: 1
    editable: true
`

func EarlySetup(iniPath string, provisioningDir string, pluginsDir string, pluginUrl string, grafanaPort int) error {
	if pluginUrl == "" {
		pluginUrl = "http://127.0.0.1:8851"
	}
	if grafanaPort == 0 {
		grafanaPort = 8850
	}
	// grafana.ini
	err := copyFile(iniPath, iniPath+".backup")
	if err != nil {
		return err
	}
	newIni, err := fixIni(iniPath, grafanaPort)
	if err != nil {
		return err
	}
	err = os.WriteFile(iniPath, newIni, 0644)
	if err != nil {
		return err
	}
	// datasource json
	err = os.WriteFile(path.Join(provisioningDir, "datasources", "json.yaml"), []byte(fmt.Sprintf(dataSourceYaml, pluginUrl)), 0644)
	if err != nil {
		return err
	}
	// install datasource
	err = fs.WalkDir(datasource, "simpod-json-datasource", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(path.Join(pluginsDir, p), 0755)
		}
		w, err := os.Create(path.Join(pluginsDir, p))
		if err != nil {
			return err
		}
		defer w.Close()
		r, err := datasource.Open(p)
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = io.Copy(w, r)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func copyFile(src string, dst string) error {
	s, err := os.Stat(src)
	if err != nil {
		return err
	}
	w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, s.Mode().Perm())
	if err != nil {
		return err
	}
	defer w.Close()
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}
	return nil
}

func fixIni(iniPath string, port int) ([]byte, error) {
	r, err := os.Open(iniPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	s := bufio.NewScanner(r)
	out := []byte{}
	indataproxy := false
	inauthanon := false
	inserver := false
	for s.Scan() {
		if err := s.Err(); err != nil {
			return nil, err
		}
		line := s.Text()
		if !strings.HasPrefix(line, "#") {
			if strings.Contains(line, "enable_gzip") {
				line = "enable_gzip = true"
			} else if strings.HasPrefix(line, "[dataproxy]") {
				indataproxy = true
				inauthanon = false
				inserver = false
			} else if strings.HasPrefix(line, "[auth.anonymous]") {
				inauthanon = true
				indataproxy = false
				inserver = false
			} else if strings.HasPrefix(line, "[server]") {
				indataproxy = false
				inauthanon = false
				inserver = true
			} else if strings.HasPrefix(line, "[") {
				indataproxy = false
				inauthanon = false
				inserver = false
			} else if indataproxy && strings.Contains(line, "timeout") && !strings.Contains(line, "_") {
				line = "timeout = 300"
			} else if inauthanon {
				if strings.Contains(line, "enabled") {
					line = "enabled = true"
				} else if strings.Contains(line, "org_name") {
					line = strings.TrimPrefix(line, ";")
				} else if strings.Contains(line, "org_role") {
					line = "org_role = Admin"
				}
			} else if inserver && strings.Contains(line, "http_port") {
				line = "http_port = " + strconv.Itoa(port)
			}
		}
		out = append(out, []byte(line)...)
		out = append(out, '\n')
	}
	return out, nil
}
