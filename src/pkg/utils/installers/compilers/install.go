package compilers

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed scripts
var scripts embed.FS

type Compiler string

const (
	CompilerGo              Compiler = "go"
	CompilerPython3         Compiler = "python3"
	CompilerDotnet          Compiler = "dotnet"
	CompilerBuildEssentials Compiler = "build-essentials"
)

// openjdkVersion - ex 21 ; leave empty to not install openjdk
// openjdk only installs with build-essentials
// dotnetVersion - ex 9.0 ; empty - latest version will be installed
// goVersion - ex 1.23.4 ; empty - latest version will be installed
// python3RequiredPipPackages - ex []string{"pip", "setuptools", "wheel"} - extra packages to install with pip3 - or fail
// python3OptionalPipPackages - ex []string{"numpy", "pandas"} - extra packages to install with pip3 ; continue if install fails
// python3ExtraAptPackages - ex []string{"libssl-dev"} - extra packages to install with apt before installing with pip3
// python3ExtraYumPackages - ex []string{"openssl-devel"} - extra packages to install with yum before installing with pip3
func GetInstallScript(compilers []Compiler, openjdkVersion string, dotnetVersion string, goVersion string, python3RequiredPipPackages []string, python3OptionalPipPackages []string, python3ExtraAptPackages []string, python3ExtraYumPackages []string) ([]byte, error) {
	fullScript := []byte{}
	for _, compiler := range compilers {
		switch compiler {
		case CompilerGo:
			if goVersion == "" {
				var err error
				goVersion, err = LatestGoVersion()
				if err != nil {
					return nil, err
				}
			}
			if !strings.HasPrefix(goVersion, "go") {
				goVersion = "go" + goVersion
			}
			script, err := processTemplate("scripts/golang.sh.tpl", map[string]any{
				"GoVersion": goVersion,
			})
			if err != nil {
				return nil, err
			}
			fullScript = append(fullScript, script...)
		case CompilerPython3:
			script, err := processTemplate("scripts/python3.sh.tpl", map[string]any{
				"RequiredPipPackages": python3RequiredPipPackages,
				"OptionalPipPackages": python3OptionalPipPackages,
				"ExtraAptPackages":    python3ExtraAptPackages,
				"ExtraYumPackages":    python3ExtraYumPackages,
			})
			if err != nil {
				return nil, err
			}
			fullScript = append(fullScript, script...)
		case CompilerDotnet:
			if dotnetVersion == "" {
				var err error
				dotnetVersion, err = GetLatestDotnetChannelVersion()
				if err != nil {
					return nil, err
				}
			}
			script, err := processTemplate("scripts/dotnet.sh.tpl", map[string]any{
				"DotnetVersion": dotnetVersion,
			})
			if err != nil {
				return nil, err
			}
			fullScript = append(fullScript, script...)
		case CompilerBuildEssentials:
			java := map[string]any{
				"InstallJavaApt": "",
				"InstallJavaYum": "",
			}
			if openjdkVersion != "" {
				java = map[string]any{
					"InstallJavaApt": fmt.Sprintf("openjdk-%s-jre-headless", openjdkVersion),
					"InstallJavaYum": fmt.Sprintf("java-%s-openjdk-headless", openjdkVersion),
				}
			}

			script, err := processTemplate("scripts/buildessential.sh.tpl", java)
			if err != nil {
				return nil, err
			}
			fullScript = append(fullScript, script...)
		default:
			return nil, fmt.Errorf("unknown compiler: %s", compiler)
		}
	}
	return fullScript, nil
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
