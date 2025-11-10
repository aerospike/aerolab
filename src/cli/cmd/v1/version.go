package cmd

import (
	_ "embed"
	"strings"
)

//go:generate bash -c "git rev-parse --short=7 HEAD > embed_commit.txt"
//go:generate bash version.sh
//go:generate bash -c "echo '-unofficial' > embed_tail.txt"
//go:generate bash -c "cp embed_commit.txt ../../../../web/dev/version.cfg"
//go:generate bash -c "cd ../../../pkg/expiry && bash compile.sh"

//go:embed embed_commit.txt
var vCommit string

//go:embed embed_branch.txt
var vBranch string

//go:embed embed_tail.txt
var vEdition string

func GetAerolabVersion() (branch, commit, edition, friendlyString string) {
	branch = strings.Trim(vBranch, "\t\r\n ")
	commit = strings.Trim(vCommit, "\t\r\n ")
	edition = strings.Trim(vEdition, "\t\r\n ")
	friendlyString = "v" + branch + "-" + commit + edition
	return
}
