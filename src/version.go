package main

import "fmt"

var aeroLabVersion = "2.41"

func (c *config) F_version() (err error, ret int64) {
	fmt.Println(aeroLabVersion)
	return
}
