package main

import (
	"fmt"
)

var aeroLabVersion = "v3.1.3"

func (c *config) F_version() (ret int64, err error) {
	fmt.Println(aeroLabVersion)
	return
}
