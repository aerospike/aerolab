package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	rtypes "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

// Execute implements the command execution for agi monitor create.
//
// The command creates a monitor instance that:
//  1. Creates a client instance with the appropriate IAM role (agimonitor)
//  2. Installs aerolab binary
//  3. Configures systemd service (agimonitor.service)
//  4. Applies aerolab-agi firewall for ports 80/443
//  5. Starts and enables the monitor listener service
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiMonitorCreateCmd) Execute(args []string) error {
	cmd := []string{"agi", "monitor", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.CreateMonitor(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// CreateMonitor creates a monitor instance with the specified configuration.
//
// Parameters:
//   - system: The system context containing backend and configuration
//   - inventory: The current inventory state
//   - logger: Logger for output
//   - args: Additional arguments
//
// Returns:
//   - backends.InstanceList: The created monitor instance(s)
//   - error: nil on success, or an error describing what failed
func (c *AgiMonitorCreateCmd) CreateMonitor(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "monitor", "create"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Get backend type
	backendType := system.Opts.Config.Backend.Type

	// Validate backend type - monitor is only supported on AWS and GCP
	if backendType == "docker" {
		return nil, errors.New("agi monitor can only be deployed on AWS or GCP")
	}

	// Handle ENV:: prefix for AWS credentials
	if backendType == "aws" {
		if strings.HasPrefix(c.AWS.AWSKeyId, "ENV::") {
			c.AWS.AWSKeyId = os.Getenv(strings.TrimPrefix(c.AWS.AWSKeyId, "ENV::"))
		}
		if strings.HasPrefix(c.AWS.AWSSecretKey, "ENV::") {
			c.AWS.AWSSecretKey = os.Getenv(strings.TrimPrefix(c.AWS.AWSSecretKey, "ENV::"))
		}
		// Validate that both or neither AWS credential fields are set
		if (c.AWS.AWSKeyId != "" && c.AWS.AWSSecretKey == "") || (c.AWS.AWSKeyId == "" && c.AWS.AWSSecretKey != "") {
			return nil, errors.New("both --aws-key-id and --aws-secret-key must be specified together, or neither")
		}
		// Validate that either instance profile or credentials are specified
		if c.AWS.InstanceRole == "" && c.AWS.AWSKeyId == "" {
			return nil, errors.New("AWS requires either --aws-role (instance profile) or both --aws-key-id and --aws-secret-key")
		}
	}

	// Validate AWS Route53 configuration
	if backendType == "aws" {
		if (c.AWS.Route53DomainName == "" && c.AWS.Route53ZoneId != "") ||
			(c.AWS.Route53DomainName != "" && c.AWS.Route53ZoneId == "") {
			return nil, errors.New("either both route53-zoneid and route53-domain must be filled or both must be empty")
		}
	}

	// Validate autocert configuration
	if len(c.AutoCertDomains) > 0 && c.AutoCertEmail == "" {
		return nil, errors.New("if autocert domains is in use, a valid email must be provided for letsencrypt registration")
	}

	// Set default owner if not specified
	if c.Owner == "" {
		c.Owner = GetCurrentOwnerUser()
	}

	// Generate config YAML for the monitor listener
	agiConfigYaml, err := yaml.Marshal(c.AgiMonitorListenCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal monitor config: %w", err)
	}

	// Ensure the AGI firewall exists (VPC-specific name)
	logger.Info("Checking firewall rules for AGI access")
	agiFirewallName, err := c.ensureAGIFirewall(system, inventory, logger, backendType)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure AGI firewall: %w", err)
	}
	// Refresh inventory to include new firewall
	inventory, err = system.Backend.GetRefreshedInventory()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh inventory after firewall creation: %w", err)
	}

	// Create the base client instance
	logger.Info("Creating monitor instance")
	instances, err := c.createBaseInstance(system, inventory, logger, backendType, agiFirewallName)
	if err != nil {
		return nil, fmt.Errorf("failed to create base instance: %w", err)
	}

	if len(instances) == 0 {
		return nil, errors.New("no instances were created")
	}

	instance := instances[0]

	// Configure Route53 DNS if specified (AWS only)
	if backendType == "aws" && c.AWS.Route53ZoneId != "" {
		logger.Info("Configuring Route53 DNS")
		err := c.configureRoute53(system, inventory, logger, instance)
		if err != nil {
			logger.Warn("Failed to configure Route53: %s", err)
		}
	}

	// Install aerolab binary on the instance
	logger.Info("Installing aerolab binary")
	err = c.installAerolab(system, logger, instance)
	if err != nil {
		return nil, fmt.Errorf("failed to install aerolab: %w", err)
	}

	// Upload configuration and systemd unit file
	logger.Info("Installing monitor configuration and systemd service")
	err = c.installService(system, logger, instance, agiConfigYaml, backendType)
	if err != nil {
		return nil, fmt.Errorf("failed to install service: %w", err)
	}

	// Start the monitor service
	logger.Info("Starting agimonitor service")
	err = c.startService(system, logger, instance)
	if err != nil {
		return nil, fmt.Errorf("failed to start service: %w", err)
	}

	// Display access information
	c.displayAccessInfo(system, logger, instance, backendType)

	return instances, nil
}

// ensureAGIFirewall ensures the AGI firewall exists for the specified VPC, creating it if necessary.
// The firewall allows inbound TCP traffic on ports 80 (HTTP) and 443 (HTTPS) from anywhere.
// This function handles race conditions gracefully - if another process creates the firewall
// concurrently, the "already exists" error is ignored.
//
// The firewall name is VPC-specific:
//   - AWS: AEROLAB_AGI_{project}_{vpc-id}
//   - GCP: aerolab-agi-{vpc-name} (sanitized)
//
// Returns:
//   - string: The firewall name that was created or found
//   - error: nil on success, or an error describing what failed
func (c *AgiMonitorCreateCmd) ensureAGIFirewall(system *System, inventory *backends.Inventory, logger *logger.Logger, backendType string) (string, error) {
	// Get the default network (VPC)
	networks := inventory.Networks.WithDefault(true)
	if networks == nil || networks.Count() == 0 {
		return "", fmt.Errorf("no default network found for firewall creation")
	}
	vpc := networks.Describe()[0]

	// Generate VPC-specific firewall name based on backend type
	var firewallName string
	switch backendType {
	case "aws":
		// AWS: AEROLAB_AGI_{project}_{vpc-id}
		// Use AEROLAB_PROJECT env var (aerolab project), not Backend.Project (GCP project)
		project := os.Getenv("AEROLAB_PROJECT")
		if project == "" {
			project = "default"
		}
		firewallName = "AEROLAB_AGI_" + project + "_" + vpc.NetworkId
	case "gcp":
		// GCP: aerolab-agi-{vpc-name} (sanitized)
		firewallName = sanitizeGCPName("aerolab-agi-" + vpc.Name)
	default:
		return "", fmt.Errorf("unsupported backend type for firewall: %s", backendType)
	}

	// Check if firewall already exists
	fws := inventory.Firewalls.WithName(firewallName)
	if fws.Count() > 0 {
		logger.Debug("Firewall %s already exists", firewallName)
		return firewallName, nil
	}

	logger.Info("Creating %s firewall rule for AGI access (ports 80, 443)", firewallName)

	// Create firewall rule for ports 80 and 443
	_, err := system.Backend.CreateFirewall(&backends.CreateFirewallInput{
		BackendType: backends.BackendType(backendType),
		Name:        firewallName,
		Description: "AeroLab AGI access (ports 80, 443)",
		Owner:       c.Owner,
		Ports: []*backends.Port{
			{FromPort: 80, ToPort: 80, SourceCidr: "0.0.0.0/0", Protocol: backends.ProtocolTCP},
			{FromPort: 443, ToPort: 443, SourceCidr: "0.0.0.0/0", Protocol: backends.ProtocolTCP},
		},
		Network: vpc,
	}, time.Minute)
	if err != nil {
		// Handle race condition - if firewall was created by another process, that's fine
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "AlreadyExists") || strings.Contains(err.Error(), "InvalidGroup.Duplicate") {
			logger.Debug("Firewall %s was created by another process, continuing", firewallName)
			return firewallName, nil
		}
		return "", fmt.Errorf("failed to create %s firewall: %w", firewallName, err)
	}

	logger.Info("Firewall %s created successfully", firewallName)
	return firewallName, nil
}

