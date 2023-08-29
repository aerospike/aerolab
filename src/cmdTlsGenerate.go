package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
)

type tlsGenerateCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name/Client group" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	IsClient        bool            `short:"C" long:"client" description:"set to indicate the certficates should end up on client groups"`
	TlsName         string          `short:"t" long:"tls-name" description:"Common Name (tlsname)" default:"tls1"`
	CaName          string          `short:"c" long:"ca-name" description:"Name of the CA certificate(file)" default:"cacert"`
	Bits            int             `short:"b" long:"cert-bits" description:"Bits size for the CA and certs" default:"2048"`
	NoUpload        bool            `short:"u" long:"no-upload" description:"If set, will generate certificates on the local machine but not ship them to the cluster nodes"`
	NoMesh          bool            `short:"m" long:"no-mesh" description:"If set, will not configure mesh-seed-address-port to use TLS"`
	ChDir           string          `short:"W" long:"work-dir" description:"Specify working directory. This is where all installers will download and CA certs will initially generate to."`
	ParallelThreads int             `short:"T" long:"threads" description:"Use this many threads in parallel when uploading the certificates to nodes" default:"50"`
	Help            helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *tlsGenerateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	err := chDir(c.ChDir)
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	if _, err := os.Stat("CA"); err == nil {
		log.Printf("CA directory exists, reusing existing CAs (%s/CA)", wd)
	}
	// get backend
	log.Print("Generating TLS certificates and reconfiguring hosts")

	if c.IsClient {
		b.WorkOnClients()
	}
	var nodes []int
	if !c.NoUpload {
		// check cluster exists already
		clusterList, err := b.ClusterList()
		if err != nil {
			return err
		}

		if !inslice.HasString(clusterList, string(c.ClusterName)) {
			err = fmt.Errorf("error, cluster does not exist: %s", c.ClusterName)
			return err
		}
		err = c.Nodes.ExpandNodes(string(c.ClusterName))
		if err != nil {
			return err
		}
		var nodeList []int
		nodeList, err = b.NodeListInCluster(string(c.ClusterName))
		if err != nil {
			return err
		}
		if c.Nodes == "" {
			nodes = nodeList
		} else {
			for _, nodeString := range strings.Split(c.Nodes.String(), ",") {
				nodeInt, err := strconv.Atoi(nodeString)
				if err != nil {
					return err
				}
				nodes = append(nodes, nodeInt)
			}
			for _, i := range nodes {
				if !inslice.HasInt(nodeList, i) {
					return fmt.Errorf("node %d does not exist", i)
				}
			}
		}
	}
	// we have 'nodes' var with list of nodes to install the cert on

	var commands [][]string
	comm := "openssl"
	_, errA := os.Stat(path.Join("CA", "private", c.CaName+".key"))
	_, errB := os.Stat(path.Join("CA", c.CaName+".pem"))
	if errA != nil || errB != nil {
		commands = append(commands, []string{"req", "-new", "-nodes", "-x509", "-extensions", "v3_ca", "-keyout", path.Join("private", c.CaName+".key"), "-out", c.CaName + ".pem", "-days", "3650", "-config", "openssl.cnf", "-subj", fmt.Sprintf("/C=US/ST=Denial/L=Springfield/O=Dis/CN=%s", c.CaName)})
	}
	commands = append(commands, []string{"req", "-new", "-nodes", "-extensions", "v3_req", "-out", "req.pem", "-config", "openssl.cnf", "-subj", fmt.Sprintf("/C=US/ST=Denial/L=Springfield/O=Dis/CN=%s", c.TlsName)})
	commands = append(commands, []string{"ca", "-batch", "-extensions", "v3_req", "-out", "cert.pem", "-config", "openssl.cnf", "-infiles", "req.pem"})
	//os.RemoveAll("CA")
	if _, err := os.Stat("CA"); err != nil {
		os.Mkdir("CA", 0755)
	}
	err = os.Chdir("CA")
	if err != nil {
		return err
	}
	err = os.WriteFile("openssl.cnf", []byte(tls_create_openssl_config(c.TlsName, c.CaName, c.Bits)), 0644)
	if err != nil {
		return err
	}
	for _, i := range []string{"private", "newcerts"} {
		//os.RemoveAll(i)
		if _, err := os.Stat(i); err != nil {
			err = os.Mkdir(i, 0755)
			if err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat("index.txt"); err != nil {
		err = os.WriteFile("index.txt", []byte{}, 0644)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat("serial"); err != nil {
		err = os.WriteFile("serial", []byte("01"), 0644)
		if err != nil {
			return err
		}
	}
	for _, command := range commands {
		out, err := exec.Command(comm, command...).CombinedOutput()
		if checkExecRetcode(err) != 0 {
			return fmt.Errorf("error executing command: %s\n%s\nopenssl %s", err, out, strings.Join(command, " "))
		}
	}

	if !c.NoUpload {
		if c.ParallelThreads == 1 || len(nodes) == 1 {
			_, err = b.RunCommands(string(c.ClusterName), [][]string{{"mkdir", "-p", fmt.Sprintf("/etc/aerospike/ssl/%s", c.TlsName)}}, nodes)
			if err != nil {
				return fmt.Errorf("could not mkdir ssl location: %s", err)
			}

			files := []string{"cert.pem", "key.pem", c.CaName + ".pem"}
			fl := []fileList{}
			for _, file := range files {
				ct, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				fl = append(fl, fileList{fmt.Sprintf("/etc/aerospike/ssl/%s/%s", c.TlsName, file), string(ct), len(ct)})
			}
			err = b.CopyFilesToCluster(string(c.ClusterName), fl, nodes)
			if err != nil {
				return err
			}
		} else {
			parallel := make(chan int, c.ParallelThreads)
			hasError := make(chan bool, len(nodes))
			wait := new(sync.WaitGroup)
			for _, node := range nodes {
				parallel <- 1
				wait.Add(1)
				go func(node int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
					defer func() {
						<-parallel
						wait.Done()
					}()
					_, err := b.RunCommands(string(c.ClusterName), [][]string{{"mkdir", "-p", fmt.Sprintf("/etc/aerospike/ssl/%s", c.TlsName)}}, []int{node})
					if err != nil {
						log.Printf("could not mkdir ssl location: %s", err)
						hasError <- true
					}
					files := []string{"cert.pem", "key.pem", c.CaName + ".pem"}
					fl := []fileList{}
					for _, file := range files {
						ct, err := os.ReadFile(file)
						if err != nil {
							log.Println(err)
							hasError <- true
						}
						fl = append(fl, fileList{fmt.Sprintf("/etc/aerospike/ssl/%s/%s", c.TlsName, file), string(ct), len(ct)})
					}
					err = b.CopyFilesToCluster(string(c.ClusterName), fl, []int{node})
					if err != nil {
						log.Println(err)
						hasError <- true
					}
				}(node, parallel, wait, hasError)
			}
			wait.Wait()
			if len(hasError) > 0 {
				return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
			}
		}
	}

	os.Chdir("..")

	if !c.NoUpload && !c.NoMesh {
		//for each node, read config
		var nodeIps []string
		nodeIps, err = b.GetClusterNodeIps(string(c.ClusterName))
		if err != nil {
			return err
		}
		if c.ParallelThreads == 1 || len(nodes) == 1 {
			for _, node := range nodes {
				err = c.fixMesh(node, nodeIps)
				if err != nil {
					return err
				}
			}
		} else {
			parallel := make(chan int, c.ParallelThreads)
			hasError := make(chan bool, len(nodes))
			wait := new(sync.WaitGroup)
			for _, node := range nodes {
				parallel <- 1
				wait.Add(1)
				go c.fixMeshParallel(node, nodeIps, parallel, wait, hasError)
			}
			wait.Wait()
			if len(hasError) > 0 {
				return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
			}
		}
	}
	fmt.Println("--- aerospike.conf snippet ---")
	fmt.Printf(`network {
    tls tls1 {
		cert-file /etc/aerospike/ssl/%s/cert.pem
		key-file /etc/aerospike/ssl/%s/key.pem
		ca-file /etc/aerospike/ssl/%s/%s.pem
	}
	...
`, c.TlsName, c.TlsName, c.TlsName, c.CaName)
	fmt.Println("--- aerospike.conf end ---")
	log.Print("Done")
	return nil
}

func (c *tlsGenerateCmd) fixMeshParallel(node int, nodeIps []string, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.fixMesh(node, nodeIps)
	if err != nil {
		log.Printf("ERROR getting logs from node %d: %s", node, err)
		hasError <- true
	}
}

func (c *tlsGenerateCmd) fixMesh(node int, nodeIps []string) error {
	var r [][]string
	r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
	conf, err := b.RunCommands(string(c.ClusterName), r, []int{node})
	if err != nil {
		return err
	}
	if strings.Contains(string(conf[0]), "mode mesh") {
		//enable tls for mesh
		newconf := ""
		scanner := bufio.NewScanner(strings.NewReader(string(conf[0])))
		for scanner.Scan() {
			t := scanner.Text()
			t = strings.Trim(t, "\r")
			if strings.Contains(t, "port 3002") && !strings.Contains(t, "tls-port") {
				t = "tls-port 3012\ntls-name " + c.TlsName + "\n"
				for _, nodeIp := range nodeIps {
					//t = t + fmt.Sprintf("mesh-seed-address-port %s 3002\n", mesh_ip_list[j])
					t = t + fmt.Sprintf("tls-mesh-seed-address-port %s 3012\n", nodeIp)
				}
			} else if strings.Contains(t, "mesh-seed-address-port") {
				t = ""
			}
			if strings.TrimSpace(t) != "" {
				newconf = newconf + "\n" + t
			}
		}
		err = b.CopyFilesToCluster(string(c.ClusterName), []fileList{{"/etc/aerospike/aerospike.conf", newconf, len(newconf)}}, []int{node})
		if err != nil {
			return err
		}
	}
	return nil
}

func tls_create_openssl_config(tlsName string, caName string, bits int) string {
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
	return fmt.Sprintf(conf, bits, tlsName, caName, caName)
}
