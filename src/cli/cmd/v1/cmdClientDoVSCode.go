package cmd

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/compilers"
	"github.com/aerospike/aerolab/pkg/utils/installers/vscode"
	"github.com/rglonek/logger"
)

type ClientCreateVSCodeCmd struct {
	ClientCreateNoneCmd
	VSCodePassword string `long:"vscode-password" description:"VSCode password for web access; leave empty for no password" default:""`
	Kernels        string `short:"k" long:"kernels" description:"Comma-separated list of language kernels to install; options: go,python,java,dotnet; default: all kernels" default:""`
}

func (c *ClientCreateVSCodeCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "vscode"}
	} else {
		cmd = []string{"client", "create", "vscode"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Auto-expose port 8080 for Docker backend if not already exposed
	if system.Opts.Config.Backend.Type == "docker" {
		hasPort8080 := false
		for _, port := range c.Docker.ExposePorts {
			if strings.Contains(port, ":8080") {
				hasPort8080 = true
				break
			}
		}
		if !hasPort8080 {
			system.Logger.Info("Auto-exposing port 8080 for VSCode access")
			c.Docker.ExposePorts = append([]string{"8080:8080"}, c.Docker.ExposePorts...)
		}
	}

	defer UpdateDiskCache(system)
	err = c.createVSCodeClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateVSCodeCmd) createVSCodeClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "vscode"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "vscode"
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Parse kernels to install
	kernelsToInstall := c.parseKernels()
	logger.Info("Installing VSCode Server with kernels: %v", kernelsToInstall)

	// Build compiler installation script
	compilersScript, err := c.buildCompilersScript(kernelsToInstall)
	if err != nil {
		return err
	}

	// Build VSCode extensions lists
	requiredExtensions, optionalExtensions := c.buildExtensionsList(kernelsToInstall)

	for _, client := range clients.Describe() {
		logger.Info("Installing VSCode on %s:%d", client.ClusterName, client.NodeNo)

		// Get VSCode installer
		password := c.VSCodePassword
		var passwordPtr *string
		if password != "" {
			passwordPtr = &password
		}
		bindAddr := "0.0.0.0:8080"
		defaultFolder := "/opt/code"
		vscodeScript, err := vscode.GetLinuxInstallScript(
			true,               // enable
			false,              // start (we'll start after other setup)
			passwordPtr,        // password
			&bindAddr,          // bindAddr
			requiredExtensions, // requiredExtensions
			optionalExtensions, // optionalExtensions
			true,               // patchExtensions
			&defaultFolder,     // overrideDefaultFolder
			"/root",            // userHome
			"root",             // username
		)
		if err != nil {
			logger.Warn("Failed to get VSCode installer for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Build the example code setup script
		exampleCodeScript := c.buildExampleCodeScript()

		// Combine all scripts
		fullScript := string(vscodeScript) + "\n\n" + string(compilersScript) + "\n\n" + exampleCodeScript + "\n\nsystemctl start vscode\n"

		conf, err := client.GetSftpConfig("root")
		if err != nil {
			logger.Warn("Failed to get SFTP config for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		sftpClient, err := sshexec.NewSftp(conf)
		if err != nil {
			logger.Warn("Failed to create SFTP client for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install-vscode-full.sh",
			Source:      strings.NewReader(fullScript),
			Permissions: 0755,
		})
		sftpClient.Close()
		if err != nil {
			logger.Warn("Failed to upload VSCode installer to %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Execute installer
		logger.Info("Running installation script on %s:%d (this may take several minutes)", client.ClusterName, client.NodeNo)

		// If debug level is selected, output to stdout/stderr and enable terminal mode
		var stdout, stderr *os.File
		var stdin io.ReadCloser
		terminal := false
		if system.logLevel >= 5 {
			stdout = os.Stdout
			stderr = os.Stderr
			stdin = io.NopCloser(os.Stdin)
			terminal = true
		}
		execDetail := sshexec.ExecDetail{
			Command:        []string{"bash", "/tmp/install-vscode-full.sh"},
			SessionTimeout: 30 * time.Minute,
			Terminal:       terminal,
		}
		if system.logLevel >= 5 {
			execDetail.Stdin = stdin
			execDetail.Stdout = stdout
			execDetail.Stderr = stderr
		}

		output := client.Exec(&backends.ExecInput{
			ExecDetail:     execDetail,
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			logger.Warn("Failed to install VSCode on %s:%d: %s", client.ClusterName, client.NodeNo, output.Output.Err)
			logger.Warn("stdout: %s", output.Output.Stdout)
			logger.Warn("stderr: %s", output.Output.Stderr)
		} else {
			logger.Info("Successfully installed VSCode on %s:%d", client.ClusterName, client.NodeNo)

			// Determine access URL based on backend type
			var accessHost string
			var accessPort string = "8080"

			if system.Opts.Config.Backend.Type == "docker" {
				accessHost = "localhost"
				// Parse exposed ports to find the host port for container port 8080
				for _, port := range c.Docker.ExposePorts {
					// Format: [+]hostPort:containerPort or host=IP:hostPort,container=containerPort,incr
					if strings.Contains(port, ":8080") {
						// Simple format: hostPort:containerPort
						parts := strings.Split(strings.TrimPrefix(port, "+"), ":")
						if len(parts) == 2 {
							accessPort = parts[0]
							break
						}
					}
				}
			} else {
				accessHost = client.IP.Public
			}

			accessInfo := "Access VSCode at: http://" + accessHost + ":" + accessPort
			if c.VSCodePassword != "" {
				accessInfo += " (password: " + c.VSCodePassword + ")"
			} else {
				accessInfo += " (no password)"
			}
			logger.Info(accessInfo)
		}
	}

	return nil
}

// parseKernels parses the kernels parameter and returns a list of kernels to install.
// If empty or "all", returns all supported kernels.
func (c *ClientCreateVSCodeCmd) parseKernels() []string {
	if c.Kernels == "" || c.Kernels == "all" {
		return []string{"go", "python", "java", "dotnet"}
	}
	return strings.Split(c.Kernels, ",")
}

// buildCompilersScript builds the compiler installation script based on selected kernels.
func (c *ClientCreateVSCodeCmd) buildCompilersScript(kernelsToInstall []string) ([]byte, error) {
	var compilersToInstall []compilers.Compiler
	var openjdkVersion string

	for _, kernel := range kernelsToInstall {
		kernel = strings.TrimSpace(kernel)
		switch kernel {
		case "go":
			compilersToInstall = append(compilersToInstall, compilers.CompilerGo)
		case "python":
			compilersToInstall = append(compilersToInstall, compilers.CompilerPython3)
		case "java":
			// Build essentials includes java
			openjdkVersion = "21"
			compilersToInstall = append(compilersToInstall, compilers.CompilerBuildEssentials)
		case "dotnet":
			compilersToInstall = append(compilersToInstall, compilers.CompilerDotnet)
		}
	}

	if len(compilersToInstall) == 0 {
		return []byte{}, nil
	}

	// Add aerospike client for python if python is being installed
	pythonPipPackages := []string{}
	for _, kernel := range kernelsToInstall {
		if kernel == "python" {
			pythonPipPackages = []string{"aerospike"}
			break
		}
	}

	return compilers.GetInstallScript(
		compilersToInstall,
		openjdkVersion,    // openjdk version for java
		"",                // dotnet version (empty = latest)
		"",                // go version (empty = latest)
		pythonPipPackages, // required pip packages
		[]string{},        // optional pip packages
		[]string{},        // extra apt packages
		[]string{},        // extra yum packages
	)
}

// buildExtensionsList builds the VSCode extensions list based on selected kernels.
func (c *ClientCreateVSCodeCmd) buildExtensionsList(kernelsToInstall []string) (required []string, optional []string) {
	required = []string{}
	optional = []string{}

	for _, kernel := range kernelsToInstall {
		kernel = strings.TrimSpace(kernel)
		switch kernel {
		case "go":
			required = append(required, "golang.go")
		case "python":
			required = append(required, "ms-python.python")
		case "java":
			required = append(required,
				"redhat.java",
				"vscjava.vscode-java-debug",
				"vscjava.vscode-maven",
				"vscjava.vscode-java-dependency",
				"vscjava.vscode-java-test",
			)
		case "dotnet":
			required = append(required,
				"ms-dotnettools.vscode-dotnet-runtime",
				"ms-dotnettools.csharp",
			)
		}
	}

	return required, optional
}

// buildExampleCodeScript builds the script to set up example code from GitHub.
func (c *ClientCreateVSCodeCmd) buildExampleCodeScript() string {
	return `
# Setup example code from GitHub
mkdir -p /opt/code
cd /opt/code
git clone -b code-server-examples --depth 1 https://github.com/aerospike/aerolab.git temp || true
if [ -d temp ]; then
    mv temp/* . 2>/dev/null || true
    mv temp/.vscode . 2>/dev/null || true
    rm -rf temp
fi

# Install Maven for Java if needed
if command -v java &> /dev/null; then
    if ! command -v mvn &> /dev/null; then
        cd /tmp
        wget -q https://dlcdn.apache.org/maven/maven-3/3.9.9/binaries/apache-maven-3.9.9-bin.tar.gz || true
        if [ -f apache-maven-3.9.9-bin.tar.gz ]; then
            tar xzf apache-maven-3.9.9-bin.tar.gz
            mkdir -p /usr/share/maven
            cp -r apache-maven-3.9.9/* /usr/share/maven/
            ln -s /usr/share/maven/bin/mvn /usr/bin/mvn
        fi
    fi
fi

# Install Go tools if Go is installed
if command -v go &> /dev/null; then
    export PATH=$PATH:/usr/local/go/bin:/root/go/bin
    go install github.com/cweill/gotests/gotests@latest || true
    go install github.com/fatih/gomodifytags@latest || true
    go install github.com/josharian/impl@latest || true
    go install github.com/haya14busa/goplay/cmd/goplay@latest || true
    go install github.com/go-delve/delve/cmd/dlv@latest || true
    go install honnef.co/go/tools/cmd/staticcheck@latest || true
    go install golang.org/x/tools/gopls@latest || true
    go install github.com/ramya-rao-a/go-outline@latest || true
fi

# Setup dotnet tools if dotnet is installed
if command -v dotnet &> /dev/null; then
    export DOTNET_ROOT=/usr/local/dotnet
    dotnet tool install --global Microsoft.dotnet-interactive --version 1.0.556801 || true
    if [ -d /opt/code/dotnet ]; then
        cd /opt/code/dotnet && dotnet restore || true
    fi
fi

chown -R root:root /opt/code
`
}
