package eksctl

import (
	"bytes"
	"embed"
	"text/template"

	"github.com/aerospike/aerolab/pkg/utils/installers"
)

//go:embed scripts
var scripts embed.FS

// GetInstallScript returns the eksctl and kubectl installation script.
func GetInstallScript() ([]byte, error) {
	s := installers.Software{
		Debug: true,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
			},
		},
	}
	installScript, err := processTemplate("scripts/eksctl.sh.tpl", map[string]any{})
	if err != nil {
		return nil, err
	}
	return installers.GetInstallScript(s, installScript)
}

// GetBootstrapScript returns the bootstrap script that configures AWS CLI, kubectl, and credentials.
func GetBootstrapScript() ([]byte, error) {
	return processTemplate("scripts/bootstrap.sh.tpl", map[string]any{})
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
