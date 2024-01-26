package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/jobqueue"
	"github.com/aerospike/aerolab/webui"
	"github.com/bestmethod/inslice"
	"github.com/lithammer/shortuuid"
	flags "github.com/rglonek/jeddevdk-goflags"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type webCmd struct {
	ListenAddr        string        `long:"listen" description:"listing host:port, or just :port" default:"127.0.0.1:3333"`
	WebRoot           string        `long:"webroot" description:"set the web root that should be served, useful if proxying from eg /aerolab on a webserver" default:"/"`
	AbsoluteTimeout   time.Duration `long:"timeout" description:"Absolute timeout to set for command execution" default:"30m"`
	MaxConcurrentJobs int           `long:"max-concurrent-job" description:"Max number of jobs to run concurrently" default:"5"`
	MaxQueuedJobs     int           `long:"max-queued-job" description:"Max number of jobs to queue for execution" default:"10"`
	WebPath           string        `long:"web-path" hidden:"true"`
	WebNoOverride     bool          `long:"web-no-override" hidden:"true"`
	DebugRequests     bool          `long:"debug-requests" hidden:"true"`
	Help              helpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
	menuItems         []*webui.MenuItem
	commands          []*apiCommand
	commandsIndex     map[string]int
	titler            cases.Caser
	joblist           *jobTrack
	jobqueue          *jobqueue.Queue
}

type jobTrack struct {
	sync.Mutex
	j map[string]*exec.Cmd
}

func (j *jobTrack) Add(jobID string, cmd *exec.Cmd) {
	j.Lock()
	defer j.Unlock()
	j.j[jobID] = cmd
}

func (j *jobTrack) Delete(jobID string) {
	j.Lock()
	defer j.Unlock()
	delete(j.j, jobID)
}

func (j *jobTrack) Get(jobID string) *exec.Cmd {
	j.Lock()
	defer j.Unlock()
	answer := j.j[jobID]
	return answer
}

func (j *jobTrack) GetStat() int {
	j.Lock()
	defer j.Unlock()
	count := len(j.j)
	return count
}

func (c *webCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	c.joblist = &jobTrack{
		j: make(map[string]*exec.Cmd),
	}
	c.jobqueue = jobqueue.New(c.MaxConcurrentJobs, c.MaxQueuedJobs)
	go func() {
		var statc, statq, statj int
		for {
			nstatj := c.joblist.GetStat()
			nstatc, nstatq := c.jobqueue.GetSize()
			if nstatc != statc || nstatq != statq || nstatj != statj {
				statc = nstatc
				statq = nstatq
				statj = nstatj
				log.Printf("STAT: queue_active_jobs=%d queued_jobs=%d jobs_tracked=%d", statc, statq, statj)
			}
			time.Sleep(60 * time.Second)
		}
	}()
	c.WebRoot = "/" + strings.Trim(c.WebRoot, "/") + "/"
	if c.WebRoot == "//" {
		c.WebRoot = "/"
	}
	err := c.genMenu()
	if err != nil {
		return err
	}
	if c.WebPath == "" {
		c.WebPath, err = a.aerolabRootDir()
		if err != nil {
			return err
		}
		c.WebPath = filepath.Join(c.WebPath, "www")
	}
	wwwVersion, err := os.ReadFile(filepath.Join(c.WebPath, "version.cfg"))
	if err != nil || strings.Trim(string(wwwVersion), "\r\n\t ") != webuiVersion {
		if c.WebNoOverride {
			log.Print("WARNING: web version mismatch, not overriding")
		} else {
			log.Printf("Installing latest %s", c.WebPath)
			err = os.RemoveAll(c.WebPath)
			if err != nil {
				return err
			}
			err = webui.InstallWebsite(c.WebPath)
			if err != nil {
				return err
			}
		}
	}
	http.HandleFunc(c.WebRoot+"dist/", c.static)
	http.HandleFunc(c.WebRoot+"plugins/", c.static)
	http.HandleFunc(c.WebRoot+"job/", c.jobs)
	http.HandleFunc(c.WebRoot, c.serve)
	if c.WebRoot != "/" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, c.WebRoot, http.StatusTemporaryRedirect)
		})
	}
	log.Printf("Listening on %s", c.ListenAddr)
	return http.ListenAndServe(c.ListenAddr, nil)
}

