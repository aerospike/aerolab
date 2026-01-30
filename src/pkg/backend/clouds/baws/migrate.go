package baws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	etypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
)

// MigrateV7Resources discovers and migrates v7 AWS resources (instances, volumes, images)
// to the v8 tagging format.
//
// Parameters:
//   - input: Migration configuration including project name, dry-run mode, and SSH key paths
//
// Returns:
//   - *backends.MigrationResult: Details of the migration including counts and any errors
//   - error: nil on success, or an error if the migration could not be performed
func (s *b) MigrateV7Resources(input *backends.MigrateV7Input) (*backends.MigrationResult, error) {
	log := s.log.WithPrefix("MigrateV7Resources: job=" + shortuuid.New() + " ")
	log.Detail("Start dry_run=%v force=%v", input.DryRun, input.Force)
	defer log.Detail("End")

	result := &backends.MigrationResult{DryRun: input.DryRun}
	migratedKeys := make(map[string]bool) // Track keys to avoid duplicate copies

	// Group instances by cluster+region for consistent UUID assignment
	clusterUUIDs := make(map[string]string) // key: "clusterName_region"

	// 1. Discover and migrate instances
	log.Detail("Discovering v7 instances...")
	oldInstances, err := s.discoverOldInstances(input.Force)
	if err != nil {
		return nil, fmt.Errorf("failed to discover v7 instances: %w", err)
	}
	log.Detail("Found %d v7 instances", len(oldInstances))

	// Process instances
	for _, inst := range oldInstances {
		detail := backends.MigrationInstanceDetail{
			InstanceID:  inst.InstanceID,
			Name:        inst.Name,
			ClusterName: inst.ClusterName,
			NodeNo:      inst.NodeNo,
			Zone:        inst.Zone,
			IsClient:    inst.IsClient,
		}

		// Get or create UUID for this cluster+region
		clusterKey := fmt.Sprintf("%s_%s", inst.ClusterName, inst.Zone)
		if _, ok := clusterUUIDs[clusterKey]; !ok {
			clusterUUIDs[clusterKey] = uuid.New().String()
		}
		clusterUUID := clusterUUIDs[clusterKey]

		// Translate tags
		var newTags map[string]string
		if inst.IsClient {
			newTags = s.translateClientTags(inst, input.Project, clusterUUID, input.AerolabVersion)
		} else {
			newTags = s.translateServerTags(inst, input.Project, clusterUUID, input.AerolabVersion)
		}
		detail.TagsToAdd = newTags

		// Calculate SSH key paths
		if input.SSHKeyInfo != nil && input.SSHKeyInfo.SharedKeyPath == "" {
			oldKeyName := fmt.Sprintf("aerolab-%s_%s", inst.ClusterName, inst.Zone)
			oldKeyPath := filepath.Join(input.SSHKeyInfo.KeysDir, oldKeyName)
			newKeyPath := filepath.Join(s.sshKeysDir, "old", oldKeyName)
			detail.SSHKeyFrom = oldKeyPath
			detail.SSHKeyTo = newKeyPath
		}

		if input.DryRun {
			detail.MigrationStatus = "pending"
			result.DryRunInstances = append(result.DryRunInstances, detail)
		} else {
			// Apply tags
			err := s.applyInstanceTags(inst.InstanceID, inst.Zone, newTags)
			if err != nil {
				detail.MigrationStatus = "failed"
				detail.MigrationError = err.Error()
				result.Errors = append(result.Errors, fmt.Errorf("instance %s: %w", inst.InstanceID, err))
			} else {
				detail.MigrationStatus = "success"
				detail.TagsAdded = newTags
				result.InstancesMigrated++

				// Migrate SSH key if not already done
				if detail.SSHKeyFrom != "" && !migratedKeys[detail.SSHKeyFrom] {
					if err := s.migrateSSHKey(detail.SSHKeyFrom, detail.SSHKeyTo); err != nil {
						log.Warn("Failed to migrate SSH key for cluster %s: %s", inst.ClusterName, err)
					} else {
						detail.SSHKeyMigrated = true
						migratedKeys[detail.SSHKeyFrom] = true
						result.SSHKeysMigrated++
					}
				}
			}
			result.MigratedInstances = append(result.MigratedInstances, detail)
		}

		// Discover and tag volumes attached to this instance
		attachedVolumes, err := s.discoverAttachedVolumes(inst.InstanceID, inst.Zone, input.Force)
		if err != nil {
			log.Warn("Failed to discover attached volumes for instance %s: %s", inst.InstanceID, err)
		} else {
			for _, vol := range attachedVolumes {
				volDetail := backends.MigrationVolumeDetail{
					VolumeID:            vol.VolumeID,
					VolumeType:          vol.VolumeType,
					Name:                vol.Name,
					Zone:                vol.Zone,
					AttachedToInstance:  inst.InstanceID,
					DeleteOnTermination: vol.DeleteOnTermination,
				}

				// Use instance tags for attached volumes
				volTags := s.translateAttachedVolumeTags(inst, vol, input.Project, clusterUUID, input.AerolabVersion)
				volDetail.TagsToAdd = volTags

				if input.DryRun {
					volDetail.MigrationStatus = "pending"
					result.DryRunVolumes = append(result.DryRunVolumes, volDetail)
				} else {
					err := s.applyVolumeTags(vol.VolumeID, vol.Zone, volTags)
					if err != nil {
						volDetail.MigrationStatus = "failed"
						volDetail.MigrationError = err.Error()
						result.Errors = append(result.Errors, fmt.Errorf("attached volume %s: %w", vol.VolumeID, err))
					} else {
						volDetail.MigrationStatus = "success"
						volDetail.TagsAdded = volTags
						result.VolumesMigrated++
					}
					result.MigratedVolumes = append(result.MigratedVolumes, volDetail)
				}
			}
		}
	}

	// 2. Discover and migrate standalone volumes (UsedBy = aerolab7)
	log.Detail("Discovering v7 standalone volumes...")
	oldVolumes, err := s.discoverOldVolumes(input.Force)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to discover v7 volumes: %w", err))
	} else {
		log.Detail("Found %d v7 volumes", len(oldVolumes))
		for _, vol := range oldVolumes {
			detail := backends.MigrationVolumeDetail{
				VolumeID:   vol.VolumeID,
				VolumeType: vol.VolumeType,
				Name:       vol.Name,
				Zone:       vol.Zone,
			}

			newTags := s.translateVolumeTags(vol, input.Project, input.AerolabVersion)
			detail.TagsToAdd = newTags

			if input.DryRun {
				detail.MigrationStatus = "pending"
				result.DryRunVolumes = append(result.DryRunVolumes, detail)
			} else {
				err := s.applyVolumeTags(vol.VolumeID, vol.Zone, newTags)
				if err != nil {
					detail.MigrationStatus = "failed"
					detail.MigrationError = err.Error()
					result.Errors = append(result.Errors, fmt.Errorf("volume %s: %w", vol.VolumeID, err))
				} else {
					detail.MigrationStatus = "success"
					detail.TagsAdded = newTags
					result.VolumesMigrated++
				}
				result.MigratedVolumes = append(result.MigratedVolumes, detail)
			}
		}
	}

	// 3. Discover and migrate images
	log.Detail("Discovering v7 images...")
	oldImages, err := s.discoverOldImages(input.Force)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to discover v7 images: %w", err))
	} else {
		log.Detail("Found %d v7 images", len(oldImages))
		for _, img := range oldImages {
			detail := backends.MigrationImageDetail{
				ImageID: img.ImageID,
				Name:    img.Name,
				Zone:    img.Zone,
			}

			// Parse image name to extract OS, version, arch
			osName, osVersion, asVersion, arch := parseV7ImageName(img.Name)
			detail.OSName = osName
			detail.OSVersion = osVersion
			detail.AerospikeVersion = asVersion
			detail.Architecture = arch

			newTags := s.translateImageTags(img, input.Project, input.AerolabVersion, osName, osVersion, asVersion, arch)
			detail.TagsToAdd = newTags

			if input.DryRun {
				detail.MigrationStatus = "pending"
				result.DryRunImages = append(result.DryRunImages, detail)
			} else {
				err := s.applyImageTags(img.ImageID, img.Zone, newTags)
				if err != nil {
					detail.MigrationStatus = "failed"
					detail.MigrationError = err.Error()
					result.Errors = append(result.Errors, fmt.Errorf("image %s: %w", img.ImageID, err))
				} else {
					detail.MigrationStatus = "success"
					detail.TagsAdded = newTags
					result.ImagesMigrated++
				}
				result.MigratedImages = append(result.MigratedImages, detail)
			}
		}
	}

	// 4. Discover and migrate firewalls (security groups)
	log.Detail("Discovering v7 firewalls...")
	oldFirewalls, err := s.discoverOldFirewalls(input.Force)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to discover v7 firewalls: %w", err))
	} else {
		log.Detail("Found %d v7 firewalls", len(oldFirewalls))
		for _, fw := range oldFirewalls {
			detail := backends.MigrationFirewallDetail{
				FirewallID: fw.FirewallID,
				Name:       fw.Name,
				Zone:       fw.Zone,
				VPCID:      fw.VPCID,
			}

			newTags := s.translateFirewallTags(fw, input.Project, input.AerolabVersion)
			detail.TagsToAdd = newTags

			if input.DryRun {
				detail.MigrationStatus = "pending"
				result.DryRunFirewalls = append(result.DryRunFirewalls, detail)
			} else {
				err := s.applyFirewallTags(fw.FirewallID, fw.Zone, newTags)
				if err != nil {
					detail.MigrationStatus = "failed"
					detail.MigrationError = err.Error()
					result.Errors = append(result.Errors, fmt.Errorf("firewall %s: %w", fw.FirewallID, err))
				} else {
					detail.MigrationStatus = "success"
					detail.TagsAdded = newTags
					result.FirewallsMigrated++
				}
				result.MigratedFirewalls = append(result.MigratedFirewalls, detail)
			}
		}
	}

	// Build SSH key migration details for dry-run
	if input.DryRun && input.SSHKeyInfo != nil && input.SSHKeyInfo.SharedKeyPath == "" {
		// Collect unique SSH key details from instances
		seenKeys := make(map[string]bool)
		for _, inst := range result.DryRunInstances {
			if inst.SSHKeyFrom != "" && !seenKeys[inst.SSHKeyFrom] {
				seenKeys[inst.SSHKeyFrom] = true
				detail := backends.MigrationSSHKeyDetail{
					ClusterName: inst.ClusterName,
					Region:      inst.Zone,
					FromPath:    inst.SSHKeyFrom,
					ToPath:      inst.SSHKeyTo,
				}
				// Check if source exists
				if _, err := os.Stat(inst.SSHKeyFrom); err != nil {
					detail.Error = "source file not found"
				}
				result.DryRunSSHKeys = append(result.DryRunSSHKeys, detail)
			}
		}
	}

	return result, nil
}

