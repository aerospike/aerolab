package baws

import (
	"embed"
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
