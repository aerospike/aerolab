package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type dlVersion struct {
	distroName    string
	distroVersion string
	url           string
	isArm         bool
}

func fixClusterNameConfig(conf string, cluster_name string) (newconf string, err error) {
	newconf = ""
	changed := false
	service_stenza := 0 // 0 - before, 1 - in, 2 - after
	network_stenza := false
	t := ""

	scanner := bufio.NewScanner(strings.NewReader(string(conf)))

	for scanner.Scan() {
		t = scanner.Text()
		if !changed && service_stenza < 2 && !network_stenza {

			// network has a "service" stenza as well, exclude it
			if strings.Contains(t, "network") {
				network_stenza = true
			} else if network_stenza &&
				(strings.Contains(t, "security") ||
					strings.Contains(t, "logging") ||
					strings.Contains(t, "xdr") ||
					strings.Contains(t, "namespace") ||
					strings.Contains(t, "mod-lua")) {
				network_stenza = false
			}

			// only add cluster name if this is the level 0 service and not network's service
			if strings.Contains(t, "service") && !network_stenza {
				service_stenza = 1
			}

			if service_stenza == 1 {
				// cluster name in the config file
				if strings.Contains(t, "cluster-name") {
					t = fmt.Sprintf("cluster-name %s", cluster_name)
					changed = true
				}
				// service stenza without cluster name
				if strings.Contains(t, "}") {
					service_stenza = 2

					// handle "service {}" - edge case
					if strings.Contains(t, "service") {
						t = fmt.Sprintf("service {\ncluster-name %s\n}", cluster_name)
					} else {
						t = fmt.Sprintf("cluster-name %s\n}", cluster_name)
					}

					changed = true
				}
			}
		}
		if strings.TrimSpace(t) != "" {
			newconf = newconf + "\n" + t
		}
	}

	// config file without a service stenza, add it
	if !changed {
		t = fmt.Sprintf("service {\n\tcluster-name %s \n}", cluster_name)
		newconf = newconf + "\n" + t
	}

	return newconf, nil

}

func fixAerospikeConfig(conf string, mgroup string, mesh string, mesh_ip_list []string, node_list []int) (newconf string, err error) {
	if mesh == "mcast" && mgroup != "" {
		newconf = ""
		changed := false
		scanner := bufio.NewScanner(strings.NewReader(string(conf)))
		for scanner.Scan() {
			t := scanner.Text()
			if strings.Contains(t, "multicast-group") {
				t = fmt.Sprintf("multicast-group %s", mgroup)
				changed = true
			}
			newconf = newconf + "\n" + t
		}
		if !changed {
			err = errors.New(fmt.Sprintln("WARNING: Could not nodify multicast-group in the config file, search failed"))
			return conf, err
		}
	} else if mesh == "mesh" {
		for range node_list {
			changed := 0
			added_mesh_list := false
			newconf = ""
			scanner := bufio.NewScanner(strings.NewReader(string(conf)))
			for scanner.Scan() {
				t := scanner.Text()
				t = strings.Trim(t, "\r")
				if strings.Contains(t, "multicast-group") {
					t = ""
					changed = changed + 1
				} else if strings.Contains(t, "mode multicast") {
					t = "mode mesh"
					changed = changed + 1
				} else if strings.Contains(t, "mode mesh") {
					changed = changed + 2
				} else if strings.Contains(t, "mesh-seed-address-port") {
					t = ""
				} else if strings.Contains(t, "port 9918") {
					t = "port 3002\n"
					for j := 0; j < len(mesh_ip_list); j++ {
						t = t + fmt.Sprintf("mesh-seed-address-port %s 3002\n", mesh_ip_list[j])
					}
					added_mesh_list = true
				} else if strings.Contains(t, "port 3002") {
					t = "port 3002\n"
					for j := 0; j < len(mesh_ip_list); j++ {
						t = t + fmt.Sprintf("mesh-seed-address-port %s 3002\n", mesh_ip_list[j])
					}
					added_mesh_list = true
				} else if strings.Contains(t, "tls-port 3012") {
					t = "tls-port 3012\n"
					for j := 0; j < len(mesh_ip_list); j++ {
						t = t + fmt.Sprintf("tls-mesh-seed-address-port %s 3012\n", mesh_ip_list[j])
					}
					added_mesh_list = true
				}
				if strings.TrimSpace(t) != "" {
					newconf = newconf + "\n" + t
				}
			}
			if changed < 2 {
				err = errors.New(fmt.Sprintln("WARNING: Tried removing multicast-group and changing 'mode multicast' to 'mode mesh'. One of those ops failed"))
				return "", err
			}
			if !added_mesh_list {
				err = errors.New(fmt.Sprintln("WARNING: Could not locate line stating 'port 9918' in pleace of which we would put 'port 3002' and mesh address list. Mesh config has no nodes added!!!"))
				return "", err
			}
		}
	} else if mesh == "default" {
		newconf = conf
	}
	return newconf, nil
}

