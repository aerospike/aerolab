package cmd

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/openbrowser"
	"github.com/rglonek/logger"
)

// AgiAddTokenCmd adds an authentication token to the AGI proxy.
// The token is stored in /opt/agi/tokens/{name} and the proxy is
// signaled to reload tokens via SIGHUP.
//
// Tokens must be at least 64 characters long for security.
type AgiAddTokenCmd struct {
	ClusterName TypeAgiClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	TokenName   string             `short:"u" long:"token-name" description:"A unique token name; default: auto-generate timestamp"`
	TokenSize   int                `short:"s" long:"size" description:"Size of the new token to be generated" default:"128"`
	Token       string             `short:"t" long:"token" description:"A 64+ character long token to use; if not specified, a random token will be generated"`
	GenURL      bool               `long:"url" description:"Generate and display a direct-access token URL"`
	Open        bool               `short:"o" long:"open" description:"Open the AGI URL in the default browser (implies --url)"`
	Remove      bool               `short:"r" long:"remove" description:"Remove the token instead of adding it"`
	List        bool               `short:"l" long:"list" description:"List all tokens for the AGI instance"`
	Help        HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi add-auth-token.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiAddTokenCmd) Execute(args []string) error {
	cmd := []string{"agi", "add-auth-token"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.AddToken(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// AddToken adds or manages authentication tokens for the AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiAddTokenCmd) AddToken(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "add-auth-token"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Validate token size
	if c.TokenSize < 64 {
		return fmt.Errorf("minimum token size is 64 characters")
	}

	// Get AGI instance
	instance := inventory.Instances.WithClusterName(string(c.ClusterName)).WithState(backends.LifeCycleStateRunning)
	if instance.Count() == 0 {
		return fmt.Errorf("AGI instance %s not found or not running", c.ClusterName)
	}

	inst := instance.Describe()[0]

	// Handle list operation
	if c.List {
		return c.listTokens(inst, logger)
	}

	// Generate token name if not specified
	if c.TokenName == "" {
		c.TokenName = strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	// Handle remove operation
	if c.Remove {
		return c.removeToken(inst, logger)
	}

	// Generate access URL if requested (--open implies --url)
	var accessURL string
	if c.GenURL || c.Open {
		var urlErr error
		accessURL, urlErr = c.buildAccessURL(inst, system)
		if urlErr != nil {
			return fmt.Errorf("failed to build access URL: %w", urlErr)
		}
	}

	// Generate or use provided token
	var token string
	if c.Token != "" {
		if len(c.Token) < 64 {
			return fmt.Errorf("provided token must be at least 64 characters long")
		}
		token = c.Token
	} else {
		token = generateRandomToken(c.TokenSize, rand.NewSource(time.Now().UnixNano()))
	}

	// Write token file to remote
	confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		// Ensure tokens directory exists
		_ = cli.RawClient().MkdirAll("/opt/agi/tokens")

		// Write token file
		tokenPath := fmt.Sprintf("/opt/agi/tokens/%s", c.TokenName)
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    tokenPath,
			Source:      bytes.NewReader([]byte(token)),
			Permissions: 0600,
		})
		if err != nil {
			return fmt.Errorf("failed to write token file: %w", err)
		}
	}

	// Signal proxy to reload tokens via SIGHUP
	outputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "kill -HUP $(systemctl show --property MainPID --value agi-proxy) 2>/dev/null || true"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	for _, o := range outputs {
		if o.Output.Err != nil {
			logger.Warn("Could not signal proxy to reload tokens: %s", o.Output.Err)
		}
	}

	// Wait a moment for proxy to reload
	time.Sleep(time.Second)

	// Build the full URL if needed
	fullURL := token
	if accessURL != "" {
		fullURL = accessURL + token
	}

	// Output result
	fmt.Println(fullURL)

	// Open browser if requested
	if c.Open {
		logger.Info("Opening AGI in browser...")
		if err := openbrowser.Open(fullURL); err != nil {
			return fmt.Errorf("failed to open browser: %w", err)
		}
	}

	return nil
}

