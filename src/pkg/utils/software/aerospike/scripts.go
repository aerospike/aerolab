package aerospike

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
)

//go:embed scripts
var scripts embed.FS

type handlers struct {
	fInstallScript func(f File, fDetail *NameParts, debug bool, download bool, install bool, upgrade bool) ([]byte, error)
	vInstallScript func(files Files, arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error)
}

var scriptHandlers = make(map[ProductType]handlers)

func (f File) GetInstallScript(debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	fDetail := f.ParseNameParts()
	if fDetail == nil {
		return nil, errors.New("could not parse file name")
	}
	handlers, ok := scriptHandlers[fDetail.ProductType]
	if !ok {
		return nil, errors.New("unsupported product type")
	}
	return handlers.fInstallScript(f, fDetail, debug, download, install, upgrade)
}

func (files Files) GetInstallScript(arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	if len(files) == 0 {
		return nil, errors.New("no files found for version")
	}
	fDetail := files[0].ParseNameParts()
	if fDetail == nil {
		return nil, errors.New("could not parse file name")
	}
	handlers, ok := scriptHandlers[fDetail.ProductType]
	if !ok {
		return nil, errors.New("unsupported product type")
	}
	return handlers.vInstallScript(files, arch, osName, osVersion, debug, download, install, upgrade)
}

func processTemplate(scriptFile string, data any) ([]byte, error) {
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
