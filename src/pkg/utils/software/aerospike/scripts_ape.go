package aerospike

import (
	"errors"
	"strconv"
)

func init() {
	scriptHandlers[ProductTypeExporter] = handlers{
		fInstallScript: func(f File, fDetail *NameParts, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
			return f.getInstallScriptApe(fDetail, debug, download, install, upgrade)
		},
		vInstallScript: func(files Files, arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
			return files.getInstallScriptApe(arch, osName, osVersion, debug, download, install, upgrade)
		},
	}
}

func (f Files) getInstallScriptApe(arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	_ = osName
	_ = osVersion
	for _, file := range f {
		fDetail := file.ParseNameParts()
		if fDetail == nil {
			continue
		}
		if fDetail.ProductType != ProductTypeExporter {
			continue
		}
		if fDetail.FileType != FileTypeTGZ {
			continue
		}
		if fDetail.Architecture != arch {
			continue
		}
		return file.getInstallScriptApe(fDetail, debug, download, install, upgrade)
	}
	return nil, errors.New("no matching file found for given architecture, os name, and os version")
}

func (f File) getInstallScriptApe(fDetail *NameParts, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	_ = fDetail
	dependencies := []string{"curl"}
	packages := []string{"curl"}
	// .Debug, .Dependencies, .Packages
	data := struct {
		Debug        string
		Dependencies []string
		Packages     []string
	}{
		Debug:        strconv.FormatBool(debug),
		Dependencies: dependencies,
		Packages:     packages,
	}
	script, err := processTemplate("scripts/start.sh.tpl", data)
	if err != nil {
		return nil, err
	}
	if download {
		// .FileName, .FileUrl
		data := struct {
			FileName string
			FileUrl  string
		}{
			FileName: "/opt/aerolab/files/" + f.Name,
			FileUrl:  f.DownloadLink,
		}
		dlScript, err := processTemplate("scripts/download.sh.tpl", data)
		if err != nil {
			return nil, err
		}
		script = append(script, dlScript...)
	}

	if install {
		// .Upgrade, .FileName
		data := struct {
			Upgrade  bool
			FileName string
		}{
			Upgrade:  upgrade,
			FileName: "/opt/aerolab/files/" + f.Name,
		}
		ilScript, err := processTemplate("scripts/install_ape.sh.tpl", data)
		if err != nil {
			return nil, err
		}
		script = append(script, ilScript...)
	}
	return script, nil
}