// listTokens lists all tokens for the AGI instance.
func (c *AgiAddTokenCmd) listTokens(inst *backends.Instance, logger *logger.Logger) error {
	outputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"ls", "-la", "/opt/agi/tokens/"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if len(outputs) > 0 {
		if outputs[0].Output.Err != nil {
			return fmt.Errorf("failed to list tokens: %w", outputs[0].Output.Err)
		}
		fmt.Println(string(outputs[0].Output.Stdout))
	}

	return nil
}

// removeToken removes a token from the AGI instance.
func (c *AgiAddTokenCmd) removeToken(inst *backends.Instance, logger *logger.Logger) error {
	if c.TokenName == "" {
		return fmt.Errorf("token name is required for removal")
	}

	tokenPath := fmt.Sprintf("/opt/agi/tokens/%s", c.TokenName)

	outputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"rm", "-f", tokenPath},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	for _, o := range outputs {
		if o.Output.Err != nil {
			return fmt.Errorf("failed to remove token: %w", o.Output.Err)
		}
	}

	// Signal proxy to reload tokens
	backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "kill -HUP $(systemctl show --property MainPID --value agi-proxy) 2>/dev/null || true"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	logger.Info("Token %s removed", c.TokenName)
	return nil
}

// buildAccessURL builds the access URL for the AGI instance.
func (c *AgiAddTokenCmd) buildAccessURL(inst *backends.Instance, system *System) (string, error) {
	ip := inst.IP.Public
	if ip == "" {
		ip = inst.IP.Private
	}
	if ip == "" {
		return "", fmt.Errorf("AGI node IP is empty, is AGI down?")
	}

	// Determine protocol and container port
	protocol := "https://"
	containerPort := "443"
	if inst.Tags != nil {
		if ssl, ok := inst.Tags["aerolab4ssl"]; ok && ssl == "false" {
			protocol = "http://"
			containerPort = "80"
		}
	}

	// Check if using Docker with exposed ports
	port := ""
	if system.Opts.Config.Backend.Type == "docker" {
		// For Docker, use localhost with exposed port from firewall rules
		ip = "127.0.0.1"
		// Extract actual host port from firewall rules
		// Format: host=0.0.0.0:9443,container=443
		for _, fw := range inst.Firewalls {
			parts := strings.Split(fw, ",")
			if len(parts) != 2 {
				continue
			}
			var hp, cp string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "host=") {
					hostPart := strings.TrimPrefix(part, "host=")
					if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx >= 0 {
						hp = hostPart[colonIdx+1:]
					}
				} else if strings.HasPrefix(part, "container=") {
					cp = strings.TrimPrefix(part, "container=")
				}
			}
			if cp == containerPort && hp != "" {
				port = ":" + hp
				break
			}
		}
	}

	// Check for Route53 domain
	if system.Opts.Config.Backend.Type == "aws" {
		if inst.Tags != nil {
			// First check for full DNS name (set by configureAGIDNS for EFS/shortuuid prefixes)
			if dnsName, ok := inst.Tags["agiDNSName"]; ok && dnsName != "" {
				ip = dnsName
			} else if domain, ok := inst.Tags["agiDomain"]; ok && domain != "" {
				// Fallback to instance ID based domain for backwards compatibility
				ip = inst.InstanceID + "." + system.Opts.Config.Backend.Region + ".agi." + domain
			}
		}
	}

	return protocol + ip + port + "/agi/menu?AGI_TOKEN=", nil
}

// generateRandomToken generates a random token of the specified size.
func generateRandomToken(n int, src rand.Source) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const (
		letterIdxBits = 6                    // 6 bits to represent a letter index
		letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
		letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	)

	sb := strings.Builder{}
	sb.Grow(n)

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
