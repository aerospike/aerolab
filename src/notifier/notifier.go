package notifier

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bestmethod/inslice"
)

type HTTPSNotify struct {
	Endpoint          string   `long:"notify-web-endpoint" description:"http(s) URL to contact with a notification" yaml:"endpoint"`
	Headers           []string `long:"notify-web-header" description:"a header to set for notification; for example to use Authorization tokens; format: Name=value" yaml:"headers"`
	AbortOnFail       bool     `long:"notify-web-abort-on-fail" description:"if set, ingest will be aborted if the notification system receives an error response" yaml:"abortOnFail"`
	AbortOnCode       []int    `long:"notify-web-abort-code" description:"set to status codes on which to abort the operation" yaml:"abortStatusCodes"`
	IgnoreInvalidCert bool     `long:"notify-web-ignore-cert" description:"set to make https calls ignore invalid server certificate"`
	wg                *sync.WaitGroup
}

func (h *HTTPSNotify) Init() {
	h.wg = new(sync.WaitGroup)
}

func (h *HTTPSNotify) Close() {
	h.wg.Wait()
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
