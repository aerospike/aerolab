package confeditor

// TODO FUTURE: move to https://github.com/charmbracelet/bubbletea

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jroimartin/gocui"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

type menuItem struct {
	Type     int
	Item     int
	Label    string
	Selected bool
	Children []menuItem
}

const (
	typeMenuItemText     = 1
	typeMenuItemRadio    = 2
	typeMenuItemCheckbox = 3
	typeMenuItemEmpty    = 4
)

const (
	itemRackAwareness                = 1
	itemStrongConsistency            = 2
	itemStorageEngineMemory          = 3
	itemStorageDisk                  = 4
	itemStorageEngineDeviceAndMemory = 5
	itemStorageEngineEncryption      = 6
	itemLoggingDestinationFile       = 7
	itemLoggingDestinationCOnsole    = 8
	itemLoggingLevelInfo             = 9
	itemLoggingLevelDebug            = 10
	itemLoggingLevelDetail           = 11
	itemTlsEnabled                   = 12
	itemTlsService                   = 13
	itemTlsFabric                    = 14
	itemSecurityOff                  = 15
	itemSecurityOnBasic              = 16
	itemSecurityOnLdap               = 17
	itemSecurityLoggingDetail        = 18
	itemStorageEngineCompression     = 19
	itemStorageEngineAllFlash        = 20
)

var menuItems = []menuItem{}

func drawMenuItems(v *gocui.View, items []menuItem, linePadding int, depth int) {
	for _, item := range items {
		if item.Type == typeMenuItemEmpty {
			fmt.Fprint(v, "\n")
			continue
		}
		line := item.Label
		if item.Type == typeMenuItemCheckbox && item.Selected {
			line = "[x] " + line
		} else if item.Type == typeMenuItemCheckbox && !item.Selected {
			line = "[ ] " + line
		} else if item.Type == typeMenuItemRadio && item.Selected {
			line = "(*) " + line
		} else if item.Type == typeMenuItemRadio && !item.Selected {
			line = "( ) " + line
		}
		p := depth * 2
		d := ""
		for len(d) < p {
			d = d + " "
		}
		line = d + line
		for len(line) < linePadding {
			line = line + " "
		}
		fmt.Fprintln(v, line)
		drawMenuItems(v, item.Children, linePadding, depth+1)
	}
}

func fillMenuItems() {
	menuItems = []menuItem{
		menuItem{
			Type:  typeMenuItemText,
			Label: "namespace",
			Children: []menuItem{
				menuItem{
					Type:  typeMenuItemCheckbox,
					Label: "rack awareness",
					Item:  itemRackAwareness,
				},
				menuItem{
					Type:  typeMenuItemCheckbox,
					Label: "strong consistency mode",
					Item:  itemStrongConsistency,
				},
				menuItem{
					Type:  typeMenuItemCheckbox,
					Label: "all-flash index on disk",
					Item:  itemStorageEngineAllFlash,
				},
			},
		},
		menuItem{
			Type: typeMenuItemEmpty,
		},
		menuItem{
			Type:  typeMenuItemText,
			Label: "namespace storage engine",
			Children: []menuItem{
				menuItem{
					Type:  typeMenuItemRadio,
					Label: "memory",
					Item:  itemStorageEngineMemory,
				},
				menuItem{
					Type:  typeMenuItemRadio,
					Label: "device (file)",
					Item:  itemStorageDisk,
					Children: []menuItem{
						menuItem{
							Type:  typeMenuItemCheckbox,
							Label: "data also in memory",
							Item:  itemStorageEngineDeviceAndMemory,
						},
						menuItem{
							Type:  typeMenuItemCheckbox,
							Label: "encryption at rest",
							Item:  itemStorageEngineEncryption,
						},
						menuItem{
							Type:  typeMenuItemCheckbox,
							Label: "compression",
							Item:  itemStorageEngineCompression,
						},
					},
				},
			},
		},
		menuItem{
			Type: typeMenuItemEmpty,
		},
		menuItem{
			Type:  typeMenuItemText,
			Label: "logging",
			Children: []menuItem{
				menuItem{
					Type:  typeMenuItemText,
					Label: "destination",
					Children: []menuItem{
						menuItem{
							Type:  typeMenuItemRadio,
							Label: "file",
							Item:  itemLoggingDestinationFile,
						},
						menuItem{
							Type:  typeMenuItemRadio,
							Label: "console",
							Item:  itemLoggingDestinationCOnsole,
						},
					},
				},
				menuItem{
					Type:  typeMenuItemText,
					Label: "level",
					Children: []menuItem{
						menuItem{
							Type:  typeMenuItemRadio,
							Label: "info",
							Item:  itemLoggingLevelInfo,
						},
						menuItem{
							Type:  typeMenuItemRadio,
							Label: "debug",
							Item:  itemLoggingLevelDebug,
						},
						menuItem{
							Type:  typeMenuItemRadio,
							Label: "detail",
							Item:  itemLoggingLevelDetail,
						},
					},
				},
			},
		},
		menuItem{
			Type: typeMenuItemEmpty,
		},
		menuItem{
			Type:  typeMenuItemText,
			Label: "network - tls",
			Children: []menuItem{
				menuItem{
					Type:  typeMenuItemCheckbox,
					Label: "enabled",
					Item:  itemTlsEnabled,
					Children: []menuItem{
						menuItem{
							Type:  typeMenuItemCheckbox,
							Label: "service port",
							Item:  itemTlsService,
						},
						menuItem{
							Type:  typeMenuItemCheckbox,
							Label: "fabric port",
							Item:  itemTlsFabric,
						},
					},
				},
			},
		},
		menuItem{
			Type: typeMenuItemEmpty,
		},
		menuItem{
			Type:  typeMenuItemText,
			Label: "security",
			Children: []menuItem{
				menuItem{
					Type:  typeMenuItemRadio,
					Label: "disabled",
					Item:  itemSecurityOff,
				},
				menuItem{
					Type:  typeMenuItemRadio,
					Label: "enabled - builtin",
					Item:  itemSecurityOnBasic,
				},
				menuItem{
					Type:  typeMenuItemRadio,
					Label: "enabled - ldap",
					Item:  itemSecurityOnLdap,
				},
			},
		},
		menuItem{
			Type: typeMenuItemEmpty,
		},
		menuItem{
			Type:  typeMenuItemText,
			Label: "security logging",
			Children: []menuItem{
				menuItem{
					Type:  typeMenuItemCheckbox,
					Label: "detail level",
					Item:  itemSecurityLoggingDetail,
				},
			},
		},
	}
}