func (c *webCmd) static(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join(c.WebPath, strings.TrimPrefix(r.URL.Path, c.WebRoot)))
}

func (c *webCmd) jobAction(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	requestID := shortuuid.New()
	log.Printf("[%s] %s:%s", requestID, r.Method, r.RequestURI)
	if c.DebugRequests {
		for k, v := range r.PostForm {
			log.Printf("[%s]    %s=%s", requestID, k, v)
		}
	}

	urlParse := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, c.WebRoot), "/"), "/")
	jobID := urlParse[len(urlParse)-1]
	actions := r.PostForm["action"]
	if len(actions) == 0 {
		actions = []string{"sigint"}
	}
	run := c.joblist.Get(jobID)
	if run == nil {
		http.Error(w, "JobID not found, or job already complete", http.StatusBadRequest)
		return
	}
	if run.ProcessState != nil {
		http.Error(w, "Job already complete", http.StatusBadRequest)
		return
	}
	if run.Process == nil {
		http.Error(w, "Job not running", http.StatusBadRequest)
		return
	}
	var err error
	switch actions[0] {
	case "sigint":
		err = run.Process.Signal(os.Interrupt)
	case "sigkill":
		err = run.Process.Signal(os.Kill)
	default:
		http.Error(w, "abort signal type not supported", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "Failed to send signal to process: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

func (c *webCmd) jobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		c.jobAction(w, r)
		return
	}
	log.Printf("[%s] %s:%s", shortuuid.New(), r.Method, r.RequestURI)

	urlParse := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, c.WebRoot), "/"), "/")
	requestID := urlParse[len(urlParse)-1]

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}
	rootDir, err := a.aerolabRootDir()
	if err != nil {
		http.Error(w, "unable to get aerolab root dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fn := path.Join(rootDir, "weblog", requestID+".log")
	f, err := os.Open(fn)
	if err != nil {
		http.Error(w, "job not found", http.StatusBadRequest)
		return
	}
	defer f.Close()
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	start := time.Now()
	for {
		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, f); err != nil && err != io.EOF {
			w.Write([]byte("ERROR receiving data from subprocess: " + err.Error()))
			break
		}
		data := buf.String()
		w.Write([]byte(data))
		flusher.Flush()
		if strings.Contains(data, "-=-=-=-=- [END] -=-=-=-=-") {
			break
		}
		if time.Since(start) > c.AbsoluteTimeout {
			break
		}
		time.Sleep(time.Second)
	}
}

func (c *webCmd) fillMenu(commandMap map[string]interface{}, titler cases.Caser, commands []*apiCommand, commandsIndex map[string]int, spath string, hiddenItems []string) (ret []*webui.MenuItem) {
	for comm, sub := range commandMap {
		wpath := path.Join(spath, comm)
		isHidden := false
		for _, hiddenItem := range hiddenItems {
			if wpath == hiddenItem || strings.HasPrefix(wpath, hiddenItem+"/") {
				isHidden = true
				break
			}
		}
		if isHidden {
			continue
		}
		name := titler.String(strings.ReplaceAll(comm, "-", " "))
		ret = append(ret, &webui.MenuItem{
			Icon:    commands[commandsIndex[wpath]].icon,
			Name:    name,
			Href:    c.WebRoot + commands[commandsIndex[wpath]].path,
			Tooltip: commands[commandsIndex[wpath]].description,
		})
		if len(sub.(map[string]interface{})) > 0 {
			ret[len(ret)-1].Items = c.fillMenu(sub.(map[string]interface{}), titler, commands, commandsIndex, wpath, hiddenItems)
		}
	}
	return ret
}

func (c *webCmd) sortMenu(items []*webui.MenuItem, commandsIndex map[string]int) {
	sort.Slice(items, func(i, j int) bool {
		return commandsIndex[items[i].Href[len(c.WebRoot):]] < commandsIndex[items[j].Href[len(c.WebRoot):]]
	})
	for i := range items {
		if len(items[i].Items) > 0 {
			c.sortMenu(items[i].Items, commandsIndex)
		}
	}
}