// discoverOldInstances finds v7 instances across all enabled regions
func (s *b) discoverOldInstances(force bool) ([]backends.OldInstance, error) {
	var instances []backends.OldInstance
	lock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	var errs error

	zones, _ := s.ListEnabledZones()
	for _, zone := range zones {
		wg.Add(1)
		go func(zone string) {
			defer wg.Done()
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				lock.Lock()
				errs = errors.Join(errs, err)
				lock.Unlock()
				return
			}

			// Filter for v7 instances (UsedBy = aerolab4 or aerolab4client)
			listFilters := []types.Filter{
				{
					Name:   aws.String("tag:" + V7_TAG_USED_BY),
					Values: []string{V7_TAG_SERVER_MARKER, V7_TAG_CLIENT_MARKER},
				},
				{
					Name:   aws.String("instance-state-name"),
					Values: []string{"pending", "running", "stopping", "stopped"},
				},
			}

			paginator := ec2.NewDescribeInstancesPaginator(cli, &ec2.DescribeInstancesInput{
				Filters: listFilters,
			})

			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					lock.Lock()
					errs = errors.Join(errs, err)
					lock.Unlock()
					return
				}
				for _, res := range out.Reservations {
					for _, inst := range res.Instances {
						// Convert tags to map
						tags := make(map[string]string)
						for _, t := range inst.Tags {
							tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
						}

						// Skip already migrated (unless --force)
						if !force && tags[TAG_V7_MIGRATED] == "true" {
							continue
						}

						// Determine if client or server
						isClient := tags[V7_TAG_USED_BY] == V7_TAG_CLIENT_MARKER

						var clusterName string
						var nodeNo int
						if isClient {
							clusterName = tags[V7_TAG_CLIENT_CLUSTER_NAME]
							nodeNo, _ = strconv.Atoi(tags[V7_TAG_CLIENT_NODE_NUMBER])
						} else {
							clusterName = tags[V7_TAG_CLUSTER_NAME]
							nodeNo, _ = strconv.Atoi(tags[V7_TAG_NODE_NUMBER])
						}

						lock.Lock()
						instances = append(instances, backends.OldInstance{
							InstanceID:  aws.ToString(inst.InstanceId),
							Name:        tags[TAG_NAME],
							ClusterName: clusterName,
							NodeNo:      nodeNo,
							Zone:        zone,
							IsClient:    isClient,
							Tags:        tags,
						})
						lock.Unlock()
					}
				}
			}
		}(zone)
	}
	wg.Wait()

	return instances, errs
}

