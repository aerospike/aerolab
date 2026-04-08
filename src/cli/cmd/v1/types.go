package cmd

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	flags "github.com/rglonek/go-flags"
)

// TypeExpiry is a time.Duration that supports calendar-aware Y (years), M (months)
// and fixed W (weeks), D (days) units in addition to standard Go duration units.
type TypeExpiry time.Duration

func (t TypeExpiry) Duration() time.Duration {
	return time.Duration(t)
}

func (t TypeExpiry) String() string {
	return time.Duration(t).String()
}

func (t *TypeExpiry) UnmarshalFlag(value string) error {
	d, err := ParseExtendedDuration(value)
	if err != nil {
		return err
	}
	*t = TypeExpiry(d)
	return nil
}

func (t TypeExpiry) MarshalFlag() (string, error) {
	return time.Duration(t).String(), nil
}

// ParseExtendedDuration parses a duration string with extended calendar units.
//
// Calendar units (use time.Now and AddDate for correct rollover):
//
//	Y/y = calendar years, M = calendar months
//
// Fixed units:
//
//	W/w = 7 days, D/d = 24 hours
//
// Standard Go units (lowercase only, handled by time.ParseDuration):
//
//	h, m (minutes), s, ms, us, ns
//
// Composite values are supported: "1Y6M", "1D2h30m", "2W12h".
// Extended units must appear before standard units.
func ParseExtendedDuration(s string) (time.Duration, error) {
	return parseExtendedDurationFrom(s, time.Now())
}

func parseExtendedDurationFrom(s string, now time.Time) (time.Duration, error) {
	if s == "" || s == "0" {
		return 0, nil
	}

	original := s
	var years, months int
	var total time.Duration

	for len(s) > 0 {
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(s) {
			break
		}

		n, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, fmt.Errorf("time: invalid duration %q", original)
		}

		switch s[i] {
		case 'Y', 'y':
			years += n
		case 'M':
			months += n
		case 'W', 'w':
			total += time.Duration(n) * 7 * 24 * time.Hour
		case 'D', 'd':
			total += time.Duration(n) * 24 * time.Hour
		default:
			goto hms
		}
		s = s[i+1:]
		continue
	hms:
		break
	}

	if years > 0 || months > 0 {
		total += now.AddDate(years, months, 0).Sub(now)
	}

	if s == "" {
		return total, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("time: invalid duration %q", original)
	}
	return total + d, nil
}

