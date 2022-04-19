package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func aeroFindUrl(version string, user string, pass string) (url string, v string, err error) {

	var baseUrl string
	if version == "latest" || version == "latestc" {
		if version[len(version)-1] != 'c' {
			baseUrl = "https://artifacts.aerospike.com/aerospike-server-enterprise/"
		} else {
			baseUrl = "https://artifacts.aerospike.com/aerospike-server-community/"
		}
		client := &http.Client{}
		req, err := http.NewRequest("GET", baseUrl, nil)
		if err != nil {
			return url, v, err
		}
		req.SetBasicAuth(user, pass)
		response, err := client.Do(req)
		if err != nil {
			return "", "", err
		}

		if response.StatusCode != 200 {
			err = fmt.Errorf(ERR_URL_NOT_FOUND, response.StatusCode)
			return "", "", err
		}

		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return "", "", err
		}
		ver := ""
		for _, line := range strings.Split(string(responseData), "\n") {
			if strings.Contains(line, "folder.gif") {
				rp := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+[\.]*[0-9]*[^/]*`)
				nver := rp.FindString(line)
				if ver == "" {
					ver = nver
				} else {
					if VersionOrdinal(nver) > VersionOrdinal(ver) {
						ver = nver
					}
				}
			}
		}
		if version[len(version)-1] != 'c' {
			version = ver
		} else {
			version = ver + "c"
		}
	}

	if version[len(version)-1] != 'c' {
		//baseUrl = "https://www.aerospike.com/artifacts/aerospike-server-enterprise/" + version + "/"
		baseUrl = "https://artifacts.aerospike.com/aerospike-server-enterprise/" + version + "/"
	} else {
		baseUrl = "https://artifacts.aerospike.com/aerospike-server-community/" + version[:len(version)-1] + "/"
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
		err = fmt.Errorf(ERR_URL_NOT_FOUND, response.StatusCode)
		return
	}
	url = baseUrl
	v = version
	return
}

// file downloader starts here

type PassThru struct {
	io.Reader
	total     int64 // Total # of bytes transferred
	filetotal int64
	startTime time.Time
}

func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	pt.total += int64(n)

	if err == nil {
		percent := int64(float64(pt.total) / float64(pt.filetotal) * 100)
		delta := int64(time.Since(pt.startTime).Seconds())
		var speed int64
		if delta > 0 {
			speed = pt.total / delta
		} else {
			speed = 0
		}
		fmt.Printf("\rProgress: %d%% (%s of %s @ %s / s)                    ", percent, convSize(pt.total), convSize(pt.filetotal), convSize(speed))
	}

	return n, err
}

func convSize(size int64) string {
	var sizeString string
	if size > 1023 && size < 1024*1024 {
		sizeString = fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else if size < 1024 {
		sizeString = fmt.Sprintf("%v B", size)
	} else if size >= 1024*1024 && size < 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f MB", float64(size)/1024/1024)
	} else if size >= 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f GB", float64(size)/1024/1024/1024)
	}
	return sizeString
}

func downloadFile(url string, filename string, user string, pass string) (err error) {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		err = fmt.Errorf(ERR_URL_NOT_FOUND, response.StatusCode, url)
		return
	}
	src := &PassThru{Reader: response.Body}
	t, _ := strconv.Atoi(response.Header.Get("Content-Length"))
	src.filetotal = int64(t)
	src.startTime = time.Now()
	_, err = io.Copy(out, src)
	fmt.Print("\r")
	if err != nil {
		return err
	}
	return
}

func fixClusteNameConfig(conf string, cluster_name string) (newconf string, err error) {
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

func VersionOrdinal(version string) string {
	// ISO/IEC 14651:2011
	const maxByte = 1<<8 - 1
	vo := make([]byte, 0, len(version)+8)
	j := -1
	for i := 0; i < len(version); i++ {
		b := version[i]
		if '0' > b || b > '9' {
			vo = append(vo, b)
			j = -1
			continue
		}
		if j == -1 {
			vo = append(vo, 0x00)
			j = len(vo) - 1
		}
		if vo[j] == 1 && vo[j+1] == '0' {
			vo[j+1] = b
			continue
		}
		if vo[j]+1 > maxByte {
			panic("VersionOrdinal: invalid version")
		}
		vo = append(vo, b)
		vo[j]++
	}
	return string(vo)
}
