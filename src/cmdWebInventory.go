package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/webui"
	"github.com/bestmethod/inslice"
	"github.com/gabemarshall/pty"
	"github.com/gorilla/websocket"
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
	agiStatusLock   *sync.Mutex
	c               *webCmd
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

var webuiInventoryListParams = []string{"inventory", "list", "-j", "-p", "--webui"}

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
	out, err := exec.Command(i.executable, webuiInventoryListParams...).CombinedOutput()
	if err != nil {
		if isUpdated {
			i.Lock()
			i.inv = &inventoryJson{}
			i.Unlock()
		}
		return fmt.Errorf("data.Get(1):%s: %s", err, string(out))
	}
	inv := &inventoryJson{}
	err = json.Unmarshal(out, inv)
	if err != nil {
		return fmt.Errorf("json.Unmarshal:1:%s:%s", err, string(out))
	}
	for i := range inv.Clusters {
		if strings.ToLower(inv.Clusters[i].State) == "running" || strings.HasPrefix(inv.Clusters[i].State, "Up_") {
			inv.Clusters[i].IsRunning = true
		}
		expDate, err := time.Parse(time.RFC3339, inv.Clusters[i].Expires)
		if err == nil {
			inv.Clusters[i].Expires = time.Until(expDate).Truncate(time.Second).String()
		}
	}
	for i := range inv.Clients {
		if strings.ToLower(inv.Clients[i].State) == "running" || strings.HasPrefix(inv.Clients[i].State, "Up_") {
			inv.Clients[i].IsRunning = true
		}
		expDate, err := time.Parse(time.RFC3339, inv.Clients[i].Expires)
		if err == nil {
			inv.Clients[i].Expires = time.Until(expDate).Truncate(time.Second).String()
		}
	}
	for i := range inv.AGI {
		if strings.ToLower(inv.AGI[i].State) == "running" || strings.HasPrefix(inv.AGI[i].State, "Up_") {
			inv.AGI[i].IsRunning = true
		}
		expDate, err := time.Parse(time.RFC3339, inv.AGI[i].Expires)
		if err == nil {
			inv.AGI[i].Expires = time.Until(expDate).Truncate(time.Second).String()
		}
	}
	agiList := []*agiWebTokenRequest{}
	for _, i := range inv.AGI {
		if !i.IsRunning {
			continue
		}
		apipsplit := strings.Split(i.AccessURL, "/")
		accessProtIP := ""
		if len(apipsplit) >= 3 {
			accessProtIP = strings.Join(apipsplit[0:3], "/")
		}
		if a.opts.Config.Backend.Type != "docker" || accessProtIP == "" {
			if i.PublicIP != "" {
				accessProtIP = i.AccessProtocol + i.PublicIP
			} else if i.PrivateIP != "" {
				accessProtIP = i.AccessProtocol + i.PrivateIP
			}
		}
		agiList = append(agiList, &agiWebTokenRequest{
			Name:         i.Name,
			PublicIP:     i.PublicIP,
			PrivateIP:    i.PrivateIP,
			AccessProtIP: accessProtIP,
			InstanceID:   i.InstanceID,
		})
	}
	i.Lock()
	i.inv = inv
	go i.asyncGetAGIStatus(agiList)
	i.Unlock()
	return nil
}