func VersionCheck(v1 string, v2 string) int {
	if v1 == v2 {
		return 0
	}
	v1a, t1 := VersionFromString(v1)
	v2a, t2 := VersionFromString(v2)
	for i := range v1a {
		if len(v2a) > i {
			if v1a[i] > v2a[i] {
				return -1
			}
			if v2a[i] > v1a[i] {
				return 1
			}
		}
	}
	if len(v1a) > len(v2a) {
		return -1
	}
	if len(v1a) < len(v2a) {
		return 1
	}
	if t1 == t2 {
		return 0
	}
	if t1 == "" {
		return -1
	}
	if t2 == "" {
		return 1
	}
	if t1 > t2 {
		return -1
	}
	if t2 > t1 {
		return 1
	}
	return 0
}

func VersionFromString(v string) (vv []int, tail string) {
	vlist := strings.Split(strings.ReplaceAll(v, "-", "."), ".")
	for i, c := range vlist {
		no, err := strconv.Atoi(c)
		if err != nil {
			tail = strings.Join(vlist[i:], ".")
			return
		} else {
			vv = append(vv, no)
		}
	}
	return
}

func aerospikeGetUrl(bv *backendVersion, user string, pass string) (url string, err error) {
	var version string
	url, version, err = aeroFindUrl(bv.aerospikeVersion, user, pass)
	if err != nil {
		if strings.Contains(fmt.Sprintf("%s", err), "401") {
			err = fmt.Errorf("%s, Unauthorized access, check enterprise download username and password", err)
		}
		return
	}
	bv.aerospikeVersion = version

	// resolve latest available distro version for the given aerospike version
	installers, err := aeroFindInstallers(url, user, pass)
	if err != nil {
		return url, err
	}

	if bv.distroVersion != "latest" {
		for _, installer := range installers {
			if simulateArmInstaller && bv.isArm {
				installer.isArm = bv.isArm
			}
			if installer.isArm != bv.isArm {
				continue
			}
			if installer.distroName != bv.distroName {
				continue
			}
			if installer.distroVersion != bv.distroVersion {
				continue
			}
			url = installer.url
			return
		}
		err = errors.New("installer for given OS:VERSION:Architecture not found")
		return
	}

	nver := -1
	found := &dlVersion{}
	for _, installer := range installers {
		if simulateArmInstaller && bv.isArm {
			installer.isArm = bv.isArm
		}
		if installer.isArm != bv.isArm {
			continue
		}
		if installer.distroName != bv.distroName {
			continue
		}
		nv, err := strconv.Atoi(strings.ReplaceAll(installer.distroVersion, ".", ""))
		if err != nil {
			return url, err
		}
		if nver >= nv {
			continue
		}
		nver = nv
		found = installer
	}
	if nver < 0 {
		return url, errors.New("could not determine best OS version for given aerospike version")
	}
	bv.distroVersion = found.distroVersion
	url = found.url
	return
}

