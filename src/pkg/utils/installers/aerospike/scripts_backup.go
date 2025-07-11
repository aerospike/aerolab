package aerospike

import (
	"errors"

	"github.com/aerospike/aerolab/pkg/utils/installers"
)

func init() {
	scriptHandlers[ProductTypeBackup] = handlers{
		fInstallScript: func(f File, fDetail *NameParts, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
			return f.getInstallScriptBackup(fDetail, debug, download, install, upgrade)
		},
		vInstallScript: func(files Files, arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
			return files.getInstallScriptBackup(arch, osName, osVersion, debug, download, install, upgrade)
		},
	}
}

func (f Files) getInstallScriptBackup(arch ArchitectureType, osName OSName, osVersion string, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	_ = osVersion
	ftype := FileTypeUnknown
	switch osName {
	case OSNameCentOS, OSNameAmazon:
		ftype = FileTypeRPM
	case OSNameDebian, OSNameUbuntu:
		ftype = FileTypeDEB
	}
	for _, file := range f {
		fDetail := file.ParseNameParts()
		if fDetail == nil {
			continue
		}
		if fDetail.ProductType != ProductTypeBackup {
			continue
		}
		if fDetail.FileType != ftype {
			continue
		}
		if fDetail.Architecture != arch {
			continue
		}
		return file.getInstallScriptBackup(fDetail, debug, download, install, upgrade)
	}
	return nil, errors.New("no matching file found for given architecture, os name, and os version")
}

func (f File) getInstallScriptBackup(fDetail *NameParts, debug bool, download bool, install bool, upgrade bool) ([]byte, error) {
	_ = fDetail
	s := installers.Software{
		Debug: debug,
		Required: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
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
		ilScript, err := processTemplate("scripts/install_backup.sh.tpl", data)
		if err != nil {
			return nil, err
		}
		script = append(script, ilScript...)
	}
	return script, nil
}
