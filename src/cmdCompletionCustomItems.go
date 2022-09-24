package main

import (
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
)

type TypeClusterName string

func (t *TypeClusterName) Complete(match string) []flags.Completion {
	if os.Getenv("AEROLAB_COMPLETION_BACKEND") != "1" {
		return []flags.Completion{}
	}
	var err error
	b, err = getBackend()
	if err != nil {
		logFatal("Could not get backend: %s", err)
	}
	if b == nil {
		logFatal("Invalid backend")
	}
	err = b.Init()
	if err != nil {
		logFatal("Could not init backend: %s", err)
	}
	clist, err := b.ClusterList()
	if err != nil {
		logFatal("Backend query failed: %s", err)
	}
	out := []flags.Completion{}
	for _, item := range clist {
		if match == "" || strings.HasPrefix(item, match) {
			out = append(out, flags.Completion{
				Item: item,
			})
		}
	}
	return out
}
