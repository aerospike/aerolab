package grafanafix

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"time"

	"github.com/creasty/defaults"
	"github.com/rglonek/envconfig"
	"gopkg.in/yaml.v3"
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
	GrafanaURL     string   `yaml:"grafanaURL" envconfig:"GRAFANAFIX_URL" default:"http://127.0.0.1:8850"`
	AnnotationFile string   `yaml:"annotationFile" envconfig:"GRAFANAFIX_ANNOTATIONS" default:"annotations.json"`
	LabelFiles     []string `yaml:"labelFiles" envconfig:"GRAFANAFIX_LABEL_FILE"`
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
	if len(g.LabelFiles) > 0 {
		fmt.Println("Setting HTML Title to Label")
		err := g.setLabel()
		if err != nil {
			log.Println(err)
		}
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

func (g *GrafanaFix) setLabel() error {
	for _, labelFile := range g.LabelFiles {
		if _, err := os.Stat(labelFile); err != nil {
			return errors.New("file does not exist")
		}
	}
	nlabel := []byte{}
	for _, labelFile := range g.LabelFiles {
		nlabela, _ := os.ReadFile(labelFile)
		if string(nlabela) == "" {
			continue
		}
		if len(nlabel) == 0 {
			nlabel = nlabela
		} else {
			nlabel = append(nlabel, []byte(" - ")...)
			nlabel = append(nlabel, nlabela...)
		}
	}
	for i := range nlabel {
		if nlabel[i] == 32 || nlabel[i] == 45 || nlabel[i] == 46 || nlabel[i] == 61 || nlabel[i] == 95 {
			continue
		}
		if nlabel[i] >= 48 && nlabel[i] <= 58 {
			continue
		}
		if nlabel[i] >= 65 && nlabel[i] <= 90 {
			continue
		}
		if nlabel[i] >= 97 && nlabel[i] <= 122 {
			continue
		}
		nlabel[i] = ' '
	}
	fmt.Println("Grafana Label Override to: " + string(nlabel))
	out, err := exec.Command("find", "/usr/share/grafana/public/build/", "-name", "*.js", "-exec", "sed", "-i", "-E", fmt.Sprintf(`s/this.AppTitle="[^"]+"/this.AppTitle="%s"/g`, string(nlabel)), "{}", ";").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}