// createBaseInstance creates the base client instance for the monitor.
// The agiFirewallName parameter is the VPC-specific firewall name for cloud backends.
func (c *AgiMonitorCreateCmd) createBaseInstance(system *System, inventory *backends.Inventory, logger *logger.Logger, backendType string, agiFirewallName string) (backends.InstanceList, error) {
	// Build the create command
	createCmd := &ClientCreateNoneCmd{
		ClientName:         TypeClientName(c.Name),
		ClientCount:        1,
		OS:                 "ubuntu",
		Version:            "24.04",
		Owner:              c.Owner,
		TypeOverride:       "agimonitor",
		ParallelSSHThreads: 10,
	}

	// Set backend-specific options
	switch backendType {
	case "aws":
		// Use the VPC-specific AGI firewall name
		firewalls := []string{agiFirewallName}
		// Add any additional user-specified security groups
		if c.AWS.SecurityGroupID != "" {
			firewalls = append(firewalls, strings.Split(c.AWS.SecurityGroupID, ",")...)
		}
		// Only apply instance role if AWS credentials are not provided
		instanceRole := c.AWS.InstanceRole
		if c.AWS.AWSKeyId != "" && c.AWS.AWSSecretKey != "" {
			instanceRole = ""
		}
		createCmd.AWS = InstancesCreateCmdAws{
			InstanceType:       c.AWS.InstanceType,
			Firewalls:          firewalls,
			IAMInstanceProfile: instanceRole,
			Disks:              []string{"type=gp2,size=20"}, // 20GB for monitor
			Expire:             c.AWS.Expires,
			NetworkPlacement:   c.AWS.SubnetID,
		}
	case "gcp":
		instanceRole := c.GCP.InstanceRole
		if c.DisablePricingAPI {
			instanceRole = c.GCP.InstanceRole + "::nopricing"
		}
		// Use the VPC-specific AGI firewall name
		firewalls := []string{agiFirewallName}
		createCmd.GCP = InstancesCreateCmdGcp{
			InstanceType:       c.GCP.InstanceType,
			Zone:               c.GCP.Zone,
			Firewalls:          firewalls,
			IAMInstanceProfile: instanceRole,
			Disks:              []string{"type=pd-ssd,size=20"}, // 20GB for monitor
			Expire:             c.GCP.Expires,
		}
	}

	// Create the instance
	instances, err := createCmd.createNoneClient(system, inventory, logger, nil, false)
	if err != nil {
		return nil, err
	}

	// Associate Elastic IP if specified (AWS only)
	if backendType == "aws" && c.AWS.ElasticIP != "" && len(instances) > 0 {
		logger.Info("Associating Elastic IP with monitor instance")
		elasticIP, err := c.associateElasticIP(system, logger, instances[0])
		if err != nil {
			return nil, fmt.Errorf("failed to associate Elastic IP: %w", err)
		}
		// Update the instance's public IP in memory so subsequent operations use the correct IP
		instances[0].IP.Public = elasticIP
		logger.Info("Elastic IP %s associated with instance %s", elasticIP, instances[0].InstanceID)
	}

	// Add Route53 tags if AWS and configured
	if backendType == "aws" && c.AWS.Route53ZoneId != "" && len(instances) > 0 {
		// Get the hosted zone's domain name for proper tag setup
		dnsName, domainName, err := c.getRoute53DomainInfo(system, logger)
		if err != nil {
			logger.Warn("Failed to get Route53 domain info: %s", err)
			// Fall back to basic tags
			err = instances.AddTags(map[string]string{
				"agimUrl":  c.AWS.Route53DomainName,
				"agimZone": c.AWS.Route53ZoneId,
			})
			if err != nil {
				logger.Warn("Failed to add Route53 tags: %s", err)
			}
		} else {
			// Add both AGI-specific tags and standard DNS tags for proper cleanup
			region := instances[0].ZoneID
			err = instances.AddTags(map[string]string{
				"agimUrl":                 c.AWS.Route53DomainName,
				"agimZone":                c.AWS.Route53ZoneId,
				"AEROLAB_DNS_NAME":        dnsName,
				"AEROLAB_DNS_DOMAIN_ID":   c.AWS.Route53ZoneId,
				"AEROLAB_DNS_DOMAIN_NAME": domainName,
				"AEROLAB_DNS_REGION":      region,
			})
			if err != nil {
				logger.Warn("Failed to add Route53 tags: %s", err)
			}
		}
	}

	return instances, nil
}

