package gcplabels

import (
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func PackToMap(key string, value string) map[string]string {
	ret := make(map[string]string)
	s := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(value)))
	n := 63
	chunk := 0
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		ret[key+strconv.Itoa(chunk)] = s[i:end]
		chunk++
	}
	return ret
}

func Unpack(labels map[string]string, key string) (string, error) {
	chunk := 0
	b64 := ""
	for {
		if v, ok := labels[key+strconv.Itoa(chunk)]; ok {
			b64 = b64 + v
			chunk++
			continue
		}
		break
	}
	if b64 == "" {
		return "", errors.New("NOT FOUND")
	}
	ret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(b64))
	return string(ret), err
}

func UnpackNoErr(labels map[string]string, key string) string {
	chunk := 0
	b64 := ""
	for {
		if v, ok := labels[key+strconv.Itoa(chunk)]; ok {
			b64 = b64 + v
			chunk++
			continue
		}
		break
	}
	if b64 == "" {
		return ""
	}
	ret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(b64))
	return string(ret)
}

func PackToKV(key string, value string) []string {
	ret := []string{}
	s := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(value)))
	n := 63
	chunk := 0
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		ret = append(ret, fmt.Sprintf("%s=%s", key+strconv.Itoa(chunk), s[i:end]))
		chunk++
	}
	return ret
}
