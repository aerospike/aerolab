package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/ingest"
	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
	ps "github.com/mitchellh/go-ps"
)

type agiExecProxyCmd struct {
	InitialLabel       string        `short:"L" long:"label" description:"freeform label that will appear in the dashboards if set"`
	IngestProgressPath string        `short:"i" long:"ingest-progress-path" default:"/opt/agi/ingest/" description:"path to where ingest stores it's json progress"`
	ListenPort         int           `short:"l" long:"listen-port" default:"80" description:"port to listen on"`
	HTTPS              bool          `short:"S" long:"https" description:"set to enable https listener"`
	CertFile           string        `short:"C" long:"cert-file" description:"required path to server cert file for tls"`
	KeyFile            string        `short:"K" long:"key-file" description:"required path to server key file for tls"`
	EntryDir           string        `short:"d" long:"entry-dir" default:"/opt/agi/files" description:"Entrypoint for ttyd and filebrowser"`
	MaxInactivity      time.Duration `short:"m" long:"max-inactivity" default:"1h" description:"Max user inactivity period after which the system will be shut down; 0=disable"`
	MaxUptime          time.Duration `short:"M" long:"max-uptime" default:"24h" description:"Max hard instance uptime; 0=disable"`
	ShutdownCommand    string        `short:"c" long:"shutdown-command" default:"/sbin/poweroff" description:"Command to execute on max uptime or max inactivity being breached"`
	AuthType           string        `short:"a" long:"auth-type" default:"none" description:"Authentication type; supported: none|basic"`
	BasicAuthUser      string        `short:"u" long:"basic-auth-user" default:"admin" description:"Basic authentication username"`
	BasicAuthPass      string        `short:"p" long:"basic-auth-pass" default:"secure" description:"Basic authentication password"`
	Help               helpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
	isBasicAuth        bool
	lastActivity       *activity
	grafanaUrl         *url.URL
	grafanaProxy       *httputil.ReverseProxy
	ttydUrl            *url.URL
	ttydProxy          *httputil.ReverseProxy
	fbUrl              *url.URL
	fbProxy            *httputil.ReverseProxy
	gottyConns         *counter
	srv                *http.Server
}

type counter struct {
	sync.Mutex
	c string
}

func (a *counter) Set(t string) {
	a.Lock()
	a.c = t
	a.Unlock()
}

func (a *counter) Get() (t string) {
	a.Lock()
	t = a.c
	a.Unlock()
	return
}

type activity struct {
	sync.Mutex
	lastActivity time.Time
}

func (a *activity) Set(t time.Time) {
	a.Lock()
	a.lastActivity = t
	a.Unlock()
}

func (a *activity) Get() (t time.Time) {
	a.Lock()
	t = a.lastActivity
	a.Unlock()
	return
}

func (c *agiExecProxyCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	os.MkdirAll(c.EntryDir, 0755)
	os.WriteFile("/opt/agi/proxy.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/proxy.pid")
	if _, err := os.Stat("/opt/agi/label"); err != nil {
		os.WriteFile("/opt/agi/label", []byte(c.InitialLabel), 0644)
	}
	plist, err := ps.Processes()
	asdRunning := false
	if err == nil {
		for _, p := range plist {
			if strings.HasSuffix(p.Executable(), "asd") {
				asdRunning = true
				break
			}
		}
	}
	if !asdRunning {
		exec.Command("service", "aerospike", "start").CombinedOutput()
	}
	c.lastActivity = new(activity)
	c.gottyConns = new(counter)
	c.gottyConns.Set("0")
	c.lastActivity.Set(time.Now())
	gurl, _ := url.Parse("http://127.0.0.1:8850/")
	gproxy := httputil.NewSingleHostReverseProxy(gurl)
	c.grafanaUrl = gurl
	c.grafanaProxy = gproxy
	turl, _ := url.Parse("http://127.0.0.1:8852/")
	tproxy := httputil.NewSingleHostReverseProxy(turl)
	c.ttydUrl = turl
	c.ttydProxy = tproxy
	furl, _ := url.Parse("http://127.0.0.1:8853/")
	fproxy := httputil.NewSingleHostReverseProxy(furl)
	c.fbUrl = furl
	c.fbProxy = fproxy
	if c.AuthType == "basic" {
		c.isBasicAuth = true
	}
	go c.getDeps()
	if c.MaxInactivity > 0 {
		go c.activityMonitor()
	}
	if c.MaxUptime > 0 {
		go c.maxUptime()
	}
	http.HandleFunc("/agi/ttyd", c.ttydHandler)        // web console tty
	http.HandleFunc("/agi/ttyd/", c.ttydHandler)       // web console tty
	http.HandleFunc("/agi/filebrowser", c.fbHandler)   // file browser
	http.HandleFunc("/agi/filebrowser/", c.fbHandler)  // file browser
	http.HandleFunc("/agi/menu", c.handleList)         // simple URL list
	http.HandleFunc("/agi/shutdown", c.handleShutdown) // gracefully shutdown the proxy
	http.HandleFunc("/agi/poweroff", c.handlePoweroff) // poweroff the instance
	http.HandleFunc("/agi/reingest", c.handleReingest) // retrigger the logingest service; form: ?serviceName=logingest
	http.HandleFunc("/agi/status", c.handleStatus)     // high-level agi service status
	http.HandleFunc("/agi/detail", c.handleDetail)     // detailed logingest progress json; form: ?detail=[]string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json"}
	http.HandleFunc("/agi/exec", c.handleExec)         // execute command on this machine; form: ?command=...
	http.HandleFunc("/agi/relabel", c.handleRelabel)   // change the case label static string; form: ?label=...
	http.HandleFunc("/", c.grafanaHandler)             // grafana
	c.srv = &http.Server{Addr: "0.0.0.0:" + strconv.Itoa(c.ListenPort)}
	if c.HTTPS {
		if err := c.srv.ListenAndServeTLS(c.CertFile, c.KeyFile); err != http.ErrServerClosed {
			return err
		} else {
			return nil
		}
	}
	if err := c.srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	} else {
		return nil
	}
}