// discoverOldVolumes finds v7 standalone EBS volumes and EFS file systems
func (s *b) discoverOldVolumes(force bool) ([]backends.OldVolume, error) {
	var volumes []backends.OldVolume
	lock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	var errs error

	zones, _ := s.ListEnabledZones()
	for _, zone := range zones {
		// Discover EBS volumes
		wg.Add(1)
		go func(zone string) {
			defer wg.Done()
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				lock.Lock()
				errs = errors.Join(errs, err)
				lock.Unlock()
				return
			}

			// Filter for v7 volumes (UsedBy = aerolab7)
			listFilters := []types.Filter{
				{
					Name:   aws.String("tag:" + V7_TAG_USED_BY),
					Values: []string{V7_TAG_VOLUME_MARKER},
				},
			}

			paginator := ec2.NewDescribeVolumesPaginator(cli, &ec2.DescribeVolumesInput{
				Filters: listFilters,
			})

			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					lock.Lock()
					errs = errors.Join(errs, err)
					lock.Unlock()
					return
				}
				for _, vol := range out.Volumes {
					// Convert tags to map
					tags := make(map[string]string)
					for _, t := range vol.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}

					// Skip already migrated (unless --force)
					if !force && tags[TAG_V7_MIGRATED] == "true" {
						continue
					}

					lock.Lock()
					volumes = append(volumes, backends.OldVolume{
						VolumeID:   aws.ToString(vol.VolumeId),
						Name:       tags[V7_TAG_VOLUME_NAME],
						VolumeType: "ebs:" + string(vol.VolumeType),
						Zone:       zone,
						Tags:       tags,
					})
					lock.Unlock()
				}
			}
		}(zone)

		// Discover EFS file systems
		wg.Add(1)
		go func(zone string) {
			defer wg.Done()
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				lock.Lock()
				errs = errors.Join(errs, err)
				lock.Unlock()
				return
			}

			paginator := efs.NewDescribeFileSystemsPaginator(cli, &efs.DescribeFileSystemsInput{})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					lock.Lock()
					errs = errors.Join(errs, err)
					lock.Unlock()
					return
				}
				for _, fs := range out.FileSystems {
					// Convert tags to map
					tags := make(map[string]string)
					for _, t := range fs.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}

					// Check for v7 marker (UsedBy = aerolab7)
					if tags[V7_TAG_USED_BY] != V7_TAG_VOLUME_MARKER {
						continue
					}

					// Skip already migrated (unless --force)
					if !force && tags[TAG_V7_MIGRATED] == "true" {
						continue
					}

					lock.Lock()
					volumes = append(volumes, backends.OldVolume{
						VolumeID:   aws.ToString(fs.FileSystemId),
						Name:       tags[V7_TAG_VOLUME_NAME],
						VolumeType: "efs",
						Zone:       zone,
						Tags:       tags,
					})
					lock.Unlock()
				}
			}
		}(zone)
	}
	wg.Wait()

	return volumes, errs
}

