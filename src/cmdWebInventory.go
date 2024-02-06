package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strings"
	"sync"
	"text/template"
	"time"

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
}

func (i *inventoryCache) Start(update func() (bool, error)) error {
	i.update = update
	err := i.run(time.Time{})
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
	out, err := exec.Command("aerolab", "inventory", "list", "-j", "-p").CombinedOutput()
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
					name := ntype.Field(x).Name
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
	c.cache.ilcMutex.RLock()
	defer c.cache.ilcMutex.RUnlock()
	c.cache.RLock()
	defer c.cache.RUnlock()
	json.NewEncoder(w).Encode(c.cache.inv.Volumes)
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
