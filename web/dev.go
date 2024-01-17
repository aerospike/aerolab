package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
)

type Page struct {
	Navigation *Nav
	Menu       *MainMenu
}

type Nav struct {
	Top []*NavTop
}

type NavTop struct {
	Name string // button name
	Href string // translates to #nav={{.Href}}
}

type MainMenu struct {
	Items []*MenuItem
}

const (
	BadgeTypeWarning = "badge-warning"
	BadgeTypeSuccess = "badge-success"
	BadgeTypeDanger  = "badge-danger"
)

type MenuItem struct {
	HasChildren bool
	Icon        string
	Name        string
	Href        string
	IsActive    bool
	Badge       MenuItemBadge
	Items       []*MenuItem
}

type MenuItemBadge struct {
	Show bool
	Type string
	Text string
}

func main() {
	http.Handle("/dist/", http.FileServer(http.Dir("dev/")))
	http.Handle("/plugins/", http.FileServer(http.Dir("dev/")))
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
		Navigation: &Nav{
			Top: []*NavTop{
				{
					Name: "Home",
					Href: "home",
				},
				{
					Name: "Logout",
					Href: "logout",
				},
			},
		},
		Menu: &MainMenu{
			Items: []*MenuItem{
				{
					Icon: "fas fa-angle-left",
					Name: "x1",
					Href: "x1",
					Badge: MenuItemBadge{
						Show: true,
						Type: BadgeTypeDanger,
						Text: "bob",
					},
				},
				{
					HasChildren: true,
					Icon:        "fas fa-angle-left",
					Name:        "one",
					Href:        "one",
					Items: []*MenuItem{
						{
							Icon: "fas fa-angle-left",
							Name: "x2",
							Href: "x2",
						},
						{
							HasChildren: true,
							Icon:        "fas fa-angle-left",
							Name:        "two",
							Href:        "two",
							Items: []*MenuItem{
								{
									Icon: "fas fa-angle-left",
									Name: "x3",
									Href: "x3",
								},
								{
									HasChildren: true,
									Icon:        "fas fa-angle-left",
									Name:        "three",
									Href:        "three",
									Items: []*MenuItem{
										{
											Icon: "fas fa-angle-left",
											Name: "x4",
											Href: "x4",
										},
										{
											HasChildren: true,
											Icon:        "fas fa-angle-left",
											Name:        "four",
											Href:        "four",
											Items: []*MenuItem{
												{
													Icon: "fas fa-angle-left",
													Name: "xy",
													Href: "xy",
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
	www := os.DirFS("dev")
	t, err := template.ParseFS(www, "index.html", "index.js")
	if err != nil {
		log.Fatal(err)
	}
	err = t.ExecuteTemplate(w, "main", p)
	if err != nil {
		log.Fatal(err)
	}
}