type TypeClusterName string
type TypeAgiClusterName string
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
func (t *TypeAgiClusterName) String() string {
	return string(*t)
}
func (t *TypeNodes) String() string {
	if strings.EqualFold(string(*t), "all") {
		return ""
	}
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

// guiZone is a special string type for zone parameters in create commands.
// It provides dynamic choices for the web UI via the List() method.
//
// Web UI Dynamic Choices Pattern:
//
// To make a command parameter render as a dropdown in the web UI with dynamically-fetched options:
//  1. Define a custom type based on string (e.g. type guiZone string)
//  2. Implement String() string on it
//  3. Implement List(system *System) ([][]string, string, error) where:
//     - The [][]string return is a list of [value, displayLabel] pairs
//     - The string return is the default value to pre-select
//     - The error return signals any failure in fetching choices
//  4. Use the type on a struct field and add the tag: webchoice:"method::List"
//
// The web UI reflect system (ResolveDynamicChoices in cmdWebUIReflect.go) will:
//   - Detect the webchoice:"method::List" tag
//   - Call the List() method via reflection
//   - Populate the Choices field in the ParameterInfo sent to the frontend
//   - The frontend SelectInput component renders it as a searchable dropdown
//
// For static choices, use webchoice:"value1,value2,value3" instead.
type guiZone string

func (g guiZone) String() string {
	return string(g)
}

// List returns a list of available zones for the web UI zone picker.
func (g guiZone) List(system *System) ([][]string, string, error) {
	zones, err := system.Backend.ListAvailableZones(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return nil, "", err
	}
	z := [][]string{}
	for _, zone := range zones {
		z = append(z, []string{zone, zone})
	}
	def := ""
	if string(g) != "" {
		def = string(g)
	} else if len(zones) > 0 {
		def = zones[0]
	}
	return z, def, nil
}

// guiVpc is a special string type for VPC network parameters in GCP create commands.
// It provides dynamic choices for the web UI via the List() method.
// See guiZone documentation above for the full pattern description.
type guiVpc string

func (g guiVpc) String() string {
	return string(g)
}

// List returns a list of available VPC networks for the web UI VPC picker.
func (g guiVpc) List(system *System) ([][]string, string, error) {
	if system.Opts.Config.Backend.Type != "gcp" {
		return nil, "", nil
	}
	inventory := system.Backend.GetInventory()
	vpcs := [][]string{}
	for _, n := range inventory.Networks.WithBackendType(backends.BackendTypeGCP).Describe() {
		vpcs = append(vpcs, []string{n.Name, n.Name})
	}
	def := ""
	if string(g) != "" {
		def = string(g)
	}
	return vpcs, def, nil
}

// guiInstanceType is a special string type for instance type parameters in create commands.
// It provides dynamic choices for the web UI via the List() method.
// See guiZone documentation above for the full pattern description.
type guiInstanceType string

func (g guiInstanceType) String() string {
	return string(g)
}

// List returns a sorted list of instance types with details (arch, CPUs, RAM, NVMe, pricing),
// and the default instance type for the current backend.
func (g guiInstanceType) List(system *System) ([][]string, string, error) {
	instanceTypes, err := system.Backend.GetInstanceTypes(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return nil, "", err
	}
	var itypes backends.InstanceTypeList
	def := ""
	for _, it := range instanceTypes {
		if it.Region != system.Opts.Config.Backend.Region {
			continue
		}
		if len(it.Arch) == 0 {
			continue
		}
		if it.Name == string(g) {
			def = it.Name
		}
		itypes = append(itypes, it)
	}
	sep := "."
	if system.Opts.Config.Backend.Type == "gcp" {
		sep = "-"
	}
	sort.Slice(itypes, func(i, j int) bool {
		ni := strings.Split(itypes[i].Name, sep)[0]
		nj := strings.Split(itypes[j].Name, sep)[0]
		if ni < nj {
			return true
		}
		if ni > nj {
			return false
		}
		if itypes[i].CPUs != itypes[j].CPUs {
			return itypes[i].CPUs < itypes[j].CPUs
		}
		return itypes[i].MemoryGiB < itypes[j].MemoryGiB
	})
	types := [][]string{}
	for _, nType := range itypes {
		arch := "amd64"
		if slices.Contains(nType.Arch, backends.ArchitectureARM64) {
			arch = "arm64"
		}
		types = append(types, []string{nType.Name, fmt.Sprintf("%s (ARCH:%s CPUs:%d RamGB:%0.2f NVMe:%d/%dG on-demand:$%0.2f/hr spot:$%0.2f/hr)", nType.Name, arch, nType.CPUs, nType.MemoryGiB, nType.NvmeCount, nType.NvmeTotalSizeGiB, nType.PricePerHour.OnDemand, nType.PricePerHour.Spot)})
	}
	if def == "" {
		def = "e2-standard-4"
		if system.Opts.Config.Backend.Type == "aws" {
			def = "t3a.xlarge"
		}
	}
	return types, def, nil
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

func (t *TypeAgiClusterName) Complete(match string) []flags.Completion {
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, []string{"COMPLETION"}, nil)
	if err != nil {
		return nil
	}
	inv := system.Backend.GetInventory()
	inst := inv.Instances.WithTags(map[string]string{"aerolab.type": "agi"})
	agis := []string{}
	for _, i := range inst.Describe() {
		if !slices.Contains(agis, i.ClusterName) && strings.Contains(i.ClusterName, match) {
			agis = append(agis, i.ClusterName)
		}
	}
	sort.Strings(agis)
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
	clist := []string{"24.04", "22.04", "20.04", "18.04", "10", "9", "8", "7", "2", "2023", "13", "12", "11"}
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

func (t *TypeClusterName) GetInstanceList(inventory *backends.Inventory, interactiveStates ...backends.LifeCycleState) (backends.Instances, error) {
	if inventory == nil {
		return nil, errors.New("inventory is required")
	}
	tstr := string(*t)
	instances, err := getInstanceListForClusterName(&tstr, inventory, []string{"server", "aerospike"}, interactiveStates...)
	if err != nil {
		return nil, err
	}
	*t = TypeClusterName(tstr)
	return instances, nil
}

func (t *TypeAgiClusterName) GetInstanceList(inventory *backends.Inventory, interactiveStates ...backends.LifeCycleState) (backends.Instances, error) {
	if inventory == nil {
		return nil, errors.New("inventory is required")
	}
	tstr := string(*t)
	instances, err := getInstanceListForClusterName(&tstr, inventory, []string{"agi"}, interactiveStates...)
	if err != nil {
		return nil, err
	}
	*t = TypeAgiClusterName(tstr)
	return instances, nil
}

func (t *TypeClientName) GetInstanceList(inventory *backends.Inventory, interactiveStates ...backends.LifeCycleState) (backends.Instances, error) {
	if inventory == nil {
		return nil, errors.New("inventory is required")
	}
	tstr := string(*t)
	instances, err := getInstanceListForClusterName(&tstr, inventory, []string{"client"}, interactiveStates...)
	if err != nil {
		return nil, err
	}
	*t = TypeClientName(tstr)
	return instances, nil
}

// returns the cluster instance list for a given cluster name, or an error if cluster is not found.
// When instanceTypes contains "client", also matches instances with aerolab.old.type=client (legacy/Docker clients).
func getInstanceListForClusterName(t *string, inventory *backends.Inventory, instanceTypes []string, interactiveStates ...backends.LifeCycleState) (backends.Instances, error) {
	base := inventory.Instances.WithClusterName(*t).WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated)
	byType := base.WithType(instanceTypes...)
	var cluster backends.Instances
	if slices.Contains(instanceTypes, "client") {
		byOldType := base.WithOldType(instanceTypes...)
		cluster = mergeInstanceListsByID(byType.Describe(), byOldType.Describe())
	} else {
		cluster = byType.Describe()
	}
	if cluster == nil || cluster.Count() == 0 {
		if IsInteractive() {
			// get cluster list (include instances matching type, and old type only for client)
			baseAll := inventory.Instances.WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated)
			var allMerged backends.InstanceList
			if slices.Contains(instanceTypes, "client") {
				allMerged = mergeInstanceListsByID(
					baseAll.WithType(instanceTypes...).Describe(),
					baseAll.WithOldType(instanceTypes...).Describe(),
				)
			} else {
				allMerged = baseAll.WithType(instanceTypes...).Describe()
			}
			clusters := []string{}
			seenClusters := map[string]struct{}{}
			for _, i := range allMerged {
				if len(interactiveStates) > 0 && !slices.Contains(interactiveStates, i.InstanceState) {
					continue
				}
				if strings.Contains(i.ClusterName, *t) {
					if _, ok := seenClusters[i.ClusterName]; !ok {
						seenClusters[i.ClusterName] = struct{}{}
						clusters = append(clusters, i.ClusterName)
					}
				}
			}
			if len(clusters) == 0 {
				return nil, errors.New("cluster not found")
			}
			sort.Strings(clusters)
			// ask user to select a cluster interactively
			choice, quitting, err := choice.Choice(fmt.Sprintf("Cluster %s not found, select an existing cluster:", *t), choice.StringSliceToItems(clusters))
			if err != nil || quitting {
				return nil, err
			}
			*t = choice
			base = inventory.Instances.WithClusterName(*t).WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated)
			byType = base.WithType(instanceTypes...)
			if slices.Contains(instanceTypes, "client") {
				cluster = mergeInstanceListsByID(byType.Describe(), base.WithOldType(instanceTypes...).Describe())
			} else {
				cluster = byType.Describe()
			}
		} else {
			return nil, errors.New("cluster not found")
		}
	}
	return cluster.Describe(), nil
}

// mergeInstanceListsByID returns an InstanceList with instances from both lists, deduplicated by InstanceID.
func mergeInstanceListsByID(a, b backends.InstanceList) backends.InstanceList {
	seen := make(map[string]struct{})
	var merged backends.InstanceList
	for _, inst := range a {
		if _, ok := seen[inst.InstanceID]; ok {
			continue
		}
		seen[inst.InstanceID] = struct{}{}
		merged = append(merged, inst)
	}
	for _, inst := range b {
		if _, ok := seen[inst.InstanceID]; ok {
			continue
		}
		seen[inst.InstanceID] = struct{}{}
		merged = append(merged, inst)
	}
	return merged
}
