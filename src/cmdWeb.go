package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/jobqueue"
	"github.com/aerospike/aerolab/jupyter"
	"github.com/aerospike/aerolab/webui"
	"github.com/bestmethod/inslice"
	"github.com/lithammer/shortuuid"
	"github.com/pkg/browser"
	flags "github.com/rglonek/jeddevdk-goflags"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type webCmd struct {
	ListenAddr          []string       `long:"listen" description:"host:port, or just :port, for IPv6 use for example '[::1]:3333'; can be specified multiple times" default:"127.0.0.1:3333"`
	WebRoot             string         `long:"webroot" description:"set the web root that should be served, useful if proxying from eg /aerolab on a webserver" default:"/"`
	AbsoluteTimeout     time.Duration  `long:"timeout" description:"Absolute timeout to set for command execution" default:"30m"`
	MaxConcurrentJobs   int            `long:"max-concurrent-job" description:"Max number of jobs to run concurrently" default:"5"`
	MaxQueuedJobs       int            `long:"max-queued-job" description:"Max number of jobs to queue for execution" default:"10"`
	JobHistoryExpiry    time.Duration  `long:"history-expires" description:"time to keep job from their start in history" default:"72h"`
	ShowMaxHistoryItems int            `long:"show-max-history" description:"show only this amount of completed historical items max" default:"100"`
	NoBrowser           bool           `long:"nobrowser" description:"set to prevent aerolab automatically opening a browser and navigating to the UI page"`
	RefreshInterval     time.Duration  `long:"refresh-interval" description:"change interval at which the inventory is refreshed in the background" default:"30s"`
	MinInterval         time.Duration  `long:"minimum-interval" description:"minimum interval between inventory refreshes (avoid API limit exhaustion)" default:"10s"`
	BlockServerLs       bool           `long:"block-server-ls" description:"block file exploration on the server altogether"`
	AllowLsEverywhere   bool           `long:"always-server-ls" description:"by default server filebrowser only works on localhost, enable this to allow from everywhere"`
	MaxUploadSizeBytes  int            `long:"max-upload-size-bytes" description:"max size of files to allow uploading via the webui temp if server-ls is blocked (hosted mode); 0=disabled" default:"209715200"`
	UploadTempDir       flags.Filename `long:"upload-temp-dir" description:"if sever ls is blocked, temporary directory to use for file uploads"`
	UniqueFirewalls     bool           `long:"unique-firewalls" description:"for multi-user hosted mode: enable per-username firewalls"`
	AGIStrictTLS        bool           `long:"agi-strict-tls" description:"when performing inventory lookup, expect valid AGI certificates"`
	WSProxyOrigins      []string       `long:"ws-proxy-origin" description:"when using proxies, set this to host (or host:port) URI that Origin header should also be accepted for (the URI browser uses to connect)"`
	WebPath             string         `long:"web-path" hidden:"true"`
	WebNoOverride       bool           `long:"web-no-override" hidden:"true"`
	DebugRequests       bool           `long:"debug-requests" hidden:"true"`
	Real                bool           `long:"real" hidden:"true"`
	Help                helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
	menuItems           []*webui.MenuItem
	commands            []*apiCommand
	commandsIndex       map[string]int
	titler              cases.Caser
	joblist             *jobTrack
	jobqueue            *jobqueue.Queue
	cache               *inventoryCache
	inventoryNames      map[string]*webui.InventoryItem
	agiTokens           *agiWebTokens
	cfgTs               time.Time
	wsCount             *wsCounters
}

type jobTrack struct {
	sync.Mutex
	j map[string]*exec.Cmd
}

func (j *jobTrack) Add(jobID string, cmd *exec.Cmd) {
	j.Lock()
	defer j.Unlock()
	j.j[jobID] = cmd
}

func (j *jobTrack) Delete(jobID string) {
	j.Lock()
	defer j.Unlock()
	delete(j.j, jobID)
}

func (j *jobTrack) Get(jobID string) *exec.Cmd {
	j.Lock()
	defer j.Unlock()
	answer := j.j[jobID]
	return answer
}

func (j *jobTrack) GetStat() int {
	j.Lock()
	defer j.Unlock()
	count := len(j.j)
	return count
}

func (c *webCmd) runLoop() error {
	firstRun := true
	me, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get aerolab executable path: %s", err)
	}
	for {
		args := append(os.Args[1:], "--real")
		if !firstRun && !inslice.HasString(args, "--nobrowser") {
			args = append(args, "--nobrowser")
		}
		cmd := exec.Command(me, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		err = cmd.Run()
		if err != nil {
			return err
		}
		firstRun = false
		time.Sleep(time.Second)
	}
}

func (c *webCmd) jobCleaner() {
	rootDir, err := a.aerolabRootDir()
	if err != nil {
		log.Printf("ERROR: CLEANER: failed to get aerolab root dir, cleaner will not run: %s", err)
		return
	}
	rootDir = path.Join(rootDir, "weblog")
	os.MkdirAll(rootDir, 0755)
	for {
		err = filepath.Walk(rootDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".log") {
				return nil
			}
			_, fn := filepath.Split(path)
			fnsplit := strings.Split(fn, "_")
			if len(fnsplit) != 3 {
				return nil
			}
			reqID := strings.TrimSuffix(fnsplit[2], ".log")
			if c.joblist.Get(reqID) != nil {
				return nil
			}
			ts, err := time.ParseInLocation("2006-01-02_15-04-05", fnsplit[0]+"_"+fnsplit[1], time.Local)
			if err != nil {
				return nil
			}
			if time.Since(ts) < c.JobHistoryExpiry {
				return nil
			}
			err = os.Remove(path)
			if err != nil {
				log.Printf("CLEANER: WARN: Failed to remove history item %s: %s", fn, err)
			} else if c.DebugRequests {
				log.Printf("Removing history item %s", fn)
			}
			return nil
		})
		if err != nil {
			log.Printf("ERROR: CLEANER: failed to walk job history directory: %s", err)
		}
		time.Sleep(time.Minute)
	}
}

func (c *webCmd) CheckUpdateTs() (updated bool, err error) {
	cfgFile, _, err := a.configFileName()
	if err != nil {
		return false, err
	}
	tsTmp, err := os.ReadFile(cfgFile + ".ts")
	if err != nil {
		return false, nil
	}
	lastChangeTs, err := time.Parse(time.RFC3339, string(tsTmp))
	if err != nil {
		return false, err
	}
	if !lastChangeTs.After(c.cfgTs) {
		return false, nil
	}
	log.Println("Config file changed, refreshing settings")
	c.defaultsRefresh()
	c.inventoryNames = c.getInventoryNames()
	c.cfgTs = lastChangeTs
	return true, nil
}

