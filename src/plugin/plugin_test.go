package plugin

import (
	"os"
	"strings"
	"testing"
)

// TODO: logFileTimeRanges in general stats dashboard: can we still plot it?
// TODO: test if warnings dashboard works after all the aggregations
// TODO: test histograms dashboard
// TODO: once all work, code datapoint reduction
// TODO: once datapoint reduction is coded, test again
// TODO: once tests are complete, redo the dashboards so all work
// TODO: once dashboards are done, code aerolab glue code

func TestAll(t *testing.T) {
	os.Setenv("PLUGIN_LOGLEVEL", "6")
	os.Remove("cpu.pprof")
	os.Setenv("PLUGIN_CPUPROFILE_FILE", "cpu.pprof")
	yamlConfig := "aerospike:\n  namespace: \"test\"\n  port: 3100\n  connectionQueueSize: 128"
	config, err := MakeConfigReader(true, strings.NewReader(yamlConfig), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Init ingest system")
	p, err := Init(config)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	t.Log("Entering listener")
	err = p.Listen()
	if err != nil {
		t.Fatal(err)
	}
}
