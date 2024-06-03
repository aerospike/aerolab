package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/lithammer/shortuuid"
)

func main() {
	// create logging files - text and json
	f, err := os.OpenFile("proxy.json", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	p, err := os.OpenFile("proxy.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	// create proxy handlers
	destUrl, _ := url.Parse("http://127.0.0.1:3333/")
	proxy := httputil.NewSingleHostReverseProxy(destUrl)
	proxy.Transport = &myTransport{
		JSONLog: f,
		TextLOG: p,
	}

	// proxy handler router and function
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// override request URL host and scheme to proxy destination
		r.URL.Host = destUrl.Host
		r.URL.Scheme = destUrl.Scheme
		// add proxy forwarding headers
		r.Header.Set("X-Forwarded-Host", headerAppend(r.Header.Get("X-Forwarded-Host"), r.Header.Get("Host"), ","))
		r.Header.Set("X-Forwarded-For", headerAppend(r.Header.Get("X-Forwarded-For"), r.RemoteAddr, ","))
		// set fake user header for testing
		r.Header.Set("x-auth-aerolab-user", "fakeUser")
		// override r.Host with destination URL host
		r.Host = destUrl.Host
		// service proxy
		proxy.ServeHTTP(w, r)
	})

	// start webserver
	bind := "0.0.0.0:8080"
	log.Println("Listening on " + bind)
	http.ListenAndServe(bind, nil)
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
	if rterr != nil {
		logLine = fmt.Sprintf("uuid=%s req: remoteAddr=%s requestURI=%s host=%s method=%s err=%v", uuid, r.RemoteAddr, r.RequestURI, r.Host, r.Method, rterr)
	} else {
		logLine = fmt.Sprintf("uuid=%s req: remoteAddr=%s requestURI=%s host=%s method=%s resp: status=%s contentLength=%d rtt=%s", uuid, r.RemoteAddr, r.RequestURI, r.Host, r.Method, w.Status, w.ContentLength, respTime.Sub(reqTime))
	}

	// print log line
	log.Print(logLine)

	// store text log line to file, with timestamp
	_, err := fmt.Fprintf(t.TextLOG, "%s %s\n", time.Now().Format("2006/01/02 15:04:05"), logLine)
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