func (c *webCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	if !c.Real {
		return c.runLoop()
	}
	if isWebuiBeta {
		log.Print("Running webui; this feature is in beta.")
	}
	if c.UploadTempDir == "" {
		utemp, _ := a.aerolabRootDir()
		c.UploadTempDir = flags.Filename(path.Join(utemp, "web.tmp"))
	}
	c.agiTokens = NewAgiWebTokenHandler(c.AGIStrictTLS)
	c.wsCount = new(wsCounters)
	go c.wsCount.PrintTimer(time.Second)
	c.cfgTs = time.Now()
	c.joblist = &jobTrack{
		j: make(map[string]*exec.Cmd),
	}
	c.jobqueue = jobqueue.New(c.MaxConcurrentJobs, c.MaxQueuedJobs)
	go c.jobCleaner()
	c.cache = &inventoryCache{
		RefreshInterval: c.RefreshInterval,
		MinimumInterval: c.MinInterval,
		runLock:         new(sync.Mutex),
		inv:             &inventoryJson{},
		ilcMutex:        new(sync.RWMutex),
		agiStatusLock:   new(sync.Mutex),
		c:               c,
	}
	c.inventoryNames = c.getInventoryNames()
	c.cache.ilcMutex.Lock()
	go func() {
		defer c.cache.ilcMutex.Unlock()
		log.Print("Getting initial inventory state")
		err := c.cache.Start(c.CheckUpdateTs)
		if err != nil {
			log.Printf("WARNING: Inventory query failure: %s", err)
		} else {
			log.Print("Initial Inventory obtained")
			if a.opts.Config.Backend.Type != "docker" {
				log.Print("Obtaining initial inventory instance-types")
				zone := "us-central1-a"
				if a.opts.Cluster.Create.Gcp.Zone != "" {
					zone = string(a.opts.Cluster.Create.Gcp.Zone)
				}
				exec.Command(os.Args[0], "inventory", "instance-types", "-j", "--zone", zone).CombinedOutput()
				exec.Command(os.Args[0], "inventory", "instance-types", "-j", "--arm", "--zone", zone).CombinedOutput()
				log.Print("Instance Type cache refreshed")
			}
		}
	}()
	go func() {
		var statc, statq, statj int
		for {
			nstatj := c.joblist.GetStat()
			nstatc, nstatq := c.jobqueue.GetSize()
			if nstatc != statc || nstatq != statq || nstatj != statj {
				statc = nstatc
				statq = nstatq
				statj = nstatj
				log.Printf("STAT: queue_active_jobs=%d queued_jobs=%d jobs_tracked=%d", statc, statq, statj)
			}
			time.Sleep(60 * time.Second)
		}
	}()
	c.WebRoot = "/" + strings.Trim(c.WebRoot, "/") + "/"
	if c.WebRoot == "//" {
		c.WebRoot = "/"
	}
	err := c.genMenu()
	if err != nil {
		return err
	}
	if c.WebPath == "" {
		c.WebPath, err = a.aerolabRootDir()
		if err != nil {
			return err
		}
		c.WebPath = filepath.Join(c.WebPath, "www")
	}
	wwwVersion, err := os.ReadFile(filepath.Join(c.WebPath, "version.cfg"))
	log.Printf("WebUI version: %s, currently installed version: %s", strings.Trim(vCommit, "\r\n\t "), strings.Trim(string(wwwVersion), "\r\n\t "))
	if err != nil || strings.Trim(string(wwwVersion), "\r\n\t ") != strings.Trim(vCommit, "\r\n\t ") {
		if c.WebNoOverride {
			log.Print("WARNING: web version mismatch, not overriding")
		} else {
			log.Printf("Installing latest %s", c.WebPath)
			err = os.RemoveAll(c.WebPath)
			if err != nil {
				return err
			}
			err = webui.InstallWebsite(c.WebPath, webui.Website)
			if err != nil {
				return err
			}
		}
	}
	http.HandleFunc(c.WebRoot+"www/dist/", c.static)
	http.HandleFunc(c.WebRoot+"www/plugins/", c.static)
	http.HandleFunc(c.WebRoot+"www/api/job/", c.job)
	http.HandleFunc(c.WebRoot+"www/api/jobs/", c.jobs)
	http.HandleFunc(c.WebRoot+"www/api/commands", c.commandScript)
	http.HandleFunc(c.WebRoot+"www/api/commandh", c.commandHistory)
	http.HandleFunc(c.WebRoot+"www/api/commandjb", c.commandJupyterBash)
	http.HandleFunc(c.WebRoot+"www/api/commandjm", c.commandJupyterMagic)
	if !c.BlockServerLs {
		http.HandleFunc(c.WebRoot+"www/api/ls", c.ls)
		http.HandleFunc(c.WebRoot+"www/api/homedir", c.homedir)
	}

	c.addInventoryHandlers()
	http.HandleFunc(c.WebRoot, c.serve)
	if c.WebRoot != "/" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, c.WebRoot, http.StatusTemporaryRedirect)
		})
	}
	ret := make(chan error, len(c.ListenAddr))
	locker := new(sync.Mutex)
	for _, listenAddr := range c.ListenAddr {
		locker.Lock()
		if len(ret) > 0 {
			locker.Unlock()
			return <-ret
		}
		go func(listenAddr string) {
			prot := "tcp4"
			if strings.Count(listenAddr, ":") >= 2 {
				prot = "tcp6"
			}
			log.Printf("Listening on %s %s", prot, listenAddr)
			l, err := net.Listen(prot, listenAddr)
			if err != nil {
				time.Sleep(time.Second)
				l, err = net.Listen(prot, listenAddr)
				if err != nil {
					ret <- err
					locker.Unlock()
					return
				}
			}
			locker.Unlock()
			err = http.Serve(l, nil)
			if err != nil {
				ret <- err
			}
		}(listenAddr)
	}
	locker.Lock()
	defer locker.Unlock()
	if len(ret) > 0 {
		return <-ret
	}
	if !c.NoBrowser {
		openurl := strings.ReplaceAll(strings.ReplaceAll(c.ListenAddr[0], "0.0.0.0", "127.0.0.1"), "[::]", "[::1]")
		browser.OpenURL("http://" + openurl)
	}
	return <-ret
}

func (c *webCmd) allowls(r *http.Request) bool {
	if c.BlockServerLs {
		return false // blocked everywhere
	}
	if c.AllowLsEverywhere {
		return true // allowed everywhere
	}
	if (strings.HasPrefix(r.Host, "127.0.0.1") || strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "[::1]")) && (strings.HasPrefix(r.RemoteAddr, "127.0.0.1") || strings.HasPrefix(r.RemoteAddr, "localhost") || strings.HasPrefix(r.RemoteAddr, "[::1]")) {
		if r.Header.Get("X-Real-IP") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("Forwarded") != "" {
			return false // localhost, but using proxy, blocked
		}
		return true // localhost, no proxy detected, allowed
	}
	return false // not localhost, blocked
}

func (c *webCmd) homedir(w http.ResponseWriter, r *http.Request) {
	if a.opts.Config.Backend.Type == "" {
		http.Error(w, "pick config->backend first", http.StatusBadRequest)
		return
	}
	allowls := c.allowls(r)
	if !allowls {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	r.ParseForm()
	npath := r.FormValue("path")
	if npath == "" {
		h, e := os.UserHomeDir()
		if e != nil {
			http.Error(w, e.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte(h + "/"))
		return
	}
	p, e := os.Stat(npath)
	if e != nil {
		http.Error(w, e.Error(), http.StatusNotFound)
		return
	}
	if !p.IsDir() {
		ndir, _ := filepath.Split(npath)
		w.Write([]byte(ndir))
		return
	}
	w.Write([]byte(npath))
}

func (c *webCmd) ls(w http.ResponseWriter, r *http.Request) {
	if a.opts.Config.Backend.Type == "" {
		http.Error(w, "pick config->backend first", http.StatusBadRequest)
		return
	}
	allowls := c.allowls(r)
	if !allowls {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	r.ParseForm()
	npath := r.FormValue("path")
	if npath == "" {
		http.Error(w, "path cannot be empty", http.StatusBadRequest)
		return
	}
	s, e := os.Stat(npath)
	if e != nil {
		http.Error(w, e.Error(), http.StatusNotFound)
		return
	}
	out := make(map[string]interface{})
	if !s.IsDir() {
		w.Write([]byte("GOUP"))
		return
	}
	entries, err := os.ReadDir(npath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()] = struct{}{}
			continue
		}
		out[e.Name()] = path.Join(npath, e.Name())
	}
	json.NewEncoder(w).Encode(out)
}

func (c *webCmd) commandScript(w http.ResponseWriter, r *http.Request) {
	c.jobsAndCommands(w, r, WebUIJobsActionSCRIPT)
}

func (c *webCmd) commandHistory(w http.ResponseWriter, r *http.Request) {
	c.jobsAndCommands(w, r, WebUIJobsActionHIST)
}

func (c *webCmd) commandJupyterBash(w http.ResponseWriter, r *http.Request) {
	c.jobsAndCommands(w, r, WebUIJobsActionBash)
}

func (c *webCmd) commandJupyterMagic(w http.ResponseWriter, r *http.Request) {
	c.jobsAndCommands(w, r, WebUIJobsActionMagic)
}

func (c *webCmd) static(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join(c.WebPath, strings.TrimPrefix(r.URL.Path, c.WebRoot+"www/")))
}

func (c *webCmd) jobAction(w http.ResponseWriter, r *http.Request) {
	if a.opts.Config.Backend.Type == "" {
		http.Error(w, "pick config->backend first", http.StatusBadRequest)
		return
	}
	r.ParseForm()
	requestID := shortuuid.New()
	log.Printf("[%s] %s %s:%s", requestID, r.RemoteAddr, r.Method, r.RequestURI)
	if c.DebugRequests {
		for k, v := range r.PostForm {
			log.Printf("[%s]    %s=%s", requestID, k, v)
		}
	}

	urlParse := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, c.WebRoot), "/"), "/")
	jobID := urlParse[len(urlParse)-1]
	actions := r.PostForm["action"]
	if len(actions) == 0 {
		actions = []string{"sigint"}
	}
	run := c.joblist.Get(jobID)
	if run == nil {
		http.Error(w, "JobID not found, or job already complete", http.StatusBadRequest)
		return
	}
	if run.ProcessState != nil {
		http.Error(w, "Job already complete", http.StatusBadRequest)
		return
	}
	if run.Process == nil {
		http.Error(w, "Job not running", http.StatusBadRequest)
		return
	}
	var err error
	switch actions[0] {
	case "sigint":
		err = run.Process.Signal(os.Interrupt)
	case "sigkill":
		err = run.Process.Signal(os.Kill)
	default:
		http.Error(w, "abort signal type not supported", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "Failed to send signal to process: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

var ErrSuccess = errors.New("success")

func (c *webCmd) getJobPath(requestID string) (string, error) {
	rootDir, err := a.aerolabRootDir()
	if err != nil {
		return "", fmt.Errorf("failed to get root aerolab dir: %s", err)
	}
	fileSuffix := "_" + requestID + ".log"
	rootDir = path.Join(rootDir, "weblog")
	answer := ""
	err = filepath.Walk(rootDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, fileSuffix) {
			answer = path
			return ErrSuccess
		}
		return nil
	})
	if err != nil && err == ErrSuccess {
		return answer, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to walk weblog dir: %s", err)
	}
	return "", errors.New("job not found")
}

