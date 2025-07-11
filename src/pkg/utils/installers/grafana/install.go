package grafana

import (
	"bytes"
	"embed"
	"strconv"
	"text/template"
)

//go:embed scripts
var scripts embed.FS

func GetInstallScript(version string, enableGrafana bool, startGrafana bool) ([]byte, error) {
	script, err := processTemplate("scripts/install.sh.tpl", map[string]any{
		"Version":       version,
		"EnableGrafana": strconv.FormatBool(enableGrafana),
		"StartGrafana":  strconv.FormatBool(startGrafana),
	})
	if err != nil {
		return nil, err
	}
	return script, nil
}

func processTemplate(scriptFile string, data map[string]any) ([]byte, error) {
	script, err := scripts.ReadFile(scriptFile)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("script").Parse(string(script))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
