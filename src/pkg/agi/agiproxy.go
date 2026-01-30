// Package agi provides shared constants and utilities for the Aerospike Grafana Integration.
package agi

import (
	_ "embed"
)

//go:generate sh -c "cd ../../../web/agiproxy && tar -zcf ../../src/pkg/agi/agiproxy.tgz *"

// AgiProxyWeb contains the embedded web UI assets for the AGI proxy.
// This tarball is generated from web/agiproxy/ and contains:
//   - index.html - Template with {{.HTTPTitle}}, {{.Title}}, {{.Description}} variables
//   - dist/ - Static CSS and JavaScript assets (bootstrap, datatables, fontawesome, etc.)
//
//go:embed agiproxy.tgz
var AgiProxyWeb []byte