func (c *webCmd) genMenu() error {
	commandMap := make(map[string]interface{})
	commands := []*apiCommand{}
	commandsIndex := make(map[string]int) // map[path]idx <- commands[idx]
	ret := make(chan apiCommand, 1)
	keyField := reflect.ValueOf(a.opts).Elem()
	var hiddenItems []string
	go a.opts.Rest.getCommands(keyField, "", ret, "")
	for {
		val, ok := <-ret
		if !ok {
			break
		}
		if val.isHidden || val.isWebHidden {
			hiddenItems = append(hiddenItems, val.path)
			continue
		}
		if val.pathStack[len(val.pathStack)-1] == "help" {
			continue
		}
		commandsIndex[val.path] = len(commands)
		commands = append(commands, &val)
		cm := commandMap
		for _, pp := range val.pathStack {
			if _, ok := cm[pp]; !ok {
				cm[pp] = make(map[string]interface{})
			}
			cm = cm[pp].(map[string]interface{})
		}
	}
	titler := cases.Title(language.English)
	c.menuItems = c.fillMenu(commandMap, titler, commands, commandsIndex, "", hiddenItems)
	c.sortMenu(c.menuItems, commandsIndex)
	c.commands = commands
	c.commandsIndex = commandsIndex
	c.titler = titler
	return nil
}

func (c *webCmd) getFormItems(urlPath string) ([]*webui.FormItem, error) {
	cindex, ok := c.commandsIndex[strings.TrimPrefix(urlPath, c.WebRoot)]
	if !ok {
		return nil, errors.New("command not found")
	}
	command := c.commands[cindex]
	return c.getFormItemsRecursive(command.Value, "")
}

