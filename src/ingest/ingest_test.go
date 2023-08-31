package ingest

import (
	"os"
	"testing"
)

func TestInit(t *testing.T) {
	os.Remove("cpu.pprof")
	os.Setenv("LOGINGEST_CPUPROFILE_FILE", "cpu.pprof")
	i, err := InitWithConfig(true, "", true)
	if err != nil {
		t.Fatal(err)
	}
	i.Close()
}