func (i *inventoryCache) asyncGetAGIStatus(agiList []*agiWebTokenRequest) {
	// only one run at a time, abandon if previous is still running
	if !i.agiStatusLock.TryLock() {
		return
	}
	defer i.agiStatusLock.Unlock()
	threads := make(chan struct{}, 20)
	wg := new(sync.WaitGroup)
	for _, agis := range agiList {
		threads <- struct{}{}
		wg.Add(1)
		go func(agis *agiWebTokenRequest) {
			defer func() {
				<-threads
				wg.Done()
			}()
			ip := agis.AccessProtIP
			token, err := i.c.agiTokens.GetToken(*agis)
			if err != nil {
				if i.c.DebugRequests {
					log.Printf("error getting token for %s with ip %s: %s", agis.Name, ip, err)
				}
				return
			}
			tr := &http.Transport{
				DisableKeepAlives: true,
				IdleConnTimeout:   10 * time.Second,
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			}
			client := &http.Client{
				Timeout:   10 * time.Second,
				Transport: tr,
			}
			defer client.CloseIdleConnections()
			req, err := http.NewRequest("GET", ip+"/agi/status", nil)
			if err != nil {
				if i.c.DebugRequests {
					log.Printf("error getting agi status request for %s with ip %s: %s", agis.Name, ip, err)
				}
				return
			}
			req.AddCookie(&http.Cookie{Name: "AGI_TOKEN", Value: token})
			req.AddCookie(&http.Cookie{Name: "X-AGI-CALLER", Value: "webui"})
			response, err := client.Do(req)
			if err != nil {
				if i.c.DebugRequests {
					log.Printf("error getting agi status for %s with ip %s: %s", agis.Name, ip, err)
				}
				return
			}
			if response.StatusCode == http.StatusUnauthorized {
				if i.c.DebugRequests {
					log.Printf("error getting agi status for %s with ip %s: %s", agis.Name, ip, "got unauthorized, attempting to invalidate token and try again")
				}
				token, err = i.c.agiTokens.GetNewToken(*agis)
				if err != nil {
					if i.c.DebugRequests {
						log.Printf("error getting new token for %s with ip %s: %s", agis.Name, ip, err)
					}
					return
				}
				req, err = http.NewRequest("GET", ip+"/agi/status", nil)
				if err != nil {
					if i.c.DebugRequests {
						log.Printf("error getting agi status request for %s with ip %s: %s", agis.Name, ip, err)
					}
					return
				}
				req.AddCookie(&http.Cookie{Name: "AGI_TOKEN", Value: token})
				req.AddCookie(&http.Cookie{Name: "X-AGI-CALLER", Value: "webui"})
				response, err = client.Do(req)
				if err != nil {
					if i.c.DebugRequests {
						log.Printf("error getting agi status for %s with ip %s: %s", agis.Name, ip, err)
					}
					return
				}
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				if i.c.DebugRequests {
					body, _ := io.ReadAll(response.Body)
					err = fmt.Errorf("exit code (%d), message: %s", response.StatusCode, string(body))
					log.Printf("error getting agi status for %s with ip %s: %s", agis.Name, ip, err)
				}
				return
			}
			statusMsg := "unknown"
			clusterStatus := &ingest.IngestStatusStruct{}
			respb, err := io.ReadAll(response.Body)
			if err != nil {
				if i.c.DebugRequests {
					log.Printf("Failed to read response body of 200: %s", err)
				}
			} else {
				err = json.Unmarshal(respb, clusterStatus)
				if err == nil {
					hasErrors := false
					if len(clusterStatus.Ingest.Errors) > 0 {
						hasErrors = true
					}
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
						if hasErrors {
							statusMsg = "READY, HasErrors"
						}
					}
					if !strings.HasPrefix(statusMsg, "READY") && !clusterStatus.Ingest.Running {
						statusMsg = "ERR: INGEST DOWN"
					}
				} else if i.c.DebugRequests {
					log.Printf("json.Unmarshal:2:%s:%s", err, string(respb))
					log.Printf("curl -k -X GET --cookie=\"AGI_TOKEN=%s\" %s", token, ip+"/agi/status")
				}
			}
			i.Lock()
			for ni := range i.inv.AGI {
				if i.inv.AGI[ni].Name == agis.Name {
					i.inv.AGI[ni].Status = statusMsg
					break
				}
			}
			i.Unlock()
		}(agis)
	}
	wg.Wait()
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
		BetaTag:                                 isWebuiBeta,
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
	http.HandleFunc(c.WebRoot+"www/api/inventory/cluster/ws", c.inventoryClusterWs)
	http.HandleFunc(c.WebRoot+"www/api/inventory/client/ws", c.inventoryClientWs)
	http.HandleFunc(c.WebRoot+"www/api/inventory/cluster/connect", c.inventoryClusterConnect)
	http.HandleFunc(c.WebRoot+"www/api/inventory/client/connect", c.inventoryClientConnect)
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
		itypes := strings.Split(cluster.InstanceType, "/")
		cluster.InstanceType = itypes[len(itypes)-1]
		clusters = append(clusters, cluster)
	}
	json.NewEncoder(w).Encode(clusters)
}

func (c *webCmd) inventoryClients(w http.ResponseWriter, r *http.Request) {
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	clients := []inventoryClient{}
	for _, client := range c.cache.inv.Clients {
		itypes := strings.Split(client.InstanceType, "/")
		client.InstanceType = itypes[len(itypes)-1]
		clients = append(clients, client)
	}
	json.NewEncoder(w).Encode(clients)
}

