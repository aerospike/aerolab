package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
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
}

type MenuItemBadge struct {
	Show bool
	Type string
	Text string
}

const serveDir = "dev/" // prod/ or dev/

func main() {
	http.Handle("/dist/", http.FileServer(http.Dir(serveDir)))
	http.Handle("/plugins/", http.FileServer(http.Dir(serveDir)))
	http.HandleFunc("/", serve)
	log.Println("Starting")
	log.Println(http.ListenAndServe("0.0.0.0:8080", nil))
}

func serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	log.Println(r.RequestURI)
	p := &Page{
		FixedNavbar:                             true,
		FixedFooter:                             true,
		PendingActionsShowAllUsersToggle:        true,
		PendingActionsShowAllUsersToggleChecked: false,
		Navigation: &Nav{
			Top: []*NavTop{
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
		Menu: &MainMenu{
			Items: []*MenuItem{
				{
					Icon: "fas fa-angle-left",
					Name: "x1",
					Href: "/x1",
					Badge: MenuItemBadge{
						Show: true,
						Type: BadgeTypeDanger,
						Text: "bob",
					},
				},
				{
					Icon: "fas fa-angle-left",
					Name: "one",
					Href: "/one",
					Items: []*MenuItem{
						{
							Icon: "fas fa-angle-left",
							Name: "x2",
							Href: "/one/x2",
						},
						{
							Icon: "fas fa-angle-left",
							Name: "two",
							Href: "/one/two",
							Items: []*MenuItem{
								{
									Icon: "fas fa-angle-left",
									Name: "x3",
									Href: "/one/two/x3",
								},
								{
									Icon: "fas fa-angle-left",
									Name: "three",
									Href: "/one/two/three",
									Items: []*MenuItem{
										{
											Icon: "fas fa-angle-left",
											Name: "x4",
											Href: "/one/two/three/x4",
										},
										{
											Icon: "fas fa-angle-left",
											Name: "four",
											Href: "/one/two/three/four",
											Items: []*MenuItem{
												{
													Icon: "fas fa-angle-left",
													Name: "xy",
													Href: "/one/two/three/four/xy",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	p.Menu.Items.Set(r.URL.Path)
	www := os.DirFS(serveDir)
	t, err := template.ParseFS(www, "index.html", "index.js", "index.css")
	if err != nil {
		log.Fatal(err)
	}
	err = t.ExecuteTemplate(w, "main", p)
	if err != nil {
		log.Fatal(err)
	}
}

func (m MenuItems) Set(path string) {
	m.SetTemplate()
	m.MakeActive(path)
}

func (m MenuItems) SetTemplate() {
	for i := range m {
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