func (c *webCmd) jobs(w http.ResponseWriter, r *http.Request) {
	c.jobsAndCommands(w, r, WebUIJobsActionJOBS)
}

const (
	WebUIJobsActionJOBS   = 1
	WebUIJobsActionSCRIPT = 2
	WebUIJobsActionHIST   = 3
	WebUIJobsActionBash   = 4
	WebUIJobsActionMagic  = 5
)

func (c *webCmd) jobsAndCommands(w http.ResponseWriter, r *http.Request, jobType int) {
	// handle truncate timestamp cookie
	truncate, err := r.Cookie(webui.TruncateTimestampCookieName)
	truncatestring := ""
	if err == nil && truncate.Value != "" {
		truncatestring = truncate.Value
	}
	r.ParseForm()
	if trstr := r.Form.Get(webui.TruncateTimestampCookieName); trstr != "" {
		truncatestring = trstr
	}
	truncatets := time.Time{}
	if truncatestring != "" {
		// javscript: new Date().toISOString() == 2021-07-02T13:06:53.422Z
		trunc, err := time.Parse("2006-01-02T15:04:05.000Z0700", truncatestring)
		if err == nil {
			truncatets = trunc
		}
	}
	type jsonJob struct {
		Icon           string
		RequestID      string
		Command        string
		StartedWhen    string    // 5 days / 3 hours / 8 mins
		IsRunning      bool      // is it still running
		IsFailed       bool      // if it is not running, has it failed with an error instead of success
		startTimestamp time.Time // for sorting purposes
		CmdLine        string
		FilePath       string
		UserName       string
	}
	type jsonJobs struct {
		Jobs         []*jsonJob
		HasRunning   bool // does the joblist have running jobs in the list
		HasFailed    bool // does the joblist have failed jobs in the list
		RunningCount int
	}
	var jobs = &jsonJobs{}
	rootDir, err := a.aerolabRootDir()
	if err != nil {
		http.Error(w, "could not get aerolab root directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rootDir = path.Join(rootDir, "weblog")
	os.MkdirAll(rootDir, 0755)
	err = filepath.Walk(rootDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".log") {
			return nil
		}
		ndir, fn := filepath.Split(path)
		_, nusr := filepath.Split(strings.TrimRight(ndir, "/"))
		if nusr == "weblog" && !strings.HasSuffix(strings.TrimRight(ndir, "/"), "weblog/weblog") {
			nusr = "N/A"
		}
		nUser := r.Header.Get("x-auth-aerolab-user")
		if nUser == "" {
			nUser = currentOwnerUser
		}
		if r.FormValue("jobsAllUsers") != "true" {
			if nUser != nusr && nusr != "N/A" {
				return nil
			}
			nusr = ""
		}
		fnsplit := strings.Split(fn, "_")
		if len(fnsplit) != 3 {
			return nil
		}
		ts, err := time.ParseInLocation("2006-01-02_15-04-05", fnsplit[0]+"_"+fnsplit[1], time.Local)
		if err != nil {
			return nil
		}
		reqID := strings.TrimSuffix(fnsplit[2], ".log")
		runJob := c.joblist.Get(reqID)
		if ts.Before(truncatets) && runJob == nil {
			return nil
		}
		startedWhen := ""
		tsDur := time.Since(ts)
		if tsDur > 24*time.Hour {
			days := tsDur.Hours() / 24
			if days > 365 && days < 730 {
				startedWhen = "1 year"
			} else if days > 730 {
				startedWhen = ">1 year"
			} else if days < 7 {
				startedWhen = strconv.Itoa(int(math.Ceil(days))) + " days"
			} else {
				startedWhen = strconv.Itoa(int(math.Ceil(days/7))) + " weeks"
			}
		} else if tsDur > time.Hour {
			startedWhen = strconv.Itoa(int(math.Ceil(tsDur.Hours()))) + " hours"
		} else if tsDur > time.Minute {
			startedWhen = strconv.Itoa(int(math.Ceil(tsDur.Minutes()))) + " mins"
		} else {
			startedWhen = strconv.Itoa(int(math.Ceil(tsDur.Seconds()))) + " secs"
		}
		j := &jsonJob{
			RequestID:      reqID,
			startTimestamp: ts,
			StartedWhen:    startedWhen,
			FilePath:       path,
			UserName:       nusr,
		}
		if runJob != nil {
			j.IsRunning = true
			jobs.HasRunning = true
			jobs.RunningCount++
		}
		f, err := os.Open(path)
		if err != nil {
			log.Printf("Failed to read `%s` for job list: %s", path, err)
			return nil
		}
		defer f.Close()
		rd := bufio.NewScanner(f)
		for rd.Scan() {
			line := strings.Trim(rd.Text(), "\r\n\t ")
			if strings.HasPrefix(line, "-=-=-=-=- [command] ") && strings.HasSuffix(line, " -=-=-=-=-") {
				ncmd := strings.TrimSuffix(strings.TrimPrefix(line, "-=-=-=-=- [command] "), " -=-=-=-=-")
				j.Command = ncmd
				if runJob != nil && j.CmdLine != "" {
					break
				}
			}
			if strings.HasPrefix(line, "-=-=-=-=- [cmdline] ") && strings.HasSuffix(line, " -=-=-=-=-") {
				ncmd := strings.TrimSuffix(strings.TrimPrefix(line, "-=-=-=-=- [cmdline] "), " -=-=-=-=-")
				j.CmdLine = ncmd
				if runJob != nil && j.Command != "" {
					break
				}
			}
			if strings.HasPrefix(line, "-=-=-=-=- [ExitCode] ") && strings.HasSuffix(line, " -=-=-=-=-") {
				nexitCode := strings.TrimSuffix(strings.TrimPrefix(line, "-=-=-=-=- [ExitCode] "), " -=-=-=-=-")
				if nexitCode != "0" {
					j.IsFailed = true
					jobs.HasFailed = true
				}
			}
		}
		for _, comm := range c.commands {
			if comm.path == strings.Join(strings.Split(j.Command, " "), "/") {
				j.Icon = comm.icon
				break
			}
		}
		jobs.Jobs = append(jobs.Jobs, j)
		return nil
	})
	if err != nil {
		http.Error(w, "could not get job list: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jupyterType := jupyter.TypeBash
	switch jobType {
	case WebUIJobsActionMagic:
		jupyterType = jupyter.TypeMagic
		fallthrough
	case WebUIJobsActionBash:
		j := jupyter.New(jupyterType)
		for _, job := range jobs.Jobs {
			jobStatus := "SUCCESS exitCode"
			errorCode := 0
			if job.IsFailed {
				jobStatus = "FAILED exitCode"
				errorCode = 1
			} else if job.IsRunning {
				jobStatus = "RUNNING exitCode"
				errorCode = 255
			}
			fc, err := os.ReadFile(job.FilePath)
			if err != nil {
				fc = []byte("-=-=-=-=- [Log] -=-=-=-=-\n" + err.Error())
			}
			ansLog := strings.Split(string(fc), "-=-=-=-=- [Log] -=-=-=-=-\n")
			nLog := ""
			exitCode := ""
			if len(ansLog) > 1 {
				ansCode := strings.Split(ansLog[1], "-=-=-=-=- [ExitCode]")
				nLog = strings.TrimRight(ansCode[0], "\n")
				if len(ansCode) > 1 {
					ansCode = strings.Split(ansCode[1], " -=-=-=-=-")
					exitCode = strings.TrimRight(ansCode[0], "\n ")
				}
			}
			if job.IsFailed && exitCode != "0" {
				errorCode, err = strconv.Atoi(exitCode)
				if err != nil {
					errorCode = 1
				}
			}
			j.AddCell(job.CmdLine, nLog, errorCode, jobStatus)
		}
		je := json.NewEncoder(w)
		je.SetIndent("", "  ")
		je.Encode(j)
	case WebUIJobsActionHIST:
		sort.Slice(jobs.Jobs, func(i, j int) bool {
			return jobs.Jobs[i].startTimestamp.Before(jobs.Jobs[j].startTimestamp)
		})
		w.Write([]byte("# AeroLab Command History\n\n" + time.Now().Format(time.RFC1123) + "\n\n## Command List\n\nCommand | Start Time | Status\n--- | --- | ---\n"))
		for _, job := range jobs.Jobs {
			jobStatus := "SUCCESS"
			if job.IsFailed {
				jobStatus = "FAILED"
			} else if job.IsRunning {
				jobStatus = "RUNNING"
			}
			w.Write([]byte("[`" + job.CmdLine + "`](#JOBID-" + job.RequestID + ") | " + job.startTimestamp.Format(time.RFC3339) + " | " + jobStatus + "\n")) //[`aerolab inventory list`](#inventory-list) | 22:04:57 | RUNNING
		}
		w.Write([]byte("\n## Command Details\n\n"))
		for _, job := range jobs.Jobs {
			fc, err := os.ReadFile(job.FilePath)
			if err != nil {
				fc = []byte("-=-=-=-=- [Log] -=-=-=-=-\n" + err.Error())
			}
			ans := strings.ReplaceAll(string(fc), "```", "`'`")
			ansLog := strings.Split(ans, "-=-=-=-=- [Log] -=-=-=-=-\n")
			nLog := ""
			exitCode := ""
			if len(ansLog) > 1 {
				ansCode := strings.Split(ansLog[1], "-=-=-=-=- [ExitCode]")
				nLog = strings.TrimRight(ansCode[0], "\n")
				if len(ansCode) > 1 {
					exitCode = "-=-=-=-=- [ExitCode]" + strings.TrimRight(strings.TrimSuffix(ansCode[1], "-=-=-=-=- [END] -=-=-=-=-"), "\n")
				}
			}
			w.Write([]byte("### " + job.Command + "\n\n#### JOBID-" + job.RequestID + "\n\n#### Command\n\n" + "```\n" + job.CmdLine + "\n```\n\n#### Output\n\n```\n" + nLog))
			w.Write([]byte("\n```\n\n"))
			if job.IsRunning {
				w.Write([]byte("#### Job still running\n\n"))
			} else {
				w.Write([]byte("#### Exit Code\n\n```\n" + exitCode))
				w.Write([]byte("\n```\n\n"))
			}
		}
	case WebUIJobsActionSCRIPT:
		sort.Slice(jobs.Jobs, func(i, j int) bool {
			return jobs.Jobs[i].startTimestamp.Before(jobs.Jobs[j].startTimestamp)
		})
		script := "# [ALL_JOBS_SUCCEEDED]\n"
		if jobs.HasFailed && jobs.HasRunning {
			script = "# [HAS_FAILED_JOBS] [HAS_RUNNING_JOBS]\n"
		} else if jobs.HasFailed {
			script = "# [HAS_FAILED_JOBS]\n"
		} else if jobs.HasRunning {
			script = "# [HAS_RUNNING_JOBS]\n"
		}
		w.Write([]byte(script))
		for _, job := range jobs.Jobs {
			script := "\n" + job.CmdLine
			if job.IsFailed {
				script = script + " #FAILED"
			}
			if job.IsRunning {
				script = script + " #RUNNING"
			}
			script = script + "\n"
			script = script + "# " + job.startTimestamp.Format(time.RFC3339)
			if job.IsFailed {
				script = script + " [FAILED]"
			} else if job.IsRunning {
				script = script + " [RUNNING]"
			} else {
				script = script + " [SUCCESS]"
			}
			script = script + " [LOG:" + job.FilePath + "]\n"
			w.Write([]byte(script))
		}
	case WebUIJobsActionJOBS:
		fallthrough
	default:
		sort.Slice(jobs.Jobs, func(i, j int) bool {
			if jobs.Jobs[i].IsRunning && !jobs.Jobs[j].IsRunning {
				return true
			}
			if !jobs.Jobs[i].IsRunning && jobs.Jobs[j].IsRunning {
				return false
			}
			return jobs.Jobs[i].startTimestamp.After(jobs.Jobs[j].startTimestamp)
		})
		if len(jobs.Jobs)-jobs.RunningCount > c.ShowMaxHistoryItems {
			jobs.Jobs = jobs.Jobs[0:(jobs.RunningCount + c.ShowMaxHistoryItems)]
		}
		json.NewEncoder(w).Encode(jobs)
	}
}

func (c *webCmd) job(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		c.jobAction(w, r)
		return
	}

	urlParse := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, c.WebRoot), "/"), "/")
	requestID := urlParse[len(urlParse)-1]

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}
	fn, err := c.getJobPath(requestID)
	if err != nil {
		http.Error(w, "job not found", http.StatusBadRequest)
		return
	}
	f, err := os.Open(fn)
	if err != nil {
		http.Error(w, "job file read error: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	start := time.Now()
	for {
		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, f); err != nil && err != io.EOF {
			w.Write([]byte("ERROR receiving data from subprocess: " + err.Error()))
			break
		}
		data := buf.String()
		w.Write([]byte(data))
		flusher.Flush()
		if strings.Contains(data, "-=-=-=-=- [END] -=-=-=-=-") {
			break
		}
		if time.Since(start) > c.AbsoluteTimeout {
			break
		}
		time.Sleep(time.Second)
	}
}

