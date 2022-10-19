package main

import (
	"log"
	"os"
	"strings"

	flags "github.com/rglonek/jeddevdk-goflags"
)

func completionCustomCheck() bool {
	if os.Getenv("AEROLAB_COMPLETION_BACKEND") != "1" {
		return false
	}
	var err error
	b, err = getBackend()
	if err != nil {
		log.Fatalf("Could not get backend: %s", err)
	}
	if b == nil {
		log.Fatalf("Invalid backend")
	}
	err = b.Init()
	if err != nil {
		log.Fatalf("Could not init backend: %s", err)
	}
	return true
}

type TypeClusterName string
type TypeNodes string              // depends on cluster name, will not autocomplete
type TypeNodesPlusAllOption string // depends on cluster name, will not autocomplete
type TypeNode int                  // depends on cluster name, will not autocomplete
type TypeYesNo string
type TypeHBMode string
type TypeDistro string
type TypeDistroVersion string
type TypeAerospikeVersion string
type TypeClientVersion string
type TypeExistsAction string
type TypeNetType string
type TypeNetBlockOn string
type TypeNetStatisticMode string
type TypeNetLossAction string
type TypeXDRVersion string
type TypeClientName string
type TypeMachines string

func (t *TypeClientName) String() string {
	return string(*t)
}
func (t *TypeMachines) String() string {
	return string(*t)
}
func (t *TypeClusterName) String() string {
	return string(*t)
}
func (t *TypeNodes) String() string {
	return string(*t)
}
func (t *TypeNodesPlusAllOption) String() string {
	return string(*t)
}
func (t *TypeYesNo) String() string {
	return string(*t)
}
func (t *TypeHBMode) String() string {
	return string(*t)
}
func (t *TypeAerospikeVersion) String() string {
	return string(*t)
}
func (t *TypeDistro) String() string {
	return string(*t)
}
func (t *TypeDistroVersion) String() string {
	return string(*t)
}
func (t *TypeClientVersion) String() string {
	return string(*t)
}
func (t *TypeExistsAction) String() string {
	return string(*t)
}
func (t *TypeNetType) String() string {
	return string(*t)
}
func (t *TypeNetBlockOn) String() string {
	return string(*t)
}
func (t *TypeNetStatisticMode) String() string {
	return string(*t)
}
func (t *TypeNetLossAction) String() string {
	return string(*t)
}
func (t *TypeXDRVersion) String() string {
	return string(*t)
}
func (t *TypeNode) Int() int {
	return int(*t)
}

func (t *TypeClientName) Complete(match string) []flags.Completion {
	if !completionCustomCheck() {
		return []flags.Completion{}
	}
	b.WorkOnClients()
	clist, err := b.ClusterList()
	if err != nil {
		log.Fatalf("Backend query failed: %s", err)
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

func (t *TypeClusterName) Complete(match string) []flags.Completion {
	if !completionCustomCheck() {
		return []flags.Completion{}
	}
	b.WorkOnServers()
	clist, err := b.ClusterList()
	if err != nil {
		log.Fatalf("Backend query failed: %s", err)
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

func (t *TypeYesNo) Complete(match string) []flags.Completion {
	clist := []string{"y", "n", "yes", "no"}
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

func (t *TypeHBMode) Complete(match string) []flags.Completion {
	clist := []string{"mesh", "mcast", "default"}
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

func (t *TypeClientVersion) Complete(match string) []flags.Completion {
	clist := []string{"4", "5", "6"}
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

func (t *TypeExistsAction) Complete(match string) []flags.Completion {
	clist := []string{"CREATE_ONLY", "REPLACE_ONLY", "REPLACE", "UPDATE_ONLY", "UPDATE"}
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

func (t *TypeNetType) Complete(match string) []flags.Completion {
	clist := []string{"reject", "drop"}
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

func (t *TypeNetBlockOn) Complete(match string) []flags.Completion {
	clist := []string{"input", "output"}
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

func (t *TypeNetStatisticMode) Complete(match string) []flags.Completion {
	clist := []string{"random", "nth"}
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

func (t *TypeNetLossAction) Complete(match string) []flags.Completion {
	clist := []string{"set", "del", "delall", "show"}
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

func (t *TypeXDRVersion) Complete(match string) []flags.Completion {
	clist := []string{"4", "5", "auto"}
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

func (t *TypeDistro) Complete(match string) []flags.Completion {
	clist := []string{"ubuntu", "amazon", "centos"}
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

func (t *TypeDistroVersion) Complete(match string) []flags.Completion {
	clist := []string{"22.04", "20.04", "18.04", "8", "7", "2"}
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

func (t *TypeAerospikeVersion) Complete(match string) []flags.Completion {
	clist := []string{"latest", "latestc"}
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
