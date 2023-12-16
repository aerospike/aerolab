package notifier

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/slack"
	"github.com/bestmethod/inslice"
	"github.com/google/uuid"
)

type HTTPSNotify struct {
	AGIMonitorUrl        string   `long:"agi-monitor-url" description:"AWS/GCP: AGI Monitor endpoint url to send the notifications to for sizing" yaml:"agiMonitor"`
	AGIMonitorCertIgnore bool     `long:"agi-monitor-ignore-cert" description:"set to make https calls ignore invalid server certificate"`
	Endpoint             string   `long:"notify-web-endpoint" description:"http(s) URL to contact with a notification" yaml:"endpoint"`
	Headers              []string `long:"notify-web-header" description:"a header to set for notification; for example to use Authorization tokens; format: Name=value" yaml:"headers"`
	AbortOnFail          bool     `long:"notify-web-abort-on-fail" description:"if set, ingest will be aborted if the notification system receives an error response or no response" yaml:"abortOnFail"`
	AbortOnCode          []int    `long:"notify-web-abort-code" description:"set to status codes on which to abort the operation" yaml:"abortStatusCodes"`
	IgnoreInvalidCert    bool     `long:"notify-web-ignore-cert" description:"set to make https calls ignore invalid server certificate"`
	SlackToken           string   `long:"notify-slack-token" description:"set to enable slack notifications for events"`
	SlackChannel         string   `long:"notify-slack-channel" description:"set to the channel to notify to"`
	SlackEvents          string   `long:"notify-slack-events" description:"comma-separated list of events to notify for" default:"INGEST_FINISHED,SERVICE_DOWN,SERVICE_UP,MAX_AGE_REACHED,MAX_INACTIVITY_REACHED,SPOT_INSTANCE_CAPACITY_SHUTDOWN"`
	slackEvents          []string
	slack                *slack.Slack
	wg                   *sync.WaitGroup
}

func (h *HTTPSNotify) Init() {
	h.wg = new(sync.WaitGroup)
	if h.SlackToken != "" && h.SlackChannel != "" {
		h.slackEvents = strings.Split(h.SlackEvents, ",")
		h.slack = &slack.Slack{
			Token:   h.SlackToken,
			Channel: h.SlackChannel,
		}
		err := h.slack.Join()
		if err != nil {
			log.Printf("Slack Channel Join Failure: %s", err)
		}
	}
}

func (h *HTTPSNotify) Close() {
	h.wg.Wait()
}

func (h *HTTPSNotify) NotifySlack(event string, message string, threadedMessage string) {
	if h.slack == nil {
		return
	}
	if !inslice.HasString(h.slackEvents, event) {
		return
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		thrId, err := h.slack.Send(nil, message)
		if err != nil {
			log.Println(err)
		} else if threadedMessage != "" {
			_, err = h.slack.Send(thrId, threadedMessage)
			if err != nil {
				log.Println(err)
			}
		}
	}()
}

func (h *HTTPSNotify) NotifyJSON(payload interface{}) error {
	if h.Endpoint == "" && h.AGIMonitorUrl == "" {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return h.NotifyData(data)
}

func (h *HTTPSNotify) NotifyData(data []byte) error {
	if h.Endpoint == "" && h.AGIMonitorUrl == "" {
		return nil
	}
	var httpErr error
	if h.Endpoint != "" {
		if !h.AbortOnFail && len(h.AbortOnCode) == 0 {
			h.wg.Add(1)
			go func() {
				defer h.wg.Done()
				h.notify(data)
			}()
		} else {
			httpErr = h.notify(data)
		}
	}
	if h.AGIMonitorUrl == "" {
		return httpErr
	}
	err := h.notifyAgi(data)
	if err != nil {
		return err
	}
	if httpErr != nil {
		return httpErr
	}
	return nil
}

func (h *HTTPSNotify) notify(data []byte) error {
	headers := make(map[string]string)
	for _, head := range h.Headers {
		splitter := strings.Split(head, "=")
		if len(splitter) < 2 {
			return errors.New("header format must be Name=Value")
		}
		headers[splitter[0]] = strings.Join(splitter[1:], "=")
	}

	req, err := http.NewRequest(http.MethodPost, h.Endpoint, bytes.NewReader(data))
	if err != nil {
		if h.AbortOnFail {
			return err
		} else {
			log.Printf("notifyJSON: %s", err)
			return nil
		}
	}

	req.Header.Set("Content-Type", "application/json")
	for n, v := range headers {
		req.Header.Add(n, v)
	}

	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   30 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: h.IgnoreInvalidCert},
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		if h.AbortOnFail {
			return err
		} else {
			log.Printf("notifyJSON: %s", err)
			return nil
		}
	}
	defer resp.Body.Close()
	if (h.AbortOnFail && resp.StatusCode < 200 || resp.StatusCode > 299) || inslice.HasInt(h.AbortOnCode, resp.StatusCode) {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			body = append(body, []byte("<ERROR:BODY READ ERROR>")...)
		}
		return fmt.Errorf("statusCode:%d status:%s body:%s", resp.StatusCode, resp.Status, string(body))
	}
	return nil
}