func (c *webCmd) fillMenu(commandMap map[string]interface{}, titler cases.Caser, commands []*apiCommand, commandsIndex map[string]int, spath string, hiddenItems []string) (ret []*webui.MenuItem) {
	for comm, sub := range commandMap {
		wpath := path.Join(spath, comm)
		isHidden := false
		for _, hiddenItem := range hiddenItems {
			if wpath == hiddenItem || strings.HasPrefix(wpath, hiddenItem+"/") {
				isHidden = true
				break
			}
		}
		if isHidden {
			continue
		}
		name := titler.String(strings.ReplaceAll(comm, "-", " "))
		ret = append(ret, &webui.MenuItem{
			Icon:    commands[commandsIndex[wpath]].icon,
			Name:    name,
			Href:    c.WebRoot + commands[commandsIndex[wpath]].path,
			Tooltip: commands[commandsIndex[wpath]].description,
		})
		if len(sub.(map[string]interface{})) > 0 {
			ret[len(ret)-1].Items = c.fillMenu(sub.(map[string]interface{}), titler, commands, commandsIndex, wpath, hiddenItems)
		}
	}
	return ret
}

func (c *webCmd) sortMenu(items []*webui.MenuItem, commandsIndex map[string]int) {
	sort.Slice(items, func(i, j int) bool {
		return commandsIndex[items[i].Href[len(c.WebRoot):]] < commandsIndex[items[j].Href[len(c.WebRoot):]]
	})
	for i := range items {
		if len(items[i].Items) > 0 {
			c.sortMenu(items[i].Items, commandsIndex)
		}
	}
}

func (c *webCmd) genMenu() error {
	commandMap := make(map[string]interface{})
	commands := []*apiCommand{}
	commandsIndex := make(map[string]int) // map[path]idx <- commands[idx]
	ret := make(chan apiCommand, 1)
	keyField := reflect.ValueOf(a.opts).Elem()
	var hiddenItems []string
	go a.opts.Rest.getCommands(keyField, "", ret, "")
	for {
		val, ok := <-ret
		if !ok {
			break
		}
		if val.isHidden || val.isWebHidden {
			hiddenItems = append(hiddenItems, val.path)
			continue
		}
		if val.pathStack[len(val.pathStack)-1] == "help" {
			continue
		}
		commandsIndex[val.path] = len(commands)
		commands = append(commands, &val)
		cm := commandMap
		for _, pp := range val.pathStack {
			if _, ok := cm[pp]; !ok {
				cm[pp] = make(map[string]interface{})
			}
			cm = cm[pp].(map[string]interface{})
		}
	}
	titler := cases.Title(language.English)
	c.menuItems = append([]*webui.MenuItem{
		{
			Icon:          "fas fa-list",
			Name:          "Inventory",
			Href:          c.WebRoot,
			DrawSeparator: true,
		},
	}, c.fillMenu(commandMap, titler, commands, commandsIndex, "", hiddenItems)...)
	c.sortMenu(c.menuItems, commandsIndex)
	c.commands = commands
	c.commandsIndex = commandsIndex
	c.titler = titler
	return nil
}

func (c *webCmd) getFormItems(urlPath string, r *http.Request) ([]*webui.FormItem, error) {
	cindex, ok := c.commandsIndex[strings.TrimPrefix(urlPath, c.WebRoot)]
	if !ok {
		return nil, errors.New("command not found")
	}
	command := c.commands[cindex]
	return c.getFormItemsRecursive(command.Value, "", r)
}