func (c *webCmd) getFormItemsRecursive(commandValue reflect.Value, prefix string) ([]*webui.FormItem, error) {
	wf := []*webui.FormItem{}
	for i := 0; i < commandValue.Type().NumField(); i++ {
		name := commandValue.Type().Field(i).Name
		kind := commandValue.Field(i).Kind()
		tags := commandValue.Type().Field(i).Tag
		if name[0] < 65 || name[0] > 90 {
			if kind == reflect.Struct {
				wfs, err := c.getFormItemsRecursive(commandValue.Field(i), prefix)
				if err != nil {
					return nil, err
				}
				wf = append(wf, wfs...)
			}
			continue
		}
		switch kind {
		case reflect.String:
			// select items - choice/multichoice
			if tags.Get("webchoice") != "" {
				multi := false
				if tags.Get("webmulti") != "" {
					multi = true
				}
				choices := []*webui.FormItemSelectItem{}
				required := false
				if tags.Get("webrequired") == "true" && commandValue.Field(i).String() == "" {
					required = true
				}
				for _, choice := range strings.Split(tags.Get("webchoice"), ",") {
					isSelected := false
					if choice == commandValue.Field(i).String() {
						isSelected = true
					}
					choices = append(choices, &webui.FormItemSelectItem{
						Name:     choice,
						Value:    choice,
						Selected: isSelected,
					})
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Select: true,
					},
					Select: webui.FormItemSelect{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Description: tags.Get("description"),
						Multiple:    multi,
						Items:       choices,
						Required:    required,
					},
				})
			} else {
				// input item text (possible multiple types)
				textType := tags.Get("webtype")
				if textType == "" {
					textType = "text"
				}
				required := false
				if tags.Get("webrequired") == "true" && commandValue.Field(i).String() == "" {
					required = true
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        textType,
						Default:     commandValue.Field(i).String(),
						Description: tags.Get("description"),
						Required:    required,
					},
				})
			}
		case reflect.Float64:
			// input item number
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Input: true,
				},
				Input: webui.FormItemInput{
					Name:        name,
					ID:          "xx" + prefix + "xx" + name,
					Type:        "number",
					Default:     strconv.FormatFloat(commandValue.Field(i).Float(), 'f', 4, 64),
					Description: tags.Get("description"),
				},
			})
		case reflect.Int:
			// input item number
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Input: true,
				},
				Input: webui.FormItemInput{
					Name:        name,
					ID:          "xx" + prefix + "xx" + name,
					Type:        "number",
					Default:     strconv.Itoa(int(commandValue.Field(i).Int())),
					Description: tags.Get("description"),
				},
			})
		case reflect.Bool:
			// toggle on-off
			paramOn := commandValue.Field(i).Bool()
			paramDisable := false
			descriptionHead := ""
			if tags.Get("webdisable") == "true" {
				paramDisable = true
				descriptionHead = "(disabled for webui) "
			}
			if tags.Get("webset") == "true" {
				paramOn = true
			}
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Toggle: true,
				},
				Toggle: webui.FormItemToggle{
					Name:        name,
					Description: descriptionHead + tags.Get("description"),
					ID:          "xx" + prefix + "xx" + name,
					On:          paramOn,
					Disabled:    paramDisable,
				},
			})
		case reflect.Struct:
			// recursion
			if name == "Help" {
				continue
			}
			if (name == "Aws" && a.opts.Config.Backend.Type == "aws") || (name == "Docker" && a.opts.Config.Backend.Type == "docker") || (name == "Gcp" && a.opts.Config.Backend.Type == "gcp") || (!inslice.HasString([]string{"Aws", "Gcp", "Docker"}, name)) {
				sep := name
				if prefix != "" {
					sep = prefix + "." + name
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Separator: true,
					},
					Separator: webui.FormItemSeparator{
						Name: sep,
					},
				})
				wfs, err := c.getFormItemsRecursive(commandValue.Field(i), sep)
				if err != nil {
					return nil, err
				}
				wf = append(wf, wfs...)
			}
		case reflect.Slice:
			// tag input
			val := []string{}
			for j := 0; j < commandValue.Field(i).Len(); j++ {
				val = append(val, commandValue.Field(i).Index(j).String())
			}
			required := false
			if tags.Get("webrequired") == "true" && len(val) == 0 {
				required = true
			}
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Input: true,
				},
				Input: webui.FormItemInput{
					Name:        name,
					ID:          "xx" + prefix + "xx" + name,
					Type:        "text",
					Default:     strings.Join(val, ","),
					Description: tags.Get("description"),
					Tags:        true,
					Required:    required,
				},
			})
		case reflect.Int64:
			if commandValue.Field(i).Type().String() == "time.Duration" {
				// duration
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        "text",
						Default:     time.Duration(commandValue.Field(i).Int()).String(),
						Description: tags.Get("description"),
					},
				})
			} else if commandValue.Field(i).Type().String() == "int64" {
				// input item number
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        "number",
						Default:     strconv.Itoa(int(commandValue.Field(i).Int())),
						Description: tags.Get("description"),
					},
				})
			} else {
				return nil, fmt.Errorf("unknown field type (name=%s kind=%s type=%s)", name, kind.String(), commandValue.Field(i).Type().String())
			}
		case reflect.Ptr:
			if commandValue.Field(i).Type().String() == "*flags.Filename" {
				// input type text
				textType := tags.Get("webtype")
				if textType == "" {
					textType = "text"
				}
				required := false
				if tags.Get("webrequired") == "true" && commandValue.Field(i).String() == "" {
					required = true
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        textType,
						Default:     commandValue.Field(i).String(),
						Description: tags.Get("description"),
						Required:    required,
					},
				})
			} else {
				return nil, fmt.Errorf("unknown field type (name=%s kind=%s type=%s)", name, kind.String(), commandValue.Field(i).Type().String())
			}
		default:
			return nil, fmt.Errorf("unknown field type (name=%s kind=%s type=%s)", name, kind.String(), commandValue.Field(i).Type().String())
		}
	}
	return wf, nil
}

func (c *webCmd) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// if posting command, run and exit
		c.command(w, r)
		return
	}

	// log method and URI
	log.Printf("[%s] %s:%s", shortuuid.New(), r.Method, r.RequestURI)

	// create webpage
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	title := strings.Trim(strings.TrimPrefix(r.URL.Path, c.WebRoot), "\r\n\t /")
	title = c.titler.String(strings.ReplaceAll(title, "/", " / "))
	title = strings.ReplaceAll(title, "-", " ")

	var errStr string
	var errTitle string
	var isError bool
	formItems, err := c.getFormItems(r.URL.Path)
	if err != nil {
		errStr = err.Error()
		errTitle = "Failed to generate form items"
		isError = true
	}

	p := &webui.Page{
		WebRoot:                                 c.WebRoot,
		FixedNavbar:                             true,
		FixedFooter:                             true,
		PendingActionsShowAllUsersToggle:        true,
		PendingActionsShowAllUsersToggleChecked: false,
		IsError:                                 isError,
		ErrorString:                             errStr,
		ErrorTitle:                              errTitle,
		IsForm:                                  true,
		FormItems:                               formItems,
		FormCommandTitle:                        title,
		Navigation: &webui.Nav{
			Top: []*webui.NavTop{
				{
					Name: "Home",
					Href: c.WebRoot,
				},
				{
					Name: "Logout",
					Href: c.WebRoot + "logout",
				},
			},
		},
		Menu: &webui.MainMenu{
			Items: c.menuItems,
		},
	}
	p.Menu.Items.Set(r.URL.Path)
	www := os.DirFS(c.WebPath)
	t, err := template.ParseFS(www, "index.html", "index.js", "index.css", "highlighter.css", "ansiup.js")
	if err != nil {
		log.Fatal(err)
	}
	err = t.ExecuteTemplate(w, "main", p)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *webCmd) switchName(useShortSwitches bool, tag reflect.StructTag) string {
	if useShortSwitches {
		ret := "-" + tag.Get("short")
		if ret == "-" {
			ret = "--" + tag.Get("long")
		}
		return ret
	}
	return "--" + tag.Get("long")
}

