package aerospike

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Products []Product

type Product struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

func GetProducts(timeout time.Duration) (Products, error) {
	req, err := http.NewRequest("GET", "https://download.aerospike.com/artifacts", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	client := &http.Client{
		Timeout: timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Join(err, errors.New(string(body)))
	}
	data := getHtmlData(resp.Body)
	products := make(Products, 0)
	for _, item := range data {
		if item.name == "Parent Directory" {
			continue
		}
		if item.link == "" {
			continue
		}
		link, err := url.JoinPath("https://download.aerospike.com/artifacts", item.link)
		if err != nil {
			return nil, err
		}
		products = append(products, Product{
			Name: strings.TrimSuffix(item.name, "/"),
			Link: link,
		})
	}
	sort.Slice(products, func(i, j int) bool {
		return products[i].Name < products[j].Name
	})
	return products, nil
}

func (p Products) WithName(name string) Products {
	products := make(Products, 0)
	for _, product := range p {
		if product.Name == name {
			products = append(products, product)
		}
	}
	return products
}

func (p Products) WithNamePrefix(prefix string) Products {
	products := make(Products, 0)
	for _, product := range p {
		if strings.HasPrefix(product.Name, prefix) {
			products = append(products, product)
		}
	}
	return products
}

func (p Products) WithNameSuffix(suffix string) Products {
	products := make(Products, 0)
	for _, product := range p {
		if strings.HasSuffix(product.Name, suffix) {
			products = append(products, product)
		}
	}
	return products
}

func (p Products) WithNameContains(contains string) Products {
	products := make(Products, 0)
	for _, product := range p {
		if strings.Contains(product.Name, contains) {
			products = append(products, product)
		}
	}
	return products
}

func (p Products) WithNamePattern(pattern string) Products {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	products := make(Products, 0)
	for _, product := range p {
		if re.MatchString(product.Name) {
			products = append(products, product)
		}
	}
	return products
}
