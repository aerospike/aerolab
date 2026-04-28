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

// grafanaReadyTimeout is the maximum time we will wait for Grafana to become
// reachable on its HTTP API before giving up. On slow cloud instances (notably
// AWS with cold EBS volumes plus the simpod-json-datasource plugin
// initialising for the first time) Grafana can take well over a minute to
// bind its port, so the budget is generous.
const grafanaReadyTimeout = 5 * time.Minute

// Run drives the Grafana helper lifecycle: it waits for Grafana to come up,
// applies the timezone fix, imports dashboards, sets the home dashboard,
// loads annotations and then enters a periodic save-annotations loop.
//
// If Grafana does not become reachable within grafanaReadyTimeout, Run
// returns an error so the caller (and systemd, via Restart=always) can
// retry from a clean slate instead of silently leaving Grafana
// misconfigured. Once the readiness probe succeeds, post-startup
// operations log their errors and continue, matching pre-existing
// behaviour.
//
// Run only returns under error conditions; on success it blocks
// indefinitely inside the save-annotations loop.
func Run(g *GrafanaFix) error {
	if g == nil {
		var err error
		g, err = MakeConfig(true, nil, true)
		if err != nil {
			return fmt.Errorf("could not create default config: %w", err)
		}
	}
	log.Printf("Waiting for grafana to be up and fixing timezone (up to %s)", grafanaReadyTimeout)
	deadline := time.Now().Add(grafanaReadyTimeout)
	var lastErr error
	for {
		time.Sleep(time.Second)
		lastErr = g.fixTimezone()
		if lastErr == nil {
			break
		}
		log.Println(lastErr)
		if !time.Now().Before(deadline) {
			return fmt.Errorf("grafana did not become ready within %s: %w", grafanaReadyTimeout, lastErr)
		}
	}
	log.Println("Importing dashboards")
	if err := g.importDashboards(); err != nil {
		log.Println(err)
	}
	log.Println("Setting home dashboard")
	if err := g.homeDashboard(); err != nil {
		log.Println(err)
		if err := g.homeDashboard(); err != nil {
			log.Println(err)
		}
	}
	log.Println("Loading annotations")
	if err := g.loadAnnotations(); err != nil {
		log.Print(err)
	}
	if len(g.LabelFiles) > 0 {
		fmt.Println("Setting HTML Title to Label")
		if err := g.setLabel(); err != nil {
			log.Println(err)
		}
	}
	log.Println("Entering sleep-save-annotation loop")
	for {
		time.Sleep(time.Minute * 5)
		if err := g.saveAnnotations(); err != nil {
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