// associateElasticIP associates a pre-allocated Elastic IP with an EC2 instance.
// This is useful when DNS is already configured to point to a specific Elastic IP,
// allowing autocert to work immediately after instance creation.
//
// The elasticIP parameter can be either:
//   - An allocation ID (e.g., "eipalloc-0123456789abcdef0")
//   - An IP address (e.g., "1.2.3.4")
//
// Parameters:
//   - system: The system context containing backend and configuration
//   - logger: Logger for output
//   - instance: The instance to associate the Elastic IP with
//
// Returns:
//   - string: The public IP address of the Elastic IP
//   - error: nil on success, or an error describing what failed
func (c *AgiMonitorCreateCmd) associateElasticIP(system *System, logger *logger.Logger, instance *backends.Instance) (string, error) {
	// Get the region from the instance
	region := instance.ZoneID
	if region == "" {
		return "", errors.New("instance has no region/zone information")
	}

	// Get EC2 client
	ec2Client, err := baws.GetEc2Client(system.Backend.GetCredentials(), &region)
	if err != nil {
		return "", fmt.Errorf("failed to get EC2 client: %w", err)
	}

	elasticIPValue := c.AWS.ElasticIP
	var allocationID string
	var publicIP string

	// Determine if the input is an allocation ID or an IP address
	if strings.HasPrefix(elasticIPValue, "eipalloc-") {
		// It's an allocation ID
		allocationID = elasticIPValue
	} else {
		// Assume it's an IP address - look up the allocation ID
		describeResult, err := ec2Client.DescribeAddresses(context.Background(), &ec2.DescribeAddressesInput{
			PublicIps: []string{elasticIPValue},
		})
		if err != nil {
			return "", fmt.Errorf("failed to find Elastic IP %s: %w", elasticIPValue, err)
		}
		if len(describeResult.Addresses) == 0 {
			return "", fmt.Errorf("Elastic IP %s not found in region %s", elasticIPValue, region)
		}
		allocationID = aws.ToString(describeResult.Addresses[0].AllocationId)
		publicIP = elasticIPValue
		if allocationID == "" {
			return "", fmt.Errorf("Elastic IP %s has no allocation ID (EC2-Classic is not supported)", elasticIPValue)
		}
	}

	// Associate the Elastic IP with the instance
	_, err = ec2Client.AssociateAddress(context.Background(), &ec2.AssociateAddressInput{
		AllocationId: aws.String(allocationID),
		InstanceId:   aws.String(instance.InstanceID),
	})
	if err != nil {
		return "", fmt.Errorf("failed to associate Elastic IP %s with instance %s: %w", elasticIPValue, instance.InstanceID, err)
	}

	// If we don't have the public IP yet (input was allocation ID), get it now
	if publicIP == "" {
		describeResult, err := ec2Client.DescribeAddresses(context.Background(), &ec2.DescribeAddressesInput{
			AllocationIds: []string{allocationID},
		})
		if err != nil {
			return "", fmt.Errorf("failed to describe Elastic IP %s: %w", allocationID, err)
		}
		if len(describeResult.Addresses) == 0 {
			return "", fmt.Errorf("Elastic IP %s not found", allocationID)
		}
		publicIP = aws.ToString(describeResult.Addresses[0].PublicIp)
		if publicIP == "" {
			return "", fmt.Errorf("Elastic IP %s has no public IP address", allocationID)
		}
	}

	return publicIP, nil
}

