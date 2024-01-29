package main

import (
	"log"
	"net/http"
	"os"
	"text/template"

	"github.com/aerospike/aerolab/webui"
)

func (c *webCmd) inventory(w http.ResponseWriter, r *http.Request) {
	p := &webui.Page{
		WebRoot:                                 c.WebRoot,
		FixedNavbar:                             true,
		FixedFooter:                             true,
		PendingActionsShowAllUsersToggle:        false,
		PendingActionsShowAllUsersToggleChecked: false,
		IsInventory:                             true,
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