type Editor struct {
	Path       string
	Colors     bool
	loaded     bool
	View       string
	g          *gocui.Gui
	ynQuestion string
	action     int
	showyn     bool
	isUiInit   bool
	uiLoc      int
	confView   *gocui.View
	uiView     *gocui.View
}

const (
	actionSave     = 1
	actionQuit     = 2
	actionSaveQuit = 3
)

func (e *Editor) Run() error {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return err
	}
	defer g.Close()
	e.g = g

	e.View = "ui"
	g.SetManagerFunc(e.layout)
	g.Cursor = false

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, e.quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyCtrlX, gocui.ModNone, e.saveQuit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyCtrlS, gocui.ModNone, e.save); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, e.switchView); err != nil {
		return err
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}

func (e *Editor) layout(g *gocui.Gui) error {
	err := e.viewConfFile(g)
	if err != nil {
		return err
	}
	err = e.viewHelpBar(g)
	if err != nil {
		return err
	}
	err = e.viewUi(g)
	if err != nil {
		return err
	}
	if e.showyn {
		err = e.viewAreYouSure(g)
		if err != nil {
			return err
		}
	}
	if _, err := g.SetCurrentView(e.View); err != nil {
		return err
	}
	return nil
}

func (e *Editor) viewConfFile(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("conf", maxX/2, 0, maxX-1, maxY-4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		e.confView = v
		v.Editable = true
		v.Wrap = true
		v.Editor = gocui.DefaultEditor
		if e.Colors {
			v.BgColor = gocui.ColorGreen
		}
		v.Title = "<aerospike.conf>"
		if !e.loaded {
			if _, err := os.Stat(e.Path); err != nil {
				fmt.Fprint(v, loadDefaultAerospikeConfig())
			} else {
				c, err := os.ReadFile(e.Path)
				if err != nil {
					return err
				}
				fmt.Fprint(v, string(c))
			}
			e.loaded = true
		}
	}
	return nil
}

func (e *Editor) viewUi(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("ui", 0, 0, maxX/2-1, maxY-4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		if !e.isUiInit {
			e.isUiInit = true
			err = e.parseConfToUi(g)
			if err != nil {
				e.View = "conf"
				return err
			}
		}
		e.uiView = v
		v.Editable = true
		v.Wrap = true
		v.Highlight = true
		v.SelBgColor = gocui.ColorWhite
		v.SelFgColor = gocui.ColorBlack
		v.Editor = gocui.EditorFunc(e.ui)
		v.Title = "< Configurator >"
	}
	return nil
}