// getRoute53DomainInfo queries Route53 to get the hosted zone's domain name
// and computes the DNS record name (subdomain) from the FQDN.
//
// Returns:
//   - dnsName: The subdomain part (e.g., "monitor" from "monitor.example.com")
//   - domainName: The zone's domain name (e.g., "example.com")
//   - error: nil on success, or an error describing what failed
func (c *AgiMonitorCreateCmd) getRoute53DomainInfo(system *System, logger *logger.Logger) (dnsName string, domainName string, err error) {
	// Get a region for the Route53 client
	regions, err := system.Backend.ListEnabledRegions(backends.BackendTypeAWS)
	if err != nil || len(regions) == 0 {
		return "", "", fmt.Errorf("failed to get AWS region: %w", err)
	}
	region := regions[0]

	// Get Route53 client
	cli, err := baws.GetRoute53Client(system.Backend.GetCredentials(), &region)
	if err != nil {
		return "", "", fmt.Errorf("failed to get Route53 client: %w", err)
	}

	// Get the hosted zone to determine its domain name
	hostedZone, err := cli.GetHostedZone(context.Background(), &route53.GetHostedZoneInput{
		Id: aws.String(c.AWS.Route53ZoneId),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to get hosted zone: %w", err)
	}

	// The hosted zone name includes a trailing dot, remove it
	domainName = strings.TrimSuffix(aws.ToString(hostedZone.HostedZone.Name), ".")

	// Compute the subdomain by removing the domain from the FQDN
	fqdn := strings.TrimSuffix(c.AWS.Route53DomainName, ".")
	if strings.HasSuffix(fqdn, "."+domainName) {
		dnsName = strings.TrimSuffix(fqdn, "."+domainName)
	} else if fqdn == domainName {
		// The FQDN is the domain itself (apex record)
		dnsName = ""
	} else {
		// The FQDN doesn't match the zone - use the full FQDN as the name
		logger.Warn("FQDN %s doesn't appear to be in zone %s", fqdn, domainName)
		dnsName = fqdn
	}

	return dnsName, domainName, nil
}

// configureRoute53 configures Route53 DNS for the monitor (AWS only).
// It creates an A record pointing the specified domain to the instance's public IP.
//
// Parameters:
//   - system: The system context containing backend and configuration
//   - inventory: The current inventory state (unused but kept for consistency)
//   - logger: Logger for output
//   - instance: The instance to configure DNS for
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiMonitorCreateCmd) configureRoute53(system *System, inventory *backends.Inventory, logger *logger.Logger, instance *backends.Instance) error {
	// Get the instance's public IP
	publicIP := instance.IP.Public
	if publicIP == "" {
		return errors.New("instance has no public IP for Route53 configuration")
	}

	// Get the first available region for Route53 client (Route53 is a global service)
	region := instance.ZoneID
	if region == "" {
		regions, err := system.Backend.ListEnabledRegions(backends.BackendTypeAWS)
		if err != nil || len(regions) == 0 {
			return fmt.Errorf("failed to get AWS region for Route53 client: %w", err)
		}
		region = regions[0]
	}

	// Get Route53 client using the exported function from baws package
	cli, err := baws.GetRoute53Client(system.Backend.GetCredentials(), &region)
	if err != nil {
		return fmt.Errorf("failed to get Route53 client: %w", err)
	}

	// Tag the hosted zone for aerolab tracking (for expiry cleanup)
	// Use AEROLAB_PROJECT env var (aerolab project), not Backend.Project (GCP project)
	aerolabProject := os.Getenv("AEROLAB_PROJECT")
	if aerolabProject == "" {
		aerolabProject = "default"
	}
	_, err = cli.ChangeTagsForResource(context.Background(), &route53.ChangeTagsForResourceInput{
		ResourceType: rtypes.TagResourceTypeHostedzone,
		ResourceId:   aws.String(c.AWS.Route53ZoneId),
		AddTags: []rtypes.Tag{
			{Key: aws.String(baws.TAG_AEROLAB_PROJECT), Value: aws.String(aerolabProject)},
		},
	})
	if err != nil {
		logger.Warn("Failed to add tags to hosted zone (expiry cleanup may not work): %s", err)
	}

	// Create A record pointing domain to instance IP
	// Use UPSERT to create or update the record
	change, err := cli.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(c.AWS.Route53ZoneId),
		ChangeBatch: &rtypes.ChangeBatch{
			Changes: []rtypes.Change{
				{
					Action: rtypes.ChangeActionUpsert,
					ResourceRecordSet: &rtypes.ResourceRecordSet{
						Name: aws.String(c.AWS.Route53DomainName),
						Type: rtypes.RRTypeA,
						TTL:  aws.Int64(60),
						ResourceRecords: []rtypes.ResourceRecord{
							{Value: aws.String(publicIP)},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create Route53 A record: %w", err)
	}

	// Wait for the DNS change to propagate (with timeout)
	logger.Info("Waiting for DNS record to propagate: %s -> %s", c.AWS.Route53DomainName, publicIP)
	waiter := route53.NewResourceRecordSetsChangedWaiter(cli, func(o *route53.ResourceRecordSetsChangedWaiterOptions) {
		o.MinDelay = 5 * time.Second
		o.MaxDelay = 10 * time.Second
	})
	err = waiter.Wait(context.Background(), &route53.GetChangeInput{
		Id: change.ChangeInfo.Id,
	}, 2*time.Minute)
	if err != nil {
		logger.Warn("DNS record created but propagation wait timed out: %s", err)
		// Don't return error - the record was created, just the wait timed out
	}

	logger.Info("Route53 DNS record created: %s -> %s", c.AWS.Route53DomainName, publicIP)
	return nil
}

// installAerolab installs the aerolab binary on the monitor instance.
func (c *AgiMonitorCreateCmd) installAerolab(system *System, logger *logger.Logger, instance *backends.Instance) error {
	// Check if running unofficial build
	_, _, edition, currentAerolabVersion := GetAerolabVersion()
	isUnofficial := strings.Contains(edition, "unofficial")
	useLocalBinary := c.AerolabBinary != ""

	if isUnofficial && !useLocalBinary {
		return fmt.Errorf("running unofficial aerolab build (%s); --aerolab-binary flag is required to specify the path to a Linux aerolab binary", currentAerolabVersion)
	}

	// Get SFTP connection
	conf, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	// Create SFTP client
	sftpClient, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	if useLocalBinary {
		// Upload local binary directly
		logger.Info("Uploading local aerolab binary: %s", c.AerolabBinary)
		binaryData, err := os.ReadFile(string(c.AerolabBinary))
		if err != nil {
			return fmt.Errorf("failed to read local aerolab binary: %w", err)
		}

		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/usr/local/bin/aerolab",
			Source:      bytes.NewReader(binaryData),
			Permissions: 0755,
		})
		if err != nil {
			return fmt.Errorf("failed to upload aerolab binary: %w", err)
		}
	} else {
		// Get the install script for aerolab (same version as the user is running)
		installScript, err := aerolab.GetLinuxInstallScript(currentAerolabVersion, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to get aerolab install script: %w", err)
		}

		// Upload install script
		now := time.Now().Format("20060102150405")
		scriptPath := "/tmp/install-aerolab.sh." + now
		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    scriptPath,
			Source:      bytes.NewReader(installScript),
			Permissions: 0755,
		})
		if err != nil {
			return fmt.Errorf("failed to upload aerolab install script: %w", err)
		}

		// Execute install script
		var stdout, stderr *os.File
		if system.logLevel >= 5 {
			stdout = os.Stdout
			stderr = os.Stderr
		}
		output := instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", scriptPath},
				Stdout:         stdout,
				Stderr:         stderr,
				SessionTimeout: 10 * time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			return fmt.Errorf("failed to install aerolab: %w (stdout: %s, stderr: %s)",
				output.Output.Err, output.Output.Stdout, output.Output.Stderr)
		}
	}

	return nil
}

