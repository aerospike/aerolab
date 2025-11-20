package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type TlsGenerateCmd struct {
	ClusterName    TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes          TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	TlsName        string          `short:"t" long:"tls-name" description:"Common Name (tlsname)" default:"tls1"`
	CaName         string          `short:"c" long:"ca-name" description:"Name of the CA certificate (file)" default:"cacert"`
	Bits           int             `short:"b" long:"cert-bits" description:"Bits size for the CA and certs" default:"2048"`
	CaExpiryDays   int             `short:"e" long:"ca-expiry-days" description:"Number of days the CA certificate should be valid for" default:"3650"`
	CertExpiryDays int             `short:"E" long:"cert-expiry-days" description:"Number of days the certificate should be valid for" default:"365"`
	NoUpload       bool            `short:"u" long:"no-upload" description:"If set, will generate certificates on the local machine but not ship them to the cluster nodes"`
	NoMesh         bool            `short:"m" long:"no-mesh" description:"If set, will not configure mesh-seed-address-port to use TLS"`
	ChDir          string          `short:"W" long:"work-dir" description:"Specify working directory. This is where all installers will download and CA certs will initially generate to." default:"."`
	Threads        int             `long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help           HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TlsGenerateCmd) Execute(args []string) error {
	cmd := []string{"tls", "generate"}
	system, err := Initialize(&Init{InitBackend: !c.NoUpload, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.GenerateTLS(system, system.Backend.GetInventory(), system.Logger, args, "generate")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	if instances != nil {
		system.Logger.Info("Generated and uploaded TLS certificates to %d instances", instances.Count())
		for _, i := range instances.Describe() {
			system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
		}
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *TlsGenerateCmd) GenerateTLS(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	// Change to working directory
	if c.ChDir != "" && c.ChDir != "." {
		if err := os.Chdir(c.ChDir); err != nil {
			return nil, fmt.Errorf("failed to change to working directory %s: %w", c.ChDir, err)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	if _, err := os.Stat("CA"); err == nil {
		logger.Info("CA directory exists, reusing existing CAs (%s/CA)", wd)
	}

	var instances backends.InstanceList
	if !c.NoUpload {
		if system == nil {
			system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"tls", action}, c, args...)
			if err != nil {
				return nil, err
			}
		}
		if inventory == nil {
			inventory = system.Backend.GetInventory()
		}

		// Support multi-cluster
		if c.ClusterName.String() == "" {
			return nil, fmt.Errorf("cluster name is required")
		}
		if strings.Contains(c.ClusterName.String(), ",") {
			clusters := strings.Split(c.ClusterName.String(), ",")
			var allInstances backends.InstanceList
			for _, cluster := range clusters {
				c.ClusterName = TypeClusterName(cluster)
				inst, err := c.GenerateTLS(system, inventory, logger, args, action)
				if err != nil {
					return nil, err
				}
				allInstances = append(allInstances, inst...)
			}
			return allInstances, nil
		}

		cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
		if cluster == nil {
			return nil, fmt.Errorf("cluster %s not found", c.ClusterName.String())
		}

		if c.Nodes.String() != "" {
			nodes, err := expandNodeNumbers(c.Nodes.String())
			if err != nil {
				return nil, err
			}
			cluster = cluster.WithNodeNo(nodes...)
			if cluster.Count() != len(nodes) {
				return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
			}
		}

		cluster = cluster.WithState(backends.LifeCycleStateRunning)
		if cluster.Count() == 0 {
			logger.Info("No running instances found for cluster %s", c.ClusterName.String())
			return nil, nil
		}

		instances = cluster.Describe()
	}

	// Generate certificates
	logger.Info("Generating TLS certificates")
	err = c.generateCertificates()
	if err != nil {
		return nil, err
	}

	// Upload certificates to cluster nodes
	if !c.NoUpload && instances.Count() > 0 {
		logger.Info("Uploading certificates to %d nodes", instances.Count())
		err = c.uploadCertificates(instances, logger)
		if err != nil {
			return nil, err
		}

		// Fix mesh configuration
		if !c.NoMesh {
			logger.Info("Configuring mesh for TLS")
			err = c.fixMeshConfig(instances, logger)
			if err != nil {
				return nil, err
			}
		}
	}

	// Print configuration snippet
	fmt.Println("\n--- aerospike.conf snippet ---")
	fmt.Printf(`network {
    tls %s {
        cert-file /etc/aerospike/ssl/%s/cert.pem
        key-file /etc/aerospike/ssl/%s/key.pem
        ca-file /etc/aerospike/ssl/%s/%s.pem
    }
    ...
`, c.TlsName, c.TlsName, c.TlsName, c.TlsName, c.CaName)
	fmt.Println("--- aerospike.conf end ---")

	return instances, nil
}

// generateCertificates generates the TLS certificates locally
func (c *TlsGenerateCmd) generateCertificates() error {
	var commands [][]string

	// Check if CA already exists
	_, errA := os.Stat(path.Join("CA", "private", c.CaName+".key"))
	_, errB := os.Stat(path.Join("CA", c.CaName+".pem"))
	if errA != nil || errB != nil {
		commands = append(commands, []string{"req", "-new", "-nodes", "-x509", "-extensions", "v3_ca", "-keyout", path.Join("private", c.CaName+".key"), "-out", c.CaName + ".pem", "-days", strconv.Itoa(c.CaExpiryDays), "-config", "openssl.cnf", "-subj", fmt.Sprintf("/C=US/ST=CA/L=Cyberspace/O=Aerolab/CN=%s", c.CaName)})
	}

	commands = append(commands, []string{"req", "-new", "-nodes", "-extensions", "v3_req", "-out", "req.pem", "-config", "openssl.cnf", "-subj", fmt.Sprintf("/C=US/ST=CA/L=Cyberspace/O=Aerolab/CN=%s", c.TlsName)})
	commands = append(commands, []string{"ca", "-batch", "-extensions", "v3_req", "-days", strconv.Itoa(c.CertExpiryDays), "-out", "cert.pem", "-config", "openssl.cnf", "-infiles", "req.pem"})

	// Create CA directory structure
	if _, err := os.Stat("CA"); err != nil {
		if err := os.Mkdir("CA", 0755); err != nil {
			return fmt.Errorf("failed to create CA directory: %w", err)
		}
	}

	if err := os.Chdir("CA"); err != nil {
		return fmt.Errorf("failed to change to CA directory: %w", err)
	}
	defer os.Chdir("..")

	// Write openssl config
	err := os.WriteFile("openssl.cnf", []byte(c.createOpensslConfig()), 0644)
	if err != nil {
		return fmt.Errorf("failed to write openssl.cnf: %w", err)
	}

	// Create subdirectories
	for _, dir := range []string{"private", "newcerts"} {
		if _, err := os.Stat(dir); err != nil {
			if err := os.Mkdir(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}

	// Create index and serial files
	if _, err := os.Stat("index.txt"); err != nil {
		if err := os.WriteFile("index.txt", []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to create index.txt: %w", err)
		}
	}
	if _, err := os.Stat("serial"); err != nil {
		if err := os.WriteFile("serial", []byte("01"), 0644); err != nil {
			return fmt.Errorf("failed to create serial: %w", err)
		}
	}

	// Execute openssl commands
	for _, command := range commands {
		out, err := exec.Command("openssl", command...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("error executing openssl command: %s\n%s\nopenssl %s", err, out, strings.Join(command, " "))
		}
	}

	return nil
}

// uploadCertificates uploads the generated certificates to cluster nodes
func (c *TlsGenerateCmd) uploadCertificates(instances backends.InstanceList, logger *logger.Logger) error {
	// Read certificate files
	certFiles := []string{"cert.pem", "key.pem", c.CaName + ".pem"}
	certContents := make(map[string][]byte)

	for _, file := range certFiles {
		content, err := os.ReadFile(path.Join("CA", file))
		if err != nil {
			return fmt.Errorf("failed to read certificate file %s: %w", file, err)
		}
		certContents[file] = content
	}

	// Get SFTP configs for all instances
	confs, err := instances.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP configs: %w", err)
	}

	type uploadTask struct {
		conf     *sshexec.ClientConf
		instance *backends.Instance
	}

	tasks := make([]uploadTask, len(confs))
	for i, conf := range confs {
		tasks[i] = uploadTask{
			conf:     conf,
			instance: instances.Describe()[i],
		}
	}

	var hasErr error
	parallelize.ForEachLimit(tasks, c.Threads, func(task uploadTask) {
		instance := task.instance

		// Create SSL directory
		output := instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"mkdir", "-p", fmt.Sprintf("/etc/aerospike/ssl/%s", c.TlsName)},
				SessionTimeout: 30 * time.Second,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		if output.Output.Err != nil {
			logger.Error("Failed to create SSL directory on %s:%d: %s", instance.ClusterName, instance.NodeNo, output.Output.Err)
			hasErr = errors.New("some nodes failed to create SSL directory")
			return
		}

		// Upload certificate files
		client, err := sshexec.NewSftp(task.conf)
		if err != nil {
			logger.Error("Failed to create SFTP client for %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed SFTP connection")
			return
		}
		defer client.Close()

		for _, file := range certFiles {
			err = client.WriteFile(true, &sshexec.FileWriter{
				DestPath:    fmt.Sprintf("/etc/aerospike/ssl/%s/%s", c.TlsName, file),
				Source:      bytes.NewReader(certContents[file]),
				Permissions: 0644,
			})
			if err != nil {
				logger.Error("Failed to upload %s to %s:%d: %s", file, instance.ClusterName, instance.NodeNo, err)
				hasErr = errors.New("some nodes failed to upload certificates")
				return
			}
		}

		logger.Debug("Uploaded certificates to %s:%d", instance.ClusterName, instance.NodeNo)
	})

	return hasErr
}

// fixMeshConfig updates the aerospike.conf to use TLS for mesh
func (c *TlsGenerateCmd) fixMeshConfig(instances backends.InstanceList, logger *logger.Logger) error {
	// Get all node IPs
	nodeIPs := make([]string, instances.Count())
	for i, instance := range instances.Describe() {
		nodeIPs[i] = instance.IP.Private
	}

	// Get SFTP configs
	confs, err := instances.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP configs: %w", err)
	}

	type meshTask struct {
		conf     *sshexec.ClientConf
		instance *backends.Instance
	}

	tasks := make([]meshTask, len(confs))
	for i, conf := range confs {
		tasks[i] = meshTask{
			conf:     conf,
			instance: instances.Describe()[i],
		}
	}

	var hasErr error
	parallelize.ForEachLimit(tasks, c.Threads, func(task meshTask) {
		instance := task.instance

		// Read current config
		client, err := sshexec.NewSftp(task.conf)
		if err != nil {
			logger.Error("Failed to create SFTP client for %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed SFTP connection")
			return
		}
		defer client.Close()

		var buf bytes.Buffer
		err = client.ReadFile(&sshexec.FileReader{
			SourcePath:  "/etc/aerospike/aerospike.conf",
			Destination: &buf,
		})
		if err != nil {
			logger.Error("Failed to read config from %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed to read config")
			return
		}

		confContent := buf.String()
		if !strings.Contains(confContent, "mode mesh") {
			logger.Debug("Node %s:%d is not using mesh mode, skipping", instance.ClusterName, instance.NodeNo)
			return
		}

		// Update config to use TLS mesh
		newConf := c.updateMeshConfig(confContent, nodeIPs)

		// Write updated config
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/etc/aerospike/aerospike.conf",
			Source:      strings.NewReader(newConf),
			Permissions: 0644,
		})
		if err != nil {
			logger.Error("Failed to write config to %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed to write config")
			return
		}

		logger.Debug("Updated mesh config on %s:%d", instance.ClusterName, instance.NodeNo)
	})

	return hasErr
}

// updateMeshConfig modifies the config to use TLS for mesh
func (c *TlsGenerateCmd) updateMeshConfig(config string, nodeIPs []string) string {
	var newConfig strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(config))

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "port 3002") && !strings.Contains(line, "tls-port") {
			// Replace with TLS port and mesh seeds
			newConfig.WriteString("\t\t\ttls-port 3012\n")
			newConfig.WriteString(fmt.Sprintf("\t\t\ttls-name %s\n", c.TlsName))
			for _, ip := range nodeIPs {
				newConfig.WriteString(fmt.Sprintf("\t\t\ttls-mesh-seed-address-port %s 3012\n", ip))
			}
		} else if strings.Contains(line, "mesh-seed-address-port") {
			// Skip old mesh-seed-address-port lines
			continue
		} else {
			newConfig.WriteString(line)
			newConfig.WriteString("\n")
		}
	}

	return newConfig.String()
}

// createOpensslConfig generates the OpenSSL configuration
func (c *TlsGenerateCmd) createOpensslConfig() string {
	conf := `#
# OpenSSL configuration file.
#

# Establish working directory.

dir			= .

[ req ]
default_bits  	    = %d		# Size of keys
default_keyfile     = key.pem		# name of generated keys
default_md          = sha256		# message digest algorithm
string_mask         = nombstr		# permitted characters
distinguished_name  = req_distinguished_name
req_extensions      = v3_req

[ req_distinguished_name ]
# Variable name		        Prompt string
#----------------------   ----------------------------------
0.organizationName        = Organization Name (company)
organizationalUnitName    = Organizational Unit Name (department, division)
emailAddress              = Email Address
emailAddress_max          = 40
localityName              = Locality Name (city, district)
stateOrProvinceName       = State or Province Name (full name)
countryName               = Country Name (2 letter code)
countryName_min           = 2
countryName_max           = 2
commonName                = Common Name (hostname, IP, or your name)
commonName_max            = 64

# Default values for the above, for consistency and less typing.
# Variable name			  Value
#------------------------------	  ------------------------------
0.organizationName_default         = Aerospike Inc
organizationalUnitName_default     = operations
emailAddress_default               = operations@aerospike.com
localityName_default               = Bangalore
stateOrProvinceName_default	   = Karnataka
countryName_default		   = IN
commonName_default                 = harvey 

[ v3_ca ]
basicConstraints	= CA:TRUE
subjectKeyIdentifier	= hash
authorityKeyIdentifier	= keyid:always,issuer:always
subjectAltName = IP:127.0.0.1

[ v3_req ]
basicConstraints	= CA:FALSE
subjectKeyIdentifier	= hash
subjectAltName = @alt_names

[alt_names]
DNS.1   = %s
IP.1 = 127.0.0.1

[ ca ]
default_ca		= CA_default

[ CA_default ]
serial			= $dir/serial
database		= $dir/index.txt
new_certs_dir		= $dir/newcerts
certificate		= $dir/%s.pem
private_key		= $dir/private/%s.key
default_days		= 365
default_md		= sha256
preserve		= no
email_in_dn		= no
nameopt			= default_ca
certopt			= default_ca
policy			= policy_match

[ policy_match ]
countryName		= match
stateOrProvinceName	= match
organizationName	= match
organizationalUnitName	= optional
commonName		= supplied
emailAddress		= optional

`
	return fmt.Sprintf(conf, c.Bits, c.TlsName, c.CaName, c.CaName)
}
