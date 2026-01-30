# Simple golang wget module with progress callback

## Features:

* basic authentication
* progress callback to a callback function
* summary on completion
* custom timeout
* ease of use

## Full Usage Example

```go
package main

import (
	"bytes"
	"log"
	"time"

	wget "github.com/rglonek/go-wget"
)

func main() {
	// define some buffer, could be a file
	w := new(bytes.Buffer)

	// define input
	// note Auth and Timeout are optional
	timeout := time.Minute
	input := &wget.GetInput{
		Url:               "http://212.183.159.230/20MB.zip",
		Writer:            w,
		CallbackFrequency: time.Second,
		CallbackFunc:      callback,
		Auth: &wget.Auth{
			Username: "bob",
			Password: "test",
		},
		Timeout: &timeout,
	}

	// call get with progress
	output, err := wget.GetWithProgress(input)

	// if no content header exists, call get without progress
	if err != nil && err == wget.ErrNoContentLengthHeader {
		output, err = wget.Get(input)
	}

	// handle errors
	if err != nil {
		log.Printf("Get Error: %s", err)
		return
	}

	// print summary
	log.Printf("transferred:%s statusCode:%d status:%s total:%s", wget.SizeToString(output.NumBytes), output.ResponseCode, output.Response, wget.SizeToString(output.TotalBytes))
}

// callback function for progress
func callback(p *wget.Progress) {
	log.Printf("%d%% complete @ %s / second (%s elapsed)", p.PctComplete, wget.SizeToString(p.BytesPerSecond), p.TimeElapsed.Round(time.Second))
}
```

## GetReader and GetReaderWithProgress

These functions works like Get and GetWithProgress, except they do not use the w writer. Instead they return the reader in GetOutput.R and expect the caller to read and close.