// discoverOldImages finds v7 template AMIs
func (s *b) discoverOldImages(force bool) ([]backends.OldImage, error) {
	var images []backends.OldImage
	lock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	var errs error

	zones, _ := s.ListEnabledZones()
	for _, zone := range zones {
		wg.Add(1)
		go func(zone string) {
			defer wg.Done()
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				lock.Lock()
				errs = errors.Join(errs, err)
				lock.Unlock()
				return
			}

			// Filter by name pattern: aerolab4-template-*
			listFilters := []types.Filter{
				{
					Name:   aws.String("name"),
					Values: []string{"aerolab4-template-*"},
				},
			}

			paginator := ec2.NewDescribeImagesPaginator(cli, &ec2.DescribeImagesInput{
				Filters: listFilters,
				Owners:  []string{"self"},
			})

			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					lock.Lock()
					errs = errors.Join(errs, err)
					lock.Unlock()
					return
				}
				for _, img := range out.Images {
					// Convert tags to map
					tags := make(map[string]string)
					for _, t := range img.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}

					// Skip already migrated (unless --force)
					if !force && tags[TAG_V7_MIGRATED] == "true" {
						continue
					}

					lock.Lock()
					images = append(images, backends.OldImage{
						ImageID: aws.ToString(img.ImageId),
						Name:    aws.ToString(img.Name),
						Zone:    zone,
						Tags:    tags,
					})
					lock.Unlock()
				}
			}
		}(zone)
	}
	wg.Wait()

	return images, errs
}

// isAGIInstance checks if a v7 instance is an AGI instance by looking for AGI-specific tags.
func isAGIInstance(tags map[string]string) bool {
	agiIndicators := []string{
		V7_TAG_AGI_AV,        // aerolab7agiav
		V7_TAG_AGI_LABEL,     // agiLabel
		V7_TAG_AGI_INSTANCE,  // agiinstance
		V7_TAG_AGI_NODIM,     // aginodim
		V7_TAG_AGI_SRC_LOCAL, // agiSrcLocal
		V7_TAG_AGI_SRC_SFTP,  // agiSrcSftp
		V7_TAG_AGI_SRC_S3,    // agiSrcS3
	}
	for _, tag := range agiIndicators {
		if tags[tag] != "" {
			return true
		}
	}
	return false
}

// translateServerTags converts v7 server instance tags to v8 format
func (s *b) translateServerTags(inst backends.OldInstance, project, clusterUUID, aerolabVersion string) map[string]string {
	// Check if this is an AGI instance
	isAGI := isAGIInstance(inst.Tags)

	tags := map[string]string{
		TAG_CLUSTER_NAME:    inst.Tags[V7_TAG_CLUSTER_NAME],
		TAG_NODE_NO:         inst.Tags[V7_TAG_NODE_NUMBER],
		TAG_OS_NAME:         inst.Tags[V7_TAG_OPERATING_SYSTEM],
		TAG_OS_VERSION:      inst.Tags[V7_TAG_OPERATING_SYS_VER],
		TAG_EXPIRES:         inst.Tags[V7_TAG_EXPIRES],
		TAG_OWNER:           inst.Tags[V7_TAG_OWNER],
		TAG_COST_PPH:        inst.Tags[V7_TAG_COST_PPH],
		TAG_COST_SO_FAR:     inst.Tags[V7_TAG_COST_SO_FAR],
		TAG_START_TIME:      inst.Tags[V7_TAG_COST_START_TIME],
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
		TAG_V7_MIGRATED:     "true",
	}

	if isAGI {
		// For AGI instances, set type to "agi" so v8 AGI commands can find them
		tags[TAG_SOFT_TYPE] = "agi"

		// Preserve AGI-specific tags WITHOUT v7- prefix so v8 can read them
		s.preserveAGITags(inst.Tags, tags)
	} else {
		// Regular server type is "aerospike"
		tags[TAG_SOFT_TYPE] = "aerospike"

		// Add v7-prefixed tags for preserved values (non-AGI)
		s.addV7PrefixedTags(inst.Tags, tags, false)
	}

	// Add software version if present
	if v := inst.Tags[V7_TAG_AEROSPIKE_VERSION]; v != "" {
		tags[TAG_SOFT_VERSION] = v
	}

	// Preserve architecture
	if v := inst.Tags[V7_TAG_ARCH]; v != "" {
		tags["v7-arch"] = v
	}

	return tags
}

// isAGIMonitor checks if a v7 client instance is an AGI Monitor by looking for monitor-specific tags.
func isAGIMonitor(tags map[string]string) bool {
	// V7 AGI Monitor has agimUrl and/or agimZone tags for Route53 configuration
	// Also check for client type "agimonitor"
	if tags["agimUrl"] != "" || tags["agimZone"] != "" {
		return true
	}
	if tags[V7_TAG_CLIENT_TYPE] == "agimonitor" {
		return true
	}
	return false
}

