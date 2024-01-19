package main

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/aerospike/aerolab/webui"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type webCmd struct {
	ListenAddr string `long:"listen" description:"listing host:port, or just :port" default:"0.0.0.0:3333"`
	// TODO: TLS+letsencrypt
	// TODO: google sso auth
	// TODO: page settings (all users/not all/ etc)
	WebPath       string  `long:"web-path" hidden:"true"`
	WebNoOverride bool    `long:"web-no-override" hidden:"true"`
	Help          helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	menuItems     []*webui.MenuItem
}

func (c *webCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
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
	http.Handle("/dist/", http.FileServer(http.Dir(c.WebPath)))
	http.Handle("/plugins/", http.FileServer(http.Dir(c.WebPath)))
	http.HandleFunc("/", c.serve)
	log.Printf("Listening on %s", c.ListenAddr)
	return http.ListenAndServe(c.ListenAddr, nil)
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
			Href:    "/" + commands[commandsIndex[wpath]].path,
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
		return commandsIndex[items[i].Href[1:]] < commandsIndex[items[j].Href[1:]]
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
		if val.isHidden {
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
	return nil
}

func (c *webCmd) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	log.Println(r.RequestURI)
	p := &webui.Page{
		FixedNavbar:                             true,
		FixedFooter:                             true,
		PendingActionsShowAllUsersToggle:        true,
		PendingActionsShowAllUsersToggleChecked: false,
		Navigation: &webui.Nav{
			Top: []*webui.NavTop{
				{
					Name: "Home",
					Href: "/",
				},
				{
					Name: "Logout",
					Href: "/logout",
				},
			},
		},
		Menu: &webui.MainMenu{
			Items: c.menuItems,
		},
	}
	p.Menu.Items.Set(r.URL.Path)
	www := os.DirFS(c.WebPath)
	t, err := template.ParseFS(www, "index.html", "index.js", "index.css")
	if err != nil {
		log.Fatal(err)
	}
	err = t.ExecuteTemplate(w, "main", p)
	if err != nil {
		log.Fatal(err)
	}
}