func (e *Editor) viewHelpBar(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("help", 0, maxY-3, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		if e.Colors {
			v.BgColor = gocui.ColorYellow
		}
		fmt.Fprint(v, "<TAB> switch between views | <CTRL+c> discard and quit | <CTRL+x> save and quit | <CTRL+s> save | UI: <space/enter> select object")
	}
	return nil
}

func (e *Editor) viewAreYouSure(g *gocui.Gui) error {
	if e.ynQuestion == "" {
		e.showyn = false
		e.View = "ui"
		e.g.Cursor = true
		g.DeleteView("sure")
		switch e.action {
		case actionQuit:
			return gocui.ErrQuit
		case actionSave:
			return e.saveFile()
		case actionSaveQuit:
			err := e.saveFile()
			if err != nil {
				return err
			}
			return gocui.ErrQuit
		}
		return nil
	}
	maxX, maxY := g.Size()
	if v, err := g.SetView("sure", maxX/2-len(e.ynQuestion)/2-1, maxY/2-1, maxX/2+len(e.ynQuestion)/2+1, maxY/2+1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		fmt.Fprintln(v, e.ynQuestion)
		e.View = "sure"
		v.Editable = true
		if e.Colors {
			v.BgColor = gocui.ColorRed
		}
		v.Editor = gocui.EditorFunc(e.yn)
		g.Cursor = false
	}
	return nil
}

func (e *Editor) quit(g *gocui.Gui, v *gocui.View) error {
	if e.ynQuestion == "" {
		e.ynQuestion = "Quit without Saving (y/n)?"
		e.action = actionQuit
		e.showyn = true
	}
	return nil
}

func (e *Editor) save(g *gocui.Gui, v *gocui.View) error {
	if e.ynQuestion == "" {
		e.ynQuestion = "Save File (y/n)?"
		e.action = actionSave
		e.showyn = true
	}
	return nil
}

func (e *Editor) saveQuit(g *gocui.Gui, v *gocui.View) error {
	if e.ynQuestion == "" {
		e.ynQuestion = "Save and Quit (y/n)?"
		e.action = actionSaveQuit
		e.showyn = true
	}
	return nil
}

func (e *Editor) switchView(g *gocui.Gui, v *gocui.View) error {
	if e.View == "ui" {
		e.g.Cursor = true
		e.View = "conf"
		e.uiView.Highlight = false
	} else if e.View == "conf" {
		err := e.parseConfToUi(g)
		if err == nil {
			e.View = "ui"
			e.g.Cursor = false
			e.uiView.Highlight = true
		}
	}
	return nil
}

func (e *Editor) yn(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	if ch == 'y' && mod == 0 {
		e.ynQuestion = ""
	} else if ch == 'n' && mod == 0 {
		e.action = 0
		e.ynQuestion = ""
	}
}

func (e *Editor) saveFile() error {
	views := e.g.Views()
	for _, view := range views {
		if view.Name() != "conf" {
			continue
		}
		return os.WriteFile(e.Path, []byte(view.Buffer()), 0644)
	}
	return errors.New("GUI error - view not found")
}

func loadDefaultAerospikeConfig() string {
	return `service {
    proto-fd-max 15000
}
logging {
    console {
        context any info
    }
}
network {
    service {
        address any
        port 3000
    }
    heartbeat {
        interval 150
        mode multicast
        multicast-group 239.1.99.222
        port 9918
        timeout 10
    }
    fabric {
        port 3001
    }
    info {
        port 3003
    }
}
namespace test {
    default-ttl 0
    memory-size 4G
    replication-factor 2
    storage-engine memory
}
`
}

func (e *Editor) parseConfToUi(g *gocui.Gui) error {
	var uiView *gocui.View
	var confView *gocui.View
	for _, view := range e.g.Views() {
		if view.Name() == "conf" {
			confView = view
		} else if view.Name() == "ui" {
			uiView = view
		}

	}
	aeroConfig, err := aeroconf.Parse(strings.NewReader(confView.Buffer()))
	if err != nil {
		return nil
	}
	uiView.Clear()
	fillMenuItems()
	menuItems, err = selectMenuItems(menuItems, aeroConfig)
	if err != nil {
		fmt.Fprintf(uiView, "ERROR parsing configuration: %s\n", err)
	}
	maxX, _ := g.Size()
	lenX := maxX/2 - 3
	drawMenuItems(uiView, menuItems, lenX, 0)
	e.uiLoc = 0
	uiView.SetCursor(0, e.uiLoc)
	return err
}

