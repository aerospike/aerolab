package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/webui"
	"github.com/bestmethod/inslice"
	"github.com/lithammer/shortuuid"
)

type inventoryCache struct {
	update          func() (bool, error)
	runLock         *sync.Mutex
	RefreshInterval time.Duration
	inv             *inventoryJson
	sync.RWMutex
	ilcMutex        *sync.RWMutex
	MinimumInterval time.Duration
	lastRun         time.Time
	executable      string
}

func (i *inventoryCache) Start(update func() (bool, error)) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	i.executable = executable
	i.update = update
	err = i.run(time.Time{})
	if err != nil {
		return err
	}
	go func() {
		for {
			time.Sleep(i.RefreshInterval)
			err = i.run(time.Time{})
			if err != nil {
				log.Printf("ERROR: Inventory refresh failure: %s", err)
			}
		}
	}()
	return nil
}

func (i *inventoryCache) run(jobEndTimestamp time.Time) error {
	i.runLock.Lock()
	defer i.runLock.Unlock()
	isUpdated, err := i.update()
	if err != nil {
		log.Printf("INVENTORY CACHE: WARNING: %s", err)
	}
	if !jobEndTimestamp.IsZero() && i.lastRun.After(jobEndTimestamp) && !isUpdated {
		return nil
	}
	if i.lastRun.Add(i.MinimumInterval).After(time.Now()) {
		if jobEndTimestamp.IsZero() {
			return nil
		}
		sleepTimer := time.Until(i.lastRun.Add(i.MinimumInterval))
		if sleepTimer > 0 {
			time.Sleep(sleepTimer)
		}
	}
	i.lastRun = time.Now()
	out, err := exec.Command(i.executable, "inventory", "list", "-j", "-p").CombinedOutput()
	if err != nil {
		if isUpdated {
			i.Lock()
			i.inv = &inventoryJson{}
			i.Unlock()
		}
		return fmt.Errorf("%s: %s", err, string(out))
	}
	inv := &inventoryJson{}
	err = json.Unmarshal(out, inv)
	if err != nil {
		return err
	}
	for i := range inv.Clusters {
		if strings.ToLower(inv.Clusters[i].State) == "running" || strings.HasPrefix(inv.Clusters[i].State, "Up_") {
			inv.Clusters[i].IsRunning = true
		}
	}
	for i := range inv.Clients {
		if strings.ToLower(inv.Clients[i].State) == "running" || strings.HasPrefix(inv.Clients[i].State, "Up_") {
			inv.Clients[i].IsRunning = true
		}
	}
	for i := range inv.AGI {
		if strings.ToLower(inv.AGI[i].State) == "running" || strings.HasPrefix(inv.AGI[i].State, "Up_") {
			inv.AGI[i].IsRunning = true
		}
	}
	i.Lock()
	i.inv = inv
	i.Unlock()
	return nil
}

func (c *webCmd) getInventoryNames() map[string]*webui.InventoryItem {
	out := make(map[string]*webui.InventoryItem)
	m := reflect.TypeOf(inventoryJson{})
	for i := 0; i < m.NumField(); i++ {
		out[m.Field(i).Name] = &webui.InventoryItem{}
		for j := 0; j < m.Field(i).Type.Elem().NumField(); j++ {
			if m.Field(i).Type.Elem().Field(j).Tag.Get("hidden") != "" {
				continue
			}
			if m.Field(i).Type.Elem().Field(j).Tag.Get("backends") != "" {
				backends := strings.Split(m.Field(i).Type.Elem().Field(j).Tag.Get("backends"), ",")
				if !inslice.HasString(backends, a.opts.Config.Backend.Type) {
					continue
				}
			}
			name := m.Field(i).Type.Elem().Field(j).Name
			ntype := m.Field(i).Type.Elem().Field(j).Type
			if ntype.Kind() == reflect.Ptr {
				ntype = m.Field(i).Type.Elem().Field(j).Type.Elem()
			}
			if ntype.Kind() == reflect.Struct && inslice.HasString([]string{"AWS", "GCP", "Docker"}, name) {
				backend := name + "."
				if strings.ToLower(name) != a.opts.Config.Backend.Type {
					continue
				}
				// get data from underneath
				for x := 0; x < ntype.NumField(); x++ {
					if ntype.Field(x).Tag.Get("hidden") != "" {
						continue
					}
					if ntype.Field(x).Tag.Get("backends") != "" {
						backends := strings.Split(ntype.Field(x).Tag.Get("backends"), ",")
						if !inslice.HasString(backends, a.opts.Config.Backend.Type) {
							continue
						}
					}
					name := ntype.Field(x).Name
					if name[0] >= 'a' && name[0] <= 'z' { // do not process parameters which start from lowercase - these will not be exported
						continue
					}
					fname := ntype.Field(x).Tag.Get("row")
					if fname == "" {
						fname = name
					}
					out[m.Field(i).Name].Fields = append(out[m.Field(i).Name].Fields, &webui.InventoryItemField{
						Name:         name,
						FriendlyName: fname,
						Backend:      backend,
					})
				}
				continue
			}
			if name[0] >= 'a' && name[0] <= 'z' { // do not process parameters which start from lowercase - these will not be exported
				continue
			}
			fname := m.Field(i).Type.Elem().Field(j).Tag.Get("row")
			if fname == "" {
				fname = name
			}
			out[m.Field(i).Name].Fields = append(out[m.Field(i).Name].Fields, &webui.InventoryItemField{
				Name:         name,
				FriendlyName: fname,
			})
		}
	}
	return out
}