// translateClientTags converts v7 client instance tags to v8 format
func (s *b) translateClientTags(inst backends.OldInstance, project, clusterUUID, aerolabVersion string) map[string]string {
	// Check if this is an AGI Monitor instance
	isMonitor := isAGIMonitor(inst.Tags)

	tags := map[string]string{
		TAG_CLUSTER_NAME:    inst.Tags[V7_TAG_CLIENT_CLUSTER_NAME],
		TAG_NODE_NO:         inst.Tags[V7_TAG_CLIENT_NODE_NUMBER],
		TAG_OS_NAME:         inst.Tags[V7_TAG_CLIENT_OS],
		TAG_OS_VERSION:      inst.Tags[V7_TAG_CLIENT_OS_VER],
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
		TAG_V7_MIGRATED:     "true",
		"aerolab.old.type":  "client", // Required for v8 client commands to identify client instances
	}

	// Add owner and expires if present
	if v := inst.Tags[V7_TAG_OWNER]; v != "" {
		tags[TAG_OWNER] = v
	}
	if v := inst.Tags[V7_TAG_EXPIRES]; v != "" {
		tags[TAG_EXPIRES] = v
	}

	// Set aerolab.type from client type
	if v := inst.Tags[V7_TAG_CLIENT_TYPE]; v != "" {
		tags[TAG_SOFT_TYPE] = v
		tags[TAG_CLIENT_TYPE] = v
	}

	// Add software version if present
	if v := inst.Tags[V7_TAG_CLIENT_AS_VER]; v != "" {
		tags[TAG_SOFT_VERSION] = v
	}

	// For AGI Monitor, preserve monitor-specific tags without prefix
	if isMonitor {
		s.preserveAGIMonitorTags(inst.Tags, tags)
	}

	// Add v7-prefixed tags for preserved values
	s.addV7PrefixedTags(inst.Tags, tags, false)

	return tags
}

// preserveAGIMonitorTags preserves AGI Monitor-specific tags WITHOUT prefix.
func (s *b) preserveAGIMonitorTags(oldTags, newTags map[string]string) {
	// AGI Monitor-specific tags to preserve with their ORIGINAL names
	monitorTagsToPreserve := []string{
		"agimUrl",  // Route53 domain URL
		"agimZone", // Route53 zone ID
	}

	for _, tag := range monitorTagsToPreserve {
		if v := oldTags[tag]; v != "" {
			newTags[tag] = v
		}
	}
}

// preserveAGITags preserves AGI-specific tags WITHOUT prefix so v8 AGI commands can read them.
// This is called for AGI instances during migration.
func (s *b) preserveAGITags(oldTags, newTags map[string]string) {
	// Migrate telemetry tag to new v8 format
	if v := oldTags[V7_TAG_TELEMETRY]; v != "" {
		newTags[TAG_TELEMETRY] = v
	}

	// AGI-specific tags to preserve with their ORIGINAL names (no prefix)
	// These are needed by v8 AGI commands to read configuration
	agiTagsToPreserve := []string{
		V7_TAG_AGI_AV,        // aerolab7agiav - Aerospike version for AGI
		V7_TAG_FEATURES,      // aerolab4features - feature flags
		V7_TAG_SSL,           // aerolab4ssl - SSL enabled
		V7_TAG_AGI_LABEL,     // agiLabel - AGI label
		V7_TAG_AGI_INSTANCE,  // agiinstance - instance type
		V7_TAG_AGI_NODIM,     // aginodim - NoDIM mode
		V7_TAG_TERM_ON_POW,   // termonpow - terminate on poweroff
		V7_TAG_IS_SPOT,       // isspot - spot instance
		V7_TAG_AGI_SRC_LOCAL, // agiSrcLocal - local source
		V7_TAG_AGI_SRC_SFTP,  // agiSrcSftp - SFTP source
		V7_TAG_AGI_SRC_S3,    // agiSrcS3 - S3 source
		V7_TAG_AGI_DOMAIN,    // agiDomain - Route53 domain
	}

	for _, tag := range agiTagsToPreserve {
		if v := oldTags[tag]; v != "" {
			newTags[tag] = v
		}
	}

	// Also preserve v7-spot if present
	if v := oldTags[V7_TAG_SPOT]; v != "" {
		newTags["v7-spot"] = v
	}
}

// addV7PrefixedTags adds v7- prefix to preserved tags and migrates telemetry.
// If isAGI is true, AGI-specific tags are handled separately by preserveAGITags.
func (s *b) addV7PrefixedTags(oldTags, newTags map[string]string, isAGI bool) {
	// Migrate telemetry tag to new v8 format
	if v := oldTags[V7_TAG_TELEMETRY]; v != "" {
		newTags[TAG_TELEMETRY] = v
	}

	// Map of old tag names to their v7- prefixed versions
	// Note: AGI-specific tags are only prefixed for non-AGI instances
	tagsToPrefix := map[string]string{
		V7_TAG_SPOT: "v7-spot",
	}

	// Only add AGI tags with prefix for non-AGI instances
	// AGI instances preserve these tags without prefix via preserveAGITags()
	if !isAGI {
		tagsToPrefix[V7_TAG_AGI_AV] = "v7-agiav"
		tagsToPrefix[V7_TAG_FEATURES] = "v7-features"
		tagsToPrefix[V7_TAG_SSL] = "v7-ssl"
		tagsToPrefix[V7_TAG_AGI_LABEL] = "v7-agilabel"
		tagsToPrefix[V7_TAG_AGI_INSTANCE] = "v7-agiinstance"
		tagsToPrefix[V7_TAG_AGI_NODIM] = "v7-aginodim"
		tagsToPrefix[V7_TAG_TERM_ON_POW] = "v7-termonpow"
		tagsToPrefix[V7_TAG_IS_SPOT] = "v7-isspot"
		tagsToPrefix[V7_TAG_AGI_SRC_LOCAL] = "v7-agisrclocal"
		tagsToPrefix[V7_TAG_AGI_SRC_SFTP] = "v7-agisrcsftp"
		tagsToPrefix[V7_TAG_AGI_SRC_S3] = "v7-agisrcs3"
		tagsToPrefix[V7_TAG_AGI_DOMAIN] = "v7-agidomain"
	}

	for oldKey, newKey := range tagsToPrefix {
		if v := oldTags[oldKey]; v != "" {
			newTags[newKey] = v
		}
	}
}

