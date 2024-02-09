package main

import (
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
	http.HandleFunc(c.WebRoot+"www/api/inventory/nodes", c.inventoryNodesAction)
	http.HandleFunc(c.WebRoot+"www/api/inventory/", c.inventory)
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
	c.joblist.Add(reqID, &exec.Cmd{})
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
		defer c.joblist.Delete(reqID)
		defer invlog.Close()
		isError := false
		for _, rule := range rules {
			var genRule interface{}
			switch a.opts.Config.Backend.Type {
			case "aws":
				genRule = rule.AWS
				a.opts.Config.Aws.DestroySecGroups.NamePrefix = strings.Split(rule.AWS.SecurityGroupName, "-")[0]
				a.opts.Config.Aws.DestroySecGroups.VPC = rule.AWS.VPC
				a.opts.Config.Aws.DestroySecGroups.Internal = false
				err = a.opts.Config.Aws.DestroySecGroups.Execute(nil)
			case "gcp":
				genRule = rule.GCP
				a.opts.Config.Gcp.DestroySecGroups.NamePrefix = rule.GCP.FirewallName
				a.opts.Config.Gcp.DestroySecGroups.Internal = false
				err = a.opts.Config.Gcp.DestroySecGroups.Execute(nil)
			case "docker":
				genRule = rule.Docker
				a.opts.Config.Docker.DeleteNetwork.Name = rule.Docker.NetworkName
				err = a.opts.Config.Docker.DeleteNetwork.Execute(nil)
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
	c.joblist.Add(reqID, &exec.Cmd{})
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
		defer c.joblist.Delete(reqID)
		defer invlog.Close()
		isError := false
		for _, vol := range vols {
			a.opts.Volume.Delete.Name = vol.Name
			a.opts.Volume.Delete.Gcp.Zone = vol.AvailabilityZoneName
			err = a.opts.Volume.Delete.Execute(nil)
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
	c.joblist.Add(reqID, &exec.Cmd{})
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
		defer c.joblist.Delete(reqID)
		defer invlog.Close()
		isError := false
		for _, template := range templates {
			a.opts.Template.Delete.AerospikeVersion = TypeAerospikeVersion(template.AerospikeVersion)
			a.opts.Template.Delete.DistroName = TypeDistro(template.Distribution)
			a.opts.Template.Delete.DistroVersion = TypeDistroVersion(template.OSVersion)
			isArm := true
			if template.Arch == "amd64" {
				isArm = false
			}
			a.opts.Template.Delete.Aws.IsArm = isArm
			a.opts.Template.Delete.Gcp.IsArm = isArm
			err = a.opts.Template.Delete.Execute(nil)
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
	statusThreads := make(chan int, 5)
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
			if (v.PublicIP != "") || (a.opts.Config.Backend.Type == "docker" && v.PublicIP != "") {
				out, err := b.RunCommands(v.Name, [][]string{{"aerolab", "agi", "exec", "ingest-status"}}, []int{1})
				if err == nil {
					clusterStatus := &ingest.IngestStatusStruct{}
					err = json.Unmarshal(out[0], clusterStatus)
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
	switch action {
	case "destroy":
		c.inventoryNodesActionDestroy(w, r, reqID, data, action)
	case "delete":
		c.inventoryNodesActionDestroy(w, r, reqID, data, action)
	default:
		http.Error(w, "invalid action: "+action, http.StatusBadRequest)
	}
}

func (c *webCmd) inventoryNodesActionDestroy(w http.ResponseWriter, r *http.Request, reqID string, data map[string]interface{}, action string) {
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
	c.joblist.Add(reqID, &exec.Cmd{})
	invlog, err := c.inventoryLogFile(reqID)
	if err != nil {
		http.Error(w, "could not create log file: %s"+err.Error(), http.StatusInternalServerError)
		return
	}
	invlog.WriteString("-=-=-=-=- [path] /" + ntype + "/destroy -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [cmdline] WEBUI: " + ntype + " destroy -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [command] " + ntype + " destroy -=-=-=-=-\n")
	invlog.WriteString("-=-=-=-=- [Log] -=-=-=-=-\n")
	xtype := ntype
	if xtype == "cluster" {
		xtype = "node"
	}
	invlog.WriteString(fmt.Sprintf("[%s] DELETE %d "+xtype+"s\n", reqID, len(list)))
	log.Printf("[%s] DELETE %d "+xtype+"s", reqID, len(list))
	go func() {
		c.jobqueue.Start()
		defer c.jobqueue.Remove()
		defer c.jobqueue.End()
		defer c.joblist.Delete(reqID)
		defer invlog.Close()
		isError := false
		clist := make(map[string][]string)
		for _, item := range list {
			switch i := item.(type) {
			case map[string]interface{}:
				switch ntype {
				case "cluster":
					clusterName := ""
					nodeNo := ""
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
					if _, ok := clist[clusterName]; !ok {
						clist[clusterName] = []string{}
					}
					clist[clusterName] = append(clist[clusterName], nodeNo)
				case "client":
					clientName := ""
					nodeNo := ""
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
					if _, ok := clist[clientName]; !ok {
						clist[clientName] = []string{}
					}
					clist[clientName] = append(clist[clientName], nodeNo)
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
					agiZone, err = getString(i["Zone"])
					if err != nil {
						isError = true
						invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "agiZone undefined"))
						log.Printf("[%s] ERROR %s", reqID, "agiZone undefined")
						continue
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
		for name, nodes := range clist {
			nodeNo := strings.Join(nodes, ",")
			switch ntype {
			case "cluster":
				clusterName := name
				a.opts.Cluster.Destroy.Force = true
				a.opts.Cluster.Destroy.ClusterName = TypeClusterName(clusterName)
				a.opts.Cluster.Destroy.Nodes = TypeNodes(nodeNo)
				a.opts.Cluster.Destroy.Parallel = true
				err = a.opts.Cluster.Destroy.Execute(nil)
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
				a.opts.Client.Destroy.Force = true
				a.opts.Client.Destroy.ClientName = TypeClientName(clientName)
				a.opts.Client.Destroy.Machines = TypeMachines(nodeNo)
				a.opts.Client.Destroy.Parallel = true
				err = a.opts.Client.Destroy.Execute(nil)
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
					a.opts.AGI.Destroy.Force = true
					a.opts.AGI.Destroy.ClusterName = TypeClusterName(agiName)
					a.opts.AGI.Destroy.Parallel = true
					err = a.opts.AGI.Destroy.Execute(nil)
				} else {
					agiZone := nodes[0]
					a.opts.AGI.Delete.ClusterName = TypeClusterName(agiName)
					a.opts.AGI.Delete.Force = true
					a.opts.AGI.Delete.Parallel = true
					a.opts.AGI.Delete.Gcp.Zone = agiZone
					err = a.opts.AGI.Delete.Execute(nil)
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

func getString(item interface{}) (string, error) {
	switch i := item.(type) {
	case string:
		return i, nil
	default:
		return "", errors.New("not a string")
	}
}
