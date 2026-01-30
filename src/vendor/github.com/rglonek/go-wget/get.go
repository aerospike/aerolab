package wget

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type GetInput struct {
	Url               string
	Writer            io.Writer
	CallbackFrequency time.Duration
	CallbackFunc      CallbackFunc
	Auth              *Auth
	Timeout           *time.Duration
}

type Progress struct {
	PctComplete    int64
	BytesPerSecond int64
	TimeElapsed    time.Duration
}

type CallbackFunc func(*Progress)

type Auth struct {
	Username string
	Password string
}

type GetOutput struct {
	ResponseCode int
	Response     string
	NumBytes     int64
	TotalBytes   int64
	R            io.ReadCloser
}

type PassThruReader struct {
	io.ReadCloser
	totalRead         int64
	totalSize         int64
	startTime         time.Time
	callbackFrequency time.Duration
	callbackFunc      CallbackFunc
	lock              *sync.RWMutex
	exitCallback      chan int
}

func NewReader(TotalSize int64, Reader io.ReadCloser, callbackFrequency time.Duration, callbackFunc CallbackFunc) *PassThruReader {
	pt := PassThruReader{
		ReadCloser:        Reader,
		startTime:         time.Now(),
		totalSize:         TotalSize,
		callbackFrequency: callbackFrequency,
		callbackFunc:      callbackFunc,
		lock:              new(sync.RWMutex),
		exitCallback:      make(chan int, 1),
	}
	go pt.callback()
	return &pt
}

var ErrNoContentLengthHeader = errors.New("no Content-Length header, cannot establish total size")

func (pt *PassThruReader) callback() {
	var p Progress
	var tr int64
	for len(pt.exitCallback) == 0 {
		pt.lock.RLock()
		tr = pt.totalRead
		pt.lock.RUnlock()
		p.PctComplete = int64(float64(tr) / float64(pt.totalSize) * 100)
		p.TimeElapsed = time.Since(pt.startTime)
		if int64(p.TimeElapsed.Seconds()) > 0 {
			p.BytesPerSecond = tr / int64(p.TimeElapsed.Seconds())
		} else {
			p.BytesPerSecond = 0
		}
		pt.callbackFunc(&p)
		if pt.totalSize == tr {
			return
		}
		time.Sleep(pt.callbackFrequency)
	}
	<-pt.exitCallback
}

func (pt *PassThruReader) Read(p []byte) (int, error) {
	n, err := pt.ReadCloser.Read(p)
	pt.lock.Lock()
	pt.totalRead += int64(n)
	pt.lock.Unlock()
	if err != nil {
		pt.exitCallback <- 1
	}
	return n, err
}

func (pt *PassThruReader) Close() error {
	return pt.ReadCloser.Close()
}

func GetWithProgress(input *GetInput) (output *GetOutput, err error) {
	if input.CallbackFrequency == 0 {
		input.CallbackFrequency = time.Second
	}
	if input.CallbackFunc == nil {
		return nil, errors.New("callback function is required")
	}
	output, responseBody, contentLength, err := getPrepare(input)
	if responseBody != nil {
		defer responseBody.Close()
	}
	if err != nil {
		return output, err
	}
	t, err := strconv.Atoi(contentLength)
	if err != nil {
		return output, ErrNoContentLengthHeader
	}
	output.TotalBytes = int64(t)
	src := NewReader(int64(t), responseBody, input.CallbackFrequency, input.CallbackFunc)
	noBytes, err := io.Copy(input.Writer, src)
	output.NumBytes = noBytes
	if err != nil {
		return output, err
	}
	return
}

func Get(input *GetInput) (output *GetOutput, err error) {
	output, responseBody, _, err := getPrepare(input)
	if responseBody != nil {
		defer responseBody.Close()
	}
	if err != nil {
		return output, err
	}
	output.TotalBytes = -1
	noBytes, err := io.Copy(input.Writer, responseBody)
	output.NumBytes = noBytes
	if err != nil {
		return output, err
	}
	return
}

func GetReader(input *GetInput) (output *GetOutput, err error) {
	output, responseBody, _, err := getPrepare(input)
	if err != nil {
		return output, err
	}
	output.TotalBytes = -1
	output.NumBytes = -1
	output.R = responseBody
	return
}

func GetReaderWithProgress(input *GetInput) (output *GetOutput, err error) {
	if input.CallbackFrequency == 0 {
		input.CallbackFrequency = time.Second
	}
	if input.CallbackFunc == nil {
		return nil, errors.New("callback function is required")
	}
	output, responseBody, contentLength, err := getPrepare(input)
	if err != nil {
		return output, err
	}
	t, err := strconv.Atoi(contentLength)
	if err != nil {
		return output, ErrNoContentLengthHeader
	}
	output.TotalBytes = int64(t)
	output.NumBytes = -1
	src := NewReader(int64(t), responseBody, input.CallbackFrequency, input.CallbackFunc)
	output.R = src
	return
}

func getPrepare(input *GetInput) (output *GetOutput, body io.ReadCloser, contentLength string, err error) {
	output = new(GetOutput)
	client := &http.Client{}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("GET", input.Url, nil)
	if err != nil {
		return output, nil, "", err
	}
	if input.Auth != nil {
		req.SetBasicAuth(input.Auth.Username, input.Auth.Password)
	}
	if input.Timeout != nil {
		client.Timeout = *input.Timeout
	}
	response, err := client.Do(req)
	if err != nil {
		return output, nil, "", err
	}
	output.ResponseCode = response.StatusCode
	output.Response = response.Status
	if response.StatusCode != http.StatusOK {
		return output, nil, "", fmt.Errorf("StatusCode %d returned", response.StatusCode)
	}
	return output, response.Body, response.Header.Get("Content-Length"), nil
}
