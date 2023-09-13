package grafanafix

import (
	"os"
	"os/exec"
	"testing"
)

func reset() error {
	err := copyFile("/usr/local/etc/grafana/grafana.ini.bak", "/usr/local/etc/grafana/grafana.ini")
	if err != nil {
		return err
	}
	d, err := os.Stat("/usr/local/var/lib/grafana/plugins")
	if err != nil {
		return err
	}
	perms := d.Mode().Perm()
	err = os.RemoveAll("/usr/local/var/lib/grafana/plugins")
	if err != nil {
		return err
	}
	err = os.Mkdir("/usr/local/var/lib/grafana/plugins", perms)
	if err != nil {
		return err
	}
	if _, err := os.Stat("/usr/local/var/lib/grafana/grafana.db"); err == nil {
		err = os.Remove("/usr/local/var/lib/grafana/grafana.db")
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat("/usr/local/opt/grafana/share/grafana/conf/provisioning/datasources/json.yaml"); err == nil {
		err = os.Remove("/usr/local/opt/grafana/share/grafana/conf/provisioning/datasources/json.yaml")
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat("/usr/local/var/lib/grafana/plugins/simpod-json-datasource"); err == nil {
		err = os.RemoveAll("/usr/local/var/lib/grafana/plugins/simpod-json-datasource")
		if err != nil {
			return err
		}
	}
	return nil
}

func stopGrafana() ([]byte, error) {
	return exec.Command("brew", "services", "stop", "grafana").CombinedOutput()
}

func TestAll(t *testing.T) {
	if out, err := stopGrafana(); err != nil {
		t.Fatal(string(out))
	}
	err := reset()
	if err != nil {
		t.Fatal(err)
	}
	err = EarlySetup("/usr/local/etc/grafana/grafana.ini", "/usr/local/opt/grafana/share/grafana/conf/provisioning", "/usr/local/var/lib/grafana/plugins", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("brew", "services", "run", "grafana").CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	Run(nil)
}
