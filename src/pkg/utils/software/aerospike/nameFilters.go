package aerospike

import "strings"

type FileType string

const (
	FileTypeTGZ     FileType = "tgz"
	FileTypeZIP     FileType = "zip"
	FileTypeRPM     FileType = "rpm"
	FileTypeDEB     FileType = "deb"
	FileTypeEXE     FileType = "exe"
	FileTypeMSI     FileType = "msi"
	FileTypeDMG     FileType = "dmg"
	FileTypePKG     FileType = "pkg"
	FileTypeTXT     FileType = "txt"
	FileTypeGZ      FileType = "gz"
	FileTypeTAR     FileType = "tar"
	FileTypeUnknown FileType = "unknown"
)

type ArchitectureType string

const (
	ArchitectureTypeX86_64  ArchitectureType = "x86_64"
	ArchitectureTypeAARCH64 ArchitectureType = "aarch64"
	ArchitectureTypeUnknown ArchitectureType = "unknown"
)

type ProductType string

const (
	ProductTypeServer     ProductType = "server"
	ProductTypeTools      ProductType = "tools"
	ProductTypeExporter   ProductType = "exporter"
	ProductTypeBackup     ProductType = "backup"
	ProductTypeGateway    ProductType = "gateway"
	ProductTypeDashboards ProductType = "dashboards"
	ProductTypeUnknown    ProductType = "unknown"
)

type OSName string

const (
	OSNameCentOS OSName = "centos"
	OSNameDebian OSName = "debian"
	OSNameUbuntu OSName = "ubuntu"
	OSNameAmazon OSName = "amazon"
)

type NameParts struct {
	ProductName    string
	ProductType    ProductType
	ProductVersion string
	Architecture   ArchitectureType
	OSName         OSName
	OSVersion      string
	FileType       FileType
}

