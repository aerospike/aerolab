package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/lithammer/shortuuid"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v3"
)

// cli args definition
type opts struct {
	ListenAddr     string        `long:"listen" description:"listen address; ignored if --tls is specified (listen in TLS is bound to 0.0.0.0:80+443)" default:"0.0.0.0:8080" yaml:"ListenAddr"`
	TLS            bool          `long:"tls" description:"enable TLS; this will ignore ListenAddr" yaml:"TLS"`
	HostWhitelist  []string      `long:"tls-host" description:"autocert: specify domain to respond on; this parameter can be specified multiple times" yaml:"HostWhitelist"`
	CacheDir       string        `long:"tls-cache-dir" description:"autocert: directory to use for caching TLS certificates" default:"tls-cache" yaml:"CacheDir"`
	LogTextFile    string        `long:"log-text-file" description:"path to a text file to log requests to" default:"proxy.log" yaml:"LogTextFile"`
	LogJsonFile    string        `long:"log-json-file" description:"path to a json file to log requests to" default:"proxy.json" yaml:"LogJsonFile"`
	DestURL        string        `long:"dest-url" description:"destination URL to send proxy requests to" default:"http://127.0.0.1:3333/" yaml:"DestURL"`
	CookieLifeTime time.Duration `long:"user-cookie-life" description:"duration for which logged in user should remain logged in" default:"24h" yaml:"CookieLifeTime"`
}

func main() {
	// cli args
	args := &opts{}
	tail, err := flags.Parse(args)
	if err != nil {
		os.Exit(1)
	}
	if len(tail) > 0 {
		fmt.Fprintf(os.Stderr, "unknown trailing arguments: %v\n", tail)
		os.Exit(1)
	}

	// create logging files - text and json
	ld, _ := path.Split(args.LogJsonFile)
	if ld != "" {
		os.MkdirAll(ld, 0755)
	}
	f, err := os.OpenFile(args.LogJsonFile, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	ld, _ = path.Split(args.LogTextFile)
	if ld != "" {
		os.MkdirAll(ld, 0755)
	}
	p, err := os.OpenFile(args.LogTextFile, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	// create proxy handlers
	destUrl, _ := url.Parse(args.DestURL)
	proxy := httputil.NewSingleHostReverseProxy(destUrl)
	proxy.Transport = &myTransport{
		JSONLog: f,
		TextLOG: p,
	}

	// proxy handler router and function
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// test if fake user is set
		userCookie, err := r.Cookie("proxy-fake-user")
		if err != nil {
			if r.FormValue("submit") == "" {
				// handle "set user" page
				w.Write([]byte("<html><body><form method=get><input type=text name=proxy-fake-user placeholder='enter fake username'><input type=submit name=submit value=submit></form></body></html>"))
				return
			}
			// login user
			userCookie = &http.Cookie{
				Name:    "proxy-fake-user",
				Value:   r.FormValue("proxy-fake-user"),
				Path:    "/",
				Expires: time.Now().Add(args.CookieLifeTime),
			}
			http.SetCookie(w, userCookie)
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		// override request URL host and scheme to proxy destination
		r.URL.Host = destUrl.Host
		r.URL.Scheme = destUrl.Scheme
		// add proxy forwarding headers
		r.Header.Set("X-Forwarded-Host", headerAppend(r.Header.Get("X-Forwarded-Host"), r.Header.Get("Host"), ","))
		r.Header.Set("X-Forwarded-For", headerAppend(r.Header.Get("X-Forwarded-For"), r.RemoteAddr, ","))
		// set fake user header for testing
		r.Header.Set("x-auth-aerolab-user", userCookie.Value)
		// override r.Host with destination URL host
		r.Host = destUrl.Host
		// service proxy
		proxy.ServeHTTP(w, r)
	})

	// logout handler
	http.HandleFunc("/proxy-logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:    "proxy-fake-user",
			Value:   "",
			Path:    "/",
			Expires: time.Now().Add(-5 * time.Second),
		})
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})

	// print configuration yaml
	if args.TLS {
		// tls: sanity check
		if len(args.HostWhitelist) == 0 {
			fmt.Fprint(os.Stderr, "with TLS, at least one hostWhileList FQDN must be provided\n")
			os.Exit(1)
		}
		// override ListenAddr for yaml print
		args.ListenAddr = "0.0.0.0:443, 0.0.0.0:80"
	}
	conf, _ := yaml.Marshal(map[string]*opts{
		"Config": args,
	})
	fmt.Println(string(conf))

	// start webserver: non-tls
	if !args.TLS {
		log.Println("Listening on " + args.ListenAddr)
		if err := http.ListenAndServe(args.ListenAddr, nil); err != nil {
			log.Fatal(err)
		}
		log.Print("Exiting")
		return
	}

	// tls: prepate autocert
	os.MkdirAll(args.CacheDir, 0755)
	autocertManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(args.HostWhitelist...),
		Cache:      autocert.DirCache(args.CacheDir),
	}

	// tls: listen on port 80 for callback for certificate requests
	go func() {
		srv := &http.Server{
			Addr:    ":80",
			Handler: autocertManager.HTTPHandler(nil),
		}
		log.Println("AutoCert: Listening on 0.0.0.0:80")
		err := srv.ListenAndServe()
		log.Fatal(err)
	}()

	// tls: create server
	srv := &http.Server{
		Addr:    ":443",
		Handler: nil,
		TLSConfig: &tls.Config{
			GetCertificate: autocertManager.GetCertificate,
		},
	}

	// tls: start webserver
	log.Print("TLS: Listening on 0.0.0.0:443")
	if err := srv.ListenAndServeTLS("", ""); err != nil {
		log.Fatal(err)
	}
	log.Print("Exiting")
}

