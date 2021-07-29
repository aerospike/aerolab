package main

import (
	"fmt"

	"github.com/reiver/go-telnet"
)

var aeroLabVersion = "2.66"

func (c *config) F_version() (err error, ret int64) {
	fmt.Println(aeroLabVersion)
	return
}

func (c *config) F_starwars() (err error, ret int64) {
	var caller telnet.Caller = telnet.StandardCaller
	err = telnet.DialToAndCall("towel.blinkenlights.nl:23", caller)
	if err != nil {
		ret = 666
	}
	return
}