func (c *webCmd) inventoryAGIConnect(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	req := agiWebTokenRequest{}
	c.cache.ilcMutex.RLock()
	c.cache.RLock()
	for _, agi := range c.cache.inv.AGI {
		if agi.Name == name {
			req = agiWebTokenRequest{
				Name:       agi.Name,
				PublicIP:   agi.PublicIP,
				PrivateIP:  agi.PrivateIP,
				InstanceID: agi.InstanceID,
			}
			break
		}
	}
	c.cache.RUnlock()
	c.cache.ilcMutex.RUnlock()
	token, err := c.agiTokens.GetToken(req)
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
	for _, i := range c.cache.inv.AGI {
		itypes := strings.Split(i.InstanceType, "/")
		i.InstanceType = itypes[len(itypes)-1]
		inv = append(inv, i)
	}
	c.cache.RUnlock()
	c.cache.ilcMutex.RUnlock()
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
	c.inventoryNodesActionDo(w, reqID, data, action)
}

func (c *webCmd) inventoryNodesActionDo(w http.ResponseWriter, reqID string, data map[string]interface{}, action string) {
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
		if ntype == "cluster" || ntype == "agi" {
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
					zoneclist[zone][clusterName] = append(zoneclist[zone][clusterName], nodeNo)
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
					zoneclist[zone][clientName] = append(zoneclist[zone][clientName], nodeNo)
				case "agi":
					agiName := ""
					agiName, err = getString(i["Name"])
					if err != nil || agiName == "" {
						isError = true
						invlog.WriteString(fmt.Sprintf("[%s] ERROR %s\n", reqID, "agiName undefined"))
						log.Printf("[%s] ERROR %s", reqID, "agiName undefined")
						continue
					}
					agiInstanceExists := ""
					agiInstanceExists, _ = getString(i["InstanceType"])
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
					clist[agiName] = append(clist[agiName], agiZone, agiInstanceExists)
					if _, ok := zoneclist[agiZone]; !ok {
						zoneclist[agiZone] = make(map[string][]string)
					}
					if _, ok := zoneclist[agiZone][agiName]; !ok {
						zoneclist[agiZone][agiName] = []string{}
					}
					zoneclist[agiZone][agiName] = append(zoneclist[agiZone][agiName], "1")
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
			hasError := c.inventoryNodesActionStart(reqID, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "stop":
			hasError := c.inventoryNodesActionStop(reqID, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeStart":
			hasError := c.inventoryNodesActionAerospikeStart(reqID, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeStop":
			hasError := c.inventoryNodesActionAerospikeStop(reqID, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeRestart":
			hasError := c.inventoryNodesActionAerospikeRestart(reqID, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "aerospikeStatus":
			hasError := c.inventoryNodesActionAerospikeStatus(reqID, invlog, clist, ntype)
			if hasError {
				isError = true
			}
		case "extendExpiry":
			hasError := c.inventoryNodesActionExtendExpiry(reqID, invlog, clist, ntype, data["expiry"].(string), zoneclist)
			if hasError {
				isError = true
			}
		case "delete":
			fallthrough
		case "destroy":
			hasError := c.inventoryNodesActionDestroy(reqID, action, invlog, clist, ntype)
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

func (c *webCmd) inventoryNodesActionStart(reqID string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
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
			ncmd := "/agi/start"
			if len(nodes) > 1 && nodes[1] == "" {
				ncmd = "/agi/create"
				cmdJson["Gcp"] = map[string]interface{}{
					"Zone":    nodes[0],
					"WithVol": true,
				}
				cmdJson["Aws"] = map[string]interface{}{
					"WithEFS": true,
				}
			}
			err = c.runInvCmd(reqID, ncmd, cmdJson, invlog)
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

func (c *webCmd) inventoryNodesActionStop(reqID string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
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

func (c *webCmd) inventoryNodesActionAerospikeStart(reqID string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
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

func (c *webCmd) inventoryNodesActionAerospikeStop(reqID string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
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

func (c *webCmd) inventoryNodesActionAerospikeRestart(reqID string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
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

func (c *webCmd) inventoryNodesActionAerospikeStatus(reqID string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
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

func (c *webCmd) inventoryNodesActionDestroy(reqID string, action string, invlog *os.File, clist map[string][]string, ntype string) (isError bool) {
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

func (c *webCmd) inventoryNodesActionExtendExpiry(reqID string, invlog *os.File, clist map[string][]string, ntype string, expStr string, zones map[string]map[string][]string) (isError bool) {
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
					"Expires":     int64(expiry),
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
					"Expires":     int64(expiry),
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
				clusterName := name
				cmdJson := map[string]interface{}{
					"ClusterName": clusterName,
					"Nodes":       "1",
					"Expires":     int64(expiry),
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

func (c *webCmd) inventoryClusterConnect(w http.ResponseWriter, r *http.Request) {
	c.inventoryClusterClientConnect(w, r, "cluster")
}

func (c *webCmd) inventoryClientConnect(w http.ResponseWriter, r *http.Request) {
	c.inventoryClusterClientConnect(w, r, "client")
}

func (c *webCmd) inventoryClusterWs(w http.ResponseWriter, r *http.Request) {
	c.inventoryClusterClientWs(w, r, "cluster")
}

func (c *webCmd) inventoryClientWs(w http.ResponseWriter, r *http.Request) {
	c.inventoryClusterClientWs(w, r, "client")
}

type windowSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
	X    uint16
	Y    uint16
}

type wsCounters struct {
	sync.RWMutex
	cmdReaders      int
	connections     int
	lastCmdReaders  int
	lastConnections int
}

func (ws *wsCounters) Print() {
	ws.RLock()
	defer ws.RUnlock()
	if ws.cmdReaders != ws.lastCmdReaders || ws.connections != ws.lastConnections {
		log.Printf("WebSocket stat: local-processes:%d ws-connections:%d", ws.cmdReaders, ws.connections)
		ws.lastCmdReaders = ws.cmdReaders
		ws.lastConnections = ws.connections
	}
}

func (ws *wsCounters) PrintTimer(interval time.Duration) {
	for {
		ws.Print()
		time.Sleep(interval)
	}
}

func (c *webCmd) inventoryClusterClientWs(w http.ResponseWriter, r *http.Request, target string) {
	r.ParseForm()
	name := r.FormValue("name")
	node := r.FormValue("node")
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Unable to upgrade connection to websocket: %s", err)
		return
	}
	ex, err := os.Executable()
	if err != nil {
		log.Printf("Unable to get self: %s", err)
		conn.WriteMessage(websocket.TextMessage, []byte(err.Error()))
		return
	}
	attachCmd := "shell"
	if target == "client" {
		attachCmd = "client"
	}
	cmd := exec.Command(ex, "attach", attachCmd, "-n", name, "-l", node)
	cmd.Env = append(os.Environ(), "TERM=xterm")
	tty, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Unable to start pty/cmd: %s", err)
		conn.WriteMessage(websocket.TextMessage, []byte(err.Error()))
		return
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Process.Wait()
		tty.Close()
		conn.Close()
	}()

	isExit := make(chan struct{}, 2)
	go func() {
		c.wsCount.Lock()
		c.wsCount.cmdReaders++
		c.wsCount.Unlock()
		defer func() {
			c.wsCount.Lock()
			c.wsCount.cmdReaders--
			c.wsCount.Unlock()
		}()
		firstRead := true
		for {
			buf := make([]byte, 1024)
			read, err := tty.Read(buf)
			if err != nil {
				if firstRead {
					conn.WriteMessage(websocket.TextMessage, []byte(err.Error()))
					log.Printf("Unable to read from pty/cmd: %s", err)
				}
				isExit <- struct{}{}
				cmd.Process.Kill()
				conn.Close()
				return
			}
			conn.WriteMessage(websocket.BinaryMessage, buf[:read])
			firstRead = false
		}
	}()

	c.wsCount.Lock()
	c.wsCount.connections++
	c.wsCount.Unlock()
	defer func() {
		c.wsCount.Lock()
		c.wsCount.connections--
		c.wsCount.Unlock()
	}()
	for {
		if len(isExit) > 0 {
			close(isExit)
			log.Print("Exiting")
			return
		}
		messageType, reader, err := conn.NextReader()
		if err != nil {
			if len(isExit) == 0 && !strings.Contains(err.Error(), "(going away)") {
				log.Printf("Unable to grab next reader: %s", err)
			}
			return
		}

		if messageType == websocket.TextMessage {
			log.Print("Unexpected text message")
			conn.WriteMessage(websocket.TextMessage, []byte("Unexpected text message"))
			continue
		}

		dataTypeBuf := make([]byte, 1)
		read, err := reader.Read(dataTypeBuf)
		if err != nil {
			log.Printf("Unable to read message type from reader: %s", err)
			conn.WriteMessage(websocket.TextMessage, []byte("Unable to read message type from reader"))
			return
		}

		if read != 1 {
			log.Printf("Unexpected number of bytes read: %d", read)
			return
		}

		switch dataTypeBuf[0] {
		case 0:
			copied, err := io.Copy(tty, reader)
			if err != nil {
				log.Printf("Error after copying %d bytes: %s", copied, err)
			}
		case 1:
			decoder := json.NewDecoder(reader)
			resizeMessage := windowSize{}
			err := decoder.Decode(&resizeMessage)
			if err != nil {
				conn.WriteMessage(websocket.TextMessage, []byte("Error decoding resize message: "+err.Error()))
				continue
			}
			// log.Printf("resizeMessage: %v", resizeMessage) // used for debugging only
			errno := sysResizeWindow(tty, resizeMessage)
			if errno != 0 {
				log.Printf("Cannot resize terminal: syscall Error: %d", errno)
			}
		default:
			log.Printf("Unknown data type: %v", dataTypeBuf[0])
		}
	}
}

func (c *webCmd) inventoryClusterClientConnect(w http.ResponseWriter, r *http.Request, target string) {
	r.ParseForm()
	name := r.FormValue("name")
	node := r.FormValue("node")
	wPty := ""
	if runtime.GOOS == "windows" {
		buildNo, err := getWindowsBuild()
		if err == nil {
			wPty = `windowsPty: {
			backend: 'conpty',
			buildNumber: ` + buildNo + `,
		},
			fontFamily: 'monospace',
			fontWeight: 400,
			fontWeightBold: 600,
		`
		} else {
			log.Printf("could not get windows build: %s", err)
		}
	}
	w.Write([]byte(fmt.Sprintf(xterm, c.WebRoot, c.WebRoot, c.WebRoot, c.WebRoot, c.WebRoot, target, name, node, wPty)))
}

var xterm = `<!doctype html>
<html>
  <head>
	<link rel="stylesheet" href="%swww/plugins/xtermjs/xterm.css" />
	<script src="%swww/plugins/xtermjs/xterm.js"></script>
	<script src="%swww/plugins/xtermjs/addon-attach.js"></script>
	<script src="%swww/plugins/xtermjs/addon-fit.js"></script>
	<style>
	html, body {
		margin:0px;
		height:100vh;
		width:100vw;
	  }
	  .terminal {
		background:black;
		height:100vh;
		width:100vw;
	  }
	</style>
  </head>
  <body>
	<div id="terminal"></div>
	<script>
	var prot = "ws://";
	if (window.location.protocol == "https:") {
		prot = "wss://";
	}
	var websocket = new WebSocket(prot + window.location.hostname + ":" + window.location.port + "%swww/api/inventory/%s/ws?name=%s&node=%s");
	websocket.binaryType = "arraybuffer";
	function ab2str(buf) {
		return String.fromCharCode.apply(null, new Uint8Array(buf));
	}
	websocket.onopen = function(evt) {
		term = new Terminal({
			cursorBlink: true,
			%s
		});
		var fitAddon = new FitAddon.FitAddon();
		term.loadAddon(fitAddon);

		term.onData(function(data) {
			websocket.send(new TextEncoder().encode("\x00" + data));
		});

		term.onResize(function(evt) {
			websocket.send(new TextEncoder().encode("\x01" + JSON.stringify({cols: evt.cols, rows: evt.rows})))
		});

		term.onTitleChange(function(title) {
			document.title = title;
		});

		term.open(document.getElementById('terminal'));
			fitAddon.fit();
			websocket.onmessage = function(evt) {
			if (evt.data instanceof ArrayBuffer) {
				term.write(ab2str(evt.data));
			} else {
				alert(evt.data)
			}
		}

		window.addEventListener('resize', function (e) {
			fitAddon.fit();
		})

		websocket.onclose = function(evt) {
			term.write("Session terminated");
			term.destroy();
		}

		websocket.onerror = function(evt) {
			if (typeof console.log == "function") {
				alert(evt);
			}
		}
	}
	</script>
  </body>
</html>`
