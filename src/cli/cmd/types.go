package cmd

import (
	"slices"
	"sort"
	"strings"

	flags "github.com/rglonek/go-flags"
)

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
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, []string{"COMPLETION"}, nil)
	if err != nil {
		return nil
	}
	inv := system.Backend.GetInventory()
	inst := inv.Instances.WithTags(map[string]string{"aerolab-cluster-type": "client"})
	clients := []string{}
	for _, i := range inst.Describe() {
		if !slices.Contains(clients, i.ClusterName) && strings.Contains(i.ClusterName, match) {
			clients = append(clients, i.ClusterName)
		}
	}
	sort.Strings(clients)
	return nil
}

func (t *TypeClusterName) Complete(match string) []flags.Completion {
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, []string{"COMPLETION"}, nil)
	if err != nil {
		return nil
	}
	inv := system.Backend.GetInventory()
	inst := inv.Instances.WithTags(map[string]string{"aerolab-cluster-type": "server"})
	servers := []string{}
	for _, i := range inst.Describe() {
		if !slices.Contains(servers, i.ClusterName) && strings.Contains(i.ClusterName, match) {
			servers = append(servers, i.ClusterName)
		}
	}
	sort.Strings(servers)
	return nil
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
	clist := []string{"ubuntu", "amazon", "rocky", "centos", "debian"}
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
	clist := []string{"24.04", "22.04", "20.04", "18.04", "9", "8", "7", "2", "2023", "12", "11", "10", "9"}
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
	clist := []string{"latest", "latestc", "latestf"}
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
