package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(t *testing.T) {
	os.RemoveAll("./testdata")
	os.Setenv("AEROLAB_HOME", "./testdata")
	defer os.RemoveAll("./testdata")
	os.Args = []string{"aerolab", "version"}
	var buf bytes.Buffer
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = w
	os.Stderr = w
	go func() {
		for {
			_, err := buf.ReadFrom(r)
			if err != nil {
				break
			}
		}
	}()
	run([]string{"version"})
	w.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr
	_, err = os.Stat("./testdata")
	require.NoError(t, err)
	vString := buf.String()
	require.Equal(t, true, strings.HasPrefix(vString, "v"))
	require.Equal(t, true, strings.HasSuffix(vString, "-unofficial\n"))
}