func (c *webCmd) inventoryLogFile(requestID string) (*os.File, error) {
	rootDir, err := a.aerolabRootDir()
	if err != nil {
		return nil, err
	}
	rootDir = path.Join(rootDir, "weblog")
	os.MkdirAll(rootDir, 0755)
	fn := path.Join(rootDir, time.Now().Format("2006-01-02_15-04-05_")+requestID+".log")
	return os.Create(fn)
}

func (c *webCmd) inventory(w http.ResponseWriter, r *http.Request) {
	p := &webui.Page{
		Backend:                                 a.opts.Config.Backend.Type,
		WebRoot:                                 c.WebRoot,
		FixedNavbar:                             true,
		FixedFooter:                             true,
		PendingActionsShowAllUsersToggle:        false,
		PendingActionsShowAllUsersToggleChecked: false,
		IsInventory:                             true,
		Inventory:                               c.inventoryNames,
		Navigation: &webui.Nav{
			Top: []*webui.NavTop{
				{
					Name: "Home",
					Href: c.WebRoot,
				},
			},
		},
		Menu: &webui.MainMenu{
			Items: c.menuItems,
		},
	}
	p.Menu.Items.Set(r.URL.Path, c.WebRoot)
	www := os.DirFS(c.WebPath)
	t, err := template.ParseFS(www, "*.html", "*.js", "*.css")
	if err != nil {
		log.Fatal(err)
	}
	err = t.ExecuteTemplate(w, "main", p)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *webCmd) addInventoryHandlers() {
	http.HandleFunc(c.WebRoot+"www/api/inventory/volumes", c.inventoryVolumes)
	http.HandleFunc(c.WebRoot+"www/api/inventory/firewalls", c.inventoryFirewalls)
	http.HandleFunc(c.WebRoot+"www/api/inventory/expiry", c.inventoryExpiry)
	http.HandleFunc(c.WebRoot+"www/api/inventory/subnets", c.inventorySubnets)
	http.HandleFunc(c.WebRoot+"www/api/inventory/templates", c.inventoryTemplates)
	http.HandleFunc(c.WebRoot+"www/api/inventory/clusters", c.inventoryClusters)
	http.HandleFunc(c.WebRoot+"www/api/inventory/clients", c.inventoryClients)
	http.HandleFunc(c.WebRoot+"www/api/inventory/agi", c.inventoryAGI)
	http.HandleFunc(c.WebRoot+"www/api/inventory/agi/connect", c.inventoryAGIConnect)
	http.HandleFunc(c.WebRoot+"www/api/inventory/nodes", c.inventoryNodesAction)
	http.HandleFunc(c.WebRoot+"www/api/inventory/", c.inventory)
}

func (c *webCmd) runInvCmd(reqID string, npath string, cjson map[string]interface{}, f *os.File) error {
	ex, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.AbsoluteTimeout)
	defer cancel()
	run := exec.CommandContext(ctx, ex, "webrun")
	c.joblist.Add(reqID, run)
	defer c.joblist.Delete(reqID)
	stdin, err := run.StdinPipe()
	if err != nil {
		return err
	}
	run.Stderr = f
	run.Stdout = f
	err = run.Start()
	if err != nil {
		return err
	}
	go func() {
		stdin.Write([]byte(npath + "-=-=-=-"))
		json.NewEncoder(stdin).Encode(cjson)
		stdin.Close()
	}()
	runerr := run.Wait()
	if runerr != nil {
		return runerr
	}
	exitCode := run.ProcessState.ExitCode()
	if exitCode != 0 {
		return errors.New("code:" + strconv.Itoa(exitCode))
	}
	return nil
}

func (c *webCmd) inventoryFirewalls(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		c.inventoryFirewallsAction(w, r)
		return
	}
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	json.NewEncoder(w).Encode(c.cache.inv.FirewallRules)
}

func (c *webCmd) inventoryFirewallsAction(w http.ResponseWriter, r *http.Request) {
	reqID := shortuuid.New()
	log.Printf("[%s] %s %s:%s", reqID, r.RemoteAddr, r.Method, r.RequestURI)
	data := make(map[string][]inventoryFirewallRule)
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, "could not read request: %s"+err.Error(), http.StatusBadRequest)
		return
	}
	rules := data["list"]
	if len(rules) == 0 {
		http.Error(w, "received empty request", http.StatusBadRequest)
		return
	}
	c.jobqueue.Add()
	invlog, err := c.inventoryLogFile(reqID)
	if err != nil {
		http.Error(w, "could not create log file: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	comm := ""
	switch a.opts.Config.Backend.Type {
	case "aws":
		comm = "config aws delete-security-groups"
	case "gcp":
		comm = "config gcp delete-firewall-rules"
	case "docker":
		comm = "config docker delete-network"
	}
	invlog.WriteString("-=-=-=-=- [path] /" + strings.ReplaceAll(comm, " ", "/") + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [cmdline] WEBUI: " + comm + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [command] " + comm + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [Log] -=-=-=-=-\n")
	invlog.WriteString(fmt.Sprintf("[%s] DELETE %d rules\n", reqID, len(rules)))
	log.Printf("[%s] DELETE %d rules", reqID, len(rules))
	go func() {
		c.jobqueue.Start()
		defer c.jobqueue.Remove()
		defer c.jobqueue.End()
		defer invlog.Close()
		isError := false
		for _, rule := range rules {
			var genRule interface{}
			switch a.opts.Config.Backend.Type {
			case "aws":
				genRule = rule.AWS
				cmdJson := map[string]interface{}{
					"NamePrefix": strings.Split(rule.AWS.SecurityGroupName, "-")[0],
					"VPC":        rule.AWS.VPC,
					"Internal":   false,
				}
				err = c.runInvCmd(reqID, "/"+strings.ReplaceAll(comm, " ", "/"), cmdJson, invlog)
			case "gcp":
				genRule = rule.GCP
				cmdJson := map[string]interface{}{
					"NamePrefix": rule.GCP.FirewallName,
					"Internal":   false,
				}
				err = c.runInvCmd(reqID, "/"+strings.ReplaceAll(comm, " ", "/"), cmdJson, invlog)
			case "docker":
				genRule = rule.Docker
				cmdJson := map[string]interface{}{
					"Name": rule.Docker.NetworkName,
				}
				err = c.runInvCmd(reqID, "/"+strings.ReplaceAll(comm, " ", "/"), cmdJson, invlog)
			}
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, genRule))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, rule)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] DELETED (%v)\n", reqID, genRule))
				log.Printf("[%s] DELETED (%v)", reqID, genRule)
			}
		}
		invlog.WriteString("\n->Refreshing inventory cache\n")
		c.cache.run(time.Now())
		if isError {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 1 -=-=-=-=-\nerror\n")
		} else {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 0 -=-=-=-=-\nsuccess\n")
		}
		invlog.WriteString("-=-=-=-=- [END] -=-=-=-=-")
	}()
	w.Write([]byte(reqID))
}