func (c *webCmd) getFieldNames(cmd reflect.Value) []string {
	fnames := []string{}
	for x := 0; x < cmd.NumField(); x++ {
		if !cmd.Type().Field(x).Anonymous {
			fnames = append(fnames, cmd.Type().Field(x).Name)
		} else {
			fnames = append(fnames, c.getFieldNames(cmd.Field(x))...)
		}
	}
	return fnames
}

func (c *webCmd) command(w http.ResponseWriter, r *http.Request) {
	// log method, URI and parameters
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "error parsing form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// log request
	requestID := shortuuid.New()
	log.Printf("[%s] %s:%s", requestID, r.Method, r.RequestURI)
	if c.DebugRequests {
		for k, v := range r.PostForm {
			log.Printf("[%s]    %s=%s", requestID, k, v)
		}
	}

	// get command
	cindex, ok := c.commandsIndex[strings.TrimPrefix(r.URL.Path, c.WebRoot)]
	if !ok {
		http.Error(w, "command not found: "+r.URL.Path, http.StatusBadRequest)
		return
	}
	command := c.commands[cindex].Value
	cjson := make(map[string]interface{})
	cmdline := append([]string{"aerolab"}, c.commands[cindex].pathStack...)
	action := r.PostForm["action"]
	useShortSwitchesTmp := r.PostForm["useShortSwitches"]
	useShortSwitches := false
	if len(useShortSwitchesTmp) > 0 && useShortSwitchesTmp[0] == "true" {
		useShortSwitches = true
	}
	if len(action) != 1 || (action[0] != "show" && action[0] != "run") {
		http.Error(w, "invalid action specification in form", http.StatusBadRequest)
		return
	}

	// sort postForm
	postForm := [][]interface{}{}
	for k, v := range r.PostForm {
		if !strings.HasPrefix(k, "xx") {
			continue
		}
		if len(v) == 0 || (len(v) == 1 && v[0] == "") {
			continue
		}
		postForm = append(postForm, []interface{}{k, v})
	}
	sort.Slice(postForm, func(i, j int) bool {
		ki := postForm[i][0].(string)
		kj := postForm[j][0].(string)
		kiPath := strings.Split(strings.TrimPrefix(ki, "xx"), "xx")
		kjPath := strings.Split(strings.TrimPrefix(kj, "xx"), "xx")
		if len(kiPath) < len(kjPath) {
			return true
		}
		if len(kiPath) > len(kjPath) {
			return false
		}
		for x, pathItem := range kiPath {
			if x == len(kiPath)-1 {
				break
			}
			if pathItem < kjPath[x] {
				return true
			}
			if pathItem > kjPath[x] {
				return false
			}
		}
		cmd := reflect.Indirect(command)
		for i, depth := range kiPath {
			if i == 0 && depth == "" {
				continue
			}
			if i == len(kiPath)-1 {
				break
			}
			cmd = cmd.FieldByName(depth)
		}
		parami := kiPath[len(kiPath)-1]
		paramj := kjPath[len(kjPath)-1]
		indexi := -1
		indexj := -1
		fieldNames := c.getFieldNames(cmd)
		for x, name := range fieldNames {
			if name == parami {
				indexi = x
			}
			if name == paramj {
				indexj = x
			}
			if indexi >= 0 && indexj >= 0 {
				break
			}
		}
		return indexi < indexj
	})

	// fill command struct
	tail := []string{"--"}
	for _, kv := range postForm {
		k := kv[0].(string)
		v := kv[1].([]string)
		cmd := reflect.Indirect(command)
		cj := cjson
		commandPath := strings.Split(strings.TrimPrefix(k, "xx"), "xx")
		for i, depth := range commandPath {
			if i == 0 && depth == "" {
				continue
			}
			if i == len(commandPath)-1 {
				break
			}
			cmd = cmd.FieldByName(depth)
			if _, ok := cj[depth]; !ok {
				cj[depth] = make(map[string]interface{})
			}
			cj = cj[depth].(map[string]interface{})
		}
		param := commandPath[len(commandPath)-1]
		field := cmd.FieldByName(param)
		fieldType, _ := cmd.Type().FieldByName(param)
		tag := fieldType.Tag
		switch field.Kind() {
		case reflect.String:
			if v[0] != field.String() {
				if tag.Get("long") == "" {
					tail = append(tail, "'"+strings.ReplaceAll(v[0], "'", "\\'")+"'")
				} else {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), "'"+strings.ReplaceAll(v[0], "'", "\\'")+"'")
				}
				cj[param] = v[0]
			}
		case reflect.Bool:
			val := false
			if v[0] == "on" {
				val = true
			}
			if val != field.Bool() {
				cj[param] = true
				cmdline = append(cmdline, c.switchName(useShortSwitches, tag))
			}
		case reflect.Int:
			val, err := strconv.Atoi(v[0])
			if err != nil {
				http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
				return
			}
			if val != int(field.Int()) {
				cj[param] = val
				if tag.Get("long") == "" {
					tail = append(tail, v[0])
				} else {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
				}
			}
		case reflect.Float64:
			val, err := strconv.ParseFloat(v[0], 64)
			if err != nil {
				http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
				return
			}
			if val != field.Float() {
				cj[param] = val
				if tag.Get("long") == "" {
					tail = append(tail, v[0])
				} else {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
				}
			}
		case reflect.Slice:
			cj[param] = v
			if tag.Get("long") == "" {
				tail = append(tail, v...)
			} else {
				for _, vv := range v {
					cmdline = append(cmdline, c.switchName(useShortSwitches, tag), vv)
				}
			}
		case reflect.Int64:
			if field.Type().String() == "time.Duration" {
				dur, err := time.ParseDuration(v[0])
				if err != nil {
					http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
					return
				}
				if int64(dur) != field.Int() {
					cj[param] = dur
					if tag.Get("long") == "" {
						tail = append(tail, v[0])
					} else {
						cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
					}
				}
			} else if field.Type().String() == "int64" {
				val, err := strconv.Atoi(v[0])
				if err != nil {
					http.Error(w, fmt.Sprintf("error parsing form item %s: %s", field.Type().Name(), err), http.StatusBadRequest)
					return
				}
				if val != int(field.Int()) {
					cj[param] = val
					if tag.Get("long") == "" {
						tail = append(tail, v[0])
					} else {
						cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
					}
				}
			} else {
				http.Error(w, fmt.Sprintf("field %s not supported", field.Kind().String()), http.StatusBadRequest)
				return
			}
		case reflect.Ptr:
			if field.Type().String() == "*flags.Filename" {
				if v[0] != field.String() {
					cj[param] = v[0]
					if tag.Get("long") == "" {
						tail = append(tail, v[0])
					} else {
						cmdline = append(cmdline, c.switchName(useShortSwitches, tag), v[0])
					}
				}
			} else {
				http.Error(w, fmt.Sprintf("field %s not supported", field.Kind().String()), http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, fmt.Sprintf("field %s not supported", field.Kind().String()), http.StatusBadRequest)
			return
		}
	}
	if action[0] == "show" {
		if len(tail) == 1 {
			json.NewEncoder(w).Encode(cmdline)
		} else {
			json.NewEncoder(w).Encode(append(cmdline, tail...))
		}
		return
	}

	err = c.jobqueue.Add()
	if err != nil {
		http.Error(w, "job queue full", http.StatusNotAcceptable)
		return
	}

	ex, err := os.Executable()
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to get path to aerolab executable: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rootDir, err := a.aerolabRootDir()
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to get aerolab root dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	os.Mkdir(path.Join(rootDir, "weblog"), 0755)
	fn := path.Join(rootDir, "weblog", requestID+".log") //time.Now().Format("2006-01-02_15-04-05_")+requestID+".log")

	ctx, cancel := context.WithTimeout(context.Background(), c.AbsoluteTimeout)
	run := exec.CommandContext(ctx, ex, "webrun")
	stdin, err := run.StdinPipe()
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to get stdin: "+err.Error(), http.StatusInternalServerError)
		cancel()
		return
	}

	f, err := os.Create(fn)
	if err != nil {
		c.jobqueue.Remove()
		http.Error(w, "unable to create logfile: "+err.Error(), http.StatusInternalServerError)
		cancel()
		return
	}
	f.WriteString("-=-=-=-=- [path] /" + strings.TrimPrefix(r.URL.Path, c.WebRoot) + " -=-=-=-=-\n")
	f.WriteString("-=-=-=-=- [cmdline] " + strings.Join(cmdline, " ") + " -=-=-=-=-\n")
	f.WriteString("-=-=-=-=- [command] " + strings.Join(c.commands[cindex].pathStack, " ") + " -=-=-=-=-\n")
	json.NewEncoder(f).Encode(cjson)
	f.WriteString("-=-=-=-=- [Log] -=-=-=-=-\n")
	run.Stderr = f
	run.Stdout = f

	go func() {
		c.jobqueue.Start()
		err = run.Start()
		if err != nil {
			c.jobqueue.End()
			c.jobqueue.Remove()
			stdin.Close()
			f.WriteString("-=-=-=-=- [Subprocess failed to start] -=-=-=-=-\n" + err.Error() + "\n")
			f.Close()
			cancel()
			return
		}
		go func() {
			stdin.Write([]byte(strings.TrimPrefix(r.URL.Path, c.WebRoot) + "-=-=-=-"))
			json.NewEncoder(stdin).Encode(cjson)
			stdin.Close()
		}()

		c.joblist.Add(requestID, run)

		go func(run *exec.Cmd, requestID string) {
			err := run.Wait()
			c.joblist.Delete(requestID)
			if err != nil {
				f.WriteString("\n-=-=-=-=- [ExitCode] " + strconv.Itoa(run.ProcessState.ExitCode()) + " -=-=-=-=-\n" + err.Error() + "\n")
			} else {
				f.WriteString("\n-=-=-=-=- [ExitCode] " + strconv.Itoa(run.ProcessState.ExitCode()) + " -=-=-=-=-\nsuccess\n")
			}
			f.WriteString("-=-=-=-=- [END] -=-=-=-=-")
			f.Close()
			cancel()
			c.jobqueue.End()
			c.jobqueue.Remove()
			if c.commands[cindex].path == "config/defaults" && ((len(r.PostForm["xxxxReset"]) > 0 && r.PostForm["xxxxReset"][0] == "on") || (len(r.PostForm["xxxxValue"]) > 0 && r.PostForm["xxxxValue"][0] != "")) || (c.commands[cindex].path == "config/backend" && len(r.PostForm["xxxxType"]) > 0 && r.PostForm["xxxxType"][0] != "") {
				log.Printf("[%s] Reloading interface defaults", requestID)
				a.opts = new(commands)
				a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)
				a.iniParser = flags.NewIniParser(a.parser)
				a.parseFile()
				a.parser = flags.NewParser(a.opts, flags.HelpFlag|flags.PassDoubleDash)
				for command, switchList := range backendSwitches {
					keys := strings.Split(strings.ToLower(string(command)), ".")
					var nCmd *flags.Command
					for i, key := range keys {
						if i == 0 {
							nCmd = a.parser.Find(key)
						} else {
							nCmd = nCmd.Find(key)
						}
					}
					for backend, switches := range switchList {
						grp, err := nCmd.AddGroup(string(backend), string(backend), switches)
						if err != nil {
							logExit(err)
						}
						if string(backend) != a.opts.Config.Backend.Type {
							grp.Hidden = true
						}
					}
				}
				a.iniParser = flags.NewIniParser(a.parser)
				a.early = true
				a.parseArgs(os.Args[1:])
				a.parseFile()
				c.genMenu()
				log.Printf("[%s] Reloaded interface defaults", requestID)
			}
		}(run, requestID)
	}()
	w.Write([]byte(requestID))
}