func selectMenuItems(items []menuItem, aeroConfig aeroconf.Stanza) ([]menuItem, error) {
	var retErr error
	for i, item := range items {
		switch item.Item {
		case itemRackAwareness:
			val, err := aeroConfig.Stanza("namespace test").GetValues("rack-id")
			if err != nil {
				retErr = err
			}
			if err == nil && len(val) > 0 && val[0] != nil {
				items[i].Selected = true
			}
		case itemStrongConsistency:
			val, err := aeroConfig.Stanza("namespace test").GetValues("strong-consistency")
			if err != nil {
				retErr = err
			}
			if err == nil && len(val) > 0 && val[0] != nil && strings.ToLower(*val[0]) == "true" {
				items[i].Selected = true
			}
		case itemStorageEngineMemory:
			if aeroConfig.Stanza("namespace test").Type("storage-engine") == aeroconf.ValueString {
				val, err := aeroConfig.Stanza("namespace test").GetValues("storage-engine")
				if err != nil {
					retErr = err
				}
				if err == nil && len(val) > 0 && val[0] != nil && strings.ToLower(*val[0]) == "memory" {
					items[i].Selected = true
				}
			}
		case itemStorageDisk:
			if aeroConfig.Stanza("namespace test").Type("storage-engine device") == aeroconf.ValueStanza {
				items[i].Selected = true
			} else {
				val, err := aeroConfig.Stanza("namespace test").GetValues("storage-engine")
				if err != nil {
					retErr = err
				}
				if err == nil && len(val) > 0 && val[0] != nil && strings.ToLower(*val[0]) == "device" {
					retErr = errors.New("storage-engine device must be a {} stanza")
				}
			}
		case itemStorageEngineDeviceAndMemory:
			val, err := aeroConfig.Stanza("namespace test").Stanza("storage-engine device").GetValues("data-in-memory")
			if err != nil {
				retErr = err
			}
			if err == nil && len(val) > 0 && val[0] != nil && strings.ToLower(*val[0]) == "true" {
				items[i].Selected = true
			}
		case itemStorageEngineEncryption:
			val, err := aeroConfig.Stanza("namespace test").Stanza("storage-engine device").GetValues("encryption-key-file")
			if err != nil {
				retErr = err
			}
			if err == nil && len(val) > 0 && val[0] != nil {
				items[i].Selected = true
			}
		case itemStorageEngineCompression:
			val, err := aeroConfig.Stanza("namespace test").Stanza("storage-engine device").GetValues("compression")
			if err != nil {
				retErr = err
			}
			if err == nil && len(val) > 0 && val[0] != nil && *val[0] != "none" {
				items[i].Selected = true
			}
		case itemStorageEngineAllFlash:
			if aeroConfig.Stanza("namespace test").Type("index-type flash") == aeroconf.ValueStanza {
				items[i].Selected = true
			}
		case itemLoggingDestinationFile:
			keys := aeroConfig.Stanza("logging").ListKeys()
			for _, key := range keys {
				if strings.HasPrefix(key, "file") {
					items[i].Selected = true
				}
			}
		case itemLoggingDestinationCOnsole:
			keys := aeroConfig.Stanza("logging").ListKeys()
			for _, key := range keys {
				if strings.HasPrefix(key, "console") {
					items[i].Selected = true
				}
			}
		case itemLoggingLevelInfo:
			keys := aeroConfig.Stanza("logging").ListKeys()
			for _, key := range keys {
				if strings.HasPrefix(key, "console") || strings.HasPrefix(key, "file") {
					vals, err := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
					if err != nil {
						retErr = err
					}
					for _, val := range vals {
						if val != nil && strings.HasSuffix(*val, "any info") {
							items[i].Selected = true
						}
					}
				}
			}
		case itemLoggingLevelDebug:
			keys := aeroConfig.Stanza("logging").ListKeys()
			for _, key := range keys {
				if strings.HasPrefix(key, "console") || strings.HasPrefix(key, "file") {
					vals, err := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
					if err != nil {
						retErr = err
					}
					for _, val := range vals {
						if val != nil && strings.HasSuffix(*val, "any debug") {
							items[i].Selected = true
						}
					}
				}
			}
		case itemLoggingLevelDetail:
			keys := aeroConfig.Stanza("logging").ListKeys()
			for _, key := range keys {
				if strings.HasPrefix(key, "console") || strings.HasPrefix(key, "file") {
					vals, err := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
					if err != nil {
						retErr = err
					}
					for _, val := range vals {
						if val != nil && strings.HasSuffix(*val, "any detail") {
							items[i].Selected = true
						}
					}
				}
			}
		case itemTlsEnabled:
			keys := aeroConfig.Stanza("network").ListKeys()
			for _, key := range keys {
				if strings.HasPrefix(key, "tls") && aeroConfig.Stanza("network").Type(key) == aeroconf.ValueStanza {
					items[i].Selected = true
				} else if strings.HasPrefix(key, "tls") && aeroConfig.Stanza("network").Type(key) != aeroconf.ValueNil {
					retErr = errors.New("tls definition must be a stanza {}")
				}
			}
		case itemTlsService:
			vals1, err := aeroConfig.Stanza("network").Stanza("service").GetValues("tls-port")
			if err != nil {
				retErr = err
			}
			vals2, err := aeroConfig.Stanza("network").Stanza("service").GetValues("tls-address")
			if err != nil {
				retErr = err
			}
			vals3, err := aeroConfig.Stanza("network").Stanza("service").GetValues("tls-name")
			if err != nil {
				retErr = err
			}
			if len(vals1) > 0 && len(vals2) > 0 && len(vals3) > 0 {
				items[i].Selected = true
			}
			if (len(vals1) > 0 && (len(vals2) == 0 || len(vals3) == 0)) || (len(vals2) > 0 && (len(vals1) == 0 || len(vals3) == 0)) || (len(vals3) > 0 && (len(vals2) == 0 || len(vals1) == 0)) {
				retErr = errors.New("for network.service tls setup, specify tls-port, tls-address and tls-name")
			}
		case itemTlsFabric:
			vals1, err := aeroConfig.Stanza("network").Stanza("fabric").GetValues("tls-port")
			if err != nil {
				retErr = err
			}
			vals2, err := aeroConfig.Stanza("network").Stanza("fabric").GetValues("tls-name")
			if err != nil {
				retErr = err
			}
			if len(vals1) > 0 && len(vals2) > 0 {
				items[i].Selected = true
			}
			if (len(vals1) > 0 && len(vals2) == 0) || (len(vals2) > 0 && len(vals1) == 0) {
				retErr = errors.New("for network.fabric tls setup, specify tls-port and tls-name")
			}
		case itemSecurityOff:
			if aeroConfig.Type("security") == aeroconf.ValueNil {
				items[i].Selected = true
			}
		case itemSecurityOnBasic:
			if aeroConfig.Type("security") == aeroconf.ValueStanza {
				if aeroConfig.Stanza("security").Type("ldap") == aeroconf.ValueNil {
					items[i].Selected = true
				}
			} else if aeroConfig.Type("security") != aeroconf.ValueNil {
				retErr = errors.New("security definition must be a {} stanza")
			}
		case itemSecurityOnLdap:
			if aeroConfig.Type("security") == aeroconf.ValueStanza {
				if aeroConfig.Stanza("security").Type("ldap") == aeroconf.ValueStanza {
					items[i].Selected = true
				} else if aeroConfig.Stanza("security").Type("ldap") != aeroconf.ValueNil {
					retErr = errors.New("security.ldap definition must be a {} stanza")
				}
			} else if aeroConfig.Type("security") != aeroconf.ValueNil {
				retErr = errors.New("security definition must be a {} stanza")
			}
		case itemSecurityLoggingDetail:
			keys := aeroConfig.Stanza("logging").ListKeys()
			for _, key := range keys {
				if strings.HasPrefix(key, "console") || strings.HasPrefix(key, "file") {
					vals, err := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
					if err != nil {
						retErr = err
					}
					for _, val := range vals {
						if val != nil && strings.HasSuffix(*val, "detail") && (strings.HasPrefix(*val, "security") || strings.HasPrefix(*val, "audit") || strings.HasPrefix(*val, "smd")) {
							items[i].Selected = true
						}
					}
				}
			}
		}
		if len(item.Children) > 0 {
			var err error
			items[i].Children, err = selectMenuItems(item.Children, aeroConfig)
			if err != nil {
				retErr = err
			}
		}
	}
	return items, retErr
}