func (c *webCmd) inventoryExpiry(w http.ResponseWriter, r *http.Request) {
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	json.NewEncoder(w).Encode(c.cache.inv.ExpirySystem)
}

func (c *webCmd) inventorySubnets(w http.ResponseWriter, r *http.Request) {
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	json.NewEncoder(w).Encode(c.cache.inv.Subnets)
}

func (c *webCmd) inventoryVolumes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		c.inventoryVolumesAction(w, r)
		return
	}
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	json.NewEncoder(w).Encode(c.cache.inv.Volumes)
}

func (c *webCmd) inventoryVolumesAction(w http.ResponseWriter, r *http.Request) {
	reqID := shortuuid.New()
	log.Printf("[%s] %s %s:%s", reqID, r.RemoteAddr, r.Method, r.RequestURI)
	data := make(map[string][]inventoryVolume)
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, "could not read request: %s"+err.Error(), http.StatusBadRequest)
		return
	}
	vols := data["list"]
	if len(vols) == 0 {
		http.Error(w, "received empty request", http.StatusBadRequest)
		return
	}
	c.jobqueue.Add()
	invlog, err := c.inventoryLogFile(reqID)
	if err != nil {
		http.Error(w, "could not create log file: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	comm := "volume delete"
	invlog.WriteString("-=-=-=-=- [path] /" + strings.ReplaceAll(comm, " ", "/") + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [cmdline] WEBUI: " + comm + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [command] " + comm + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [Log] -=-=-=-=-\n")
	invlog.WriteString(fmt.Sprintf("[%s] DELETE %d volumes\n", reqID, len(vols)))
	log.Printf("[%s] DELETE %d volumes", reqID, len(vols))
	go func() {
		c.jobqueue.Start()
		defer c.jobqueue.Remove()
		defer c.jobqueue.End()
		defer invlog.Close()
		isError := false
		for _, vol := range vols {
			cmdJson := map[string]interface{}{
				"Name": vol.Name,
				"GCP": map[string]interface{}{
					"Zone": vol.AvailabilityZoneName,
				},
			}
			err = c.runInvCmd(reqID, "/"+strings.ReplaceAll(comm, " ", "/"), cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, vol.Name))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, vol.Name)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] DELETED (%v)\n", reqID, vol.Name))
				log.Printf("[%s] DELETED (%v)", reqID, vol.Name)
			}
		}
		invlog.WriteString("\n->Refreshing inventory cache\n")
		c.cache.run(time.Now())
		if isError {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 1 -=-=-=-=-\nerror\n")
		} else {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 0 -=-=-=-=-\nsuccess\n")
		}
		invlog.WriteString("-=-=-=-=- [END] -=-=-=-=-")
	}()
	w.Write([]byte(reqID))
}

func (c *webCmd) inventoryTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		c.inventoryTemplatesAction(w, r)
		return
	}
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	json.NewEncoder(w).Encode(c.cache.inv.Templates)
}

func (c *webCmd) inventoryTemplatesAction(w http.ResponseWriter, r *http.Request) {
	reqID := shortuuid.New()
	log.Printf("[%s] %s %s:%s", reqID, r.RemoteAddr, r.Method, r.RequestURI)
	data := make(map[string][]inventoryTemplate)
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, "could not read request: %s"+err.Error(), http.StatusBadRequest)
		return
	}
	templates := data["list"]
	if len(templates) == 0 {
		http.Error(w, "received empty request", http.StatusBadRequest)
		return
	}
	c.jobqueue.Add()
	invlog, err := c.inventoryLogFile(reqID)
	if err != nil {
		http.Error(w, "could not create log file: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	invlog.WriteString("-=-=-=-=- [path] /template/destroy -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [cmdline] WEBUI: template destroy -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [command] template destroy -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [Log] -=-=-=-=-\n")
	invlog.WriteString(fmt.Sprintf("[%s] DELETE %d templates\n", reqID, len(templates)))
	log.Printf("[%s] DELETE %d templates", reqID, len(templates))
	go func() {
		c.jobqueue.Start()
		defer c.jobqueue.Remove()
		defer c.jobqueue.End()
		defer invlog.Close()
		isError := false
		for _, template := range templates {
			isArm := true
			if template.Arch == "amd64" {
				isArm = false
			}
			cmdJson := map[string]interface{}{
				"AerospikeVersion": template.AerospikeVersion,
				"DistroName":       template.Distribution,
				"DistroVersion":    template.OSVersion,
				"AWS": map[string]interface{}{
					"IsArm": isArm,
				},
				"GCP": map[string]interface{}{
					"IsArm": isArm,
				},
			}
			err = c.runInvCmd(reqID, "/template/destroy", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, template))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, template)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] DELETED (%v)\n", reqID, template))
				log.Printf("[%s] DELETED (%v)", reqID, template)
			}
		}
		invlog.WriteString("\n->Refreshing inventory cache\n")
		c.cache.run(time.Now())
		if isError {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 1 -=-=-=-=-\nerror\n")
		} else {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 0 -=-=-=-=-\nsuccess\n")
		}
		invlog.WriteString("-=-=-=-=- [END] -=-=-=-=-")
	}()
	w.Write([]byte(reqID))
}

