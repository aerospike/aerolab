package aerospike

import (
	"errors"

	"github.com/aerospike/aerolab/pkg/utils/installers"
)

func init() {
	scriptHandlers[ProductTypeTools] = handlers{
		fInstallScript: func(f File, fDetail *NameParts, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
			return f.getInstallScriptTools(fDetail, debug, download, install, upgrade)
		},
		vInstallScript: func(files Files, arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
			return files.getInstallScriptTools(arch, osName, osVersion, debug, download, install, upgrade)
		},
	}
}

func (f Files) getInstallScriptTools(arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	for _, file := range f {
		fDetail := file.ParseNameParts()
		if fDetail == nil {
			continue
		}
		if fDetail.ProductType != ProductTypeTools {
			continue
		}
		if fDetail.FileType != FileTypeTGZ {
			continue
		}
		if fDetail.Architecture != arch {
			continue
		}
		if fDetail.OSName != osName {
			continue
		}
		if fDetail.OSVersion != osVersion {
			continue
		}
		return file.getInstallScriptTools(fDetail, debug, download, install, upgrade)
	}
	return nil, errors.New("no matching file found for given architecture, os name, and os version")
}

func (f File) getInstallScriptTools(fDetail *NameParts, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	_ = fDetail
	s := installers.Software{
		Debug: debug,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "python3", Package: "python3"},
			},
		},
	}
	script, err := installers.GetInstallScript(s, nil)
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
		ilScript, err := processTemplate("scripts/install_tools.sh.tpl", data)
		if err != nil {
			return nil, err
		}
		script = append(script, ilScript...)
	}
	return script, nil
}