// installService installs the monitor configuration and systemd service.
func (c *AgiMonitorCreateCmd) installService(system *System, logger *logger.Logger, instance *backends.Instance, agiConfigYaml []byte, backendType string) error {
	// Get SFTP connection
	conf, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	// Create SFTP client
	sftpClient, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Build environment variables section for systemd
	envVars := "Environment=AEROLAB_SKIP_DOWNGRADE=1\n"

	// Add AWS credentials as environment variables if specified
	if backendType == "aws" && c.AWS.AWSKeyId != "" && c.AWS.AWSSecretKey != "" {
		envVars += fmt.Sprintf("Environment=AWS_ACCESS_KEY_ID=%s\n", c.AWS.AWSKeyId)
		envVars += fmt.Sprintf("Environment=AWS_SECRET_ACCESS_KEY=%s\n", c.AWS.AWSSecretKey)
		logger.Debug("AWS credentials configured via environment variables in systemd service")
	}

	// Generate systemd unit file
	var systemdUnit string
	if backendType == "gcp" {
		systemdUnit = fmt.Sprintf(`[Unit]
Description=AeroLab AGI Monitor
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
%sExecStartPre=/usr/local/bin/aerolab config backend -t gcp -o %s
ExecStart=/usr/local/bin/aerolab agi monitor listen

[Install]
WantedBy=multi-user.target
`, envVars, system.Opts.Config.Backend.Project)
	} else {
		systemdUnit = fmt.Sprintf(`[Unit]
Description=AeroLab AGI Monitor
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
%sExecStartPre=/usr/local/bin/aerolab config backend -t aws -r %s
ExecStart=/usr/local/bin/aerolab agi monitor listen

[Install]
WantedBy=multi-user.target
`, envVars, system.Opts.Config.Backend.Region)
	}

	// Upload systemd unit file
	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/usr/lib/systemd/system/agimonitor.service",
		Source:      strings.NewReader(systemdUnit),
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to upload systemd unit file: %w", err)
	}

	// Upload config file
	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/etc/agimonitor.yaml",
		Source:      bytes.NewReader(agiConfigYaml),
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to upload config file: %w", err)
	}

	return nil
}