func (c *webCmd) inventoryClusters(w http.ResponseWriter, r *http.Request) {
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	clusters := []inventoryCluster{}
	for _, cluster := range c.cache.inv.Clusters {
		if cluster.Features&ClusterFeatureAGI > 0 {
			continue
		}
		clusters = append(clusters, cluster)
	}
	json.NewEncoder(w).Encode(clusters)
}

func (c *webCmd) inventoryClients(w http.ResponseWriter, r *http.Request) {
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	json.NewEncoder(w).Encode(c.cache.inv.Clients)
}

func (c *webCmd) inventoryAGIConnect(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	token, err := c.agiTokens.GetToken(name)
	if err != nil {
		http.Error(w, "could not get token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(token))
}

func (c *webCmd) inventoryAGI(w http.ResponseWriter, r *http.Request) {
	c.cache.ilcMutex.RLock()
	c.cache.RLock()
	inv := []inventoryWebAGI{}
	agiVolNames := make(map[string]int) // map[agiName] = indexOf(inv)
	for _, vol := range c.cache.inv.Volumes {
		if !vol.AGIVolume {
			continue
		}
		agiVolNames[vol.Name] = len(inv)
		inv = append(inv, inventoryWebAGI{
			Name:         vol.Name,
			VolOwner:     vol.Owner,
			AGILabel:     vol.AgiLabel,
			VolSize:      vol.SizeString,
			VolExpires:   vol.ExpiresIn,
			Zone:         vol.AvailabilityZoneName,
			VolID:        vol.FileSystemId,
			CreationTime: vol.CreationTime,
		})
	}
	for _, inst := range c.cache.inv.Clusters {
		if inst.Features&ClusterFeatureAGI <= 0 {
			continue
		}
		if idx, ok := agiVolNames[inst.ClusterName]; ok {
			inv[idx].State = inst.State
			inv[idx].Expires = inst.Expires
			inv[idx].Owner = inst.Owner
			inv[idx].AccessURL = inst.AccessUrl
			inv[idx].AGILabel = inst.AGILabel
			inv[idx].RunningCost = inst.InstanceRunningCost
			inv[idx].PublicIP = inst.PublicIp
			inv[idx].PrivateIP = inst.PrivateIp
			inv[idx].Firewalls = inst.Firewalls
			inv[idx].Zone = inst.Zone
			inv[idx].InstanceID = inst.InstanceId
			inv[idx].ImageID = inst.ImageId
			inv[idx].InstanceType = inst.InstanceType
			inv[idx].CreationTime = time.Time{} // TODO
			inv[idx].IsRunning = inst.IsRunning
		} else {
			inv = append(inv, inventoryWebAGI{
				Name:         inst.ClusterName,
				State:        inst.State,
				Expires:      inst.Expires,
				Owner:        inst.Owner,
				AccessURL:    inst.AccessUrl,
				AGILabel:     inst.AGILabel,
				RunningCost:  inst.InstanceRunningCost,
				PublicIP:     inst.PublicIp,
				PrivateIP:    inst.PrivateIp,
				Firewalls:    inst.Firewalls,
				Zone:         inst.Zone,
				InstanceID:   inst.InstanceId,
				ImageID:      inst.ImageId,
				InstanceType: inst.InstanceType,
				CreationTime: time.Time{}, // TODO
				IsRunning:    inst.IsRunning,
			})
		}
	}
	c.cache.RUnlock()
	c.cache.ilcMutex.RUnlock()
	// get statuses for instances running
	updateLock := new(sync.Mutex)
	workers := new(sync.WaitGroup)
	statusThreads := make(chan int, 10)
	ex, err := os.Executable()
	if err != nil {
		log.Printf("ERROR could not get link for self: %s", err)
	} else {
		for i := range inv {
			if inv[i].PublicIP == "" && inv[i].PrivateIP == "" {
				continue
			}
			workers.Add(1)
			statusThreads <- 1
			go func(i int, v inventoryWebAGI) {
				defer workers.Done()
				defer func() {
					<-statusThreads
				}()
				statusMsg := "unknown"
				if (v.PublicIP != "") || (a.opts.Config.Backend.Type == "docker" && v.PrivateIP != "") {
					ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer ctxCancel()
					out, err := exec.CommandContext(ctx, ex, "attach", "shell", "-n", v.Name, "--", "aerolab", "agi", "exec", "ingest-status").CombinedOutput()
					if err == nil {
						clusterStatus := &ingest.IngestStatusStruct{}
						err = json.Unmarshal(out, clusterStatus)
						if err == nil {
							if !clusterStatus.AerospikeRunning {
								statusMsg = "ERR: ASD DOWN"
							} else if !clusterStatus.GrafanaHelperRunning {
								statusMsg = "ERR: GRAFANAFIX DOWN"
							} else if !clusterStatus.PluginRunning {
								statusMsg = "ERR: PLUGIN DOWN"
							} else if !clusterStatus.Ingest.CompleteSteps.Init {
								statusMsg = "(1/6) INIT"
							} else if !clusterStatus.Ingest.CompleteSteps.Download {
								statusMsg = fmt.Sprintf("(2/6) DOWNLOAD %d%%", clusterStatus.Ingest.DownloaderCompletePct)
							} else if !clusterStatus.Ingest.CompleteSteps.Unpack {
								statusMsg = "(3/6) UNPACK"
							} else if !clusterStatus.Ingest.CompleteSteps.PreProcess {
								statusMsg = "(4/6) PRE-PROCESS"
							} else if !clusterStatus.Ingest.CompleteSteps.ProcessLogs {
								statusMsg = fmt.Sprintf("(5/6) PROCESS %d%%", clusterStatus.Ingest.LogProcessorCompletePct)
							} else if !clusterStatus.Ingest.CompleteSteps.ProcessCollectInfo {
								statusMsg = "(6/6) COLLECTINFO"
							} else {
								statusMsg = "READY"
							}
							if statusMsg != "READY" && !clusterStatus.Ingest.Running {
								statusMsg = "ERR: INGEST DOWN"
							}
						}
					}
				} else {
					statusMsg = ""
				}
				updateLock.Lock()
				inv[i].Status = statusMsg
				updateLock.Unlock()
			}(i, inv[i])
		}
		workers.Wait()
	}
	// sort
	sort.Slice(inv, func(i, j int) bool {
		if inv[i].InstanceID != "" && inv[j].InstanceID == "" {
			return true
		}
		if inv[i].InstanceID == "" && inv[j].InstanceID != "" {
			return false
		}
		if inv[i].InstanceID == "" && inv[j].InstanceID == "" {
			return inv[i].CreationTime.After(inv[i].CreationTime)
		}
		return inv[i].Name < inv[j].Name
	})
	// send
	json.NewEncoder(w).Encode(inv)
}

func (c *webCmd) inventoryNodesAction(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]interface{})
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, "could not read request: %s"+err.Error(), http.StatusBadRequest)
		return
	}
	action := ""
	switch a := data["action"].(type) {
	case string:
		action = a
	default:
		http.Error(w, "invalid request (action)", http.StatusBadRequest)
		return
	}
	reqID := shortuuid.New()
	log.Printf("[%s] %s %s:%s action:%s", reqID, r.RemoteAddr, r.Method, r.RequestURI, action)
	if action == "" {
		http.Error(w, "received empty request", http.StatusBadRequest)
		return
	}
	c.inventoryNodesActionDo(w, r, reqID, data, action)
}

