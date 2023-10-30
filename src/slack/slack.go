package slack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Slack struct {
	Token   string
	Channel string
}

type slackMsg struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

type slackMsgThreaded struct {
	Channel  string `json:"channel"`
	Text     string `json:"text"`
	ThreadTs string `json:"thread_ts"`
}

func (s *Slack) Send(threadId *string, message string) (newThreadId *string, err error) {
	var reqBody []byte
	if threadId == nil {
		reqStruct := new(slackMsg)
		reqStruct.Channel = s.Channel
		reqStruct.Text = message
		reqBody, err = json.Marshal(reqStruct)
		if err != nil {
			return nil, err
		}
	} else {
		reqStruct := new(slackMsgThreaded)
		reqStruct.Channel = s.Channel
		reqStruct.Text = message
		reqStruct.ThreadTs = *threadId
		reqBody, err = json.Marshal(reqStruct)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.Token))
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var body []byte
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			body = nil
		}
		return nil, fmt.Errorf("status code: %d ; message: %s", resp.StatusCode, string(body))
	}
	r := new(response)
	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, r)
	if err != nil {
		return nil, err
	}
	if !r.OK {
		return nil, errors.New(string(body))
	}
	return &r.TS, nil
}

type join struct {
	Channel string `json:"channel"`
}

type response struct {
	OK bool   `json:"ok"`
	TS string `json:"ts"`
}

func (s *Slack) Join() (err error) {
	var msg []byte
	msg, err = json.Marshal(&join{
		Channel: s.Channel,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", "https://slack.com/api/conversations.join", bytes.NewReader(msg))
	if err != nil {
		log.Fatal(err)
	}
	if err != nil {
		return err
	}
	req.Header.Add("Content-type", "application/json")
	req.Header.Add("Authorization", "Bearer "+s.Token)
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%d: %s", resp.StatusCode, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	r := new(response)
	err = json.Unmarshal(body, r)
	if err != nil {
		return err
	}
	if !r.OK {
		return errors.New(string(body))
	}
	return nil
}