// for headers which append comma-separated values, either create a new value, or append a value separated by sep
func headerAppend(header string, value string, sep string) string {
	if header == "" {
		return value
	}
	return header + sep + value
}

// cutom transport for proxy service to allow logging of requests and responses
type myTransport struct {
	JSONLog *os.File
	TextLOG *os.File
}

// RoundTrip implementation of the transport interface - with logging
func (t *myTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// uuid, start time
	uuid := shortuuid.New()
	reqTime := time.Now()

	// perform proxy query request
	w, rterr := http.DefaultTransport.RoundTrip(r)

	// response time
	respTime := time.Now()

	// geerate text log line based on success/error
	var logLine string
	userName := ""
	userCookie, err := r.Cookie("proxy-fake-user")
	if err == nil {
		userName = userCookie.Value
	}
	if rterr != nil {
		logLine = fmt.Sprintf("uuid=%s userName=%s req: remoteAddr=%s requestURI=%s host=%s method=%s err=%v", uuid, userName, r.RemoteAddr, r.RequestURI, r.Host, r.Method, rterr)
	} else {
		logLine = fmt.Sprintf("uuid=%s userName=%s req: remoteAddr=%s requestURI=%s host=%s method=%s resp: status=%s contentLength=%d rtt=%s", uuid, userName, r.RemoteAddr, r.RequestURI, r.Host, r.Method, w.Status, w.ContentLength, respTime.Sub(reqTime))
	}

	// print log line
	log.Print(logLine)

	// store text log line to file, with timestamp
	_, err = fmt.Fprintf(t.TextLOG, "%s %s\n", time.Now().Format("2006/01/02 15:04:05"), logLine)
	if err != nil {
		log.Printf("uuid=%s log:TextLOG.WriteString(logLine):%s", uuid, err)
	}

	// create struct object for json logger - note request, UUID and RTT
	dlog := &dataLog{
		UUID:          uuid,
		RoundTripTime: respTime.Sub(reqTime),
		Request: &logRequest{
			Time:       reqTime,
			RemoteAddr: r.RemoteAddr,
			RequestURI: r.RequestURI,
			Host:       r.Host,
			Method:     r.Method,
			Headers:    r.Header,
		},
	}

	// we we have successful writer (no error), also add response to json log
	if w != nil {
		dlog.Response = &logResponse{
			Time:          respTime,
			StatusCode:    w.StatusCode,
			Status:        w.Status,
			ContentLength: int(w.ContentLength),
			Headers:       w.Header,
		}
	}

	// if we have an error, add round trip error to json log
	if rterr != nil {
		dlog.RoundTripError = &rtError{
			Detail: rterr,
			String: rterr.Error(),
		}
	}

	// log json
	err = json.NewEncoder(t.JSONLog).Encode(dlog)
	if err != nil {
		log.Printf("uuid=%s log:json.NewEncoder(f).Encode(dlog):%s", uuid, err)
	}

	// return result
	return w, rterr
}

// json log structs
type dataLog struct {
	UUID           string
	Request        *logRequest
	Response       *logResponse
	RoundTripError *rtError
	RoundTripTime  time.Duration
}

type rtError struct {
	String string
	Detail error
}

type logRequest struct {
	Time       time.Time
	RemoteAddr string
	RequestURI string
	Host       string
	Method     string
	Headers    map[string][]string
}

type logResponse struct {
	Time          time.Time
	StatusCode    int
	Status        string
	ContentLength int
	Headers       map[string][]string
}