func (c *webCmd) getFormItemsRecursive(commandValue reflect.Value, prefix string, r *http.Request) ([]*webui.FormItem, error) {
	allowedLs := c.allowls(r)
	wf := []*webui.FormItem{}
	for i := 0; i < commandValue.Type().NumField(); i++ {
		name := commandValue.Type().Field(i).Name
		kind := commandValue.Field(i).Kind()
		tags := commandValue.Type().Field(i).Tag
		if tags.Get("hidden") == "true" {
			continue
		}
		if name[0] < 65 || name[0] > 90 {
			if kind == reflect.Struct {
				wfs, err := c.getFormItemsRecursive(commandValue.Field(i), prefix, r)
				if err != nil {
					return nil, err
				}
				wf = append(wf, wfs...)
			}
			continue
		}
		switch kind {
		case reflect.String:
			// select items - choice/multichoice
			if tags.Get("webchoice") != "" {
				multi := false
				if tags.Get("webmulti") != "" {
					multi = true
				}
				choices := []*webui.FormItemSelectItem{}
				required := false
				if tags.Get("webrequired") == "true" && commandValue.Field(i).String() == "" {
					required = true
				}
				if strings.HasPrefix(tags.Get("webchoice"), "method::") {
					method := strings.TrimPrefix(tags.Get("webchoice"), "method::")
					nt := commandValue.Field(i).MethodByName(method)
					if nt.IsValid() && !nt.IsNil() {
						zone := "us-central1-a"
						if a.opts.Cluster.Create.Gcp.Zone != "" {
							zone = string(a.opts.Cluster.Create.Gcp.Zone)
						}
						ret := nt.Call([]reflect.Value{reflect.ValueOf(zone)})
						if len(ret) > 1 {
							retErr := ret[1]
							retDefault := commandValue.Field(i).String()
							if len(ret) > 2 {
								retErr = ret[2]
								if retDefault == "" {
									retDefault = ret[1].String()
								}
							}
							if retErr.IsNil() {
								ri := ret[0].Interface().([][]string)
								for _, choice := range ri {
									isSelected := false
									if choice[0] == retDefault {
										isSelected = true
									}
									choices = append(choices, &webui.FormItemSelectItem{
										Name:     choice[1],
										Value:    choice[0],
										Selected: isSelected,
									})
								}
							} else {
								log.Printf("WARN: instance-types: %s", retErr.Interface())
							}
						}
					}
				} else {
					for _, choice := range strings.Split(tags.Get("webchoice"), ",") {
						isSelected := false
						if choice == commandValue.Field(i).String() {
							isSelected = true
						}
						choices = append(choices, &webui.FormItemSelectItem{
							Name:     choice,
							Value:    choice,
							Selected: isSelected,
						})
					}
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Select: true,
					},
					Select: webui.FormItemSelect{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Description: tags.Get("description"),
						Multiple:    multi,
						Items:       choices,
						Required:    required,
					},
				})
			} else {
				// input item text (possible multiple types)
				isFile := false
				textType := tags.Get("webtype")
				if textType == "" {
					textType = "text"
				}
				if commandValue.Field(i).Type().String() == "flags.Filename" {
					if allowedLs {
						isFile = true
					} else if c.MaxUploadSizeBytes > 0 && tags.Get("webtype") == "" {
						textType = "file"
					}
				}
				required := false
				if tags.Get("webrequired") == "true" && commandValue.Field(i).String() == "" {
					required = true
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        textType,
						Default:     commandValue.Field(i).String(),
						Description: tags.Get("description"),
						Required:    required,
						IsFile:      isFile,
					},
				})
			}
		case reflect.Float64:
			// input item number
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Input: true,
				},
				Input: webui.FormItemInput{
					Name:        name,
					ID:          "xx" + prefix + "xx" + name,
					Type:        "number",
					Default:     strconv.FormatFloat(commandValue.Field(i).Float(), 'f', 4, 64),
					Description: tags.Get("description"),
				},
			})
		case reflect.Int:
			// input item number
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Input: true,
				},
				Input: webui.FormItemInput{
					Name:        name,
					ID:          "xx" + prefix + "xx" + name,
					Type:        "number",
					Default:     strconv.Itoa(int(commandValue.Field(i).Int())),
					Description: tags.Get("description"),
				},
			})
		case reflect.Bool:
			// toggle on-off
			paramOn := commandValue.Field(i).Bool()
			paramDisable := false
			descriptionHead := ""
			if tags.Get("webdisable") == "true" {
				paramDisable = true
				descriptionHead = "(disabled for webui) "
			}
			if tags.Get("webset") == "true" {
				paramOn = true
			}
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Toggle: true,
				},
				Toggle: webui.FormItemToggle{
					Name:        name,
					Description: descriptionHead + tags.Get("description"),
					ID:          "xx" + prefix + "xx" + name,
					On:          paramOn,
					Disabled:    paramDisable,
				},
			})
		case reflect.Struct:
			// recursion
			if name == "Help" {
				continue
			}
			if (name == "Aws" && a.opts.Config.Backend.Type == "aws") || (name == "Docker" && a.opts.Config.Backend.Type == "docker") || (name == "Gcp" && a.opts.Config.Backend.Type == "gcp") || (!inslice.HasString([]string{"Aws", "Gcp", "Docker"}, name)) {
				sep := name
				if prefix != "" {
					sep = prefix + "." + name
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Separator: true,
					},
					Separator: webui.FormItemSeparator{
						Name: sep,
					},
				})
				wfs, err := c.getFormItemsRecursive(commandValue.Field(i), sep, r)
				if err != nil {
					return nil, err
				}
				wf = append(wf, wfs...)
			}
		case reflect.Slice:
			// tag input
			val := []string{}
			for j := 0; j < commandValue.Field(i).Len(); j++ {
				val = append(val, commandValue.Field(i).Index(j).String())
			}
			required := false
			if tags.Get("webrequired") == "true" && len(val) == 0 {
				required = true
			}
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Input: true,
				},
				Input: webui.FormItemInput{
					Name:        name,
					ID:          "xx" + prefix + "xx" + name,
					Type:        "text",
					Default:     strings.Join(val, ","),
					Description: tags.Get("description"),
					Tags:        true,
					Required:    required,
				},
			})
		case reflect.Int64:
			if commandValue.Field(i).Type().String() == "time.Duration" {
				// duration
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        "text",
						Default:     time.Duration(commandValue.Field(i).Int()).String(),
						Description: tags.Get("description"),
					},
				})
			} else if commandValue.Field(i).Type().String() == "int64" {
				// input item number
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        "number",
						Default:     strconv.Itoa(int(commandValue.Field(i).Int())),
						Description: tags.Get("description"),
					},
				})
			} else {
				return nil, fmt.Errorf("unknown field type (name=%s kind=%s type=%s)", name, kind.String(), commandValue.Field(i).Type().String())
			}
		case reflect.Ptr:
			if commandValue.Field(i).Type().String() == "*flags.Filename" {
				defStr := ""
				if !commandValue.Field(i).IsNil() {
					defStr = commandValue.Field(i).Elem().String()
				}
				// input type text
				isFile := false
				textType := tags.Get("webtype")
				if textType == "" {
					textType = "text"
				}
				if allowedLs {
					isFile = true
				} else if c.MaxUploadSizeBytes > 0 && tags.Get("webtype") == "" {
					textType = "file"
				}
				required := false
				if tags.Get("webrequired") == "true" && defStr == "" {
					required = true
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        textType,
						Default:     defStr,
						Description: tags.Get("description"),
						Required:    required,
						Optional:    true,
						IsFile:      isFile,
					},
				})
			} else if commandValue.Field(i).Type().String() == "*bool" {
				// toggle on-off - special - allow setting on/off/unset
				paramOn := false
				if !commandValue.Field(i).IsNil() {
					paramOn = commandValue.Field(i).Elem().Bool()
				}
				paramDisable := false
				descriptionHead := ""
				if tags.Get("webdisable") == "true" {
					paramDisable = true
					descriptionHead = "(disabled for webui) "
				}
				if tags.Get("webset") == "true" {
					paramOn = true
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Toggle: true,
					},
					Toggle: webui.FormItemToggle{
						Name:        name,
						Description: descriptionHead + tags.Get("description"),
						ID:          "xx" + prefix + "xx" + name,
						On:          paramOn,
						Disabled:    paramDisable,
						Optional:    true,
					},
				})
			} else if commandValue.Field(i).Type().String() == "*int" {
				// input item number - special - allow no setting if it's empty
				noDef := ""
				if !commandValue.Field(i).IsZero() {
					noDef = strconv.Itoa(int(commandValue.Field(i).Elem().Int()))
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        "number",
						Default:     noDef,
						Description: tags.Get("description"),
						Optional:    true,
					},
				})
			} else if commandValue.Field(i).Type().String() == "*string" {
				// select items - choice/multichoice - special - allow not setting
				strDef := ""
				if !commandValue.Field(i).IsNil() {
					strDef = commandValue.Field(i).Elem().String()
				}
				if tags.Get("webchoice") != "" {
					multi := false
					if tags.Get("webmulti") != "" {
						multi = true
					}
					choices := []*webui.FormItemSelectItem{}
					required := false
					if tags.Get("webrequired") == "true" && strDef == "" {
						required = true
					}
					for _, choice := range strings.Split(tags.Get("webchoice"), ",") {
						isSelected := false
						if choice == strDef {
							isSelected = true
						}
						choices = append(choices, &webui.FormItemSelectItem{
							Name:     choice,
							Value:    choice,
							Selected: isSelected,
						})
					}
					wf = append(wf, &webui.FormItem{
						Type: webui.FormItemType{
							Select: true,
						},
						Select: webui.FormItemSelect{
							Name:        name,
							ID:          "xx" + prefix + "xx" + name,
							Description: tags.Get("description"),
							Multiple:    multi,
							Items:       choices,
							Required:    required,
							Optional:    true,
						},
					})
				} else {
					// input item text (possible multiple types)
					textType := tags.Get("webtype")
					if textType == "" {
						textType = "text"
					}
					required := false
					if tags.Get("webrequired") == "true" && strDef == "" {
						required = true
					}
					wf = append(wf, &webui.FormItem{
						Type: webui.FormItemType{
							Input: true,
						},
						Input: webui.FormItemInput{
							Name:        name,
							ID:          "xx" + prefix + "xx" + name,
							Type:        textType,
							Default:     strDef,
							Description: tags.Get("description"),
							Required:    required,
							Optional:    true,
						},
					})
				}
			} else {
				return nil, fmt.Errorf("unknown field type (name=%s kind=%s type=%s)", name, kind.String(), commandValue.Field(i).Type().String())
			}
		default:
			return nil, fmt.Errorf("unknown field type (name=%s kind=%s type=%s)", name, kind.String(), commandValue.Field(i).Type().String())
		}
	}
	return wf, nil
}