func (e *Editor) ui(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {

	switch {
	case key == gocui.KeyArrowDown:
		if e.uiLoc+2 < len(v.BufferLines()) {
			e.uiLoc++
			v.MoveCursor(0, 1, true)
		}
	case key == gocui.KeyArrowUp:
		if e.uiLoc > 0 {
			e.uiLoc--
			v.MoveCursor(0, -1, true)
		}
	case key == gocui.KeySpace, key == gocui.KeyEnter:
		var changes []menuItem
		menuItems, _, changes = switchItem(menuItems, e.uiLoc, 0)
		maxX, _ := e.g.Size()
		lenX := maxX/2 - 3
		e.uiView.Clear()
		drawMenuItems(e.uiView, menuItems, lenX, 0)
		aeroConfig, _ := aeroconf.Parse(strings.NewReader(e.confView.Buffer()))
		for _, change := range changes {
			switch change.Item {
			case itemRackAwareness:
				if change.Selected {
					aeroConfig.Stanza("namespace test").SetValue("rack-id", "1")
				} else {
					aeroConfig.Stanza("namespace test").Delete("rack-id")
				}
			case itemStrongConsistency:
				if change.Selected {
					aeroConfig.Stanza("namespace test").SetValue("strong-consistency", "true")
				} else {
					aeroConfig.Stanza("namespace test").Delete("strong-consistency")
				}
			case itemStorageEngineMemory:
				if !change.Selected {
					if aeroConfig.Stanza("namespace test").Type("storage-engine") == aeroconf.ValueString {
						val, _ := aeroConfig.Stanza("namespace test").GetValues("storage-engine")
						if len(val) > 0 && val[0] != nil && *val[0] == "memory" {
							aeroConfig.Stanza("namespace test").Delete("storage-engine")
						}
					}
				} else {
					for _, key := range aeroConfig.Stanza("namespace test").ListKeys() {
						if strings.HasPrefix(key, "storage-engine") {
							aeroConfig.Stanza("namespace test").Delete(key)
						}
					}
					aeroConfig.Stanza("namespace test").SetValue("storage-engine", "memory")
				}
			case itemStorageDisk:
				if change.Selected {
					if aeroConfig.Stanza("namespace test").Type("storage-engine") == aeroconf.ValueString {
						val, _ := aeroConfig.Stanza("namespace test").GetValues("storage-engine")
						if len(val) > 0 && val[0] != nil && *val[0] == "memory" {
							aeroConfig.Stanza("namespace test").Delete("storage-engine")
						}
					}
					aeroConfig.Stanza("namespace test").NewStanza("storage-engine device")
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").SetValue("file", "/opt/aerospike/data/bar.dat")
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").SetValue("filesize", "1G")
				} else {
					for _, key := range aeroConfig.Stanza("namespace test").ListKeys() {
						if strings.HasPrefix(key, "storage-engine device") {
							aeroConfig.Stanza("namespace test").Delete(key)
						}
					}
				}
			case itemStorageEngineDeviceAndMemory:
				if change.Selected {
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").SetValue("data-in-memory", "true")
				} else {
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").Delete("data-in-memory")
				}
			case itemStorageEngineEncryption:
				if change.Selected {
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").SetValue("encryption-key-file", "/opt/aerospike/key.dat")
				} else {
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").Delete("encryption-key-file")
				}
			case itemStorageEngineCompression:
				if change.Selected {
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").SetValue("compression", "zstd # alt options: none, snappy, lz4")
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").SetValue("compression-level", "1")
				} else {
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").Delete("compression")
					aeroConfig.Stanza("namespace test").Stanza("storage-engine device").Delete("compression-level")
				}
			case itemStorageEngineAllFlash:
				if change.Selected {
					if aeroConfig.Stanza("namespace test").Type("partition-tree-sprigs") == aeroconf.ValueNil {
						aeroConfig.Stanza("namespace test").SetValue("partition-tree-sprigs", "256")
					}
					aeroConfig.Stanza("namespace test").NewStanza("index-type flash")
					aeroConfig.Stanza("namespace test").Stanza("index-type flash").SetValue("mount", "/mnt")
					aeroConfig.Stanza("namespace test").Stanza("index-type flash").SetValue("mounts-size-limit", "5G")
				} else {
					aeroConfig.Stanza("namespace test").Delete("index-type flash")
					aeroConfig.Stanza("namespace test").Delete("partition-tree-sprigs")
				}
			case itemLoggingDestinationFile:
				if change.Selected {
					keys := aeroConfig.Stanza("logging").ListKeys()
					data := make(map[string][]*string)
					for _, key := range keys {
						dd := aeroConfig.Stanza("logging").Stanza(key).ListKeys()
						for _, d := range dd {
							dat, _ := aeroConfig.Stanza("logging").Stanza(key).GetValues(d)
							if _, ok := data[d]; !ok {
								data[d] = dat
							} else {
								data[d] = append(data[d], dat...)
							}
						}
					}
					aeroConfig.Stanza("logging").Delete("console")
					aeroConfig.Stanza("logging").NewStanza("file /var/log/aerospike.log")
					for k, v := range data {
						aeroConfig.Stanza("logging").Stanza("file /var/log/aerospike.log").SetValues(k, v)
					}
				}
			case itemLoggingDestinationCOnsole:
				if change.Selected {
					keys := aeroConfig.Stanza("logging").ListKeys()
					data := make(map[string][]*string)
					for _, key := range keys {
						dd := aeroConfig.Stanza("logging").Stanza(key).ListKeys()
						for _, d := range dd {
							dat, _ := aeroConfig.Stanza("logging").Stanza(key).GetValues(d)
							if _, ok := data[d]; !ok {
								data[d] = dat
							} else {
								data[d] = append(data[d], dat...)
							}
						}
						if strings.HasPrefix(key, "file ") {
							aeroConfig.Stanza("logging").Delete(key)
						}
					}
					aeroConfig.Stanza("logging").NewStanza("console")
					for k, v := range data {
						aeroConfig.Stanza("logging").Stanza("console").SetValues(k, v)
					}
				}
			case itemLoggingLevelInfo:
				l := "any info"
				if change.Selected {
					keys := aeroConfig.Stanza("logging").ListKeys()
					for _, key := range keys {
						found := false
						dat, _ := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
						for i, value := range dat {
							if strings.HasPrefix(*value, "any ") {
								dat[i] = &l
								found = true
							}
						}
						if !found {
							dat = append(dat, &l)
						}
						aeroConfig.Stanza("logging").Stanza(key).SetValues("context", dat)
					}
				}
			case itemLoggingLevelDebug:
				l := "any debug"
				if change.Selected {
					keys := aeroConfig.Stanza("logging").ListKeys()
					for _, key := range keys {
						found := false
						dat, _ := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
						for i, value := range dat {
							if strings.HasPrefix(*value, "any ") {
								dat[i] = &l
								found = true
							}
						}
						if !found {
							dat = append(dat, &l)
						}
						aeroConfig.Stanza("logging").Stanza(key).SetValues("context", dat)
					}
				}
			case itemLoggingLevelDetail:
				l := "any detail"
				if change.Selected {
					keys := aeroConfig.Stanza("logging").ListKeys()
					for _, key := range keys {
						found := false
						dat, _ := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
						for i, value := range dat {
							if strings.HasPrefix(*value, "any ") {
								dat[i] = &l
								found = true
							}
						}
						if !found {
							dat = append(dat, &l)
						}
						aeroConfig.Stanza("logging").Stanza(key).SetValues("context", dat)
					}
				}
			case itemTlsEnabled:
				if !change.Selected {
					keys := aeroConfig.Stanza("network").ListKeys()
					for _, key := range keys {
						if strings.HasPrefix(key, "tls ") {
							aeroConfig.Stanza("network").Delete(key)
						}
					}
				} else {
					aeroConfig.Stanza("network").NewStanza("tls tls1")
					aeroConfig.Stanza("network").Stanza("tls tls1").SetValue("cert-file", "/etc/aerospike/ssl/tls1/cert.pem")
					aeroConfig.Stanza("network").Stanza("tls tls1").SetValue("key-file", "/etc/aerospike/ssl/tls1/key.pem")
					aeroConfig.Stanza("network").Stanza("tls tls1").SetValue("ca-file", "/etc/aerospike/ssl/tls1/cacert.pem")
				}
			case itemTlsService:
				if change.Selected {
					aeroConfig.Stanza("network").Stanza("service").Delete("address")
					aeroConfig.Stanza("network").Stanza("service").Delete("port")
					aeroConfig.Stanza("network").Stanza("service").SetValue("tls-address", "any")
					aeroConfig.Stanza("network").Stanza("service").SetValue("tls-port", "4333")
					aeroConfig.Stanza("network").Stanza("service").SetValue("tls-name", "tls1")
					aeroConfig.Stanza("network").Stanza("service").SetValue("tls-authenticate-client", "false")
				} else {
					aeroConfig.Stanza("network").Stanza("service").SetValue("address", "any")
					aeroConfig.Stanza("network").Stanza("service").SetValue("port", "3000")
					aeroConfig.Stanza("network").Stanza("service").Delete("tls-address")
					aeroConfig.Stanza("network").Stanza("service").Delete("tls-port")
					aeroConfig.Stanza("network").Stanza("service").Delete("tls-name")
					aeroConfig.Stanza("network").Stanza("service").Delete("tls-authenticate-client")
				}
			case itemTlsFabric:
				if change.Selected {
					aeroConfig.Stanza("network").Stanza("fabric").Delete("port")
					aeroConfig.Stanza("network").Stanza("fabric").SetValue("tls-port", "3011")
					aeroConfig.Stanza("network").Stanza("fabric").SetValue("tls-name", "tls1")
				} else {
					aeroConfig.Stanza("network").Stanza("fabric").SetValue("port", "3001")
					aeroConfig.Stanza("network").Stanza("fabric").Delete("tls-port")
					aeroConfig.Stanza("network").Stanza("fabric").Delete("tls-name")
				}
			case itemSecurityOff:
				if change.Selected {
					aeroConfig.Delete("security")
				}
			case itemSecurityOnBasic:
				if change.Selected {
					aeroConfig.Delete("security")
					aeroConfig.NewStanza("security")
					aeroConfig.Stanza("security").NewStanza("log")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-authentication", "true")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-user-admin", "true")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-sys-admin", "true")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-violation", "true")
				}
			case itemSecurityOnLdap:
				if change.Selected {
					aeroConfig.Delete("security")
					aeroConfig.NewStanza("security")
					aeroConfig.Stanza("security").NewStanza("ldap")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("query-base-dn", "dc=aerospike,dc=com")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("server", "ldap://LDAPIP:389 # set to ldaps://LDAPIP:636 for tls")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("tls-ca-file", "/etc/aerospike/ssl/tls1/cacert.pem")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("disable-tls", "true")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("user-dn-pattern", "uid=${un},ou=People,dc=aerospike,dc=com")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("role-query-search-ou", "false")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("role-query-pattern", "(&(objectClass=posixGroup)(memberUid=${un}))")
					aeroConfig.Stanza("security").Stanza("ldap").SetValue("polling-period", "90")
					aeroConfig.Stanza("security").NewStanza("log")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-authentication", "true")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-user-admin", "true")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-sys-admin", "true")
					aeroConfig.Stanza("security").Stanza("log").SetValue("report-violation", "true")
				}
			case itemSecurityLoggingDetail:
				l1 := "security detail"
				l2 := "smd detail"
				l3 := "audit detail"
				if change.Selected {
					keys := aeroConfig.Stanza("logging").ListKeys()
					for _, key := range keys {
						found1 := false
						found2 := false
						found3 := false
						dat, _ := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
						for i, value := range dat {
							if strings.HasPrefix(*value, "security ") {
								dat[i] = &l1
								found1 = true
							}
							if strings.HasPrefix(*value, "smd ") {
								dat[i] = &l2
								found2 = true
							}
							if strings.HasPrefix(*value, "audit ") {
								dat[i] = &l3
								found3 = true
							}
						}
						if !found1 {
							dat = append(dat, &l1)
						}
						if !found2 {
							dat = append(dat, &l2)
						}
						if !found3 {
							dat = append(dat, &l3)
						}
						aeroConfig.Stanza("logging").Stanza(key).SetValues("context", dat)
					}
				} else {
					keys := aeroConfig.Stanza("logging").ListKeys()
					for _, key := range keys {
						newdat := []*string{}
						dat, _ := aeroConfig.Stanza("logging").Stanza(key).GetValues("context")
						for _, d := range dat {
							if *d != l1 && *d != l2 && *d != l3 {
								newdat = append(newdat, d)
							}
						}
						aeroConfig.Stanza("logging").Stanza(key).SetValues("context", newdat)
					}
				}
			}
		}
		e.confView.Clear()
		aeroConfig.Write(e.confView, "", "    ", true)
	}
}