func aeroFindInstallers(baseUrl string, user string, pass string) ([]*dlVersion, error) {
	if !strings.HasSuffix(baseUrl, "/") {
		baseUrl = baseUrl + "/"
	}
	ret := []*dlVersion{}
	client := &http.Client{}
	req, err := http.NewRequest("GET", baseUrl, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 200 {
		req.SetBasicAuth(user, pass)
		response, err = client.Do(req)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != 200 {
			err = fmt.Errorf("error code: %d, URL: %s", response.StatusCode, baseUrl)
			return nil, err
		}
	}

	s := bufio.NewScanner(response.Body)
	for s.Scan() {
		line := strings.Trim(s.Text(), "\t\r\n")
		ind := strings.Index(line, "<a href=\"aerospike-")
		if ind < 0 {
			continue
		}
		line = line[ind+9:]
		ind = strings.Index(line, "\">")
		line = line[:ind]
		if !strings.HasSuffix(line, ".tgz") {
			continue
		}
		dlv := &dlVersion{
			url: baseUrl + line,
		}
		dlv.isArm = false
		if strings.HasSuffix(line, ".arm64.tgz") || strings.HasSuffix(line, ".aarch64.tgz") || strings.HasSuffix(line, "_arm64.tgz") || strings.HasSuffix(line, "_aarch64.tgz") {
			dlv.isArm = true
		}
		bothArch := false
		if !strings.HasSuffix(line, ".x86_64.tgz") && !strings.HasSuffix(line, ".arm64.tgz") && !strings.HasSuffix(line, ".aarch64.tgz") && !strings.HasSuffix(line, ".amd64.tgz") && !strings.HasSuffix(line, "_x86_64.tgz") && !strings.HasSuffix(line, "_arm64.tgz") && !strings.HasSuffix(line, "_aarch64.tgz") && !strings.HasSuffix(line, "_amd64.tgz") {
			bothArch = true
		}
		line = strings.TrimSuffix(line[strings.LastIndex(line, "-")+1:], ".tgz")
		underscore := strings.Index(line, "_") + 1
		if len(line) < underscore+1 {
			continue
		}
		line = line[underscore:]
		line = strings.TrimSuffix(line, ".x86_64")
		line = strings.TrimSuffix(line, ".arm64")
		line = strings.TrimSuffix(line, ".aarch64")
		line = strings.TrimSuffix(line, ".amd64")
		line = strings.TrimSuffix(line, "_x86_64")
		line = strings.TrimSuffix(line, "_arm64")
		line = strings.TrimSuffix(line, "_aarch64")
		line = strings.TrimSuffix(line, "_amd64")
		line = strings.TrimLeft(line, "1234567890")
		if strings.HasPrefix(line, "ubuntu") {
			dlv.distroName = "ubuntu"
			dlv.distroVersion = strings.TrimPrefix(line, "ubuntu")
			ret = append(ret, dlv)
			if bothArch {
				dlvX := &dlVersion{
					url:           dlv.url,
					distroName:    dlv.distroName,
					distroVersion: dlv.distroVersion,
					isArm:         !dlv.isArm,
				}
				ret = append(ret, dlvX)
			}
		} else if strings.HasPrefix(line, "debian") {
			dlv.distroName = "debian"
			dlv.distroVersion = strings.TrimPrefix(line, "debian")
			ret = append(ret, dlv)
			if bothArch {
				dlvX := &dlVersion{
					url:           dlv.url,
					distroName:    dlv.distroName,
					distroVersion: dlv.distroVersion,
					isArm:         !dlv.isArm,
				}
				ret = append(ret, dlvX)
			}
		} else if strings.HasPrefix(line, "el") {
			dlv.distroName = "centos"
			dlv.distroVersion = strings.TrimPrefix(line, "el")
			ret = append(ret, dlv)
			if bothArch {
				dlvX := &dlVersion{
					url:           dlv.url,
					distroName:    dlv.distroName,
					distroVersion: dlv.distroVersion,
					isArm:         !dlv.isArm,
				}
				ret = append(ret, dlvX)
			}
			if dlv.distroName == "centos" && dlv.distroVersion == "7" {
				dlv2 := &dlVersion{
					url:           dlv.url,
					distroName:    "amazon",
					distroVersion: "2",
				}
				ret = append(ret, dlv2)
				if bothArch {
					dlv2X := &dlVersion{
						url:           dlv.url,
						distroName:    dlv.distroName,
						distroVersion: dlv.distroVersion,
						isArm:         !dlv.isArm,
					}
					ret = append(ret, dlv2X)
				}
			}
		}
	}
	return ret, nil
}

func aeroFindUrl(version string, user string, pass string) (url string, v string, err error) {
	return aeroFindUrlX(enterpriseUrl, version, user, pass)
}

func aeroFindUrlX(enterpriseUrl string, version string, user string, pass string) (url string, v string, err error) {
	var baseUrl string
	partversion := ""
	if strings.HasSuffix(version, "*") {
		partversion = strings.TrimSuffix(version, "*")
	}
	if version == "latest" || version == "latestc" || strings.HasSuffix(version, "*") {
		if version[len(version)-1] != 'c' && version[len(version)-1] != 'f' {
			baseUrl = enterpriseUrl
		} else if version[len(version)-1] == 'f' {
			baseUrl = federalUrl
		} else {
			baseUrl = communityUrl
		}
		client := &http.Client{}
		req, err := http.NewRequest("GET", baseUrl, nil)
		if err != nil {
			return url, v, err
		}
		response, err := client.Do(req)
		if err != nil {
			return "", "", err
		}

		if response.StatusCode != 200 {
			err = fmt.Errorf("error code: %d, URL: %s", response.StatusCode, baseUrl)
			return "", "", err
		}

		responseData, err := io.ReadAll(response.Body)
		if err != nil {
			return "", "", err
		}
		ver := ""
		for _, line := range strings.Split(string(responseData), "\n") {
			if strings.Contains(line, "folder.gif") {
				rp := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+[\.]*[0-9]*[^/]*`)
				nver := rp.FindString(line)
				if partversion == "" || strings.HasPrefix(nver, partversion) {
					if ver == "" {
						ver = nver
					} else {
						if VersionCheck(nver, ver) == -1 {
							ver = nver
						}
					}
				}
			}
		}
		if ver == "" {
			return "", "", errors.New("required version not found")
		}
		if version[len(version)-1] != 'c' && version[len(version)-1] != 'f' {
			version = ver
		} else if version[len(version)-1] == 'f' {
			version = ver + "f"
		} else {
			version = ver + "c"
		}
	}

	if version[len(version)-1] != 'c' && version[len(version)-1] != 'f' {
		baseUrl = enterpriseUrl + version + "/"
	} else if version[len(version)-1] == 'f' {
		baseUrl = federalUrl + version[:len(version)-1] + "/"
	} else {
		baseUrl = communityUrl + version[:len(version)-1] + "/"
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET", baseUrl, nil)
	if err != nil {
		return "", "", err
	}
	req.SetBasicAuth(user, pass)
	response, err := client.Do(req)
	if err != nil {
		return
	}
	if response.StatusCode != 200 {
		err = fmt.Errorf("error code: %d, URL: %s", response.StatusCode, baseUrl)
		return
	}
	url = baseUrl
	v = version
	return
}