func (c *webCmd) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// if posting command, run and exit
		c.command(w, r)
		return
	}

	if _, ok := c.commandsIndex[strings.TrimPrefix(r.URL.Path, c.WebRoot)]; r.URL.Path != c.WebRoot && !ok {
		http.Error(w, "command not found: "+r.URL.Path, http.StatusNotFound)
		return
	}

	if a.opts.Config.Backend.Type == "" && r.URL.Path != c.WebRoot+"config/backend" {
		http.Redirect(w, r, c.WebRoot+"config/backend", http.StatusTemporaryRedirect)
		return
	}

	// create webpage
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// homepage
	if r.URL.Path == c.WebRoot {
		c.inventory(w, r)
		return
	}

	title := strings.Trim(strings.TrimPrefix(r.URL.Path, c.WebRoot), "\r\n\t /")
	title = c.titler.String(strings.ReplaceAll(title, "/", " / "))
	title = strings.ReplaceAll(title, "-", " ")

	var errStr string
	var errTitle string
	var isError bool
	formItems, err := c.getFormItems(r.URL.Path, r)
	if err != nil {
		errStr = err.Error()
		errTitle = "Failed to generate form items"
		isError = true
	}
	backendIcon := "fa-aws"
	if a.opts.Config.Backend.Type == "gcp" {
		backendIcon = "fa-google"
	}
	if a.opts.Config.Backend.Type == "docker" {
		backendIcon = "fa-docker"
	}
	isShowUsersChecked := false
	showAllUsersCookie, err := r.Cookie("AEROLAB_SHOW_ALL_USERS")
	if err == nil {
		if showAllUsersCookie.Value == "true" {
			isShowUsersChecked = true
		}
	}
	isShowDefaults := false
	isShowDefaultsC, err := r.Cookie("aerolab_default_switches")
	if err == nil {
		if isShowDefaultsC.Value == "true" {
			isShowDefaults = true
		}
	}
	isShortSwitches := false
	isShortSwitchesC, err := r.Cookie("aerolab_short_switches")
	if err == nil {
		if isShortSwitchesC.Value == "true" {
			isShortSwitches = true
		}
	}
	p := &webui.Page{
		WebRoot:                                 c.WebRoot,
		FixedNavbar:                             true,
		FixedFooter:                             true,
		PendingActionsShowAllUsersToggle:        true,
		PendingActionsShowAllUsersToggleChecked: isShowUsersChecked,
		IsError:                                 isError,
		ErrorString:                             errStr,
		ErrorTitle:                              errTitle,
		IsForm:                                  true,
		FormItems:                               formItems,
		FormCommandTitle:                        title,
		BetaTag:                                 isWebuiBeta,
		ShortSwitches:                           isShortSwitches,
		ShowDefaults:                            isShowDefaults,
		CurrentUser:                             r.Header.Get("X-Auth-Aerolab-User"),
		Navigation: &webui.Nav{
			Top: []*webui.NavTop{
				{
					Name:   "Home",
					Href:   c.WebRoot,
					Target: "_self",
				},
				{
					Name:   "AsbenchUI",
					Href:   strings.TrimRight(c.WebRoot, "/") + "/www/dist/asbench/index.html",
					Target: "_blank",
				},
				{
					Name:   "<i class=\"fa-brands " + backendIcon + "\"></i>",
					Href:   strings.TrimRight(c.WebRoot, "/") + "/config/backend",
					Target: "_self",
				},
			},
		},
		Menu: &webui.MainMenu{
			Items: c.menuItems,
		},
	}
	p.Menu.Items.Set(r.URL.Path, c.WebRoot)
	www := os.DirFS(c.WebPath)
	t, err := template.ParseFS(www, "*.html", "*.js", "*.css")
	if err != nil {
		log.Print(err)
		return
	}
	err = t.ExecuteTemplate(w, "main", p)
	if err != nil {
		log.Print(err)
		return
	}
}

func (c *webCmd) switchName(useShortSwitches bool, tag reflect.StructTag) string {
	if useShortSwitches {
		ret := "-" + tag.Get("short")
		if ret == "-" {
			ret = "--" + tag.Get("long")
		}
		return ret
	}
	return "--" + tag.Get("long")
}

func (c *webCmd) getFieldNames(cmd reflect.Value) []string {
	fnames := []string{}
	for x := 0; x < cmd.NumField(); x++ {
		if !cmd.Type().Field(x).Anonymous {
			fnames = append(fnames, cmd.Type().Field(x).Name)
		} else {
			fnames = append(fnames, c.getFieldNames(cmd.Field(x))...)
		}
	}
	return fnames
}