func (c *webCmd) inventoryNodesActionDo(w http.ResponseWriter, r *http.Request, reqID string, data map[string]interface{}, action string) {
	ntype := ""
	switch a := data["type"].(type) {
	case string:
		ntype = a
	default:
		http.Error(w, "invalid request (type)", http.StatusBadRequest)
		return
	}
	switch ntype {
	case "cluster", "client", "agi":
	default:
		http.Error(w, "invalid type", http.StatusBadRequest)
		return
	}
	if ntype != "agi" && action == "delete" {
		http.Error(w, "invalid action for type", http.StatusBadRequest)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			http.Error(w, "malformed request", http.StatusBadRequest)
			return
		}
	}()
	list := data["list"].([]interface{})
	c.jobqueue.Add()
	invlog, err := c.inventoryLogFile(reqID)
	if err != nil {
		http.Error(w, "could not create log file: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	ncmd := []string{ntype}
	if strings.HasPrefix(action, "aerospike") {
		ncmd = []string{"aerospike"}
	}
	switch action {
	case "start":
		ncmd = append(ncmd, action)
	case "stop":
		ncmd = append(ncmd, action)
	case "aerospikeStart":
		ncmd = append(ncmd, "start")
	case "aerospikeStop":
		ncmd = append(ncmd, "stop")
	case "aerospikeRestart":
		ncmd = append(ncmd, "restart")
	case "aerospikeStatus":
		ncmd = append(ncmd, "status")
	case "delete":
		ncmd = append(ncmd, action)
	case "destroy":
		ncmd = append(ncmd, action)
	case "extendExpiry":
		if ntype == "cluster" {
			ncmd = []string{"cluster", "add", "expiry"}
		} else {
			ncmd = []string{"client", "configure", "expiry"}
		}
	default:
		http.Error(w, "invalid action: "+action, http.StatusBadRequest)
		return
	}
	invlog.WriteString("-=-=-=-=- [path] /" + strings.Join(ncmd, "/") + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [cmdline] WEBUI: " + strings.Join(ncmd, " ") + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [command] " + strings.Join(ncmd, " ") + " -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [Log] -=-=-=-=-\n")
	xtype := ntype
	if xtype == "cluster" {
		xtype = "node"
	}
	invlog.WriteString(fmt.Sprintf("[%s] RUN ON %d "+xtype+"s\n", reqID, len(list)))
	log.Printf("[%s] RUN ON %d "+xtype+"s", reqID, len(list))
	go func() {
		c.jobqueue.Start()
		defer c.jobqueue.Remove()
		defer c.jobqueue.End()
		defer invlog.Close()
		isError := false
		clist := make(map[string][]string)
		zoneclist := make(map[string]map[string][]string)
		for _, item := range list {
			switch i := item.(type) {
			case map[string]interface{}:
				switch ntype {
				case "cluster":
					clusterName := ""
					nodeNo := ""
					zone := ""
					clusterName, err = getString(i["ClusterName"])
					if err != nil || clusterName == "" {
						isError = true
						invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "clusterName undefined"))
						log.Printf("[%s] ERROR %s", reqID, "clusterName undefined")
						continue
					}
					nodeNo, err = getString(i["NodeNo"])
					if err != nil || nodeNo == "" {
						isError = true
						invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "nodeNo undefined"))
						log.Printf("[%s] ERROR %s", reqID, "nodeNo undefined")
						continue
					}
					if a.opts.Config.Backend.Type != "docker" {
						zone, err = getString(i["Zone"])
						if err != nil || zone == "" {
							isError = true
							invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "zone undefined"))
							log.Printf("[%s] ERROR %s", reqID, "zone undefined")
							continue
						}
					}
					if _, ok := clist[clusterName]; !ok {
						clist[clusterName] = []string{}
					}
					clist[clusterName] = append(clist[clusterName], nodeNo)
					if _, ok := zoneclist[zone]; !ok {
						zoneclist[zone] = make(map[string][]string)
					}
					if _, ok := zoneclist[zone][clusterName]; !ok {
						zoneclist[zone][clusterName] = []string{}
					}
					zoneclist[zone][clusterName] = append(zoneclist[zone][clusterName], zone)
				case "client":
					clientName := ""
					nodeNo := ""
					zone := ""
					i := item.(map[string]interface{})
					clientName, err = getString(i["ClientName"])
					if err != nil || clientName == "" {
						isError = true
						invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "clientName undefined"))
						log.Printf("[%s] ERROR %s", reqID, "clientName undefined")
						continue
					}
					nodeNo, err = getString(i["NodeNo"])
					if err != nil || nodeNo == "" {
						isError = true
						invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "nodeNo undefined"))
						log.Printf("[%s] ERROR %s", reqID, "nodeNo undefined")
						continue
					}
					if a.opts.Config.Backend.Type != "docker" {
						zone, err = getString(i["Zone"])
						if err != nil || zone == "" {
							isError = true
							invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "zone undefined"))
							log.Printf("[%s] ERROR %s", reqID, "zone undefined")
							continue
						}
					}
					if _, ok := clist[clientName]; !ok {
						clist[clientName] = []string{}
					}
					clist[clientName] = append(clist[clientName], nodeNo)
					if _, ok := zoneclist[zone]; !ok {
						zoneclist[zone] = make(map[string][]string)
					}
					if _, ok := zoneclist[zone][clientName]; !ok {
						zoneclist[zone][clientName] = []string{}
					}
					zoneclist[zone][clientName] = append(zoneclist[zone][clientName], zone)
				case "agi":
					agiName := ""
					agiName, err = getString(i["Name"])
					if err != nil || agiName == "" {
						isError = true
						invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "agiName undefined"))
						log.Printf("[%s] ERROR %s", reqID, "agiName undefined")
						continue
					}
					agiZone := ""
					if a.opts.Config.Backend.Type != "docker" {
						agiZone, err = getString(i["Zone"])
						if err != nil {
							isError = true
							invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "agiZone undefined"))
							log.Printf("[%s] ERROR %s", reqID, "agiZone undefined")
							continue
						}
					}
					if _, ok := clist[agiName]; !ok {
						clist[agiName] = []string{}
					}
					clist[agiName] = append(clist[agiName], agiZone)
				}
			default:
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "item type invalid"))
				log.Printf("[%s] ERROR %s", reqID, "item type invalid")
				continue
			}
		}
		switch action {
		case "start":
			hasError := c.inventoryNodesActionStart(w, r, reqID, action, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "stop":
			hasError := c.inventoryNodesActionStop(w, r, reqID, action, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeStart":
			hasError := c.inventoryNodesActionAerospikeStart(w, r, reqID, action, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeStop":
			hasError := c.inventoryNodesActionAerospikeStop(w, r, reqID, action, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeRestart":
			hasError := c.inventoryNodesActionAerospikeRestart(w, r, reqID, action, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeStatus":
			hasError := c.inventoryNodesActionAerospikeStatus(w, r, reqID, action, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "extendExpiry":
			hasError := c.inventoryNodesActionExtendExpiry(w, r, reqID, action, invlog, clist, ntype, data["expiry"].(string), zoneclist)
			if hasError {
				isError = true
			}
		case "delete":
			fallthrough
		case "destroy":
			hasError := c.inventoryNodesActionDestroy(w, r, reqID, action, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		default:
			http.Error(w, "invalid action: "+action, http.StatusBadRequest)
		}
		if !strings.HasPrefix(action, "aerospike") {
			invlog.WriteString("\n->Refreshing inventory cache\n")
			c.cache.run(time.Now())
		}
		if isError {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 1 -=-=-=-=-\nerror\n")
		} else {
			invlog.WriteString("\n-=-=-=-=- [ExitCode] 0 -=-=-=-=-\nsuccess\n")
		}
		invlog.WriteString("-=-=-=-=- [END] -=-=-=-=-")
	}()
	w.Write([]byte(reqID))
}

func (c *webCmd) inventoryNodesActionStart(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
	var err error
	for name, nodes := range clist {
		nodeNo := strings.Join(nodes, ",")
		switch ntype {
		case "cluster":
			clusterName := name
			cmdJson := map[string]interface{}{
				"ClusterName": clusterName,
				"Nodes":       nodeNo,
			}
			err = c.runInvCmd(reqID, "/cluster/start", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Started (%v)\n", reqID, clusterName+":"+nodeNo))
				log.Printf("[%s] Started (%v)", reqID, clusterName+":"+nodeNo)
			}
		case "client":
			clientName := name
			cmdJson := map[string]interface{}{
				"ClientName": clientName,
				"Machines":   nodeNo,
			}
			err = c.runInvCmd(reqID, "/client/start", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clientName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clientName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Started (%v)\n", reqID, clientName+":"+nodeNo))
				log.Printf("[%s] Started (%v)", reqID, clientName+":"+nodeNo)
			}
		case "agi":
			agiName := name
			cmdJson := map[string]interface{}{
				"ClusterName": agiName,
			}
			err = c.runInvCmd(reqID, "/agi/start", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Started (%v)\n", reqID, agiName))
				log.Printf("[%s] Started (%v)", reqID, agiName)
			}
		}
	}
	return
}

func (c *webCmd) inventoryNodesActionStop(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
	var err error
	for name, nodes := range clist {
		nodeNo := strings.Join(nodes, ",")
		switch ntype {
		case "cluster":
			clusterName := name
			cmdJson := map[string]interface{}{
				"ClusterName": clusterName,
				"Nodes":       nodeNo,
			}
			err = c.runInvCmd(reqID, "/cluster/stop", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Stopped (%v)\n", reqID, clusterName+":"+nodeNo))
				log.Printf("[%s] Stopped (%v)", reqID, clusterName+":"+nodeNo)
			}
		case "client":
			clientName := name
			cmdJson := map[string]interface{}{
				"ClientName": clientName,
				"Machines":   nodeNo,
			}
			err = c.runInvCmd(reqID, "/client/stop", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clientName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clientName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Stopped (%v)\n", reqID, clientName+":"+nodeNo))
				log.Printf("[%s] Stopped (%v)", reqID, clientName+":"+nodeNo)
			}
		case "agi":
			agiName := name
			cmdJson := map[string]interface{}{
				"ClusterName": agiName,
			}
			err = c.runInvCmd(reqID, "/agi/stop", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Stopped (%v)\n", reqID, agiName))
				log.Printf("[%s] Stopped (%v)", reqID, agiName)
			}
		}
	}
	return
}

func (c *webCmd) inventoryNodesActionAerospikeStart(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
	var err error
	for name, nodes := range clist {
		nodeNo := strings.Join(nodes, ",")
		switch ntype {
		case "cluster":
			clusterName := name
			cmdJson := map[string]interface{}{
				"ClusterName": clusterName,
				"Nodes":       nodeNo,
			}
			err = c.runInvCmd(reqID, "/aerospike/start", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Aerospike Started (%v)\n", reqID, clusterName+":"+nodeNo))
				log.Printf("[%s] Aerospike Started (%v)", reqID, clusterName+":"+nodeNo)
			}
		case "client":
			clientName := name
			isError = true
			err = errors.New("cannot start/stop aerospike on client nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clientName+":"+nodeNo))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, clientName+":"+nodeNo)
		case "agi":
			agiName := name
			isError = true
			err = errors.New("cannot start/stop aerospike on agi nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
		}
	}
	return
}

func (c *webCmd) inventoryNodesActionAerospikeStop(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
	var err error
	for name, nodes := range clist {
		nodeNo := strings.Join(nodes, ",")
		switch ntype {
		case "cluster":
			clusterName := name
			cmdJson := map[string]interface{}{
				"ClusterName": clusterName,
				"Nodes":       nodeNo,
			}
			err = c.runInvCmd(reqID, "/aerospike/stop", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Aerospike Stopped (%v)\n", reqID, clusterName+":"+nodeNo))
				log.Printf("[%s] Aerospike Stopped (%v)", reqID, clusterName+":"+nodeNo)
			}
		case "client":
			clientName := name
			isError = true
			err = errors.New("cannot start/stop aerospike on client nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clientName+":"+nodeNo))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, clientName+":"+nodeNo)
		case "agi":
			agiName := name
			isError = true
			err = errors.New("cannot start/stop aerospike on agi nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
		}
	}
	return
}

func (c *webCmd) inventoryNodesActionAerospikeRestart(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
	var err error
	for name, nodes := range clist {
		nodeNo := strings.Join(nodes, ",")
		switch ntype {
		case "cluster":
			clusterName := name
			cmdJson := map[string]interface{}{
				"ClusterName": clusterName,
				"Nodes":       nodeNo,
			}
			err = c.runInvCmd(reqID, "/aerospike/restart", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Aerospike Restarted (%v)\n", reqID, clusterName+":"+nodeNo))
				log.Printf("[%s] Aerospike Restarted (%v)", reqID, clusterName+":"+nodeNo)
			}
		case "client":
			clientName := name
			isError = true
			err = errors.New("cannot start/stop aerospike on client nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clientName+":"+nodeNo))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, clientName+":"+nodeNo)
		case "agi":
			agiName := name
			isError = true
			err = errors.New("cannot start/stop aerospike on agi nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
		}
	}
	return
}

func (c *webCmd) inventoryNodesActionAerospikeStatus(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
	var err error
	for name, nodes := range clist {
		nodeNo := strings.Join(nodes, ",")
		switch ntype {
		case "cluster":
			clusterName := name
			cmdJson := map[string]interface{}{
				"ClusterName": clusterName,
				"Nodes":       nodeNo,
			}
			err = c.runInvCmd(reqID, "/aerospike/status", cmdJson, invlog)
			invlog.WriteString("\n")
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] Aerospike Status (%v)\n", reqID, clusterName+":"+nodeNo))
				log.Printf("[%s] Aerospike Status (%v)", reqID, clusterName+":"+nodeNo)
			}
		case "client":
			clientName := name
			isError = true
			err = errors.New("cannot status aerospike on client nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clientName+":"+nodeNo))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, clientName+":"+nodeNo)
		case "agi":
			agiName := name
			isError = true
			err = errors.New("cannot status aerospike on agi nodes")
			invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
			log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
		}
	}
	return
}

func (c *webCmd) inventoryNodesActionDestroy(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
	var err error
	for name, nodes := range clist {
		nodeNo := strings.Join(nodes, ",")
		switch ntype {
		case "cluster":
			clusterName := name
			cmdJson := map[string]interface{}{
				"ClusterName": clusterName,
				"Nodes":       nodeNo,
				"Force":       true,
				"Parallel":    true,
			}
			err = c.runInvCmd(reqID, "/cluster/destroy", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] DELETED (%v)\n", reqID, clusterName+":"+nodeNo))
				log.Printf("[%s] DELETED (%v)", reqID, clusterName+":"+nodeNo)
			}
		case "client":
			clientName := name
			cmdJson := map[string]interface{}{
				"ClientName": clientName,
				"Machines":   nodeNo,
				"Force":      true,
				"Parallel":   true,
			}
			err = c.runInvCmd(reqID, "/client/destroy", cmdJson, invlog)
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clientName+":"+nodeNo))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, clientName+":"+nodeNo)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] DELETED (%v)\n", reqID, clientName+":"+nodeNo))
				log.Printf("[%s] DELETED (%v)", reqID, clientName+":"+nodeNo)
			}
		case "agi":
			agiName := name
			if action == "destroy" {
				cmdJson := map[string]interface{}{
					"ClusterName": agiName,
					"Force":       true,
					"Parallel":    true,
				}
				err = c.runInvCmd(reqID, "/agi/destroy", cmdJson, invlog)
			} else {
				agiZone := nodes[0]
				cmdJson := map[string]interface{}{
					"ClusterName": agiName,
					"Force":       true,
					"Parallel":    true,
					"Gcp": map[string]interface{}{
						"Zone": agiZone,
					},
				}
				err = c.runInvCmd(reqID, "/agi/delete", cmdJson, invlog)
			}
			if err != nil {
				isError = true
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
			} else {
				invlog.WriteString(fmt.Sprintf("[%s] DELETED (%v)\n", reqID, agiName))
				log.Printf("[%s] DELETED (%v)", reqID, agiName)
			}
		}
	}
	return
}

func (c *webCmd) inventoryNodesActionExtendExpiry(w http.ResponseWriter, r *http.Request, reqID string, action string, invlog *os.File, clist map[string][]string, ntype string, expStr string, zones map[string]map[string][]string) (isError bool) {
	expiry, err := time.ParseDuration(expStr)
	if err != nil {
		isError = true
		err = fmt.Errorf("invalid duration format: %s", err)
		invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, err))
		log.Printf("[%s] ERROR %s", reqID, err)
		return
	}
	// zones do not matter in aws
	if a.opts.Config.Backend.Type == "aws" {
		zones = make(map[string]map[string][]string)
		zones["aws"] = clist
	}
	for zone, nlist := range zones {
		for name, nodes := range nlist {
			nodeNo := strings.Join(nodes, ",")
			switch ntype {
			case "cluster":
				clusterName := name
				cmdJson := map[string]interface{}{
					"ClusterName": clusterName,
					"Nodes":       nodeNo,
					"Expires":     expiry.String(),
					"Gcp": map[string]interface{}{
						"Zone": zone,
					},
				}
				err = c.runInvCmd(reqID, "/cluster/add/expiry", cmdJson, invlog)
				if err != nil {
					isError = true
					invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
					log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
				} else {
					invlog.WriteString(fmt.Sprintf("[%s] Extend Expiry (%v)\n", reqID, clusterName+":"+nodeNo))
					log.Printf("[%s] Extend Expiry (%v)", reqID, clusterName+":"+nodeNo)
				}
			case "client":
				clusterName := name
				cmdJson := map[string]interface{}{
					"ClusterName": clusterName,
					"Nodes":       nodeNo,
					"Expires":     expiry.String(),
					"Gcp": map[string]interface{}{
						"Zone": zone,
					},
				}
				err = c.runInvCmd(reqID, "/client/configure/expiry", cmdJson, invlog)
				if err != nil {
					isError = true
					invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, clusterName+":"+nodeNo))
					log.Printf("[%s] ERROR %s (%v)", reqID, err, clusterName+":"+nodeNo)
				} else {
					invlog.WriteString(fmt.Sprintf("[%s] Extend Expiry (%v)\n", reqID, clusterName+":"+nodeNo))
					log.Printf("[%s] Extend Expiry (%v)", reqID, clusterName+":"+nodeNo)
				}
			case "agi":
				agiName := name
				isError = true
				err = errors.New("cannot extend expiry on agi nodes")
				invlog.WriteString(fmt.Sprintf("[%s] ERROR %s (%v)\n", reqID, err, agiName))
				log.Printf("[%s] ERROR %s (%v)", reqID, err, agiName)
			}
		}
	}
	return
}

func getString(item interface{}) (string, error) {
	switch i := item.(type) {
	case string:
		return i, nil
	default:
		return "", errors.New("not a string")
	}
}