// isAGIVolume checks if a v7 volume is associated with AGI by looking for AGI-specific tags.
func isAGIVolume(tags map[string]string) bool {
	agiIndicators := []string{
		V7_TAG_AGI_LABEL,    // agiLabel
		V7_TAG_AGI_INSTANCE, // agiinstance
		V7_TAG_AGI_NODIM,    // aginodim
		V7_TAG_AGI_AV,       // aerolab7agiav
	}
	for _, tag := range agiIndicators {
		if tags[tag] != "" {
			return true
		}
	}
	return false
}

// translateVolumeTags converts v7 volume tags to v8 format
func (s *b) translateVolumeTags(vol backends.OldVolume, project, aerolabVersion string) map[string]string {
	// Check if this is an AGI-associated volume
	isAGI := isAGIVolume(vol.Tags)

	tags := map[string]string{
		TAG_START_TIME:      vol.Tags[V7_TAG_VOLUME_LAST_USED],
		TAG_OWNER:           vol.Tags[V7_TAG_VOLUME_OWNER],
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_V7_MIGRATED:     "true",
	}

	// Preserve expire duration with v7- prefix
	if v := vol.Tags[V7_TAG_VOLUME_EXPIRE_DUR]; v != "" {
		tags["v7-expireduration"] = v
	}

	if isAGI {
		// For AGI volumes, preserve AGI-specific tags WITHOUT prefix
		// so v8 AGI can recognize and use these volumes
		s.preserveAGIVolumeTags(vol.Tags, tags)
	} else {
		// For non-AGI volumes, preserve AGI label with v7- prefix
		if v := vol.Tags[V7_TAG_AGI_LABEL]; v != "" {
			tags["v7-agilabel"] = v
		}
	}

	return tags
}

// preserveAGIVolumeTags preserves AGI-specific volume tags WITHOUT prefix.
// This allows v8 AGI to recognize and reuse volumes from migrated AGI instances.
func (s *b) preserveAGIVolumeTags(oldTags, newTags map[string]string) {
	// AGI volume tags to preserve with their ORIGINAL names
	agiVolumeTags := []string{
		V7_TAG_AGI_LABEL,    // agiLabel - AGI label
		V7_TAG_AGI_INSTANCE, // agiinstance - instance type used
		V7_TAG_AGI_NODIM,    // aginodim - NoDIM mode
		V7_TAG_TERM_ON_POW,  // termonpow - terminate on poweroff
		V7_TAG_IS_SPOT,      // isspot - spot instance
		V7_TAG_AGI_AV,       // aerolab7agiav - Aerospike version for AGI
	}

	for _, tag := range agiVolumeTags {
		if v := oldTags[tag]; v != "" {
			newTags[tag] = v
		}
	}
}

// translateImageTags converts v7 image to v8 format
func (s *b) translateImageTags(img backends.OldImage, project, aerolabVersion, osName, osVersion, asVersion, arch string) map[string]string {
	tags := map[string]string{
		TAG_OS_NAME:         osName,
		TAG_OS_VERSION:      osVersion,
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_V7_MIGRATED:     "true",
		TAG_IMAGE_TYPE:      "aerospike", // v7 only ever creates aerospike images
	}

	if asVersion != "" {
		// Normalize version with edition suffix (v7 only creates aerospike images)
		tags[TAG_SOFT_VERSION] = normalizeAerospikeVersion(asVersion)
	}
	if arch != "" {
		tags["v7-arch"] = arch
	}

	return tags
}

// parseV7ImageName parses AWS v7 image name format:
// aerolab4-template-{distro}_{version}_{aerospikeVersion}_{arch}
func parseV7ImageName(name string) (osName, osVersion, asVersion, arch string) {
	// Remove prefix
	name = strings.TrimPrefix(name, "aerolab4-template-")
	if name == "" {
		return
	}

	// Split by underscore
	parts := strings.Split(name, "_")
	if len(parts) >= 1 {
		osName = parts[0]
	}
	if len(parts) >= 2 {
		osVersion = parts[1]
	}
	if len(parts) >= 3 {
		asVersion = parts[2]
	}
	if len(parts) >= 4 {
		arch = parts[3]
	}

	return
}

