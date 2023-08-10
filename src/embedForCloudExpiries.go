package main

import (
	_ "embed"
)

//go:generate sh -c "cat ../expiries/gcp/go.mod > gcpMod.txt"
//go:generate sh -c "cat ../expiries/gcp/function.go > gcpFunction.txt"

//go:embed myFunction.zip
var expiriesCodeAws []byte

//go:embed gcpMod.txt
var expiriesCodeGcpMod []byte

//go:embed gcpFunction.txt
var expiriesCodeGcpFunction []byte
