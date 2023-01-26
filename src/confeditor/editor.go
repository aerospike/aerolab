package confeditor

// TODO FUTURE: move to https://github.com/charmbracelet/bubbletea

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jroimartin/gocui"
)

type Editor struct {
	Path       string
	loaded     bool
	View       string
	g          *gocui.Gui
	ynQuestion string
	action     int
	showyn     bool
	isUiInit   bool
	uiLoc      int
	confView   *gocui.View
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
		v.BgColor = gocui.ColorGreen
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
				return err
			}
		}
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
		v.BgColor = gocui.ColorYellow
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
		v.BgColor = gocui.ColorRed
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
	} else if e.View == "conf" {
		e.g.Cursor = false
		e.parseConfToUi(g)
		e.View = "ui"
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
		mode multicast
		multicast-group 239.1.99.222
		port 9918
		interval 150
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
	replication-factor 2
	memory-size 4G
	default-ttl 0
	storage-engine memory
}
namespace bar {
	replication-factor 2
	memory-size 4G
	default-ttl 0
	storage-engine device {
		file /opt/aerospike/data/bar.dat
		filesize 1G
		data-in-memory false
	}
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
	uiView.Clear()
	maxX, _ := g.Size()
	lenX := maxX/2 - 3
	// TODO parse conf view contents to UI build
	for _, line := range confView.BufferLines() {
		line = "[ ] " + line
		for len(line) < lenX {
			line = line + " "
		}
		fmt.Fprint(uiView, line+"\n")
	}
	// TODO END
	e.uiLoc = 0
	uiView.SetCursor(0, 0)
	return nil
}

func (e *Editor) ui(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {

	switch {
	case key == gocui.KeyArrowDown:
		if e.uiLoc+3 < len(v.BufferLines()) {
			e.uiLoc++
			v.MoveCursor(0, 1, true)
		}
	case key == gocui.KeyArrowUp:
		if e.uiLoc > 0 {
			e.uiLoc--
			v.MoveCursor(0, -1, true)
		}
	case key == gocui.KeySpace, key == gocui.KeyEnter:
		lines := v.BufferLines()
		if strings.HasPrefix(lines[e.uiLoc], "[ ] ") {
			lines[e.uiLoc] = strings.ReplaceAll(lines[e.uiLoc], "[ ] ", "[x] ")
		} else {
			lines[e.uiLoc] = strings.ReplaceAll(lines[e.uiLoc], "[x] ", "[ ] ")
		}
		e.confView.Clear()
		fmt.Fprint(e.confView, "ERROR, not implemented")
		// TODO update 'conf' view when UI changes are made
		v.Clear()
		fmt.Fprint(v, strings.Join(lines, "\n"))
	}
}
