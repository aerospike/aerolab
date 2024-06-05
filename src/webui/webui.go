package webui

import (
	"strings"
)

const TruncateTimestampCookieName = "aerolab_history_truncate"

const (
	ContentTypeForm  = "form"
	ContentTypeTable = "table"
)

type Page struct {
	FixedFooter                             bool
	FixedNavbar                             bool
	PendingActionsShowAllUsersToggle        bool
	PendingActionsShowAllUsersToggleChecked bool
	WebRoot                                 string
	FormCommandTitle                        string
	IsForm                                  bool
	IsInventory                             bool
	IsError                                 bool
	ErrorString                             string
	ErrorTitle                              string
	Navigation                              *Nav
	Menu                                    *MainMenu
	FormItems                               []*FormItem
	Inventory                               map[string]*InventoryItem
	Backend                                 string
	BetaTag                                 bool
	CurrentUser                             string
}

type InventoryItem struct {
	Fields []*InventoryItemField
}

type InventoryItemField struct {
	Name         string
	FriendlyName string
	Backend      string
}

type FormItem struct {
	Type      FormItemType
	Input     FormItemInput
	Toggle    FormItemToggle
	Select    FormItemSelect
	Separator FormItemSeparator
}

type FormItemSeparator struct {
	Name string
}

type FormItemType struct {
	Input     bool
	Toggle    bool
	Select    bool
	Separator bool
}

type FormItemInput struct {
	Name        string
	Description string
	ID          string
	Type        string
	Default     string
	Tags        bool
	Required    bool
	Optional    bool
	IsFile      bool
}

type FormItemSelect struct {
	Name        string
	Description string
	ID          string
	Multiple    bool
	Required    bool
	Items       []*FormItemSelectItem
	Optional    bool
}

type FormItemSelectItem struct {
	Name     string
	Value    string
	Selected bool
}

type FormItemToggle struct {
	Name        string
	Description string
	ID          string
	On          bool
	Disabled    bool
	Optional    bool
}

type Nav struct {
	Top []*NavTop
}

type NavTop struct {
	Name   string
	Href   string
	Target string
}

type MainMenu struct {
	Items MenuItems
}

type MenuItems []*MenuItem

const (
	BadgeTypeWarning = "badge-warning"
	BadgeTypeSuccess = "badge-success"
	BadgeTypeDanger  = "badge-danger"
)

const (
	ActiveColorWhite = " bg-white"
	ActiveColorBlue  = " bg-blue"
)

type MenuItem struct {
	HasChildren   bool
	Icon          string
	Name          string
	Href          string
	IsActive      bool
	ActiveColor   string
	Badge         MenuItemBadge
	Items         MenuItems
	Tooltip       string
	DrawSeparator bool
}

type MenuItemBadge struct {
	Show bool
	Type string
	Text string
}

func (m MenuItems) Set(path string, webroot string) {
	m.SetTemplate()
	m.MakeActive(path, webroot)
}

func (m MenuItems) SetTemplate() {
	for i := range m {
		m[i].IsActive = false
		if len(m[i].Items) == 0 {
			m[i].ActiveColor = ActiveColorBlue
			m[i].HasChildren = false
			continue
		}
		m[i].ActiveColor = ActiveColorWhite
		m[i].HasChildren = true
		m[i].Items.SetTemplate()
	}
}

func (m MenuItems) MakeActive(path string, webroot string) {
	for i := range m {
		if m[i].Href == path {
			m[i].IsActive = true
			return
		}
		if m[i].Href != webroot && strings.HasPrefix(path, strings.TrimSuffix(m[i].Href, "/")+"/") {
			m[i].IsActive = true
			m[i].Items.MakeActive(path, webroot)
		}
	}
}
