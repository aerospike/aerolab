package main

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type agiCmd struct {
	List      agiListCmd      `command:"list" subcommands-optional:"true" description:"List AGI instances"`
	Create    agiCreateCmd    `command:"create" subcommands-optional:"true" description:"Create AGI instance"`
	Destroy   agiDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy AGI instance"`
	Delete    agiDeleteCmd    `command:"delete" hidden:"true" subcommands-optional:"true" description:"Delete AGI volume"`
	Relabel   agiRelabelCmd   `command:"change-label" subcommands-optional:"true" description:"Change instance name label"`
	Details   agiDetailsCmd   `command:"details" subcommands-optional:"true" description:"Show details of an AGI instance"`
	Retrigger agiRetriggerCmd `command:"run-ingest" subcommands-optional:"true" description:"Retrigger log ingest again (will only do bits that have not been done before)"`
	Attach    agiAttachCmd    `command:"attach" subcommands-optional:"true" description:"Attach to an AGI Instance"`
	AddToken  agiAddTokenCmd  `command:"add-auth-token" subcommands-optional:"true" description:"Add an auth token to AGI Proxy - only valid if token auth type was selected"`
	Exec      agiExecCmd      `command:"exec" hidden:"true" subcommands-optional:"true" description:"Run an AGI subsystem"`
	Help      helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type agiListCmd struct {
	Owner   string  `long:"owner" description:"Only show resources tagged with this owner"`
	Json    bool    `short:"j" long:"json" description:"Provide output in json format"`
	NoPager bool    `long:"no-pager" description:"set to disable vertical and horizontal pager"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Inventory.List.Json = c.Json
	a.opts.Inventory.List.Owner = c.Owner
	a.opts.Inventory.List.NoPager = c.NoPager
	return a.opts.Inventory.List.run(false, false, false, false, false, inventoryShowAGI)
}

type agiAddTokenCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	TokenName   string          `short:"u" long:"token-name" description:"a unique token name; default:auto-generate"`
	TokenSize   int             `short:"s" long:"size" description:"size of the new token to be generated" default:"128"`
	Token       string          `short:"t" long:"token" description:"A 64+ character long token to use; if not specified, a random token will be generated"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiAddTokenCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.TokenSize < 64 {
		return fmt.Errorf("minimum token size is 64")
	}
	loc := "/opt/agitokens"
	if c.TokenName == "" {
		c.TokenName = strconv.Itoa(int(time.Now().UnixNano()))
	}
	newToken := randToken(c.TokenSize, rand.NewSource(int64(time.Now().UnixNano())))
	loc = path.Join(loc, c.TokenName)
	err := b.CopyFilesToClusterReader(c.ClusterName.String(), []fileListReader{{
		filePath:     loc,
		fileContents: strings.NewReader(newToken),
		fileSize:     c.TokenSize,
	}}, []int{1})
	if err != nil {
		return err
	}
	fmt.Println(newToken)
	return nil
}

func randToken(n int, src rand.Source) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const (
		letterIdxBits = 6                    // 6 bits to represent a letter index
		letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
		letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	)
	sb := strings.Builder{}
	sb.Grow(n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}

type agiDestroyCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Force       bool            `short:"f" long:"force" description:"force stop before destroy"`
	Parallel    bool            `short:"p" long:"parallel" description:"if destroying many AGI at once, set this to destroy in parallel"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Destroy.ClusterName = c.ClusterName
	a.opts.Cluster.Destroy.Force = c.Force
	a.opts.Cluster.Destroy.Parallel = c.Parallel
	return a.opts.Cluster.Destroy.doDestroy("agi", args)
}

type agiDeleteCmd struct {
	agiDestroyCmd
}

func (c *agiDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Destroy.ClusterName = c.ClusterName
	a.opts.Cluster.Destroy.Force = c.Force
	a.opts.Cluster.Destroy.Parallel = c.Parallel
	return a.opts.Cluster.Destroy.doDestroy("agi", args)
}

type agiRelabelCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	NewLabel    string          `short:"l" long:"label" description:"new label"`
	Gcpzone     string          `short:"z" long:"zone" description:"GCP only: zone where the instance is"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiRelabelCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	err := b.SetLabel(c.ClusterName.String(), "agiLabel", c.NewLabel, c.Gcpzone)
	if err != nil {
		return err
	}
	ips, err := b.GetNodeIpMap(c.ClusterName.String(), false)
	if err != nil {
		return err
	}
	if ip, ok := ips[1]; ok && ip != "" {
		err = b.CopyFilesToClusterReader(c.ClusterName.String(), []fileListReader{{
			filePath:     "/opt/agi/label",
			fileContents: strings.NewReader(c.NewLabel),
			fileSize:     len(c.NewLabel),
		}}, []int{1})
		if err != nil {
			return err
		}
	}
	return nil
}

type agiDetailsCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDetailsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiRetriggerCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiRetriggerCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiAttachCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Detach      bool            `long:"detach" description:"detach the process stdin - will not kill process on CTRL+C"`
	Tail        []string        `description:"List containing command parameters to execute, ex: [\"ls\",\"/opt\"]"`
	Help        attachCmdHelp   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiAttachCmd) Execute(args []string) error {
	a.opts.Attach.Shell.Node = "1"
	a.opts.Attach.Shell.ClusterName = c.ClusterName
	a.opts.Attach.Shell.Detach = c.Detach
	a.opts.Attach.Shell.Tail = c.Tail
	return a.opts.Attach.Shell.run(args)
}
