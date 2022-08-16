package main

import (
	"fmt"
)

var aeroLabVersion = "v3.1.2"

func (c *config) F_version() (ret int64, err error) {
	fmt.Println(aeroLabVersion)
	return
}
