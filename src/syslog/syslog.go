package syslog

/* for future use:
aerolab cluster add agi - will install `aerolab` on cluster nodes and configure systemd-service file (or /opt/autoload for docker) to start hidden `aerolab syslog`
 - this will also reconfigure aerospike.conf if required to add a new logging sink specifically for this syslog
aerolab syslog - hidden fature listening on a socket and forwarding all logs to a webserver - AGI webserver using LAN IP
aerolab agi logingest feature - will also listen on another port for ingesting data via webserver json - will receive data from aerolab syslog, and will ingest live
*/

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bestmethod/inslice"
	"github.com/google/uuid"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

type Packet struct {
	Facility int
	Hostname string
	Tag      string
	Log      string
	Order    uint
	ConnNo   uint
	NodeID   string
}

type rateTag struct {
	tag          string
	maxPerSec    int
	currentCount int
	currentTime  time.Time
}

type rateFacility struct {
	facility     int
	maxPerSec    int
	currentCount int
	currentTime  time.Time
}

func RunCLI() error {
	sockPath := flag.String("listen-socket", "/dev/log", "provide path for socket to be created")
	dstFile := flag.String("out-file", "", "set to path of destination log file to store in file")
	dstStdout := flag.Bool("out-stdout", false, "set to output to stdout")
	dstStderr := flag.Bool("out-stderr", false, "set to output to stderr")
	dstWeb := flag.String("out-web", "", "web endpoint to send output json to")
	dstWebIgnoreCert := flag.Bool("out-web-ignore-cert", false, "set when using out-web with TLS to ignore invalid certificate errors")
	dstWebTimeout := flag.Int("out-web-timeout", 30, "timeout seconds for web requests")
	nodeID := flag.String("node-id", "", "manually set node-id for output; if unset, will attmpt to obtain using aerospike.conf and asinfo")
	logFacilities := flag.String("log-facilities", "", "optional explicit comma-separated list of facility numbers to log")
	logTags := flag.String("log-tags", "", "optional explicit comma-separated list of tags to log")
	rateLimitTag := flag.String("log-tag-ratelimit", "", "optional comma-separated list of tags to rate-limit; format: TAG:MSGS-PER-SECOND,TAG:MSGS-PER-SECOND,...; ex: debug:10,audit:10")
	rateLimitFacility := flag.String("log-facility-ratelimit", "", "optional comma-separated list of facilities to rate-limit; format: FACILITYNO:MSGS-PER-SECOND,FACILITYNO:MSGS-PER-SECOND,...; ex: 1030:10")
	flag.Parse()
	return Syslog(&Config{
		SockPath:          *sockPath,
		DstFile:           *dstFile,
		DstStdout:         *dstStdout,
		DstStderr:         *dstStderr,
		DstWeb:            *dstWeb,
		DstWebIgnoreCert:  *dstWebIgnoreCert,
		DstWebTimeout:     *dstWebTimeout,
		NodeID:            *nodeID,
		LogFacilities:     *logFacilities,
		LogTags:           *logTags,
		RateLimitTag:      *rateLimitTag,
		RateLimitFacility: *rateLimitFacility,
	})
}

type Config struct {
	SockPath          string
	DstFile           string
	DstStdout         bool
	DstStderr         bool
	DstWeb            string
	DstWebIgnoreCert  bool
	DstWebTimeout     int
	NodeID            string
	LogFacilities     string
	LogTags           string
	RateLimitTag      string
	RateLimitFacility string
}

