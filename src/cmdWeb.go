package main

import (
	"errors"
	"fmt"
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
	"github.com/bestmethod/inslice"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type webCmd struct {
	ListenAddr    string  `long:"listen" description:"listing host:port, or just :port" default:"127.0.0.1:3333"`
	WebRoot       string  `long:"webroot" description:"set the web root that should be served, useful if proxying from eg /aerolab on a webserver" default:"/"`
	WebPath       string  `long:"web-path" hidden:"true"`
	WebNoOverride bool    `long:"web-no-override" hidden:"true"`
	Help          helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	menuItems     []*webui.MenuItem
	commands      []*apiCommand
	commandsIndex map[string]int
	titler        cases.Caser
}

func (c *webCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
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
	http.HandleFunc(c.WebRoot+"api/command", c.command)
	http.HandleFunc(c.WebRoot+"dist/", c.static)
	http.HandleFunc(c.WebRoot+"plugins/", c.static)
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
				for _, choice := range strings.Split(tags.Get("webchoice"), ",") {
					isSelected := false
					if choice == tags.Get("default") {
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
					},
				})
			} else {
				// input item text (possible multiple types)
				textType := tags.Get("webtype")
				if textType == "" {
					textType = "text"
				}
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        textType,
						Default:     tags.Get("default"),
						Description: tags.Get("description"),
					},
				})
			}
		case reflect.Float64:
			fallthrough
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
					Default:     tags.Get("default"),
					Description: tags.Get("description"),
				},
			})
		case reflect.Bool:
			// toggle on-off
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Toggle: true,
				},
				Toggle: webui.FormItemToggle{
					Name:        name,
					Description: tags.Get("description"),
					ID:          "xx" + prefix + "xx" + name,
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
			wf = append(wf, &webui.FormItem{
				Type: webui.FormItemType{
					Input: true,
				},
				Input: webui.FormItemInput{
					Name:        name,
					ID:          "xx" + prefix + "xx" + name,
					Type:        "text",
					Default:     tags.Get("default"),
					Description: tags.Get("description"),
					Tags:        true,
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
						Default:     tags.Get("default"),
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
						Default:     tags.Get("default"),
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
				wf = append(wf, &webui.FormItem{
					Type: webui.FormItemType{
						Input: true,
					},
					Input: webui.FormItemInput{
						Name:        name,
						ID:          "xx" + prefix + "xx" + name,
						Type:        textType,
						Default:     tags.Get("default"),
						Description: tags.Get("description"),
					},
				})
			}
		default:
			return nil, fmt.Errorf("unknown field type (name=%s kind=%s type=%s)", name, kind.String(), commandValue.Field(i).Type().String())
		}
	}
	return wf, nil
	/*
		{
			Type: webui.FormItemType{
				Select: true,
			},
			Select: webui.FormItemSelect{
				Name:        "Operating System",
				Description: "OS to use",
				ID:          "OperatingSystem",
				Items: []*webui.FormItemSelectItem{
					{
						Name:     "ubuntu",
						Value:    "ubuntu",
						Selected: true,
					},
					{
						Name:  "centos",
						Value: "centos",
					},
				},
			},
		},
		{
			Type: webui.FormItemType{
				Select: true,
			},
			Select: webui.FormItemSelect{
				Name:        "OS Version",
				Description: "OS version to use",
				ID:          "OSVersion",
				Multiple:    true,
				Items: []*webui.FormItemSelectItem{
					{
						Name:     "22.04",
						Value:    "22.04",
						Selected: true,
					},
					{
						Name:  "20.04",
						Value: "20.04",
					},
				},
			},
		},
	*/
}

func (c *webCmd) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	log.Println(r.RequestURI)

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
	t, err := template.ParseFS(www, "index.html", "index.js", "index.css")
	if err != nil {
		log.Fatal(err)
	}
	err = t.ExecuteTemplate(w, "main", p)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *webCmd) command(w http.ResponseWriter, r *http.Request) {
	// TODO: handle form
}
