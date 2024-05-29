package main

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type agiWebTokens struct {
	tokens             map[string]string
	l                  *sync.RWMutex
	getters            map[string]*sync.WaitGroup
	glock              *sync.Mutex
	insecureSkipVerify bool
}

func NewAgiWebTokenHandler(insecureSkipVerify bool) *agiWebTokens {
	return &agiWebTokens{
		tokens:             make(map[string]string),
		l:                  new(sync.RWMutex),
		getters:            make(map[string]*sync.WaitGroup),
		glock:              new(sync.Mutex),
		insecureSkipVerify: insecureSkipVerify,
	}
}

type agiWebTokenRequest struct {
	Name         string
	PublicIP     string
	PrivateIP    string
	InstanceID   string
	AccessProtIP string
	AccessURL    string
}

func (t *agiWebTokenRequest) GetUniqueValue() string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(t.Name+t.PublicIP+t.PrivateIP+t.InstanceID)))
}

// invalidate existing token and get a new one
func (t *agiWebTokens) GetNewToken(req agiWebTokenRequest) (token string, err error) {
	name := req.GetUniqueValue()
	t.l.Lock()
	delete(t.tokens, name)
	t.l.Unlock()
	return t.GetToken(req)
}

// get the token, if it fails a quick test, obtain a new token
func (t *agiWebTokens) GetTokenWithTest(req agiWebTokenRequest) (token string, err error) {
	return t.getToken(req, true)
}

// get an existing token, or request a new one if current does not exist
func (t *agiWebTokens) GetToken(req agiWebTokenRequest) (token string, err error) {
	return t.getToken(req, false)
}

// return false if token is invalid, in all other cases (including connection failed) return true
func testToken(url string, token string, insecureSkipVerify bool) bool {
	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   5 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: insecureSkipVerify}, // ping AGI to test if token is valid, expect real cert on AGI instances
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return true
	}
	req.AddCookie(&http.Cookie{Name: "AGI_TOKEN", Value: token})
	req.AddCookie(&http.Cookie{Name: "X-AGI-CALLER", Value: "webui"})
	response, err := client.Do(req)
	if err != nil {
		return true
	}
	if response.StatusCode == http.StatusUnauthorized {
		return false
	}
	return true
}

// get token or obtain new onel optionally test if token is accepted
func (t *agiWebTokens) getToken(req agiWebTokenRequest, test bool) (token string, err error) {
	name := req.GetUniqueValue()
	// if token exists, serve it
	t.l.RLock()
	if v, ok := t.tokens[name]; ok {
		t.l.RUnlock()
		if !test {
			return v, nil
		}
		if testToken(req.AccessProtIP+"/agi/ok", v, t.insecureSkipVerify) {
			return v, nil
		}
		return t.GetNewToken(req)
	}

	// token doesn't exist, obtain getter lock for getting tokens and unlock the main read lock
	t.glock.Lock()
	t.l.RUnlock()
	if g, ok := t.getters[name]; ok {
		// looks like a getter is already running, wait for it to finish
		t.glock.Unlock() // unlock getters lock, not needed no more
		g.Wait()
		// the other getter finished, try to get the token again, this time if we fail, we error
		t.l.RLock()
		defer t.l.RUnlock()
		if v, ok := t.tokens[name]; ok {
			return v, nil
		}
		return "", errors.New("could not obtain token")
	}

	// getter is not running, create a new getter mutex for tracking
	t.getters[name] = new(sync.WaitGroup)
	t.getters[name].Add(1)
	t.glock.Unlock() // added tracker, can unlock now

	// no matter what happens now, we will need to unlock and remove the getter tracking
	defer func() {
		t.glock.Lock()
		t.getters[name].Done()
		delete(t.getters, name)
		t.glock.Unlock()
	}()

	// get new token here
	token, err = t.getNewToken(req.Name)
	if err != nil {
		// error, exit
		return "", err
	}
	// store new token and respond
	t.l.Lock()
	t.tokens[name] = token
	t.l.Unlock()
	return token, nil
}

// call aerolab to get the auth token
func (t *agiWebTokens) getNewToken(name string) (token string, err error) {
	ex, err := os.Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ex, "agi", "add-auth-token", "-n", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, string(out))
	}
	return strings.Trim(string(out), "\r\n\t "), nil
}
