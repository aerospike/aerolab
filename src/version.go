package main

import (
	_ "embed"
	"strings"
)

//go:generate sh -c "git rev-parse --short HEAD > embed_commit.txt"
//go:generate sh -c "cat ../VERSION.md > embed_branch.txt"
//go:generate sh -c "echo '-unofficial' > embed_tail.txt"

//go:embed embed_commit.txt
var vCommit string

//go:embed embed_branch.txt
var vBranch string

//go:embed embed_tail.txt
var vEdition string

var version = "v" + strings.Trim(vBranch, "\t\r\n ") + "-" + strings.Trim(vCommit, "\t\r\n ") + strings.Trim(vEdition, "\t\r\n ")

var telemetryVersion = "4"

var webuiVersion = "1"

var simulateArmInstaller = false

var awsExpiryVersion = 5
var gcpExpiryVersion = 5
