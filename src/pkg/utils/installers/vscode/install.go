package vscode

import (
	"bytes"
	"embed"
	"strconv"
	"text/template"

	"github.com/aerospike/aerolab/pkg/utils/installers"
)

//go:embed scripts
var scripts embed.FS

// enable auto-start of systemd service
// start systemd service
// if password is nil, auth == none
// if bindAddr is nil, bindAddr == 0.0.0.0:8080
// if patchExtensions is true, it will patch the extensions library paths to use official marketplace
// if overrideDefaultFolder is not nil, it will override the default folder to open in vscode when it starts
// userHome is the home directory of the user that runs vscode
// username is the name of the user that runs vscode
func GetLinuxInstallScript(enable bool, start bool, password *string, bindAddr *string, requiredExtensions []string, optionalExtensions []string, patchExtensions bool, overrideDefaultFolder *string, userHome string, username string) ([]byte, error) {
	if bindAddr == nil {
		bind := "0.0.0.0:8080"
		bindAddr = &bind
	}
	authType := "none"
	if password != nil && *password != "" {
		authType = "password"
	}
	s := installers.Software{
		Debug: true,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "jq", Package: "jq"},
			},
		},
	}
	installScript, err := processTemplate("scripts/install.sh.tpl", map[string]any{
		"EnableVSCode":          strconv.FormatBool(enable),
		"StartVSCode":           strconv.FormatBool(start),
		"Password":              password,
		"BindAddr":              bindAddr,
		"AuthType":              authType,
		"RequiredExtensions":    requiredExtensions,
		"OptionalExtensions":    optionalExtensions,
		"PatchExtensions":       strconv.FormatBool(patchExtensions),
		"UserHome":              userHome,
		"Username":              username,
		"OverrideDefaultFolder": overrideDefaultFolder,
	})
	if err != nil {
		return nil, err
	}
	return installers.GetInstallScript(s, installScript)
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
