package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

// handleInventoryData handles GET /api/inventory/{type}
// Returns inventory data for the requested type: clusters, clients, agi, templates, volumes, firewalls, subnets, expiry
func (c *WebUICmd) handleInventoryData(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip root path and /api/inventory/ to get the type
	urlPath := r.URL.Path
	if c.rootPath != "" {
		urlPath = strings.TrimPrefix(urlPath, c.rootPath)
	}
	urlPath = strings.TrimPrefix(urlPath, "/api/inventory/")
	urlPath = strings.TrimSuffix(urlPath, "/")
	invType := strings.TrimSpace(urlPath)

	if invType == "" {
		http.Error(w, "inventory type is required", http.StatusBadRequest)
		return
	}

	inventory := c.getInventory()
	if inventory == nil {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode([]any{})
		return
	}

	var result any
	switch invType {
	case "clusters":
		instances := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).
			WithTags(map[string]string{"aerolab.type": "aerospike"}).Describe()
		result = instances
	case "clients":
		instances := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).
			WithTags(map[string]string{"aerolab.old.type": "client"}).Describe()
		result = instances
	case "agi":
		instances := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).
			WithTags(map[string]string{"aerolab.type": "agi"}).Describe()
		// Wrap each instance with cached ingest status
		type agiInstanceWithStatus struct {
			*backends.Instance
			Status string `json:"status"`
		}
		wrapped := make([]agiInstanceWithStatus, len(instances))
		for i, inst := range instances {
			// If the embedded monitor is actively sizing this instance, show "SIZING"
			sizingAction := c.agiSizingState.get(inst.ClusterName)
			status := ""
			if sizingAction != "" {
				status = "SIZING"
			} else {
				status = c.agiStatus.get(inst.ClusterName)
			}
			wrapped[i] = agiInstanceWithStatus{
				Instance: inst,
				Status:   status,
			}
		}
		result = wrapped
	case "templates":
		allImages := inventory.Images.WithInAccount(true).WithTags(map[string]string{"aerolab.image.type": "aerospike"}).Describe()
		// Filter out default/base images (those with no owner) so only user-created templates are shown
		userImages := make([]*backends.Image, 0, len(allImages))
		for _, img := range allImages {
			if img.Owner != "" {
				userImages = append(userImages, img)
			}
		}
		result = backends.ImageList(userImages)
	case "agi-templates":
		allImages := inventory.Images.WithInAccount(true).WithTags(map[string]string{"aerolab.image.type": "agi"}).Describe()
		userImages := make([]*backends.Image, 0, len(allImages))
		for _, img := range allImages {
			if img.Owner != "" {
				userImages = append(userImages, img)
			}
		}
		result = backends.ImageList(userImages)
	case "volumes":
		result = inventory.Volumes.Describe()
	case "firewalls":
		result = inventory.Firewalls.Describe()
	case "subnets":
		// Flatten subnets from all networks
		subnets := []*backends.Subnet{}
		for _, net := range inventory.Networks.Describe() {
			subnets = append(subnets, net.Subnets...)
		}
		result = subnets
	case "expiry":
		if c.system == nil || c.system.Backend == nil {
			result = []any{}
		} else {
			expList, err := c.system.Backend.ExpiryList()
			if err != nil {
				c.logError("Failed to get expiry list: %s", err)
				result = []any{}
			} else {
				result = expList.ExpirySystems
			}
		}
	case "instance-types":
		if c.system == nil || c.system.Backend == nil {
			result = []any{}
		} else {
			backendType := "docker"
			if c.system.Opts != nil && c.system.Opts.Config.Backend.Type != "" {
				backendType = c.system.Opts.Config.Backend.Type
			}
			instanceTypes, err := c.system.Backend.GetInstanceTypes(backends.BackendType(backendType))
			if err != nil {
				c.logError("Failed to get instance types: %s", err)
				result = []any{}
			} else {
				result = instanceTypes
			}
		}
	default:
		http.Error(w, fmt.Sprintf("unknown inventory type: %s", invType), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		c.logError("Failed to encode inventory response: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

// handleInventorySchema handles GET /api/inventory/schema
// Returns column definitions for each entity type
func (c *WebUICmd) handleInventorySchema(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	backendType := "docker"
	if c.system != nil && c.system.Opts != nil && c.system.Opts.Config.Backend.Type != "" {
		backendType = c.system.Opts.Config.Backend.Type
	}

	// Schema for Instance-based entities (clusters, clients, agi)
	instanceCols := []map[string]string{
		{"name": "Cluster Name", "field": "clusterName"},
		{"name": "Node", "field": "nodeNo"},
		{"name": "State", "field": "lifeCycleState"},
		{"name": "Public IP", "field": "IP.public"},
		{"name": "Private IP", "field": "IP.private"},
		{"name": "Zone", "field": "zoneName"},
		{"name": "Backend", "field": "backendType"},
		{"name": "Owner", "field": "owner"},
		{"name": "Expires", "field": "expires"},
	}

	// Add backend-specific columns
	if backendType == "aws" || backendType == "gcp" {
		instanceCols = append(instanceCols,
			map[string]string{"name": "Instance Type", "field": "instanceType"},
		)
	}

	// Instance ID is always the last column
	instanceCols = append(instanceCols,
		map[string]string{"name": "Instance ID", "field": "instanceId"},
	)

	// AGI gets an extra "Status" column after "State" to show ingest progress
	agiCols := make([]map[string]string, 0, len(instanceCols)+1)
	for _, col := range instanceCols {
		agiCols = append(agiCols, col)
		if col["field"] == "lifeCycleState" {
			agiCols = append(agiCols, map[string]string{"name": "Status", "field": "status"})
		}
	}

	entities := map[string]any{
		"clusters":       instanceCols,
		"clients":        instanceCols,
		"agi":            agiCols,
		"templates":      getImageSchema(),
		"agi-templates":  getImageSchema(),
		"volumes":        getVolumeSchema(),
		"firewalls":      getFirewallSchema(),
		"subnets":        getSubnetSchema(),
		"expiry":         getExpirySchema(),
		"instance-types": getInstanceTypesSchema(),
	}

	result := map[string]any{
		"backend":  backendType,
		"entities": entities,
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		c.logError("Failed to encode schema: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

func getImageSchema() []map[string]string {
	return []map[string]string{
		{"name": "Name", "field": "name"},
		{"name": "Description", "field": "description"},
		{"name": "OS", "field": "osName"},
		{"name": "OS Version", "field": "osVersion"},
		{"name": "Arch", "field": "architecture"},
		{"name": "Zone", "field": "zoneName"},
		{"name": "Owner", "field": "owner"},
	}
}

func getVolumeSchema() []map[string]string {
	return []map[string]string{
		{"name": "Name", "field": "name"},
		{"name": "Type", "field": "volumeType"},
		{"name": "Size", "field": "size"},
		{"name": "Zone", "field": "zoneName"},
		{"name": "State", "field": "state"},
		{"name": "Owner", "field": "owner"},
	}
}

func getFirewallSchema() []map[string]string {
	return []map[string]string{
		{"name": "Name", "field": "name"},
		{"name": "Network", "field": "networkId"},
		{"name": "Zone", "field": "zoneName"},
		{"name": "Owner", "field": "owner"},
	}
}

func getSubnetSchema() []map[string]string {
	return []map[string]string{
		{"name": "Name", "field": "name"},
		{"name": "Subnet ID", "field": "subnetId"},
		{"name": "CIDR", "field": "cidr"},
		{"name": "Zone", "field": "zoneName"},
		{"name": "Owner", "field": "owner"},
	}
}

func getExpirySchema() []map[string]string {
	return []map[string]string{
		{"name": "Backend", "field": "backendType"},
		{"name": "Zone", "field": "zone"},
		{"name": "Version", "field": "version"},
		{"name": "Frequency (min)", "field": "frequencyMinutes"},
		{"name": "Success", "field": "installationSuccess"},
	}
}

func getInstanceTypesSchema() []map[string]string {
	return []map[string]string{
		{"name": "Region", "field": "Region"},
		{"name": "Name", "field": "Name"},
		{"name": "Arch", "field": "Arch"},
		{"name": "vCPUs", "field": "CPUs"},
		{"name": "Memory (GiB)", "field": "MemoryGiB"},
		{"name": "NVMEs", "field": "NvmeCount"},
		{"name": "NVMe Total (GiB)", "field": "NvmeTotalSizeGiB"},
		{"name": "GPUs", "field": "GPUs"},
		{"name": "On-Demand $/h", "field": "PricePerHour.OnDemand"},
		{"name": "Spot $/h", "field": "PricePerHour.Spot"},
	}
}

// inventoryActionRequest is the JSON body for handleInventoryAction
type inventoryActionRequest struct {
	Items  []inventoryActionItem `json:"items"`
	Action string                `json:"action"`
	Type   string                `json:"type"`
	Params map[string]any        `json:"params"`
}

type inventoryActionItem struct {
	ClusterName string `json:"clusterName"`
	NodeNo      int    `json:"nodeNo"`
}

// handleInventoryAction handles POST /api/inventory/action
// Executes bulk actions on selected inventory items
func (c *WebUICmd) handleInventoryAction(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req inventoryActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %s", err), http.StatusBadRequest)
		return
	}

	if len(req.Items) == 0 {
		http.Error(w, "items is required and cannot be empty", http.StatusBadRequest)
		return
	}
	if req.Action == "" {
		http.Error(w, "action is required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		http.Error(w, "type is required (cluster|client|agi)", http.StatusBadRequest)
		return
	}

	// Group items by cluster name
	grouped := make(map[string][]int)
	for _, item := range req.Items {
		name := strings.TrimSpace(item.ClusterName)
		if name == "" {
			continue
		}
		nodes := grouped[name]
		if !containsInt(nodes, item.NodeNo) {
			nodes = append(nodes, item.NodeNo)
		}
		grouped[name] = nodes
	}

	// Build command path and params for each group
	var allJobs []*Job
	for clusterName, nodeNos := range grouped {
		sort.Ints(nodeNos)
		nodesStr := joinInts(nodeNos)

		cmdPath, params := c.buildInventoryActionParams(req.Type, req.Action, clusterName, nodesStr, req.Params)
		if cmdPath == "" {
			c.logError("Unknown action %s for type %s", req.Action, req.Type)
			continue
		}

		cmdInfo := c.commandTree.FindByPath(cmdPath)
		if cmdInfo == nil {
			c.logError("Command not found: %s", cmdPath)
			continue
		}

		cliCmd, cliErr := c.generateCLIWithReflection(cmdPath, params, false, false)
		if cliErr != nil {
			cliCmd = "" // fall back to map-based generation inside CreateJob
		}
		job, err := c.jobManager.CreateJob(c.getUserFromRequest(r), cmdPath, params, true, cliCmd)
		if err != nil {
			c.logError("Failed to create job: %s", err)
			continue
		}
		allJobs = append(allJobs, job)
		go c.executeJobAsync(job)
	}

	if len(allJobs) == 0 {
		http.Error(w, "No valid jobs could be created", http.StatusBadRequest)
		return
	}

	// Return the first job ID (frontend can poll this; for multi-group we return the first)
	jobID := allJobs[0].ID
	if len(allJobs) > 1 {
		// Store additional job IDs in response for multi-group actions
		jobIDs := make([]string, len(allJobs))
		for i, j := range allJobs {
			jobIDs[i] = j.ID
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"jobId": jobID, "jobIds": jobIDs})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{"jobId": jobID})
}

func (c *WebUICmd) buildInventoryActionParams(itemType, action, clusterName, nodesStr string, params map[string]any) (string, map[string]any) {
	baseParams := map[string]any{
		"name":  clusterName,
		"nodes": nodesStr,
	}

	// Merge in extra params (e.g. expiry duration)
	maps.Copy(baseParams, params)

	switch itemType {
	case "cluster":
		switch action {
		case "start":
			return "cluster/start", baseParams
		case "stop":
			return "cluster/stop", baseParams
		case "destroy":
			return "cluster/destroy", baseParams
		case "aerospikeStart":
			return "aerospike/start", baseParams
		case "aerospikeStop":
			return "aerospike/stop", baseParams
		case "aerospikeRestart":
			return "aerospike/restart", baseParams
		case "aerospikeStatus":
			return "aerospike/status", baseParams
		case "extendExpiry":
			if exp, ok := params["expiry"]; ok && exp != "" {
				baseParams["expiry"] = exp
			} else {
				baseParams["expiry"] = "30h"
			}
			return "cluster/add/expiry", baseParams
		}
	case "client":
		baseParams["group-name"] = clusterName
		baseParams["machines"] = nodesStr
		delete(baseParams, "name")
		delete(baseParams, "nodes")
		switch action {
		case "start":
			return "client/start", baseParams
		case "stop":
			return "client/stop", baseParams
		case "destroy":
			return "client/destroy", baseParams
		case "extendExpiry":
			if exp, ok := params["expiry"]; ok && exp != "" {
				baseParams["expiry"] = exp
			} else {
				baseParams["expiry"] = "30h"
			}
			return "client/configure/expiry", baseParams
		}
	case "agi":
		switch action {
		case "start":
			return "agi/start", map[string]any{"name": clusterName}
		case "stop":
			return "agi/stop", map[string]any{"name": clusterName}
		case "destroy":
			return "agi/destroy", map[string]any{"name": clusterName}
		case "delete":
			return "agi/delete", map[string]any{"name": clusterName}
		case "getShareLink":
			return "agi/add-auth-token", map[string]any{"name": clusterName, "url": "true"}
		case "changeLabel":
			p := map[string]any{"name": clusterName}
			if label, ok := params["label"]; ok {
				p["label"] = label
			}
			return "agi/change-label", p
		case "extendExpiry":
			exp := "30h"
			if e, ok := params["expiry"]; ok && e != "" {
				exp = fmt.Sprintf("%v", e)
			}
			return "instances/change-expiry", map[string]any{
				"filter-cluster-name": clusterName,
				"filter-type":         "agi",
				"expire-in":           exp,
			}
		}
	}
	return "", nil
}

func containsInt(s []int, v int) bool {
	return slices.Contains(s, v)
}

func joinInts(nums []int) string {
	ss := make([]string, len(nums))
	for i, n := range nums {
		ss[i] = strconv.Itoa(n)
	}
	return strings.Join(ss, ",")
}

// handleInventoryConnect handles GET/POST /api/inventory/connect/{type}
// Returns connection details for cluster/client/agi/trino/graph
func (c *WebUICmd) handleInventoryConnect(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	urlPath := r.URL.Path
	if c.rootPath != "" {
		urlPath = strings.TrimPrefix(urlPath, c.rootPath)
	}
	urlPath = strings.TrimPrefix(urlPath, "/api/inventory/connect/")
	urlPath = strings.TrimSuffix(urlPath, "/")
	connectType := strings.TrimSpace(urlPath)

	if connectType == "" {
		http.Error(w, "connect type is required", http.StatusBadRequest)
		return
	}

	inventory := c.getInventory()
	if inventory == nil {
		http.Error(w, "Backend not initialized", http.StatusInternalServerError)
		return
	}

	var result map[string]any

	switch connectType {
	case "cluster", "client":
		name := r.URL.Query().Get("name")
		nodeStr := r.URL.Query().Get("node")
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		var instances backends.InstanceList
		if connectType == "cluster" {
			instances = inventory.Instances.WithTags(map[string]string{"aerolab.type": "aerospike"}).WithClusterName(name).WithNotState(backends.LifeCycleStateTerminated).Describe()
		} else {
			instances = inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(name).WithNotState(backends.LifeCycleStateTerminated).Describe()
		}
		if nodeStr != "" {
			nodeNo, parseErr := strconv.Atoi(nodeStr)
			if parseErr != nil {
				http.Error(w, "invalid node number", http.StatusBadRequest)
				return
			}
			instances = instances.WithNodeNo(nodeNo).Describe()
		}
		if instances.Count() == 0 {
			http.Error(w, "instance not found", http.StatusNotFound)
			return
		}
		inst := instances.Describe()[0]
		result = map[string]any{
			"host":        inst.IP.Routable(),
			"publicIP":    inst.IP.Public,
			"privateIP":   inst.IP.Private,
			"clusterName": inst.ClusterName,
			"nodeNo":      inst.NodeNo,
			"sshUser":     "root",
			"sshKeyPath":  inst.GetSSHKeyPath(),
		}
	case "agi":
		if r.Method != "POST" {
			http.Error(w, "POST required for AGI connect", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %s", err), http.StatusBadRequest)
			return
		}
		if body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		instances := inventory.Instances.WithTags(map[string]string{"aerolab.type": "agi"}).
			WithClusterName(body.Name).WithState(backends.LifeCycleStateRunning)
		if instances.Count() == 0 {
			http.Error(w, "AGI instance not found or not running", http.StatusNotFound)
			return
		}
		inst := instances.Describe()[0]

		// Get or create a cached auth token for this instance
		token, tokenErr := c.getOrCreateAgiToken(body.Name, inst)
		if tokenErr != nil {
			http.Error(w, fmt.Sprintf("Failed to get AGI token: %s", tokenErr), http.StatusInternalServerError)
			return
		}

		// Build the full access URL with the token
		urlBase, urlErr := c.buildAgiTokenURLBase(inst)
		if urlErr != nil {
			http.Error(w, fmt.Sprintf("Failed to build access URL: %s", urlErr), http.StatusInternalServerError)
			return
		}

		result = map[string]any{
			"accessURL": urlBase + token,
			"name":      inst.ClusterName,
			"publicIP":  inst.IP.Public,
			"privateIP": inst.IP.Private,
		}
	case "trino":
		name := r.URL.Query().Get("name")
		nodeStr := r.URL.Query().Get("node")
		namespace := r.URL.Query().Get("namespace")
		if name == "" || nodeStr == "" {
			http.Error(w, "name and node are required", http.StatusBadRequest)
			return
		}
		instances := inventory.Instances.WithTags(map[string]string{"aerolab.type": "aerospike"}).
			WithClusterName(name).WithNotState(backends.LifeCycleStateTerminated)
		nodeNo, _ := strconv.Atoi(nodeStr)
		instances = instances.WithNodeNo(nodeNo)
		if instances.Count() == 0 {
			http.Error(w, "instance not found", http.StatusNotFound)
			return
		}
		inst := instances.Describe()[0]
		port := 8090
		result = map[string]any{
			"host":      inst.IP.Routable(),
			"port":      port,
			"namespace": namespace,
			"url":       fmt.Sprintf("http://%s:%d", inst.IP.Routable(), port),
		}
	case "graph":
		name := r.URL.Query().Get("name")
		nodeStr := r.URL.Query().Get("node")
		if name == "" || nodeStr == "" {
			http.Error(w, "name and node are required", http.StatusBadRequest)
			return
		}
		// Graph runs on client instances
		instances := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).
			WithClusterName(name).WithNotState(backends.LifeCycleStateTerminated)
		nodeNo, _ := strconv.Atoi(nodeStr)
		instances = instances.WithNodeNo(nodeNo)
		if instances.Count() == 0 {
			http.Error(w, "instance not found", http.StatusNotFound)
			return
		}
		inst := instances.Describe()[0]
		accessURL := inst.AccessURL
		if accessURL == "" {
			accessURL = fmt.Sprintf("http://%s", inst.IP.Routable())
		}
		result = map[string]any{
			"accessURL":   accessURL,
			"host":        inst.IP.Routable(),
			"clusterName": name,
			"nodeNo":      nodeStr,
		}
	default:
		http.Error(w, fmt.Sprintf("unknown connect type: %s", connectType), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		c.logError("Failed to encode connect response: %s", err)
	}
}
