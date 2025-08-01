package cmd

import (
	"os/user"
	"runtime"
	"strings"
)

var currentOwnerUser string

func init() {
	user, err := user.Current()
	if err != nil {
		return
	}
	uname := ""
	for _, r := range user.Username {
		if r < 48 {
			continue
		}
		if r > 57 && r < 65 {
			continue
		}
		if r > 90 && r < 97 {
			continue
		}
		if r > 122 {
			continue
		}
		uname = uname + string(r)
	}
	if runtime.GOOS == "windows" {
		unamex := strings.Split(uname, "\\")
		uname = unamex[len(unamex)-1]
	}
	if len(uname) > 63 {
		uname = uname[:63]
	}
	currentOwnerUser = strings.ToLower(uname)
}

func GetCurrentOwnerUser() string {
	return currentOwnerUser
}
