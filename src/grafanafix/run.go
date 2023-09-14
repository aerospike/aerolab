package grafanafix

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/creasty/defaults"
	"github.com/rglonek/envconfig"
	"gopkg.in/yaml.v2"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

type GrafanaFix struct {
	Dashboards struct {
		FromDir      string `yaml:"fromDir" envconfig:"GRAFANAFIX_SOURCE_DIR"`
		LoadEmbedded bool   `yaml:"loadEmbedded" envconfig:"GRAFANAFIX_SOURCE_EMBEDDED" default:"true"`
	} `yaml:"dashboards"`
	GrafanaURL     string `yaml:"grafanaURL" envconfig:"GRAFANAFIX_URL" default:"http://127.0.0.1:8850"`
	AnnotationFile string `yaml:"annotationFile" envconfig:"GRAFANAFIX_ANNOTATIONS" default:"annotations.json"`
}

func MakeConfig(setDefaults bool, configYaml io.Reader, parseEnv bool) (*GrafanaFix, error) {
	config := new(GrafanaFix)
	if setDefaults {
		if err := defaults.Set(config); err != nil {
			return nil, fmt.Errorf("could not set defaults: %s", err)
		}
	}
	if configYaml != nil {
		err := yaml.NewDecoder(configYaml).Decode(config)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %s", err)
		}
	}
	if parseEnv {
		err := envconfig.Process("GRAFANAFIX_", config)
		if err != nil {
			return nil, fmt.Errorf("could not process environment variables: %s", err)
		}
	}
	return config, nil
}

func Run(g *GrafanaFix) {
	if g == nil {
		var err error
		g, err = MakeConfig(true, nil, true)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Println("Waiting for grafana to be up and fixing timezone")
	try := 0
	for try < 60 {
		try++
		time.Sleep(time.Second)
		err := g.fixTimezone()
		if err == nil {
			break
		}
		log.Println(err)
	}
	log.Println("Importing dashboards")
	err := g.importDashboards()
	if err != nil {
		log.Println(err)
	}
	log.Println("Setting home dashboard")
	err = g.homeDashboard()
	if err != nil {
		log.Println(err)
		err = g.homeDashboard()
		if err != nil {
			log.Println(err)
		}
	}
	log.Println("Loading annotations")
	err = g.loadAnnotations()
	if err != nil {
		log.Print(err)
	}
	log.Println("Entering sleep-save-annotation loop")
	for {
		time.Sleep(time.Minute * 5)
		err = g.saveAnnotations()
		if err != nil {
			log.Print(err)
		}
	}
}