// applyInstanceTags adds tags to an EC2 instance
func (s *b) applyInstanceTags(instanceID, zone string, tags map[string]string) error {
	cli, err := getEc2Client(s.credentials, &zone)
	if err != nil {
		return err
	}

	awsTags := []types.Tag{}
	for k, v := range tags {
		if v == "" {
			continue
		}
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err = cli.CreateTags(context.TODO(), &ec2.CreateTagsInput{
		Resources: []string{instanceID},
		Tags:      awsTags,
	})
	return err
}

// applyVolumeTags adds tags to an EBS volume or EFS file system
func (s *b) applyVolumeTags(volumeID, zone string, tags map[string]string) error {
	// EFS file system IDs start with "fs-", EBS volume IDs start with "vol-"
	if strings.HasPrefix(volumeID, "fs-") {
		return s.applyEFSTags(volumeID, zone, tags)
	}
	return s.applyEBSTags(volumeID, zone, tags)
}

// applyEBSTags adds tags to an EBS volume
func (s *b) applyEBSTags(volumeID, zone string, tags map[string]string) error {
	cli, err := getEc2Client(s.credentials, &zone)
	if err != nil {
		return err
	}

	awsTags := []types.Tag{}
	for k, v := range tags {
		if v == "" {
			continue
		}
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err = cli.CreateTags(context.TODO(), &ec2.CreateTagsInput{
		Resources: []string{volumeID},
		Tags:      awsTags,
	})
	return err
}

// applyEFSTags adds tags to an EFS file system
func (s *b) applyEFSTags(fileSystemID, zone string, tags map[string]string) error {
	cli, err := getEfsClient(s.credentials, &zone)
	if err != nil {
		return err
	}

	efsTags := []etypes.Tag{}
	for k, v := range tags {
		if v == "" {
			continue
		}
		efsTags = append(efsTags, etypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err = cli.TagResource(context.TODO(), &efs.TagResourceInput{
		ResourceId: aws.String(fileSystemID),
		Tags:       efsTags,
	})
	return err
}

// applyImageTags adds tags to an AMI
func (s *b) applyImageTags(imageID, zone string, tags map[string]string) error {
	cli, err := getEc2Client(s.credentials, &zone)
	if err != nil {
		return err
	}

	awsTags := []types.Tag{}
	for k, v := range tags {
		if v == "" {
			continue
		}
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err = cli.CreateTags(context.TODO(), &ec2.CreateTagsInput{
		Resources: []string{imageID},
		Tags:      awsTags,
	})
	return err
}

// discoverOldFirewalls finds v7 security groups by naming pattern
func (s *b) discoverOldFirewalls(force bool) ([]backends.OldFirewall, error) {
	var firewalls []backends.OldFirewall
	lock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	var errs error

	// v7 security group naming patterns:
	// - AeroLabServer-{vpcSuffix}
	// - AeroLabClient-{vpcSuffix}
	// - aerolab-managed-external-{vpcSuffix}
	v7Prefixes := []string{"AeroLabServer-", "AeroLabClient-", "aerolab-managed-external-"}

	zones, _ := s.ListEnabledZones()
	for _, zone := range zones {
		wg.Add(1)
		go func(zone string) {
			defer wg.Done()
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				lock.Lock()
				errs = errors.Join(errs, err)
				lock.Unlock()
				return
			}

			paginator := ec2.NewDescribeSecurityGroupsPaginator(cli, &ec2.DescribeSecurityGroupsInput{})

			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					lock.Lock()
					errs = errors.Join(errs, err)
					lock.Unlock()
					return
				}
				for _, sg := range out.SecurityGroups {
					name := aws.ToString(sg.GroupName)

					// Check if name matches any v7 pattern
					isV7 := false
					for _, prefix := range v7Prefixes {
						if strings.HasPrefix(name, prefix) {
							isV7 = true
							break
						}
					}
					if !isV7 {
						continue
					}

					// Convert tags to map
					tags := make(map[string]string)
					for _, t := range sg.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}

					// Skip already migrated (unless --force)
					if !force && tags[TAG_V7_MIGRATED] == "true" {
						continue
					}

					// Skip if already has v8 tags (already managed)
					if !force && tags[TAG_AEROLAB_VERSION] != "" {
						continue
					}

					lock.Lock()
					firewalls = append(firewalls, backends.OldFirewall{
						FirewallID: aws.ToString(sg.GroupId),
						Name:       name,
						Zone:       zone,
						VPCID:      aws.ToString(sg.VpcId),
						Tags:       tags,
					})
					lock.Unlock()
				}
			}
		}(zone)
	}
	wg.Wait()

	return firewalls, errs
}

// translateFirewallTags creates v8 tags for a v7 security group
func (s *b) translateFirewallTags(fw backends.OldFirewall, project, aerolabVersion string) map[string]string {
	tags := map[string]string{
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_V7_MIGRATED:     "true",
	}

	// Preserve owner if present
	if v := fw.Tags["owner"]; v != "" {
		tags[TAG_OWNER] = v
	}

	return tags
}

// applyFirewallTags adds tags to a security group
func (s *b) applyFirewallTags(securityGroupID, zone string, tags map[string]string) error {
	cli, err := getEc2Client(s.credentials, &zone)
	if err != nil {
		return err
	}

	awsTags := []types.Tag{}
	for k, v := range tags {
		if v == "" {
			continue
		}
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err = cli.CreateTags(context.TODO(), &ec2.CreateTagsInput{
		Resources: []string{securityGroupID},
		Tags:      awsTags,
	})
	return err
}

// migrateSSHKey copies an SSH key from old location to new
func (s *b) migrateSSHKey(fromPath, toPath string) error {
	// Check if source exists
	if _, err := os.Stat(fromPath); os.IsNotExist(err) {
		return fmt.Errorf("source key not found: %s", fromPath)
	}

	// Check if destination already exists
	if _, err := os.Stat(toPath); err == nil {
		return nil // Already migrated
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(toPath), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Copy the file
	src, err := os.Open(fromPath)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(toPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return nil
}

// discoverAttachedVolumes finds EBS volumes attached to a specific instance
func (s *b) discoverAttachedVolumes(instanceID, zone string, force bool) ([]backends.OldAttachedVolume, error) {
	var volumes []backends.OldAttachedVolume

	cli, err := getEc2Client(s.credentials, &zone)
	if err != nil {
		return nil, err
	}

	// Get instance details to find attached volumes
	result, err := cli.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance: %w", err)
	}

	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	instance := result.Reservations[0].Instances[0]
	if len(instance.BlockDeviceMappings) == 0 {
		return volumes, nil
	}

	// Collect volume IDs
	volumeIDs := make([]string, 0, len(instance.BlockDeviceMappings))
	volumeInfo := make(map[string]struct {
		DeviceName          string
		DeleteOnTermination bool
	})

	for _, bdm := range instance.BlockDeviceMappings {
		if bdm.Ebs != nil && bdm.Ebs.VolumeId != nil {
			volID := aws.ToString(bdm.Ebs.VolumeId)
			volumeIDs = append(volumeIDs, volID)
			volumeInfo[volID] = struct {
				DeviceName          string
				DeleteOnTermination bool
			}{
				DeviceName:          aws.ToString(bdm.DeviceName),
				DeleteOnTermination: aws.ToBool(bdm.Ebs.DeleteOnTermination),
			}
		}
	}

	if len(volumeIDs) == 0 {
		return volumes, nil
	}

	// Describe volumes to get their tags
	volResult, err := cli.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
		VolumeIds: volumeIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe volumes: %w", err)
	}

	for _, vol := range volResult.Volumes {
		volID := aws.ToString(vol.VolumeId)
		info := volumeInfo[volID]

		// Convert tags to map
		tags := make(map[string]string)
		for _, t := range vol.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}

		// Skip already migrated (unless --force)
		if !force && tags[TAG_V7_MIGRATED] == "true" {
			continue
		}

		volumes = append(volumes, backends.OldAttachedVolume{
			VolumeID:            volID,
			Name:                tags[TAG_NAME],
			VolumeType:          string(vol.VolumeType),
			Zone:                zone,
			DeleteOnTermination: info.DeleteOnTermination,
			DeviceName:          info.DeviceName,
			Tags:                tags,
		})
	}

	return volumes, nil
}

// translateAttachedVolumeTags creates v8 tags for a volume attached to an instance
func (s *b) translateAttachedVolumeTags(inst backends.OldInstance, vol backends.OldAttachedVolume, project, clusterUUID, aerolabVersion string) map[string]string {
	// Use instance tags as base, similar to how v8 creates volumes
	tags := map[string]string{
		TAG_CLUSTER_NAME:    inst.ClusterName,
		TAG_NODE_NO:         strconv.Itoa(inst.NodeNo),
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
		TAG_V7_MIGRATED:     "true",
	}

	// Add owner from instance
	if inst.IsClient {
		if v := inst.Tags[V7_TAG_OWNER]; v != "" {
			tags[TAG_OWNER] = v
		}
	} else {
		if v := inst.Tags[V7_TAG_OWNER]; v != "" {
			tags[TAG_OWNER] = v
		}
	}

	// Add OS info from instance
	if inst.IsClient {
		if v := inst.Tags[V7_TAG_CLIENT_OS]; v != "" {
			tags[TAG_OS_NAME] = v
		}
		if v := inst.Tags[V7_TAG_CLIENT_OS_VER]; v != "" {
			tags[TAG_OS_VERSION] = v
		}
	} else {
		if v := inst.Tags[V7_TAG_OPERATING_SYSTEM]; v != "" {
			tags[TAG_OS_NAME] = v
		}
		if v := inst.Tags[V7_TAG_OPERATING_SYS_VER]; v != "" {
			tags[TAG_OS_VERSION] = v
		}
	}

	// Add expires from instance
	if v := inst.Tags[V7_TAG_EXPIRES]; v != "" {
		tags[TAG_EXPIRES] = v
	}

	// Preserve volume name if exists
	if vol.Name != "" {
		tags[TAG_NAME] = vol.Name
	}

	// Add start time (use instance start time if volume doesn't have one)
	if v := inst.Tags[V7_TAG_COST_START_TIME]; v != "" {
		tags[TAG_START_TIME] = v
	}

	return tags
}

// getOldSSHKeyPath returns the path to an old v7 SSH key for a migrated instance
func (s *b) getOldSSHKeyPath(inst *backends.Instance) string {
	if inst.Tags[TAG_V7_MIGRATED] != "true" {
		return ""
	}
	return filepath.Join(s.sshKeysDir, "old",
		fmt.Sprintf("aerolab-%s_%s", inst.ClusterName, inst.ZoneName))
}

// normalizeAerospikeVersion converts v7 Aerospike version format to v8 format with edition suffix
// - version ending with 'c' → trim 'c', append "-community"
// - version ending with 'f' → trim 'f', append "-federal"
// - otherwise → append "-enterprise"
func normalizeAerospikeVersion(version string) string {
	if version == "" {
		return version
	}
	if strings.HasSuffix(version, "c") {
		return strings.TrimSuffix(version, "c") + "-community"
	}
	if strings.HasSuffix(version, "f") {
		return strings.TrimSuffix(version, "f") + "-federal"
	}
	return version + "-enterprise"
}
