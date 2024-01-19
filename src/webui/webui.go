package webui

import (
	"strings"
)

const (
	ContentTypeForm  = "form"
	ContentTypeTable = "table"
)

type Page struct {
	FixedFooter                             bool
	FixedNavbar                             bool
	PendingActionsShowAllUsersToggle        bool
	PendingActionsShowAllUsersToggleChecked bool
	Navigation                              *Nav
	Menu                                    *MainMenu
	Content                                 []*ContentItem
}

type ContentItem struct {
	ContentType string
	Form        struct {
		Elements []*FormElement
	}
}

// TODO: also handle /api/items - respond with json containing a list of items to display in notifications section; js should refresh this regularly (every 10 seconds?), or using push somehow? Can jquery use push?
// TODO: optional google sso auth!

type FormElement struct {
	Type string // input/checkbox/multi/etc
	// TODO: multiple form elements described
	// TODO: rethink this approach - we will always render the page from server side - tables/forms; the table contents will be refreshed using javascript
	// TODO: so we should just render the page templates accordingly, and let javascript work out if it has any table element items it needs to fill
}

type Nav struct {
	Top []*NavTop
}

type NavTop struct {
	Name string
	Href string
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
	HasChildren bool
	Icon        string
	Name        string
	Href        string
	IsActive    bool
	ActiveColor string
	Badge       MenuItemBadge
	Items       MenuItems
	Tooltip     string
}

type MenuItemBadge struct {
	Show bool
	Type string
	Text string
}

func (m MenuItems) Set(path string) {
	m.SetTemplate()
	m.MakeActive(path)
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

func (m MenuItems) MakeActive(path string) {
	for i := range m {
		if m[i].Href == path {
			m[i].IsActive = true
			return
		}
		if strings.HasPrefix(path, strings.TrimSuffix(m[i].Href, "/")+"/") {
			m[i].IsActive = true
			m[i].Items.MakeActive(path)
		}
	}
}
