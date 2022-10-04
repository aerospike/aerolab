package main

import (
	"encoding/base64"
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
	if simulateArmInstaller {
		return downloadArmSimulator(out)
	}
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

func downloadArmSimulator(f *os.File) error {
	b64 := "H4sIAIk6O2MAA+1W227bRhD1M79iCj34iRddKBWtEyCA2yJAgQa9vGdFDsWFyF12L7KTIv/es6RM05HixgGKtqjnhdTuzJnZOWeWEmy07eSeY8vmwCZm5dh0RlqO18k8yZJF7LdeOb/IkmyVXnyBZVm2yXPqn+vhmS1Ww/NoNF8t5+v1Mp9vcsoWi2yVX1D+Jcmeat46YVCK2TVa8f6TfnCrqkdwjucYn/8RE0/kX1ip0IqmeUKOv+B/uczXH/G/3GxWF5T9baee2P+c/9lXqbcm3UqVsjrQVtg6avelNBR3lOrOpaNCJuvsisl6IRxdXX330/f0kkY0YcsI4OG1B72pZcPkjOeo1BGRbZg7muOH4gixR5RLvF4GnAcp7t+SQqsqmtGruwUqhRPIwBR25M4b4aRWVCFdEsHz11paAnvqvaAWbMOvZWwb65IoiF4WTH+gosE8lsho7caVndG+e7jUyTLgU3oQJjVeTSu1ZYJtJB7smCF2tWFRWvqaZlR0nm4pJ6lolWxo9HVGKCuKUH/8u2fPg7veWt2w40f9j/hxhxnuY2l1Gnp/AKOdjqsybsUtzXMoNvoQRY3e7aTaoRuj44x+1Lu+mUPztkxCkQiwHrCdcHUyOg89KfmQKt80k6beGShyfOsA8Q4VVXp0+DDN+AurklALtWyt2KELToPBUvt7BoAUTvYZOWiSBGkUuxtt9pPAUw3cmShLgxIC1Mlep42jZWjcydaMflMQWYurlFwNremm0TehseEcPCy+FUUB7PiY4y1aaUQLqkzwgscZ2NdvxpJ01aNc62KPiFpDzIPSbyQ6H7ZESNq/DTd7gC20MVy45vQ4M+r8tpG2HmKPWTC0RU2i6xpZ9GMVelGSho8hpUtQA2GFiKKBOvokZ6AxU4BidYfbtyIUGyCSMwEPe0NXr9+8ur7++eU5udQsjNsyLo+Jah8jbhZkVROyo6wS5BjCzSeNVoEwaK0GVqlRmyPru57n1jcOHbDuBK3FCXrATwpkcU4goSPhUwp5xge6dLL7JnD44qo/57ch9kWIvQysBTfTji3W1RnAgZL+ZD0vp0ll+LAfRBPG/WTTyZYxXjTPzvW4Elsji48G5HOGYz5FC8MHhdtOYNgcWzcBNDxKLK5wo4GUxbjZcqvNu9jK97jUfhiXS64EeImda+i+bItY3Bn4H4OrjI+xyB0+MdE//bV9tmd7tmf799if4AUtxgAQAAA="
	contents, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	_, err = f.Write(contents)
	return err
}
