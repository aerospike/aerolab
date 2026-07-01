package aerospike

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// EnvArtifactsURL lets the operator override the default
// https://download.aerospike.com/artifacts base. When the override points
// at a JFrog Artifactory ("*.jfrog.io") it is handled by the jfrog
// subpackage instead; this fall-through path only applies to a mirror
// that exposes the same HTML directory listings as the public site.
const EnvArtifactsURL = "AEROLAB_ARTIFACTS_URL"

// DefaultArtifactsURL is the production location of the Aerospike public
// artifacts mirror.
const DefaultArtifactsURL = "https://download.aerospike.com/artifacts"

// ArtifactsBaseURL returns the URL the GetProducts crawler will hit.
// Returns the default unless an explicit non-JFrog override is set.
func ArtifactsBaseURL() string {
	v := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvArtifactsURL)), "/")
	if v == "" {
		return DefaultArtifactsURL
	}
	if strings.Contains(strings.ToLower(v), ".jfrog.io") {
		// JFrog is handled by the jfrog subpackage; the HTML crawler
		// cannot read JFrog UI pages.
		return DefaultArtifactsURL
	}
	return v
}

type Products []Product

type Product struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

func GetProducts(timeout time.Duration) (Products, error) {
	req, err := http.NewRequest("GET", ArtifactsBaseURL(), nil)
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
		link, err := url.JoinPath(ArtifactsBaseURL(), item.link)
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
