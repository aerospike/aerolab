package notifier

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/slack"
	"github.com/bestmethod/inslice"
)

type HTTPSNotify struct {
	Endpoint          string   `long:"notify-web-endpoint" description:"http(s) URL to contact with a notification" yaml:"endpoint"`
	Headers           []string `long:"notify-web-header" description:"a header to set for notification; for example to use Authorization tokens; format: Name=value" yaml:"headers"`
	AbortOnFail       bool     `long:"notify-web-abort-on-fail" description:"if set, ingest will be aborted if the notification system receives an error response" yaml:"abortOnFail"`
	AbortOnCode       []int    `long:"notify-web-abort-code" description:"set to status codes on which to abort the operation" yaml:"abortStatusCodes"`
	IgnoreInvalidCert bool     `long:"notify-web-ignore-cert" description:"set to make https calls ignore invalid server certificate"`
	SlackToken        string   `long:"notify-slack-token" description:"set to enable slack notifications for events"`
	SlackChannel      string   `long:"notify-slack-channel" description:"set to the channel to notify to"`
	SlackEvents       string   `long:"notify-slack-events" description:"comma-separated list of events to notify for" default:"INGEST_FINISHED,SERVICE_DOWN,SERVICE_UP,MAX_AGE_REACHED,MAX_INACTIVITY_REACHED,SPOT_INSTANCE_CAPACITY_SHUTDOWN"`
	slackEvents       []string
	slack             *slack.Slack
	wg                *sync.WaitGroup
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

func (h *HTTPSNotify) NotifySlack(event string, message string) {
	if h.slack == nil {
		return
	}
	if !inslice.HasString(h.slackEvents, event) {
		return
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		err := h.slack.Send(message)
		if err != nil {
			log.Println(err)
		}
	}()
}

func (h *HTTPSNotify) NotifyJSON(payload interface{}) error {
	if h.Endpoint == "" {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return h.NotifyData(data)
}

func (h *HTTPSNotify) NotifyData(data []byte) error {
	if h.Endpoint == "" {
		return nil
	}
	if !h.AbortOnFail && len(h.AbortOnCode) == 0 {
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.notify(data)
		}()
		return nil
	}
	return h.notify(data)
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
		return err
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
		return err
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