func (f File) ParseNameParts() *NameParts {
	name := f.Name
	if strings.HasPrefix(name, "aerospike-server-") && strings.Contains(name, "_") {
		// aerospike-server-enterprise_8.0.0.8_tools-11.2.2_ubuntu24.04_x86_64.tgz
		// aerospike-server-community_8.0.0.8_tools-11.2.2_ubuntu24.04_x86_64.tgz
		// aerospike-server-federal_8.0.0.8_tools-11.2.2_ubuntu24.04_x86_64.tgz
		parts := strings.Split(name, "_")
		if strings.Contains(name, "x86_64") {
			if len(parts) != 6 {
				return nil
			}
		} else {
			if len(parts) != 5 {
				return nil
			}
		}
		architecture := ArchitectureTypeUnknown
		switch parts[4] {
		case "x86", "amd64":
			architecture = ArchitectureTypeX86_64
		case "aarch64", "arm64":
			architecture = ArchitectureTypeAARCH64
		}
		osName := ""
		osVersion := ""
		if strings.Contains(parts[3], "ubuntu") {
			osName = "ubuntu"
			osVersion = strings.TrimPrefix(parts[3], "ubuntu")
		} else if strings.HasPrefix(parts[3], "debian") {
			osName = "debian"
			osVersion = strings.TrimPrefix(parts[3], "debian")
		} else if strings.HasPrefix(parts[3], "amzn") {
			osName = "amazon"
			osVersion = strings.TrimPrefix(parts[3], "amzn")
		} else if strings.HasPrefix(parts[3], "el") {
			osName = "centos"
			osVersion = strings.TrimPrefix(parts[3], "el")
		}
		return &NameParts{
			ProductName:    parts[0],
			ProductType:    ProductTypeServer,
			ProductVersion: parts[1],
			Architecture:   architecture,
			OSName:         OSName(osName),
			OSVersion:      osVersion,
			FileType:       fileType(name),
		}
	} else if strings.HasPrefix(name, "aerospike-server-") {
		// aerospike-server-enterprise-6.1.0.22-ubuntu20.04.tgz
		parts := strings.Split(name, "-")
		if len(parts) != 5 {
			return nil
		}
		architecture := ArchitectureTypeX86_64
		osName := ""
		osVersion := ""
		if strings.Contains(parts[4], "ubuntu") {
			osName = "ubuntu"
			osVersion = strings.TrimPrefix(parts[4], "ubuntu")
		} else if strings.HasPrefix(parts[4], "debian") {
			osName = "debian"
			osVersion = strings.TrimPrefix(parts[4], "debian")
		} else if strings.HasPrefix(parts[4], "amzn") {
			osName = "amazon"
			osVersion = strings.TrimPrefix(parts[4], "amzn")
		} else if strings.HasPrefix(parts[4], "el") {
			osName = "centos"
			osVersion = strings.TrimPrefix(parts[4], "el")
		}
		if idx := strings.LastIndex(osVersion, "."); idx != -1 {
			osVersion = osVersion[:idx]
		}
		return &NameParts{
			ProductName:    parts[0] + "-" + parts[1] + "-" + parts[2],
			ProductType:    ProductTypeServer,
			ProductVersion: parts[3],
			Architecture:   architecture,
			OSName:         OSName(osName),
			OSVersion:      osVersion,
			FileType:       fileType(name),
		}
	} else if strings.HasPrefix(name, "aerospike-tools_") {
		// aerospike-tools_11.2.2_ubuntu24.04_aarch64.tgz
		parts := strings.Split(name, "_")
		if strings.Contains(name, "x86_64") {
			if len(parts) != 5 {
				return nil
			}
		} else {
			if len(parts) != 4 {
				return nil
			}
		}
		architecture := ArchitectureTypeUnknown
		if strings.Contains(parts[3], "x86") || strings.Contains(parts[3], "amd64") {
			architecture = ArchitectureTypeX86_64
		} else if strings.Contains(parts[3], "aarch64") || strings.Contains(parts[3], "arm64") {
			architecture = ArchitectureTypeAARCH64
		}
		osName := ""
		osVersion := ""
		if strings.Contains(parts[2], "ubuntu") {
			osName = "ubuntu"
			osVersion = strings.TrimPrefix(parts[2], "ubuntu")
		} else if strings.HasPrefix(parts[2], "debian") {
			osName = "debian"
			osVersion = strings.TrimPrefix(parts[2], "debian")
		} else if strings.HasPrefix(parts[2], "amzn") {
			osName = "amazon"
			osVersion = strings.TrimPrefix(parts[2], "amzn")
		} else if strings.HasPrefix(parts[2], "el") {
			osName = "centos"
			osVersion = strings.TrimPrefix(parts[2], "el")
		}
		return &NameParts{
			ProductName:    parts[0],
			ProductType:    ProductTypeTools,
			ProductVersion: parts[1],
			Architecture:   architecture,
			OSName:         OSName(osName),
			OSVersion:      osVersion,
			FileType:       fileType(name),
		}
	} else if strings.HasPrefix(name, "aerospike-tools-") {
		// aerospike-tools-3.31.1-ubuntu20.04.tgz
		parts := strings.Split(name, "-")
		if len(parts) != 4 {
			return nil
		}
		architecture := ArchitectureTypeX86_64
		osName := ""
		osVersion := ""
		if strings.Contains(parts[3], "ubuntu") {
			osName = "ubuntu"
			osVersion = strings.TrimPrefix(parts[3], "ubuntu")
		} else if strings.Contains(parts[3], "debian") {
			osName = "debian"
			osVersion = strings.TrimPrefix(parts[3], "debian")
		} else if strings.Contains(parts[3], "amzn") {
			osName = "amazon"
			osVersion = strings.TrimPrefix(parts[3], "amzn")
		} else if strings.Contains(parts[3], "el") {
			osName = "centos"
			osVersion = strings.TrimPrefix(parts[3], "el")
		}
		if idx := strings.LastIndex(osVersion, "."); idx != -1 {
			osVersion = osVersion[:idx]
		}
		return &NameParts{
			ProductName:    parts[0] + "-" + parts[1],
			ProductType:    ProductTypeTools,
			ProductVersion: parts[2],
			Architecture:   architecture,
			OSName:         OSName(osName),
			OSVersion:      osVersion,
			FileType:       fileType(name),
		}
	} else if strings.HasPrefix(name, "aerospike-prometheus-exporter") {
		// aerospike-prometheus-exporter_1.24.0_x86_64.tgz
		// aerospike-prometheus-exporter_1.24.0_aarch64.tgz
		// aerospike-prometheus-exporter_1.24.0-1_arm64.deb
		// aerospike-prometheus-exporter_1.24.0-1_amd64.deb
		// aerospike-prometheus-exporter-1.24.0-1.x86_64.rpm
		// aerospike-prometheus-exporter-1.24.0-1.aarch64.rpm
		name = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(name, ".x86_64.rpm", "_x86_64.rpm"), ".aarch64.rpm", "_aarch64.rpm"), "aerospike-prometheus-exporter-", "aerospike-prometheus-exporter_")
		parts := strings.Split(name, "_")
		if strings.Contains(name, "x86_64") {
			if len(parts) != 4 {
				return nil
			}
		} else {
			if len(parts) != 3 {
				return nil
			}
		}
		architecture := ArchitectureTypeUnknown
		if strings.Contains(parts[2], "x86") || strings.Contains(parts[2], "amd64") {
			architecture = ArchitectureTypeX86_64
		} else if strings.Contains(parts[2], "aarch64") || strings.Contains(parts[2], "arm64") {
			architecture = ArchitectureTypeAARCH64
		}
		osName := OSName("ubuntu")
		if fileType(name) == FileTypeRPM {
			osName = OSName("centos")
		}
		return &NameParts{
			ProductName:    parts[0],
			ProductType:    ProductTypeExporter,
			ProductVersion: strings.Split(parts[1], "-")[0],
			Architecture:   architecture,
			OSName:         osName,
			OSVersion:      "",
			FileType:       fileType(name),
		}
	} else if strings.HasPrefix(name, "aerospike-grafana-dashboards-") {
		// aerospike-grafana-dashboards-3.12.0.tar.gz
		parts := strings.Split(name, "-")
		if len(parts) != 4 {
			return nil
		}
		return &NameParts{
			ProductName:    parts[0] + "-" + parts[1] + "-" + parts[2],
			ProductType:    ProductTypeDashboards,
			ProductVersion: strings.TrimSuffix(parts[3], ".tar.gz"),
			Architecture:   ArchitectureTypeUnknown,
			OSName:         "",
			OSVersion:      "",
			FileType:       fileType(name),
		}
	} else if strings.HasPrefix(name, "aerospike-backup-service") {
		// aerospike-backup-service-3.1.0-1.aarch64.rpm
		// aerospike-backup-service-3.1.0-1.x86_64.rpm
		// aerospike-backup-service_3.1.0_amd64.deb
		// aerospike-backup-service_3.1.0_arm64.deb
		name = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(name, ".x86_64.rpm", "_x86_64.rpm"), ".aarch64.rpm", "_aarch64.rpm"), "aerospike-backup-service-", "aerospike-backup-service_")
		parts := strings.Split(name, "_")
		if strings.Contains(name, "x86_64") {
			if len(parts) != 4 {
				return nil
			}
		} else {
			if len(parts) != 3 {
				return nil
			}
		}
		architecture := ArchitectureTypeUnknown
		if strings.Contains(parts[2], "x86") || strings.Contains(parts[2], "amd64") {
			architecture = ArchitectureTypeX86_64
		} else if strings.Contains(parts[2], "aarch64") || strings.Contains(parts[2], "arm64") {
			architecture = ArchitectureTypeAARCH64
		}
		osName := OSName("ubuntu")
		if fileType(name) == FileTypeRPM {
			osName = OSName("centos")
		}
		return &NameParts{
			ProductName:    parts[0],
			ProductType:    ProductTypeBackup,
			ProductVersion: strings.Split(parts[1], "-")[0],
			Architecture:   architecture,
			OSName:         osName,
			OSVersion:      "",
			FileType:       fileType(name),
		}
	}
	return nil
}

func fileType(name string) FileType {
	if strings.HasSuffix(name, ".tgz") {
		return FileTypeTGZ
	}
	if strings.HasSuffix(name, ".zip") {
		return FileTypeZIP
	}
	if strings.HasSuffix(name, ".rpm") {
		return FileTypeRPM
	}
	if strings.HasSuffix(name, ".deb") {
		return FileTypeDEB
	}
	if strings.HasSuffix(name, ".exe") {
		return FileTypeEXE
	}
	if strings.HasSuffix(name, ".msi") {
		return FileTypeMSI
	}
	if strings.HasSuffix(name, ".dmg") {
		return FileTypeDMG
	}
	if strings.HasSuffix(name, ".pkg") {
		return FileTypePKG
	}
	if strings.HasSuffix(name, ".tar.gz") {
		return FileTypeTGZ
	}
	if strings.HasSuffix(name, ".tar") {
		return FileTypeTAR
	}
	if strings.HasSuffix(name, ".gz") {
		return FileTypeGZ
	}
	if strings.HasSuffix(name, ".txt") {
		return FileTypeTXT
	}
	return FileTypeUnknown
}
