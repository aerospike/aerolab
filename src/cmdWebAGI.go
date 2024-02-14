package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type agiWebTokens struct {
	tokens  map[string]string
	l       *sync.RWMutex
	getters map[string]*sync.WaitGroup
	glock   *sync.Mutex
}

func NewAgiWebTokenHandler() *agiWebTokens {
	return &agiWebTokens{
		tokens:  make(map[string]string),
		l:       new(sync.RWMutex),
		getters: make(map[string]*sync.WaitGroup),
		glock:   new(sync.Mutex),
	}
}

func (t *agiWebTokens) GetToken(name string) (token string, err error) {
	// if token exists, serve it
	t.l.RLock()
	if v, ok := t.tokens[name]; ok {
		t.l.RUnlock()
		return v, nil
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
	token, err = t.getNewToken(name)
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
