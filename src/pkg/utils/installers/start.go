package installers

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed dependencies.sh.tpl
var dependenciesScriptTemplate []byte

type Dependency struct {
	Command string `json:"command"` // command to check for
	Package string `json:"package"` // package to install if command not found
}

type Installs struct {
	Dependencies []Dependency `json:"dependencies"` // commands to check for and packages to install if command not found
	Packages     []string     `json:"packages"`     // packages to install always
}

type Software struct {
	Debug    bool     `json:"debug"`    // if true, print debug information (set -x)
	Required Installs `json:"required"` // required packages (fail if cannot install)
	Optional Installs `json:"optional"` // optional packages (try and install one at a time and continue if error)
}

func GetInstallScript(software Software, tailScript []byte) ([]byte, error) {
	script, err := processTemplate(dependenciesScriptTemplate, software)
	if err != nil {
		return nil, err
	}

	return append(script, tailScript...), nil
}

func processTemplate(scriptFile []byte, data any) ([]byte, error) {
	tmpl, err := template.New("script").Parse(string(scriptFile))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
