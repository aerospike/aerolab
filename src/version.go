package main

import (
	"fmt"
)

var aeroLabVersion = "v3.0.2"

func (c *config) F_version() (ret int64, err error) {
	fmt.Println(aeroLabVersion)
	return
}