func Syslog(conf *Config) error {
	facilityList := []int{}
	tagList := []string{}
	ratesTag := make(map[string]*rateTag)
	rflock := new(sync.Mutex)
	ratesFacility := make(map[int]*rateFacility)
	if conf.LogFacilities != "" {
		lf := strings.Split(conf.LogFacilities, ",")
		for _, l := range lf {
			li, err := strconv.Atoi(l)
			if err != nil {
				return fmt.Errorf("could not parse log-facilities list: %s", err)
			}
			facilityList = append(facilityList, li)
		}
	}
	if conf.LogTags != "" {
		tagList = strings.Split(conf.LogTags, ",")
	}
	if conf.RateLimitTag != "" {
		lf := strings.Split(conf.RateLimitTag, ",")
		for _, l := range lf {
			li := strings.Split(l, ":")
			if len(li) != 2 {
				return errors.New("could not parse log-tag-ratelimit: incorrect format")
			}
			lin, err := strconv.Atoi(li[1])
			if err != nil {
				return fmt.Errorf("could not parse log-tag-ratelimit: %s", err)
			}
			ratesTag[li[0]] = &rateTag{
				tag:       li[0],
				maxPerSec: lin,
			}
		}
	}
	if conf.RateLimitFacility != "" {
		lf := strings.Split(conf.RateLimitFacility, ",")
		for _, l := range lf {
			li := strings.Split(l, ":")
			if len(li) != 2 {
				return errors.New("could not parse log-facility-ratelimit: incorrect format")
			}
			lif, err := strconv.Atoi(li[0])
			if err != nil {
				return fmt.Errorf("could not parse log-facility-ratelimit: %s", err)
			}
			lin, err := strconv.Atoi(li[1])
			if err != nil {
				return fmt.Errorf("could not parse log-facility-ratelimit: %s", err)
			}
			ratesFacility[lif] = &rateFacility{
				facility:  lif,
				maxPerSec: lin,
			}
		}
	}

	// hostname
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	// nodeID
	if conf.NodeID == "" {
		conf.nodeID()
		go func() {
			for {
				conf.nodeID()
				time.Sleep(time.Second)
			}
		}()
	}

	// setup local writers
	var wr []*os.File
	if conf.DstFile != "" {
		f, err := os.OpenFile(conf.DstFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		wr = append(wr, f)
		defer func() {
			wr[0].Close()
		}()
	}
	if conf.DstStdout {
		wr = append(wr, os.Stdout)
	}
	if conf.DstStderr {
		wr = append(wr, os.Stderr)
	}

	// setup http writer
	httpClient := &http.Client{
		Timeout: time.Duration(conf.DstWebTimeout) * time.Second,
	}
	httpClient.Transport = &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: conf.DstWebIgnoreCert},
		TLSHandshakeTimeout: time.Duration(conf.DstWebTimeout) * time.Second,
	}

	// Create a Unix domain socket and listen for incoming connections.
	os.Remove(conf.SockPath)
	socket, err := net.Listen("unix", conf.SockPath)
	if err != nil {
		return err
	}

	// system close and cleanup the sockfile.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Remove(conf.SockPath)
		socket.Close()
	}()

	var retErr error

	// log rotation
	wr0 := new(sync.RWMutex)
	if conf.DstFile != "" {
		r := make(chan os.Signal, 1)
		signal.Notify(r, syscall.SIGUSR1)
		go func() {
			for {
				<-r
				wr0.Lock()
				wr[0].Close()
				f, err := os.OpenFile(conf.DstFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					retErr = err
					os.Remove(conf.SockPath)
					socket.Close()
				}
				wr[0] = f
				wr0.Unlock()
			}
		}()
	}

	connNo := uint(0)
	for {
		// Accept an incoming connection.
		conn, err := socket.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			log.Printf("ERROR: socket.Acccept: %s", err)
			time.Sleep(time.Second)
			continue
		}

		// Handle the connection in a separate goroutine.
		go func(conn net.Conn, connNo uint) {
			defer conn.Close()
			// Create a buffer for incoming data.
			buf := make([]byte, 65536)
			order := uint(0)
			for {
				packets := []*Packet{}
				// Read data from the connection.
				n, err := conn.Read(buf)
				if err != nil {
					if err == io.EOF {
						return
					}
					log.Printf("WARN: connection read from %s: %s", conn.RemoteAddr().String(), err)
					return
				}

				// parse
				data := []byte{}
				idx := 0
				for idx < n {
					nidx := bytes.Index(buf[idx:n], []byte{0})
					if nidx == -1 {
						break
					}
					nidx = nidx + idx
					line := buf[idx:nidx]
					if len(line) == 0 {
						continue
					}
					facility := 0
					if line[0] == '<' {
						lend := bytes.Index(line, []byte{'>'})
						if lend != -1 {
							lvals := line[1:lend]
							line = line[lend+1:]
							facility, _ = strconv.Atoi(string(lvals))
						}
					}
					if len(facilityList) != 0 && !inslice.HasInt(facilityList, facility) {
						continue
					}
					lsplit := bytes.Split(line, []byte{' '})
					tag := ""
					if len(lsplit) >= 4 {
						tag = string(lsplit[3])
						if tag[len(tag)-1] == ':' {
							tag = tag[:len(tag)-1]
						}
					}
					if len(tagList) != 0 && !inslice.HasString(tagList, tag) {
						continue
					}
					rflock.Lock()
					if rate, ok := ratesFacility[facility]; ok {
						if rate.currentTime.IsZero() {
							rate.currentTime = time.Now()
						}
						if rate.currentTime.Unix() != time.Now().Unix() {
							rate.currentTime = time.Now()
							rate.currentCount = 0
						}
						if rate.currentCount == rate.maxPerSec {
							rflock.Unlock()
							break
						}
						rate.currentCount++
					}
					if rate, ok := ratesTag[tag]; ok {
						if rate.currentTime.IsZero() {
							rate.currentTime = time.Now()
						}
						if rate.currentTime.Unix() != time.Now().Unix() {
							rate.currentTime = time.Now()
							rate.currentCount = 0
						}
						if rate.currentCount == rate.maxPerSec {
							rflock.Unlock()
							break
						}
						rate.currentCount++
					}
					rflock.Unlock()
					pkt := &Packet{
						Hostname: hostname,
						Facility: facility,
						Log:      string(line),
						Order:    order,
						ConnNo:   connNo,
						NodeID:   conf.NodeID,
						Tag:      tag,
					}
					packets = append(packets, pkt)
					datax, _ := JSONMarshal(pkt)
					data = append(data, datax...)
					idx = nidx + 1
					order++
				}

				// Write data
				for wi, w := range wr {
					if wi == 0 {
						wr0.RLock()
					}
					_, err = w.Write(data)
					if err != nil {
						log.Printf("WARN: write log to %s: %s", w.Name(), err)
					}
					if wi == 0 {
						wr0.RUnlock()
					}
				}

				// send to webserver
				if conf.DstWeb != "" {
					buf := &bytes.Buffer{}
					err = json.NewEncoder(buf).Encode(packets)
					if err != nil {
						log.Printf("WARN: json-encode: %s", err)
						continue
					}
					request, err := http.NewRequest("POST", conf.DstWeb, buf)
					if err != nil {
						log.Printf("WARN: json http request: %s", err)
						continue
					}
					request.Header.Set("Content-Type", "application/json")
					resp, err := httpClient.Do(request)
					if err != nil {
						log.Printf("WARN: could not send to json endpoint: %s", err)
						continue
					}
					if resp.StatusCode != 200 {
						respBody, _ := io.ReadAll(resp.Body)
						log.Printf("WARN: json endpoint responded with statusCode=%d status=%s msg=%s", resp.StatusCode, resp.Status, string(respBody))
					}
					resp.Body.Close()
				}
			}
		}(conn, connNo)

		connNo++
	}
	return retErr
}

