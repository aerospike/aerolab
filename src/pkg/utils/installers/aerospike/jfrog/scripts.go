package jfrog

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"github.com/aerospike/aerolab/pkg/utils/installers"
)

//go:embed scripts
var scriptsFS embed.FS

// RemoteFileDir is the directory on the target instance where the
// pre-uploaded package file is expected to live. The path matches the
// convention used by the public install scripts so log scrapers see
// the same locations regardless of source.
const RemoteFileDir = "/opt/aerolab/files"

// InstallScript returns a complete bash script that installs the
// pre-uploaded package at remotePath. It bundles:
//
//   - dependency provisioning via installers.GetInstallScript (curl,
//     python3 — same as the public flow)
//   - rpm or deb install logic derived from the file's Format
//
// `upgrade` controls whether an existing asd binary is replaced or the
// install short-circuits.
func InstallScript(f *File, debug, upgrade bool) ([]byte, error) {
	if f == nil || f.Parts == nil {
		return nil, fmt.Errorf("jfrog: install script needs a parsed file")
	}
	remotePath := RemoteFileDir + "/" + f.Name

	base, err := installers.GetInstallScript(installers.Software{
		Debug: debug,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "python3", Package: "python3"},
			},
		},
	}, nil)
	if err != nil {
		return nil, err
	}

	var tplName string
	switch f.Parts.Format {
	case "rpm":
		tplName = "scripts/install_server_rpm.sh.tpl"
	case "deb":
		tplName = "scripts/install_server_deb.sh.tpl"
	default:
		return nil, fmt.Errorf("jfrog: unsupported install format %q", f.Parts.Format)
	}

	data := struct {
		FileName string
		Upgrade  bool
	}{
		FileName: remotePath,
		Upgrade:  upgrade,
	}
	pkg, err := renderTemplate(tplName, data)
	if err != nil {
		return nil, err
	}
	return append(base, pkg...), nil
}

func renderTemplate(name string, data any) ([]byte, error) {
	raw, err := scriptsFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("jfrog: read template %s: %w", name, err)
	}
	tmpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("jfrog: parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("jfrog: render template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// RemotePackagePath returns the absolute path where the package file
// should be SFTP-uploaded on the target instance.
func RemotePackagePath(f *File) string {
	if f == nil {
		return ""
	}
	return RemoteFileDir + "/" + f.Name
}