func (c *agiExecProxyCmd) handleList(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	w.WriteHeader(http.StatusOK)
	out := []byte(`<html><head><title>AGI URLs</title></head><body><center>
	<a href="/" target="_blank"><h1>Grafana</h1></a>
	<a href="/agi/ttyd" target="_blank"><h1>Web Console (ttyd)</h1></a>
	<a href="/agi/filebrowser" target="_blank"><h1>File Browser</h1></a>
	<a href="/agi/reingest" target="_blank"><h1>Retrigger Ingest</h1></a>
	</center></body></html>`)
	w.Write(out)
}

func (c *agiExecProxyCmd) handleExec(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	logger.Info("Listener: exec request from %s", r.RemoteAddr)
	comm := r.FormValue("command")
	out, err := exec.Command("/bin/bash", "-c", comm).CombinedOutput()
	if err != nil {
		out = append(out, '\n')
		out = append(out, []byte(err.Error())...)
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	w.Write(out)
}

// form: ?detail=[]string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json"}
func (c *agiExecProxyCmd) handleDetail(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	fname := r.FormValue("detail")
	files := []string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json"}
	if !inslice.HasString(files, fname) {
		http.Error(w, "invalid detail type", http.StatusBadRequest)
		return
	}
	npath := path.Join(c.IngestProgressPath, fname)
	gz := false
	if _, err := os.Stat(npath); err != nil {
		npath = npath + ".gz"
		if _, err := os.Stat(npath); err != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		gz = true
	}
	f, err := os.Open(npath)
	if err != nil {
		http.Error(w, "could not open file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	var reader io.Reader
	reader = f
	if gz {
		fx, err := gzip.NewReader(f)
		if err != nil {
			http.Error(w, "could not open gz for reading: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer fx.Close()
		reader = fx
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

func (c *agiExecProxyCmd) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	logger.Info("Listener: status request from %s", r.RemoteAddr)
	type s struct {
		Ingest struct {
			Running                  bool
			CompleteSteps            *ingestSteps
			DownloaderCompletePct    int
			DownloaderTotalSize      int64
			DownloaderCompleteSize   int64
			LogProcessorCompletePct  int
			LogProcessorTotalSize    int64
			LogProcessorCompleteSize int64
			Errors                   []string
		}
		AerospikeRunning     bool
		PluginRunning        bool
		GrafanaHelperRunning bool
	}
	status := new(s)
	plist, err := ps.Processes()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	for _, p := range plist {
		if strings.HasSuffix(p.Executable(), "asd") {
			status.AerospikeRunning = true
			break
		}
	}
	pidf, err := os.ReadFile("/opt/agi/ingest.pid")
	if err == nil {
		pid, err := strconv.Atoi(string(pidf))
		if err == nil {
			_, err := os.FindProcess(pid)
			if err == nil {
				status.Ingest.Running = true
			}
		}
	}
	pidf, err = os.ReadFile("/opt/agi/plugin.pid")
	if err == nil {
		pid, err := strconv.Atoi(string(pidf))
		if err == nil {
			_, err := os.FindProcess(pid)
			if err == nil {
				status.PluginRunning = true
			}
		}
	}
	pidf, err = os.ReadFile("/opt/agi/grafanafix.pid")
	if err == nil {
		pid, err := strconv.Atoi(string(pidf))
		if err == nil {
			_, err := os.FindProcess(pid)
			if err == nil {
				status.GrafanaHelperRunning = true
			}
		}
	}
	steps := new(ingestSteps)
	f, err := os.ReadFile("/opt/agi/ingest/steps.json")
	if err == nil {
		json.Unmarshal(f, steps)
	}
	status.Ingest.CompleteSteps = steps

	fname := ""
	if steps.Init && !steps.Download {
		fname = "downloader.json"
	} else if steps.Download && !steps.Unpack {
		fname = "unpacker.json"
	} else if steps.Unpack && !steps.PreProcess {
		fname = "pre-processor.json"
	} else if steps.PreProcess {
		fname = "log-processor.json"
	}
	npath := path.Join(c.IngestProgressPath, fname)
	gz := false
	if _, err := os.Stat(npath); err != nil {
		npath = npath + ".gz"
		if _, err := os.Stat(npath); err != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		gz = true
	}
	fa, err := os.Open(npath)
	if err != nil {
		http.Error(w, "could not open file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer fa.Close()
	var reader io.Reader
	reader = fa
	if gz {
		fx, err := gzip.NewReader(fa)
		if err != nil {
			http.Error(w, "could not open gz for reading: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer fx.Close()
		reader = fx
	}
	if steps.Init && !steps.Download {
		p := new(ingest.ProgressDownloader)
		json.NewDecoder(reader).Decode(p)
		totalSize := int64(0)
		dlSize := int64(0)
		for fn, f := range p.S3Files {
			if f.Error != "" {
				status.Ingest.Errors = append(status.Ingest.Errors, fn+"::"+f.Error)
			}
			totalSize += f.Size
			if f.IsDownloaded {
				dlSize += f.Size
			} else {
				if nstat, err := os.Stat(fn); err == nil {
					dlSize += nstat.Size()
				}
			}
		}
		for fn, f := range p.SftpFiles {
			if f.Error != "" {
				status.Ingest.Errors = append(status.Ingest.Errors, fn+"::"+f.Error)
			}
			totalSize += f.Size
			if f.IsDownloaded {
				dlSize += f.Size
			} else {
				if nstat, err := os.Stat(fn); err == nil {
					dlSize += nstat.Size()
				}
			}
		}
		status.Ingest.DownloaderTotalSize = totalSize
		status.Ingest.DownloaderCompleteSize = dlSize
		if totalSize > 0 {
			status.Ingest.DownloaderCompletePct = int((100 * dlSize) / totalSize)
		}
	} else if steps.Download && !steps.Unpack {
		status.Ingest.DownloaderCompletePct = 100
		p := new(ingest.ProgressUnpacker)
		json.NewDecoder(reader).Decode(p)
		for fn, f := range p.Files {
			for _, nerr := range f.Errors {
				status.Ingest.Errors = append(status.Ingest.Errors, fn+"::"+nerr)
			}
		}
	} else if steps.Unpack && !steps.PreProcess {
		status.Ingest.DownloaderCompletePct = 100
		p := new(ingest.ProgressPreProcessor)
		json.NewDecoder(reader).Decode(p)
		for fn, f := range p.Files {
			for _, nerr := range f.Errors {
				status.Ingest.Errors = append(status.Ingest.Errors, fn+"::"+nerr)
			}
		}
	} else if steps.PreProcess {
		status.Ingest.DownloaderCompletePct = 100
		p := new(ingest.ProgressLogProcessor)
		json.NewDecoder(reader).Decode(p)
		totalSize := int64(0)
		dlSize := int64(0)
		for _, f := range p.Files {
			totalSize += f.Size
			if f.Finished {
				dlSize += f.Size
			} else {
				dlSize += f.Processed
			}
		}
		status.Ingest.LogProcessorTotalSize = totalSize
		status.Ingest.LogProcessorCompleteSize = dlSize
		if totalSize > 0 {
			status.Ingest.LogProcessorCompletePct = int((100 * dlSize) / totalSize)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// form: ?label=...
func (c *agiExecProxyCmd) handleRelabel(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	logger.Info("Listener: relabel request from %s", r.RemoteAddr)
	os.WriteFile("/opt/agi/label", []byte(r.FormValue("label")), 0644)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// form: ?serviceName=logingest
func (c *agiExecProxyCmd) handleReingest(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	logger.Info("Listener: reingest request from %s", r.RemoteAddr)
	pidf, err := os.ReadFile("/opt/agi/ingest.pid")
	if err == nil {
		pid, err := strconv.Atoi(string(pidf))
		if err == nil {
			_, err := os.FindProcess(pid)
			if err == nil {
				w.WriteHeader(http.StatusConflict)
				w.Write([]byte("Ingest already running"))
				return
			}
		}
	}
	os.Remove("/opt/agi/ingest/steps.json")
	serviceName := r.FormValue("serviceName")
	if serviceName == "" {
		serviceName = "logingest"
	}
	out, err := exec.Command("service", serviceName, "start").CombinedOutput()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(out)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (c *agiExecProxyCmd) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	logger.Info("Listener: shutdown request from %s", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Shutting down..."))
	go func() {
		timeout := 60 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := c.srv.Shutdown(ctx); err != nil {
			logger.Debug("Graceful Server Shutdown Failed, Forcing shutdown: %s", err)
			c.srv.Close()
		}
	}()
}

func (c *agiExecProxyCmd) handlePoweroff(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	logger.Info("Listener: shutdown request from %s", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Poweroff..."))
	go func() {
		exec.Command(c.ShutdownCommand).CombinedOutput()
	}()
}

func (c *agiExecProxyCmd) maxUptime() {
	logger.Info("MAX UPTIME: hard shutdown time: %s", time.Now().Add(c.MaxUptime).String())
	time.Sleep(c.MaxUptime)
	exec.Command(c.ShutdownCommand).CombinedOutput()
}

func (c *agiExecProxyCmd) activityMonitor() {
	var lastActivity time.Time
	for {
		time.Sleep(time.Minute)
		if _, err := os.Stat("/opt/agi/ingest.pid"); err == nil {
			c.lastActivity.Set(time.Now())
			continue
		}
		if c.gottyConns.Get() != "0" {
			c.lastActivity.Set(time.Now())
			continue
		}
		newActivity := c.lastActivity.Get()
		if time.Since(newActivity) > c.MaxInactivity {
			exec.Command(c.ShutdownCommand).CombinedOutput()
		}
		if lastActivity.IsZero() || !lastActivity.Equal(newActivity) {
			lastActivity = newActivity
			logger.Debug("INACTIVITY SHUTDOWN UPDATE: shutdown at %s", lastActivity.Add(c.MaxInactivity))
		}
	}
}

func (c *agiExecProxyCmd) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Add("Strict-Transport-Security", "max-age=31536000")
	if c.isBasicAuth {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
		usermatch := subtle.ConstantTimeCompare([]byte(user), []byte(c.BasicAuthUser))
		passmatch := subtle.ConstantTimeCompare([]byte(pass), []byte(c.BasicAuthPass))
		if usermatch == 0 || passmatch == 0 {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
	}
	// note down activity timestamp
	go c.lastActivity.Set(time.Now())
	return true
}

func (c *agiExecProxyCmd) grafanaHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.grafanaUrl.Host
	r.URL.Scheme = c.grafanaUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.grafanaUrl.Host
	c.grafanaProxy.ServeHTTP(w, r)
}

func (c *agiExecProxyCmd) ttydHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.ttydUrl.Host
	r.URL.Scheme = c.ttydUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.ttydUrl.Host
	c.ttydProxy.ServeHTTP(w, r)
}

func (c *agiExecProxyCmd) fbHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.fbUrl.Host
	r.URL.Scheme = c.fbUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.fbUrl.Host
	c.fbProxy.ServeHTTP(w, r)
}

func (c *agiExecProxyCmd) getDeps() {
	go func() {
		logger.Info("Getting ttyd...")
		fd, err := os.OpenFile("/usr/local/bin/ttyd", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			logger.Error("ttyd-MAKEFILE: %s", err)
			return
		}
		arch := "x86_64" // .aarch64
		narch, _ := exec.Command("uname", "-m").CombinedOutput()
		if strings.Contains(string(narch), "arm") || strings.Contains(string(narch), "aarch") {
			arch = "aarch64"
		}
		resp, err := http.Get("https://github.com/tsl0922/ttyd/releases/download/1.7.3/ttyd." + arch)
		if err != nil {
			logger.Error("ttyd-GET: %s", err)
			fd.Close()
			return
		}
		_, err = io.Copy(fd, resp.Body)
		resp.Body.Close()
		fd.Close()
		if err != nil {
			logger.Error("ttyd-DOWNLOAD: %s", err)
			return
		}
		logger.Info("Running gotty!")
		com := exec.Command("/usr/local/bin/ttyd", "-p", "8852", "-i", "lo", "-P", "5", "-b", "/agi/ttyd", "/bin/bash", "-c", "export TMOUT=3600 && echo '* aerospike-tools is installed' && echo '* less -S ...: enable horizontal scrolling in less using arrow keys' && echo '* showconf command: showconf collect_info.tgz' && echo '* showsysinfo command: showsysinfo collect_info.tgz' && echo '* showinterrupts command: showinterrupts collect_info.tgz' && /bin/bash")
		com.Dir = c.EntryDir
		sout, err := com.StdoutPipe()
		if err != nil {
			logger.Error("gotty cannot start: could not create stdout pipe: %s", err)
			return
		}
		serr, err2 := com.StderrPipe()
		if err2 != nil {
			logger.Error("gotty cannot start: could not create stderr pipe: %s", err2)
			return
		}
		err = com.Start()
		if err != nil {
			logger.Error("gotty cannot start: %s", err)
			return
		}
		go c.gottyWatcher(sout)
		go c.gottyWatcher(serr)
		err = com.Wait()
		if err != nil {
			logger.Error("gotty exited with error: %s", err)
			return
		}
	}()
	go func() {
		cur, err := filepath.Abs(os.Args[0])
		if err != nil {
			logger.Error("failed to get absolute path os self: %s", err)
			return
		}
		if _, err := os.Stat("/usr/local/bin/showconf"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showconf")
			if err != nil {
				logger.Error("failed to symlink showconf: %s", err)
			}
		}
		if _, err := os.Stat("/usr/local/bin/showsysinfo"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showsysinfo")
			if err != nil {
				logger.Error("failed to symlink showsysinfo: %s", err)
			}
		}
		if _, err := os.Stat("/usr/local/bin/showinterrupts"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showinterrupts")
			if err != nil {
				logger.Error("failed to symlink showinterrupts: %s", err)
			}
		}
	}()
	go func() {
		logger.Info("Getting filebrowser...")
		fd, err := os.OpenFile("/opt/filebrowser.tgz", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			logger.Error("filebrowser-MAKEFILE: %s", err)
			return
		}
		arch := "amd64"
		narch, _ := exec.Command("uname", "-m").CombinedOutput()
		if strings.Contains(string(narch), "arm") || strings.Contains(string(narch), "aarch") {
			arch = "arm64"
		}
		resp, err := http.Get("https://github.com/filebrowser/filebrowser/releases/download/v2.25.0/linux-" + arch + "-filebrowser.tar.gz")
		if err != nil {
			logger.Error("filebrowser-GET: %s", err)
			fd.Close()
			return
		}
		_, err = io.Copy(fd, resp.Body)
		resp.Body.Close()
		fd.Close()
		if err != nil {
			logger.Error("filebrowser-DOWNLOAD: %s", err)
			return
		}
		logger.Info("Unpack filebrowser")
		out, err := exec.Command("tar", "-zxvf", "/opt/filebrowser.tgz", "-C", "/usr/local/bin/", "filebrowser").CombinedOutput()
		if err != nil {
			logger.Error("filebrowser-unpack: %s (%s)", string(out), err)
			return
		}
		logger.Info("Running filebrowser!")
		com := exec.Command("/usr/local/bin/filebrowser", "-p", "8853", "-r", c.EntryDir, "--noauth", "-d", "/opt/filebrowser.db", "-b", "/agi/filebrowser/")
		com.Dir = c.EntryDir
		out, err = com.CombinedOutput()
		if err != nil {
			logger.Error("filebrowser: %s %s", err, string(out))
		}
	}()
}

func (c *agiExecProxyCmd) gottyWatcher(out io.Reader) {
	//r, _ := regexp.Compile(`connections: [0-9]+($|\n)`)
	r, _ := regexp.Compile(`clients: [0-9]+($|\n)`)
	r2, _ := regexp.Compile(`[0-9]+`)
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		line := scanner.Text()
		n := r.FindAllString(line, -1)
		if len(n) == 0 {
			continue
		}
		n1 := n[len(n)-1]
		connNew := r2.FindString(n1)
		if connNew == "" {
			continue
		}
		if connNew != c.gottyConns.Get() {
			logger.Info("GOTTY CONNS: %s", connNew)
			c.gottyConns.Set(connNew)
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Error("gottyWatcher scanner error: %s", err)
	}
	logger.Info("Exiting gottyWatcher")
}
