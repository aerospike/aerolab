package bgcp

import (
	"bytes"
	"embed"
	"encoding/gob"
	"strings"
)

//go:embed scripts/*
var scripts embed.FS

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// splits the URL using '/' and returns the last element
func getValueFromURL(url string) string {
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func deepCopy(src, dst interface{}) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(src); err != nil {
		return err
	}
	return gob.NewDecoder(&buf).Decode(dst)
}