// startService starts and enables the monitor service.
func (c *AgiMonitorCreateCmd) startService(system *System, logger *logger.Logger, instance *backends.Instance) error {
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"systemctl", "enable", "--now", "agimonitor"},
			SessionTimeout: 2 * time.Minute,
		},
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})

	if output.Output.Err != nil {
		return fmt.Errorf("failed to start agimonitor service: %w (stdout: %s, stderr: %s)",
			output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	return nil
}

// displayAccessInfo displays the monitor access information.
func (c *AgiMonitorCreateCmd) displayAccessInfo(system *System, logger *logger.Logger, instance *backends.Instance, backendType string) {
	// Determine access URL
	var accessURL string
	if backendType == "aws" && c.AWS.Route53DomainName != "" {
		if c.NoTLS {
			accessURL = fmt.Sprintf("http://%s", c.AWS.Route53DomainName)
		} else {
			accessURL = fmt.Sprintf("https://%s", c.AWS.Route53DomainName)
		}
	} else {
		ip := instance.IP.Public
		if ip == "" {
			ip = instance.IP.Private
		}
		if c.NoTLS {
			accessURL = fmt.Sprintf("http://%s", ip)
		} else {
			accessURL = fmt.Sprintf("https://%s", ip)
		}
	}

	logger.Info("")
	logger.Info("=== AGI Monitor Created Successfully ===")
	logger.Info("")
	logger.Info("Monitor Name:    %s", c.Name)
	logger.Info("Instance ID:     %s", instance.InstanceID)
	logger.Info("Access URL:      %s", accessURL)
	logger.Info("")
	logger.Info("Useful commands:")
	logger.Info("  Check status:  aerolab client list")
	logger.Info("  View logs:     aerolab attach client -n %s -- journalctl -u agimonitor -f", c.Name)
	logger.Info("  Destroy:       aerolab client destroy -n %s -f", c.Name)
	logger.Info("")
}
