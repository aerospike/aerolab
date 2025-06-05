package aerospike

import (
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type htmlData struct {
	// html.body.table.[]tr.td[1].a[href] - name is strip /, destination is with /
	// html.body.table.[]tr.td[1].a[href] - version is strip /, destination is with /
	// html.body.table.[]tr.td[1].a[href] - filename
	name string
	link string
	// html.body.table.[]tr.td[2].body
	date string // 2025-05-23 19:14
	// html.body.table.[]tr.td[3].body
	size string // 24M, 2K, 100, 22G
}

func getHtmlData(body io.Reader) []htmlData {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		panic(err)
	}

	items := make([]htmlData, 0)
	doc.Find("body table tr").Each(func(i int, s *goquery.Selection) {
		td1 := s.Find("td").Eq(1)
		td2 := s.Find("td").Eq(2)
		td3 := s.Find("td").Eq(3)
		name := strings.TrimPrefix(td1.Find("a").Text(), "/")
		link, _ := td1.Find("a").Attr("href")
		date := strings.TrimSpace(td2.Text())
		size := strings.TrimSpace(td3.Text())
		items = append(items, htmlData{
			name: name,
			link: link,
			date: date,
			size: size,
		})
	})
	return items
}