// returns: new menuItems, IGNORE, list of menuItem that changed
func switchItem(items []menuItem, pos int, curPos int) ([]menuItem, int, []menuItem) {
	var newPosItems int
	changes := []menuItem{}
	for i, item := range items {
		cPos := curPos + i
		if item.Type != typeMenuItemText && item.Type != typeMenuItemEmpty {
			if cPos == pos {
				if items[i].Selected && item.Type != typeMenuItemRadio {
					items[i].Selected = false
					changes = append(changes, items[i])
				} else {
					items[i].Selected = true
					changes = append(changes, items[i])
					for f := range items[i].Children {
						if items[i].Children[f].Type == typeMenuItemCheckbox || items[i].Children[f].Type == typeMenuItemRadio {
							changes = append(changes, items[i].Children[f])
						}
					}
					if item.Type == typeMenuItemRadio {
						j := i - 1
						for j >= 0 && items[j].Type == typeMenuItemRadio {
							if items[j].Selected {
								items[j].Selected = false
								changes = append(changes, items[j])
							}
							j--
						}
						j = i + 1
						for j < len(items) && items[j].Type == typeMenuItemRadio {
							if items[j].Selected {
								items[j].Selected = false
								changes = append(changes, items[j])
							}
							j++
						}
					}
				}
			}
		}
		var posItems int
		var cc []menuItem
		items[i].Children, posItems, cc = switchItem(items[i].Children, pos, cPos+1)
		changes = append(changes, cc...)
		newPosItems = newPosItems + posItems
		curPos = curPos + posItems
	}
	return items, len(items) + newPosItems, changes
}