func JSONMarshal(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(t)
	return buffer.Bytes(), err
}

func reverseArray(arr []byte) []byte {
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
	return arr
}

func (conf *Config) nodeID() {
	nid := strings.ToUpper(fmt.Sprintf("BB9%x", reverseArray(uuid.NodeID())))
	asd, err := aeroconf.ParseFile("/etc/aerospike/aerospike.conf")
	if err == nil {
		d, _ := asd.Stanza("service").GetValues("node-id")
		if len(d) == 1 {
			nid = *d[0]
		} else {
			hbPort := 3001
			d, _ := asd.Stanza("network").Stanza("heartbeart").GetValues("port")
			if len(d) == 1 {
				di, err := strconv.Atoi(*d[0])
				if err == nil {
					hbPort = di
				}
			}
			d, _ = asd.Stanza("network").Stanza("heartbeart").GetValues("tls-port")
			if len(d) == 1 {
				di, err := strconv.Atoi(*d[0])
				if err == nil {
					hbPort = di
				}
			}
			nid = strings.ToUpper(fmt.Sprintf("%x%x", hbPort, reverseArray(uuid.NodeID())))
		}
	}
	out, err := exec.Command("asinfo", "-v", "node").CombinedOutput()
	if err == nil {
		nid = strings.Trim(string(out), "\r\n\t ")
	}
	if nid != conf.NodeID {
		conf.NodeID = nid
	}
}
