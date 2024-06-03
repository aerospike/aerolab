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
	destUrl, _ := url.Parse("http://127.0.0.1:3333/")
	proxy := httputil.NewSingleHostReverseProxy(destUrl)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Host = destUrl.Host
		r.URL.Scheme = destUrl.Scheme
		r.Header.Set("X-Forwarded-Host", headerAppend(r.Header.Get("X-Forwarded-Host"), r.Header.Get("Host"), ","))
		r.Header.Set("X-Forwarded-For", headerAppend(r.Header.Get("X-Forwarded-For"), r.RemoteAddr, ","))
		r.Header.Set("x-auth-aerolab-user", "fakeUser")
		r.Host = destUrl.Host
		proxy.Transport = &myTransport{
			JSONLog: f,
			TextLOG: p,
		}
		proxy.ServeHTTP(w, r)
	})
	log.Println("Starting")
	http.ListenAndServe("0.0.0.0:8080", nil)
}

func headerAppend(header string, value string, sep string) string {
	if header == "" {
		return value
	}
	return header + sep + value
}

type myTransport struct {
	JSONLog *os.File
	TextLOG *os.File
}

func (t *myTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	uuid := shortuuid.New()
	reqTime := time.Now()
	w, rterr := http.DefaultTransport.RoundTrip(r)
	respTime := time.Now()
	logLine := fmt.Sprintf("uuid=%s req: remoteAddr=%s requestURI=%s host=%s method=%s resp: status=%s contentLength=%d err=%v", uuid, r.RemoteAddr, r.RequestURI, r.Host, r.Method, w.Status, w.ContentLength, rterr)
	log.Print(logLine)
	_, err := fmt.Fprintf(t.TextLOG, "%s %s\n", time.Now().Format("2006/01/02 15:04:05"), logLine)
	if err != nil {
		log.Printf("uuid=%s log:TextLOG.WriteString(logLine):%s", uuid, err)
	}
	dlog := &dataLog{
		UUID:           uuid,
		RoundTripError: rterr,
		Request: logRequest{
			Time:       reqTime,
			RemoteAddr: r.RemoteAddr,
			RequestURI: r.RequestURI,
			Host:       r.Host,
			Method:     r.Method,
			Headers:    r.Header,
		},
		Response: logResponse{
			Time:          respTime,
			StatusCode:    w.StatusCode,
			Status:        w.Status,
			ContentLength: int(w.ContentLength),
			Headers:       w.Header,
		},
	}
	err = json.NewEncoder(t.JSONLog).Encode(dlog)
	if err != nil {
		log.Printf("uuid=%s log:json.NewEncoder(f).Encode(dlog):%s", uuid, err)
	}
	return w, rterr
}

type dataLog struct {
	UUID           string
	Request        logRequest
	Response       logResponse
	RoundTripError error
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