func (h *HTTPSNotify) notifyAgi(data []byte) error {
	headers := h.notifyAgiAuthHeaders()
	req, err := http.NewRequest(http.MethodPost, h.AGIMonitorUrl, bytes.NewReader(data))
	if err != nil {
		log.Printf("notifyAGI: %s", err)
		return nil
	}

	req.Header.Set("Content-Type", "application/json")
	for n, v := range headers {
		req.Header.Add(n, v)
	}

	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   60 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: h.AGIMonitorCertIgnore},
	}
	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("notifyAGI: %s", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == 418 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			body = append(body, []byte("<ERROR:BODY READ ERROR>")...)
		}
		return fmt.Errorf("statusCode:%d status:%s body:%s", resp.StatusCode, resp.Status, string(body))
	}
	return nil
}

func (h *HTTPSNotify) notifyAgiAuthHeaders() map[string]string {
	a := make(map[string]string)
	auth, err := EncodeAuthJson()
	if err != nil {
		log.Printf("FAILED auth headers: %s", err)
		return a
	}
	a["Agi-Monitor-Auth"] = auth
	secret, err := os.ReadFile("/opt/agi/uuid")
	if err != nil {
		secret = []byte(uuid.New().String())
		os.WriteFile("/opt/agi/uuid", secret, 0644)
	}
	a["Agi-Monitor-Secret"] = strings.Trim(string(secret), "\r\n\t ")
	return a
}

// aws: http://169.254.169.254/latest/dynamic/instance-identity/document/
// gcp: set header Metadata-Flavor:Google
type AgiMonitorAuth struct {
	AccountProjectId     string `json:"accountId"`        // http://169.254.169.254/computeMetadata/v1/project/project-id: aerolab-test-project-2
	AvailabilityZoneName string `json:"availabilityZone"` // http://169.254.169.254/computeMetadata/v1/instance/zone: projects/485062447394/zones/us-central1-a
	ImageId              string `json:"imageId"`          // http://169.254.169.254/computeMetadata/v1/instance/image: projects/ubuntu-os-cloud/global/images/ubuntu-2204-jammy-v20231201
	InstanceId           string `json:"instanceId"`       // http://169.254.169.254/computeMetadata/v1/instance/name: aerolab4-client-1
	InstanceType         string `json:"instanceType"`     // http://169.254.169.254/computeMetadata/v1/instance/machine-type: projects/485062447394/machineTypes/e2-standard-2
	PrivateIp            string `json:"privateIp"`        // http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/ip: 10.128.0.16
	// aws: http://169.254.169.254/latest/meta-data/security-groups - returns one name per line
	// gcp: http://169.254.169.254/computeMetadata/v1/instance/tags: ["aerolab-managed-external","aerolab-server"]
	SecurityGroups []string `json:"securityGroups"`
}

func getJsonAuthData(a *AgiMonitorAuth) error {
	body, err := wget("http://169.254.169.254/latest/dynamic/instance-identity/document/", false)
	if err != nil {
		data, err := wget("http://169.254.169.254/computeMetadata/v1/project/project-id", true)
		if err != nil {
			return err
		}
		a.AccountProjectId = string(data)

		data, err = wget("http://169.254.169.254/computeMetadata/v1/instance/zone", true)
		if err != nil {
			return err
		}
		arr := strings.Split(string(data), "/")
		a.AvailabilityZoneName = arr[len(arr)-1]

		data, err = wget("http://169.254.169.254/computeMetadata/v1/instance/image", true)
		if err != nil {
			return err
		}
		a.ImageId = string(data)

		data, err = wget("http://169.254.169.254/computeMetadata/v1/instance/name", true)
		if err != nil {
			return err
		}
		a.InstanceId = string(data)

		data, err = wget("http://169.254.169.254/computeMetadata/v1/instance/machine-type", true)
		if err != nil {
			return err
		}
		arr = strings.Split(string(data), "/")
		a.InstanceType = arr[len(arr)-1]

		data, err = wget("http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/ip", true)
		if err != nil {
			return err
		}
		a.PrivateIp = string(data)

		data, err = wget("http://169.254.169.254/computeMetadata/v1/instance/tags", true)
		if err != nil {
			return err
		}
		mylist := []string{}
		err = json.Unmarshal(data, &mylist)
		if err != nil {
			return err
		}
		a.SecurityGroups = mylist
		return nil
	}
	err = json.Unmarshal(body, a)
	if err != nil {
		return err
	}
	secgrp, err := wget("http://169.254.169.254/latest/meta-data/security-groups", false)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(secgrp), "\n") {
		line = strings.Trim(line, "\r\n\t ")
		if line == "" {
			continue
		}
		a.SecurityGroups = append(a.SecurityGroups, line)
	}
	return nil
}

func wget(url string, gcp bool) (data []byte, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if gcp {
		req.Header.Add("Metadata-Flavor", "Google")
	}
	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   30 * time.Second,
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("statusCode: %d body: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func EncodeAuthJson() (string, error) {
	a := &AgiMonitorAuth{}
	err := getJsonAuthData(a)
	if err != nil {
		return "", err
	}
	ab, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ab), nil
}

func DecodeAuthJson(val string) (*AgiMonitorAuth, error) {
	ab, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, err
	}
	ret := &AgiMonitorAuth{}
	err = json.Unmarshal(ab, ret)
	return ret, err
}
