package baws

import (
	"embed"
	"encoding/json"
)

//go:embed scripts/*
var scripts embed.FS

//go:embed expiryiam/*
var expiryiam embed.FS

func getExpiryJSONString(name string) string {
	name = "expiryiam/" + name
	content, err := expiryiam.ReadFile(name)
	if err != nil {
		return ""
	}
	return string(content)
}

func getExpiryJSONStringList(name string) []string {
	name = "expiryiam/" + name
	content, err := expiryiam.ReadFile(name)
	if err != nil {
		return []string{}
	}
	var list []string
	err = json.Unmarshal(content, &list)
	if err != nil {
		return []string{}
	}
	return list
}
