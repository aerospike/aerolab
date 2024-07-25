package main

import (
	_ "embed"
	"strings"
)

//go:generate sh -c "git rev-parse --short HEAD > embed_commit.txt"
//go:generate sh version.sh
//go:generate sh -c "echo '-unofficial' > embed_tail.txt"
//go:generate sh -c "cp embed_commit.txt ../web/dev/version.cfg"

//go:embed embed_commit.txt
var vCommit string

//go:embed embed_branch.txt
var vBranch string

//go:embed embed_tail.txt
var vEdition string

var version = "v" + strings.Trim(vBranch, "\t\r\n ") + "-" + strings.Trim(vCommit, "\t\r\n ") + strings.Trim(vEdition, "\t\r\n ")

var telemetryVersion = "5" // remember to modify this when changing the telemetry system; remember to update the telemetry structs in cloud function if needed

var simulateArmInstaller = false

var awsExpiryVersion = 10 // remember to change this when modifying the expiry system version
var gcpExpiryVersion = 7  // remember to change this when modifying the expiry system version

var isWebuiBeta = true // switch to false to prevent the beta tag and log message for webUI
