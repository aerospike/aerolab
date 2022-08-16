package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func (c *config) F_genTlsCerts() (ret int64, err error) {

	ret, err = chDir(c.GenTlsCerts.ChDir)
	if err != nil {
		return ret, err
	}

	// get backend
	c.log.Info("Generating TLS certificates and reconfiguring hosts")
	b, err := getBackend(c.GenTlsCerts.DeployOn, c.GenTlsCerts.RemoteHost, c.GenTlsCerts.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	if inArray(clusterList, c.GenTlsCerts.ClusterName) == -1 {
		err = fmt.Errorf("Error, cluster does not exist: %s", c.GenTlsCerts.ClusterName)
		ret = E_BACKEND_ERROR
		return ret, err
	}

	var nodes []int
	var nodeList []int
	nodeList, err = b.NodeListInCluster(c.GenTlsCerts.ClusterName)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	if c.GenTlsCerts.Nodes == "" {
		nodes = nodeList
	} else {
		for _, nodeString := range strings.Split(c.GenTlsCerts.Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			nodes = append(nodes, nodeInt)
		}
		for _, i := range nodes {
			if inArray(nodeList, i) == -1 {
				ret = E_BACKEND_ERROR
				return ret, fmt.Errorf("Node %d does not exist", i)
			}
		}
	}

	// we have 'nodes' var with list of nodes to install the cert on

	var commands [][]string
	comm := "openssl"
	commands = append(commands, []string{"req", "-new", "-nodes", "-x509", "-extensions", "v3_ca", "-keyout", "private/cakey.pem", "-out", "cacert.pem", "-days", "3650", "-config", "./openssl.cnf", "-subj", fmt.Sprintf("/C=US/ST=Denial/L=Springfield/O=Dis/CN=%s", c.GenTlsCerts.TlsName)})
	commands = append(commands, []string{"req", "-new", "-nodes", "-extensions", "v3_req", "-out", "req.pem", "-config", "./openssl.cnf", "-subj", fmt.Sprintf("/C=US/ST=Denial/L=Springfield/O=Dis/CN=%s", c.GenTlsCerts.TlsName)})
	commands = append(commands, []string{"ca", "-batch", "-extensions", "v3_req", "-out", "cert.pem", "-config", "./openssl.cnf", "-infiles", "req.pem"})
	os.RemoveAll("CA")
	os.Mkdir("CA", 0755)
	os.Chdir("./CA")
	os.WriteFile("openssl.cnf", []byte(tls_create_openssl_config(c.GenTlsCerts.TlsName)), 0644)
	for _, i := range []string{"private", "newcerts"} {
		os.RemoveAll(i)
		os.Mkdir(i, 0755)
	}
	os.WriteFile("index.txt", []byte{}, 0644)
	os.WriteFile("serial", []byte("01"), 0644)
	for _, command := range commands {
		out, err := exec.Command(comm, command...).CombinedOutput()
		if checkExecRetcode(err) != 0 {
			ret = 999
			return ret, fmt.Errorf("ERROR executing command: %s\n%s\nopenssl %s\n", err, out, strings.Join(command, " "))
		}
	}

	_, err = b.RunCommand(c.GenTlsCerts.ClusterName, [][]string{[]string{"mkdir", "-p", fmt.Sprintf("/etc/aerospike/ssl/%s", c.GenTlsCerts.TlsName)}}, nodes)
	if err != nil {
		return 1, fmt.Errorf("Could not mkdir ssl location: %s", err)
	}
	files := []string{"cert.pem", "cacert.pem", "key.pem"}
	fl := []fileList{}
	for _, file := range files {
		ct, _ := os.ReadFile(file)
		fl = append(fl, fileList{fmt.Sprintf("/etc/aerospike/ssl/%s/%s", c.GenTlsCerts.TlsName, file), ct})
	}
	b.CopyFilesToCluster(c.GenTlsCerts.ClusterName, fl, nodes)

	os.Chdir("..")

	//for each node, read config
	var nodeIps []string
	nodeIps, err = b.GetClusterNodeIps(c.GenTlsCerts.ClusterName)
	if err != nil {
		return 999, err
	}
	var r [][]string
	r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
	var conf [][]byte
	for _, node := range nodes {
		conf, err = b.RunCommand(c.GenTlsCerts.ClusterName, r, []int{node})
		if strings.Contains(string(conf[0]), "mode mesh") {
			//enable tls for mesh
			newconf := ""
			scanner := bufio.NewScanner(strings.NewReader(string(conf[0])))
			for scanner.Scan() {
				t := scanner.Text()
				t = strings.Trim(t, "\r")
				if strings.Contains(t, "port 3002") && !strings.Contains(t, "tls-port") {
					t = "tls-port 3012\ntls-name " + c.GenTlsCerts.TlsName + "\n"
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
			err = b.CopyFilesToCluster(c.GenTlsCerts.ClusterName, []fileList{fileList{"/etc/aerospike/aerospike.conf", []byte(newconf)}}, []int{node})
			if err != nil {
				return 999, err
			}
		}
	}
	c.log.Info(INFO_DONE)
	return
}

func tls_create_openssl_config(tlsName string) string {
	conf := `#
# OpenSSL configuration file.
#

# Establish working directory.

dir			= .

[ req ]
default_bits  	    = 2048		# Size of keys
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
certificate		= $dir/cacert.pem
private_key		= $dir/private/cakey.pem
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
	return fmt.Sprintf(conf, tlsName)
}
