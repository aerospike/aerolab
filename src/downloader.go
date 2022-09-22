package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

type PassThru struct {
	io.Reader
	total     int64 // Total # of bytes transferred
	filetotal int64
	startTime time.Time
}

func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	pt.total += int64(n)
	var percent int64
	var delta int64
	var speed int64
	percent = int64(float64(pt.total) / float64(pt.filetotal) * 100)
	delta = int64(time.Since(pt.startTime).Seconds())
	if delta > 0 {
		speed = pt.total / delta
	} else {
		speed = 0
	}
	fmt.Printf("\rProgress: %d%% (%s of %s @ %s / s)                    ", percent, convSize(pt.total), convSize(pt.filetotal), convSize(speed))
	if err != nil {
		fmt.Print("\n")
	}
	return n, err
}

func convSize(size int64) string {
	var sizeString string
	if size > 1023 && size < 1024*1024 {
		sizeString = fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else if size < 1024 {
		sizeString = fmt.Sprintf("%v B", size)
	} else if size >= 1024*1024 && size < 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f MB", float64(size)/1024/1024)
	} else if size >= 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f GB", float64(size)/1024/1024/1024)
	}
	return sizeString
}

func downloadFile(url string, filename string, user string, pass string) (err error) {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		err = fmt.Errorf("GET '%s': exit code (%d)", url, response.StatusCode)
		return
	}
	src := &PassThru{Reader: response.Body}
	t, _ := strconv.Atoi(response.Header.Get("Content-Length"))
	src.filetotal = int64(t)
	src.startTime = time.Now()
	_, err = io.Copy(out, src)
	fmt.Print("\r")
	if err != nil {
		return err
	}
	return
}