func (c *webCmd) command(w http.ResponseWriter, r *http.Request) {
	if c.MaxUploadSizeBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, int64(c.MaxUploadSizeBytes+(1024*1024)))
	}
	// log method, URI and parameters
	var err error
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") && c.MaxUploadSizeBytes > 0 {
		err = r.ParseMultipartForm(1024 * 1024 * 10)
	} else {
		err = r.ParseForm()
	}
	if err != nil {
		http.Error(w, "error parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// log request
	requestID := shortuuid.New()
	log.Printf("[%s] %s %s:%s", requestID, r.RemoteAddr, r.Method, r.RequestURI)
	if c.DebugRequests {
		for k, v := range r.PostForm {
			log.Printf("[%s]    %s=%s", requestID, k, v)
		}
	}

	// get command
	cindex, ok := c.commandsIndex[strings.TrimPrefix(r.URL.Path, c.WebRoot)]
	if !ok {
		http.Error(w, "command not found: "+r.URL.Path, http.StatusBadRequest)
		return
	}
	command := c.commands[cindex].Value
	cjson := make(map[string]interface{})
	logjson := make(map[string]interface{})
	cmdline := append([]string{"aerolab"}, c.commands[cindex].pathStack...)
	action := r.PostForm["action"]
	useShortSwitchesTmp := r.PostForm["useShortSwitches"]
	useShortSwitches := false
	if len(useShortSwitchesTmp) > 0 && useShortSwitchesTmp[0] == "true" {
		useShortSwitches = true
	}
	useShowDefaultsTmp := r.PostForm["useShowDefaults"]
	useShowDefaults := false
	if len(useShowDefaultsTmp) > 0 && useShowDefaultsTmp[0] == "true" {
		useShowDefaults = true
	}
	if len(action) != 1 || (action[0] != "show" && action[0] != "run") {
		http.Error(w, "invalid action specification in form", http.StatusBadRequest)
		return
	}

	// sort postForm
	postForm := [][]interface{}{}
	for k, v := range r.PostForm {
		if !strings.HasPrefix(k, "xx") {
			continue
		}
		if len(v) == 0 || (len(v) == 1 && v[0] == "") {
			continue
		}
		postForm = append(postForm, []interface{}{k, v})
	}

	// handle file upload details
	tmpDir := ""
	fileUploads := make(map[*multipart.FileHeader]string)
	if action[0] != "show" && r.MultipartForm != nil {
		tmpDir = path.Join(string(c.UploadTempDir), requestID)
		fileNo := 0
		for fn, fv := range r.MultipartForm.File {
			for _, files := range fv {
				fileNo++
				newFile := path.Join(tmpDir, strconv.Itoa(fileNo))
				found := false
				for i := range postForm {
					if postForm[i][0] == fn {
						postForm[i][1] = []string{newFile}
						found = true
						break
					}
				}
				if !found {
					http.Error(w, "http form error", http.StatusInternalServerError)
					return
				}
				hd := files
				fileUploads[hd] = newFile
			}
		}
	}

	sort.Slice(postForm, func(i, j int) bool {
		ki := postForm[i][0].(string)
		kj := postForm[j][0].(string)
		kiPath := strings.Split(strings.TrimPrefix(ki, "xx"), "xx")
		kjPath := strings.Split(strings.TrimPrefix(kj, "xx"), "xx")
		if len(kiPath) < len(kjPath) {
			return true
		}
		if len(kiPath) > len(kjPath) {
			return false
		}
		for x, pathItem := range kiPath {
			if x == len(kiPath)-1 {
				break
			}
			if pathItem < kjPath[x] {
				return true
			}
			if pathItem > kjPath[x] {
				return false
			}
		}
		cmd := reflect.Indirect(command)
		for i, depth := range kiPath {
			if i == 0 && depth == "" {
				continue
			}
			if i == len(kiPath)-1 {
				break
			}
			cmd = cmd.FieldByName(depth)
		}
		parami := kiPath[len(kiPath)-1]
		paramj := kjPath[len(kjPath)-1]
		indexi := -1
		indexj := -1
		fieldNames := c.getFieldNames(cmd)
		for x, name := range fieldNames {
			if name == parami {
				indexi = x
			}
			if name == paramj {
				indexj = x
			}
			if indexi >= 0 && indexj >= 0 {
				break
			}
		}
		return indexi < indexj
	})

	if c.UniqueFirewalls && a.opts.Config.Backend.Type != "docker" && len(cmdline) >= 3 && ((cmdline[1] == "cluster" && cmdline[2] == "create") || (cmdline[1] == "client" && cmdline[2] == "create") || (cmdline[1] == "template" && cmdline[2] == "create")) {
		fwsw := "xxGcpxxNamePrefix"
		if a.opts.Config.Backend.Type == "aws" {
			fwsw = "xxAwsxxNamePrefix"
		}
		fwFound := false
		for _, kv := range postForm {
			if kv[0].(string) == fwsw {
				fwFound = true
				break
			}
		}
		if !fwFound {
			nUser := r.Header.Get("x-auth-aerolab-user")
			if nUser == "" {
				nUser = currentOwnerUser
			}
			nUser = strings.ToLower(nUser)
			nFW := ""
			for _, c := range nUser {
				if (c >= 97 && c <= 122) || (c >= 48 && c <= 57) {
					nFW = nFW + string(c)
				}
			}
			item := []interface{}{fwsw, []string{nFW}}
			postForm = append(postForm, item)
		}
	}

	// fill command struct
	tail := []string{"--"}
	for _, kv := range postForm {
		k := kv[0].(string)
		v := kv[1].([]string)
		cmd := reflect.Indirect(command)
		cj := cjson
		lj := logjson
		commandPath := strings.Split(strings.TrimPrefix(k, "xx"), "xx")
		for i, depth := range commandPath {
			if i == 0 && depth == "" {
				continue
			}
			if i == len(commandPath)-1 {
				break
			}
			cmd = cmd.FieldByName(depth)
			if _, ok := cj[depth]; !ok {
				cj[depth] = make(map[string]interface{})
			}
			cj = cj[depth].(map[string]interface{})
			if _, ok := lj[depth]; !ok {
				lj[depth] = make(map[string]interface{})
			}
			lj = lj[depth].(map[string]interface{})
		}
		param := commandPath[len(commandPath)-1]
		field := cmd.FieldByName(param)
		fieldType, _ := cmd.Type().FieldByName(param)
		tag := fieldType.Tag
		switch field.Kind() {
		case reflect.String:
			if v[0] != field.String() {
				cj[param] = v[0]
				if tag.Get("webtype") == "password" {
					v[0] = "****"
				}
				if tag.Get("long") == "" {
					tail = append(tail, "'"+strings.ReplaceAll(v[0], "'", "\\'")+"'")
				} else {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), "'"+strings.ReplaceAll(v[0], "'", "\\'")+"'")
				}
				lj[param] = v[0]
			}
		case reflect.Bool:
			val := false
			if v[0] == "on" {
				val = true
			}
			if val != field.Bool() {
				cj[param] = true
				lj[param] = true
				cmdline = append(cmdline, c.switchName(useShortSwitches, tag))
			}
		case reflect.Int:
			val, err := strconv.Atoi(v[0])
			if err != nil {
				http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
				return
			}
			if val != int(field.Int()) {
				cj[param] = val
				lj[param] = val
				if tag.Get("long") == "" {
					tail = append(tail, v[0])
				} else {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
				}
			}
		case reflect.Float64:
			val, err := strconv.ParseFloat(v[0], 64)
			if err != nil {
				http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
				return
			}
			if val != field.Float() {
				cj[param] = val
				lj[param] = val
				if tag.Get("long") == "" {
					tail = append(tail, v[0])
				} else {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
				}
			}
		case reflect.Slice:
			cj[param] = v
			lj[param] = v
			if tag.Get("long") == "" {
				tail = append(tail, v...)
			} else {
				for _, vv := range v {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), vv)
				}
			}
		case reflect.Int64:
			if field.Type().String() == "time.Duration" {
				dur, err := time.ParseDuration(v[0])
				if err != nil {
					http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
					return
				}
				if int64(dur) != field.Int() {
					cj[param] = dur
					lj[param] = dur
					if tag.Get("long") == "" {
						tail = append(tail, v[0])
					} else {
						cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
					}
				}
			} else if field.Type().String() == "int64" {
				val, err := strconv.Atoi(v[0])
				if err != nil {
					http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
					return
				}
				if val != int(field.Int()) {
					cj[param] = val
					lj[param] = val
					if tag.Get("long") == "" {
						tail = append(tail, v[0])
					} else {
						cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
					}
				}
			} else {
				http.Error(w, fmt.Sprintf("field %s not supported", field.Kind().String()), http.StatusBadRequest)
				return
			}
		case reflect.Ptr:
			if field.Type().String() == "*flags.Filename" {
				if v[0] != field.Elem().String() {
					cj[param] = v[0]
					lj[param] = v[0]
					if tag.Get("long") == "" {
						tail = append(tail, v[0])
					} else {
						cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
					}
				}
			} else if field.Type().String() == "*bool" {
				if v[0] != "unset" {
					val := false
					if v[0] == "on" {
						val = true
					}
					if field.IsNil() || val != field.Elem().Bool() {
						cj[param] = val
						lj[param] = val
						if val {
							cmdline = append(cmdline, c.switchName(useShortSwitches, tag))
						}
					}
				}
			} else if field.Type().String() == "*int" {
				isSet := r.PostForm["isSet-"+k]
				if len(isSet) > 0 && isSet[0] == "yes" {
					val, err := strconv.Atoi(v[0])
					if err != nil {
						http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
						return
					}
					if field.IsNil() || val != int(field.Elem().Int()) {
						cj[param] = val
						lj[param] = val
						if tag.Get("long") == "" {
							tail = append(tail, v[0])
						} else {
							cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
						}
					}
				}
			} else if field.Type().String() == "*string" {
				isSet := r.PostForm["isSet-"+k]
				if len(isSet) > 0 && isSet[0] == "yes" {
					if field.IsNil() || v[0] != field.Elem().String() {
						cj[param] = v[0]
						if tag.Get("webtype") == "password" {
							v[0] = "****"
						}
						if tag.Get("long") == "" {
							tail = append(tail, "'"+strings.ReplaceAll(v[0], "'", "\\'")+"'")
						} else {
							cmdline = append(cmdline, c.switchName(useShortSwitches, tag), "'"+strings.ReplaceAll(v[0], "'", "\\'")+"'")
						}
						lj[param] = v[0]
					}
				}
			} else {
				http.Error(w, fmt.Sprintf("field %s not supported", field.Kind().String()), http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, fmt.Sprintf("field %s not supported", field.Kind().String()), http.StatusBadRequest)
			return
		}
	}

	if len(cmdline) > 3 && cmdline[1] == "config" && cmdline[2] == "backend" {
		found := false
		for _, ncmd1 := range cmdline {
			if strings.HasPrefix(ncmd1, "--type") || ncmd1 == "-t" {
				found = true
			}
		}
		if !found {
			cmdline = append(cmdline, "-t", a.opts.Config.Backend.Type)
			cjson["Type"] = a.opts.Config.Backend.Type
			logjson["Type"] = a.opts.Config.Backend.Type
		}
	}

	// run through the "user-defined defaults" that defer from actual defaults, and add those switched in (if they are not there yet)
	// correct cmdline only
	if useShowDefaults {
		defs := a.opts.Config.Defaults.get(true)
		defsorted := [][]string{}
		for k, v := range defs {
			defsorted = append(defsorted, []string{k, v})
		}
		sort.Slice(defsorted, func(i, j int) bool {
			if defsorted[i][0] == defsorted[j][0] {
				return defsorted[i][1] < defsorted[j][1]
			}
			return defsorted[i][0] < defsorted[j][0]
		})
		for _, kv := range defsorted {
			k := kv[0]
			v := kv[1]
			kstack := strings.Split(k, ".")
			if len(kstack) < len(c.commands[cindex].pathStack) {
				continue
			}
			pathFound := true
			for i, p := range c.commands[cindex].pathStack {
				if strings.ToLower(kstack[i]) != p {
					pathFound = false
					break
				}
			}
			if !pathFound {
				continue
			}
			cj := cjson
			paramFound := false
			for _, nk := range kstack {
				if item, ok := cj[nk]; ok {
					switch ncj := item.(type) {
					case map[string]interface{}:
						cj = ncj
					default:
						paramFound = true
					}
				} else {
					break
				}
			}
			if !paramFound {
				ncmd := command
				var ncmdt reflect.StructField
				for _, k := range kstack[len(c.commands[cindex].pathStack):] {
					ncmdt, _ = ncmd.Type().FieldByName(k)
					ncmd = ncmd.FieldByName(k)
				}
				sw := ""
				if useShortSwitches {
					sw = "-" + ncmdt.Tag.Get("short")
				}
				if sw == "" || sw == "-" {
					sw = "--" + ncmdt.Tag.Get("long")
				}
				if sw == "" {
					continue
				}
				if ncmdt.Tag.Get("webtype") == "password" {
					v = "****"
				}
				cmdline = append(cmdline, sw, fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "\\'")))
			}
		}
	}

	if action[0] == "show" {
		if len(tail) == 1 {
			json.NewEncoder(w).Encode(cmdline)
		} else {
			json.NewEncoder(w).Encode(append(cmdline, tail...))
		}
		return
	}

	err = c.jobqueue.Add()
	if err != nil {
		http.Error(w, "job queue full", http.StatusNotAcceptable)
		return
	}

	ex, err := os.Executable()
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to get path to aerolab executable: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rootDir, err := a.aerolabRootDir()
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to get aerolab root dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nUser := r.Header.Get("x-auth-aerolab-user")
	if nUser == "" {
		nUser = currentOwnerUser
	}

	nPath := path.Join(rootDir, "weblog")
	os.Mkdir(nPath, 0755)
	if nUser != "" {
		nPath = path.Join(nPath, nUser)
		os.Mkdir(nPath, 0755)
	}
	fn := path.Join(nPath, time.Now().Format("2006-01-02_15-04-05_")+requestID+".log")

	ctx, cancel := context.WithTimeout(context.Background(), c.AbsoluteTimeout)
	run := exec.CommandContext(ctx, ex, "webrun")
	stdin, err := run.StdinPipe()
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to get stdin: "+err.Error(), http.StatusInternalServerError)
		cancel()
		return
	}

	f, err := os.Create(fn)
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to create logfile: "+err.Error(), http.StatusInternalServerError)
		cancel()
		return
	}
	cmdlineprint := cmdline
	if len(tail) > 1 {
		cmdlineprint = append(cmdline, tail...)
	}
	f.WriteString("-=-=-=-=- [path] /" + strings.TrimPrefix(r.URL.Path, c.WebRoot) + " -=-=-=-=-\n")
	f.WriteString("-=-=-=-=- [cmdline] " + strings.Join(cmdlineprint, " ") + " -=-=-=-=-\n")
	f.WriteString("-=-=-=-=- [command] " + strings.Join(c.commands[cindex].pathStack, " ") + " -=-=-=-=-\n")
	json.NewEncoder(f).Encode(logjson)
	f.WriteString("-=-=-=-=- [Log] -=-=-=-=-\n")
	run.Stderr = f
	run.Stdout = f

	go func() {
		c.jobqueue.Start()

		// handle file upload storage
		if tmpDir != "" {
			err = os.MkdirAll(tmpDir, 0755)
			if err != nil {
				c.jobqueue.End()
				c.jobqueue.Remove()
				stdin.Close()
				f.WriteString("-=-=-=-=- [failed to create temporary storage directory] -=-=-=-=-\n" + err.Error() + "\n")
				f.Close()
				cancel()
				return
			}
			for src, dst := range fileUploads {
				err = func() error {
					f, err := src.Open()
					if err != nil {
						return fmt.Errorf("failed to open %s for reading: %s", src.Filename, err)
					}
					defer f.Close()
					d, err := os.Create(dst)
					if err != nil {
						return fmt.Errorf("failed to open %s for storing %s: %s", dst, src.Filename, err)
					}
					defer d.Close()
					_, err = io.Copy(d, f)
					if err != nil {
						return fmt.Errorf("failed to store contents in %s for %s: %s", dst, src.Filename, err)
					}
					return nil
				}()
				if err != nil {
					c.jobqueue.End()
					c.jobqueue.Remove()
					stdin.Close()
					f.WriteString("-=-=-=-=- [failed to temporarily store uploaded file] -=-=-=-=-\n" + err.Error() + "\n")
					f.Close()
					cancel()
					os.RemoveAll(tmpDir)
					return
				}
			}
		}

		err = run.Start()
		if err != nil {
			c.jobqueue.End()
			c.jobqueue.Remove()
			stdin.Close()
			f.WriteString("-=-=-=-=- [Subprocess failed to start] -=-=-=-=-\n" + err.Error() + "\n")
			f.Close()
			cancel()
			if tmpDir != "" {
				os.RemoveAll(tmpDir)
			}
			return
		}
		go func() {
			stdin.Write([]byte(strings.TrimPrefix(r.URL.Path, c.WebRoot) + "-=-=-=-"))
			json.NewEncoder(stdin).Encode(cjson)
			stdin.Close()
		}()

		c.joblist.Add(requestID, run)

		go func(run *exec.Cmd, requestID string) {
			if tmpDir != "" {
				defer os.RemoveAll(tmpDir)
			}
			runerr := run.Wait()
			exitCode := run.ProcessState.ExitCode()
			if c.commands[cindex].reload || (c.commands[cindex].path == "config/defaults" && ((len(r.PostForm["xxxxReset"]) > 0 && r.PostForm["xxxxReset"][0] == "on") || (len(r.PostForm["xxxxValue"]) > 0 && r.PostForm["xxxxValue"][0] != ""))) || (c.commands[cindex].path == "config/backend" && len(r.PostForm["xxxxType"]) > 0 && r.PostForm["xxxxType"][0] != "") {
				log.Printf("[%s] Refreshing interface data", requestID)
				f.WriteString("\n->Refreshing interface data\n")
				zone := "us-central1-a"
				if a.opts.Cluster.Create.Gcp.Zone != "" {
					zone = string(a.opts.Cluster.Create.Gcp.Zone)
				}
				if a.opts.Config.Backend.Type != "docker" {
					exec.Command(os.Args[0], "inventory", "instance-types", "-j", "--zone", zone).CombinedOutput()
					exec.Command(os.Args[0], "inventory", "instance-types", "-j", "--arm", "--zone", zone).CombinedOutput()
				}
				err = c.cache.run(time.Now())
				if err != nil {
					log.Printf("[%s] ERROR: Inventory Refresh: %s", requestID, err)
					if runerr == nil {
						runerr = err
					} else {
						runerr = fmt.Errorf("%s\n%s", runerr, err)
					}
					exitCode = 1
				}
				log.Printf("[%s] Refreshed interface data", requestID)
				f.WriteString("\n->Refresh finished\n")
			}
			c.joblist.Delete(requestID)
			if runerr != nil {
				f.WriteString("\n-=-=-=-=- [ExitCode] " + strconv.Itoa(exitCode) + " -=-=-=-=-\n" + runerr.Error() + "\n")
			} else {
				f.WriteString("\n-=-=-=-=- [ExitCode] " + strconv.Itoa(exitCode) + " -=-=-=-=-\nsuccess\n")
			}
			f.WriteString("-=-=-=-=- [END] -=-=-=-=-")
			f.Close()
			cancel()
			c.jobqueue.End()
			c.jobqueue.Remove()
			if c.commands[cindex].path == "upgrade" && len(r.PostForm["xxxxDryRun"]) == 0 {
				log.Printf("[%s] Restarting aerolab webui", requestID)
				time.Sleep(time.Second)
				for c.joblist.GetStat() > 0 {
					time.Sleep(time.Second)
				}
				os.Exit(0)
			}
		}(run, requestID)
	}()
	w.Write([]byte(requestID))
}

func (c *webCmd) defaultsRefresh() {
	a.opts = new(commands)
	a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)
	a.iniParser = flags.NewIniParser(a.parser)
	a.parseFile()
	a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)
	for command, switchList := range backendSwitches {
		keys := strings.Split(strings.ToLower(string(command)), ".")
		var nCmd *flags.Command
		for i, key := range keys {
			if i == 0 {
				nCmd = a.parser.Find(key)
			} else {
				nCmd = nCmd.Find(key)
			}
		}
		for backend, switches := range switchList {
			grp, err := nCmd.AddGroup(string(backend), string(backend), switches)
			if err != nil {
				logExit(err)
			}
			if string(backend) != a.opts.Config.Backend.Type {
				grp.Hidden = true
			}
		}
	}
	a.iniParser = flags.NewIniParser(a.parser)
	a.early = true
	a.parseArgs(os.Args[1:])
	a.parseFile()
	a.early = false
	c.genMenu()
}
