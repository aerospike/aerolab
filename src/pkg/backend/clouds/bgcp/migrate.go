package bgcp

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

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

// MigrateV7Resources discovers and migrates v7 GCP resources (instances, volumes, images)
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

	// Group instances by cluster for consistent UUID assignment (GCP: no region in key)
	clusterUUIDs := make(map[string]string) // key: "clusterName"

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

		// Get or create UUID for this cluster
		if _, ok := clusterUUIDs[inst.ClusterName]; !ok {
			clusterUUIDs[inst.ClusterName] = uuid.New().String()
		}
		clusterUUID := clusterUUIDs[inst.ClusterName]

		// Translate labels
		var newLabels map[string]string
		if inst.IsClient {
			newLabels = s.translateClientLabels(inst, input.Project, clusterUUID, input.AerolabVersion)
		} else {
			newLabels = s.translateServerLabels(inst, input.Project, clusterUUID, input.AerolabVersion)
		}
		detail.TagsToAdd = newLabels

		// Calculate SSH key paths (GCP: no region in key name)
		if input.SSHKeyInfo != nil && input.SSHKeyInfo.SharedKeyPath == "" {
			oldKeyName := fmt.Sprintf("aerolab-gcp-%s", inst.ClusterName)
			oldKeyPath := filepath.Join(input.SSHKeyInfo.KeysDir, oldKeyName)
			newKeyPath := filepath.Join(s.sshKeysDir, "old", oldKeyName)
			detail.SSHKeyFrom = oldKeyPath
			detail.SSHKeyTo = newKeyPath
		}

		if input.DryRun {
			detail.MigrationStatus = "pending"
			result.DryRunInstances = append(result.DryRunInstances, detail)
		} else {
			// Apply labels
			err := s.applyInstanceLabels(inst.InstanceID, inst.Zone, inst.Tags, newLabels)
			if err != nil {
				detail.MigrationStatus = "failed"
				detail.MigrationError = err.Error()
				result.Errors = append(result.Errors, fmt.Errorf("instance %s: %w", inst.InstanceID, err))
			} else {
				detail.MigrationStatus = "success"
				detail.TagsAdded = newLabels
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

		// Discover and tag disks attached to this instance
		attachedDisks, err := s.discoverAttachedDisks(inst.InstanceID, inst.Zone, input.Force)
		if err != nil {
			log.Warn("Failed to discover attached disks for instance %s: %s", inst.InstanceID, err)
		} else {
			for _, disk := range attachedDisks {
				diskDetail := backends.MigrationVolumeDetail{
					VolumeID:            disk.VolumeID,
					VolumeType:          disk.VolumeType,
					Name:                disk.Name,
					Zone:                disk.Zone,
					AttachedToInstance:  inst.InstanceID,
					DeleteOnTermination: disk.DeleteOnTermination,
				}

				// Use instance labels for attached disks
				diskLabels := s.translateAttachedDiskLabels(inst, disk, input.Project, clusterUUID, input.AerolabVersion)
				diskDetail.TagsToAdd = diskLabels

				if input.DryRun {
					diskDetail.MigrationStatus = "pending"
					result.DryRunVolumes = append(result.DryRunVolumes, diskDetail)
				} else {
					err := s.applyVolumeLabels(disk.VolumeID, disk.Zone, disk.Tags, diskLabels)
					if err != nil {
						diskDetail.MigrationStatus = "failed"
						diskDetail.MigrationError = err.Error()
						result.Errors = append(result.Errors, fmt.Errorf("attached disk %s: %w", disk.VolumeID, err))
					} else {
						diskDetail.MigrationStatus = "success"
						diskDetail.TagsAdded = diskLabels
						result.VolumesMigrated++
					}
					result.MigratedVolumes = append(result.MigratedVolumes, diskDetail)
				}
			}
		}
	}

	// 2. Discover and migrate standalone volumes (persistent disks with usedby=aerolab7)
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

			newLabels := s.translateVolumeLabels(vol, input.Project, input.AerolabVersion)
			detail.TagsToAdd = newLabels

			if input.DryRun {
				detail.MigrationStatus = "pending"
				result.DryRunVolumes = append(result.DryRunVolumes, detail)
			} else {
				err := s.applyVolumeLabels(vol.VolumeID, vol.Zone, vol.Tags, newLabels)
				if err != nil {
					detail.MigrationStatus = "failed"
					detail.MigrationError = err.Error()
					result.Errors = append(result.Errors, fmt.Errorf("volume %s: %w", vol.VolumeID, err))
				} else {
					detail.MigrationStatus = "success"
					detail.TagsAdded = newLabels
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
			osName, osVersion, asVersion, arch := parseV7GCPImageName(img.Name)
			detail.OSName = osName
			detail.OSVersion = osVersion
			detail.AerospikeVersion = asVersion
			detail.Architecture = arch

			newLabels := s.translateImageLabels(img, input.Project, input.AerolabVersion, osName, osVersion, asVersion, arch)
			detail.TagsToAdd = newLabels

			if input.DryRun {
				detail.MigrationStatus = "pending"
				result.DryRunImages = append(result.DryRunImages, detail)
			} else {
				err := s.applyImageLabels(img.ImageID, img.Tags, newLabels)
				if err != nil {
					detail.MigrationStatus = "failed"
					detail.MigrationError = err.Error()
					result.Errors = append(result.Errors, fmt.Errorf("image %s: %w", img.ImageID, err))
				} else {
					detail.MigrationStatus = "success"
					detail.TagsAdded = newLabels
					result.ImagesMigrated++
				}
				result.MigratedImages = append(result.MigratedImages, detail)
			}
		}
	}

	// 4. Discover and adopt firewalls used by migrated instances
	// Collect network tags from all discovered instances
	instanceNetworkTags := make(map[string]bool)
	for _, inst := range oldInstances {
		// Get the instance's network tags from GCP
		tags, err := s.getInstanceNetworkTags(inst.InstanceID, inst.Zone)
		if err != nil {
			log.Warn("Failed to get network tags for instance %s: %s", inst.InstanceID, err)
			continue
		}
		for _, tag := range tags {
			instanceNetworkTags[tag] = true
		}
	}

	if len(instanceNetworkTags) > 0 {
		log.Detail("Found %d unique network tags on instances", len(instanceNetworkTags))

		// Find firewalls that target these network tags
		firewallsToAdopt, err := s.discoverFirewallsByTags(instanceNetworkTags, input.Force)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to discover firewalls: %w", err))
		} else {
			log.Detail("Found %d firewalls to adopt", len(firewallsToAdopt))

			for _, fwName := range firewallsToAdopt {
				detail := backends.MigrationFirewallDetail{
					FirewallID: fwName,
					Name:       fwName,
					Zone:       "global",
				}

				// Build tags for adoption
				newTags := s.translateAdoptedFirewallTags(input.Project, input.AerolabVersion)
				detail.TagsToAdd = newTags

				if input.DryRun {
					detail.MigrationStatus = "pending"
					result.DryRunFirewalls = append(result.DryRunFirewalls, detail)
				} else {
					err := s.applyFirewallTags(fwName, newTags)
					if err != nil {
						detail.MigrationStatus = "failed"
						detail.MigrationError = err.Error()
						result.Errors = append(result.Errors, fmt.Errorf("firewall %s: %w", fwName, err))
					} else {
						detail.MigrationStatus = "success"
						detail.TagsAdded = newTags
						result.FirewallsMigrated++
					}
					result.MigratedFirewalls = append(result.MigratedFirewalls, detail)
				}
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
					FromPath:    inst.SSHKeyFrom,
					ToPath:      inst.SSHKeyTo,
				}
				// Check if source exists
				if _, err := os.Stat(inst.SSHKeyFrom); err != nil {
					detail.Error = "source file not found"
				}
				// For GCP, also check .pub file
				pubPath := inst.SSHKeyFrom + ".pub"
				if _, err := os.Stat(pubPath); err != nil {
					if detail.Error != "" {
						detail.Error += "; public key not found"
					} else {
						detail.Error = "public key not found"
					}
				}
				result.DryRunSSHKeys = append(result.DryRunSSHKeys, detail)
			}
		}
	}

	return result, nil
}

// discoverOldInstances finds v7 instances using label filters
func (s *b) discoverOldInstances(force bool) ([]backends.OldInstance, error) {
	var instances []backends.OldInstance
	lock := new(sync.Mutex)
	var errs error

	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	enabledRegions, err := s.ListEnabledZones()
	if err != nil {
		return nil, err
	}

	// Filter for v7 instances
	filter := fmt.Sprintf(`labels.%s="%s" OR labels.%s="%s"`,
		V7_LABEL_USED_BY, V7_LABEL_SERVER_MARKER,
		V7_LABEL_USED_BY, V7_LABEL_CLIENT_MARKER)

	it := client.AggregatedList(ctx, &computepb.AggregatedListInstancesRequest{
		Project: s.credentials.Project,
		Filter:  proto.String(filter),
	})

	for {
		inst, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			errs = errors.Join(errs, err)
			break
		}

		for _, instance := range inst.Value.Instances {
			zone := getValueFromURL(instance.GetZone())
			region := zoneToRegion(zone)

			// Skip if not in enabled regions
			found := false
			for _, r := range enabledRegions {
				if r == region {
					found = true
					break
				}
			}
			if !found {
				continue
			}

			labels := instance.GetLabels()

			// Skip already migrated (unless --force)
			if !force && labels[LABEL_V7_MIGRATED] == "true" {
				continue
			}

			// Determine if client or server
			isClient := labels[V7_LABEL_USED_BY] == V7_LABEL_CLIENT_MARKER

			var clusterName string
			var nodeNo int
			if isClient {
				clusterName = labels[V7_LABEL_CLIENT_NAME]
				nodeNo, _ = strconv.Atoi(labels[V7_LABEL_CLIENT_NODE_NUMBER])
			} else {
				clusterName = labels[V7_LABEL_CLUSTER_NAME]
				nodeNo, _ = strconv.Atoi(labels[V7_LABEL_NODE_NUMBER])
			}

			lock.Lock()
			instances = append(instances, backends.OldInstance{
				InstanceID:  instance.GetName(),
				Name:        instance.GetName(),
				ClusterName: clusterName,
				NodeNo:      nodeNo,
				Zone:        zone,
				IsClient:    isClient,
				Tags:        labels,
			})
			lock.Unlock()
		}
	}

	return instances, errs
}

// discoverOldVolumes finds v7 persistent disks
func (s *b) discoverOldVolumes(force bool) ([]backends.OldVolume, error) {
	var volumes []backends.OldVolume
	lock := new(sync.Mutex)
	var errs error

	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	enabledRegions, err := s.ListEnabledZones()
	if err != nil {
		return nil, err
	}

	// Filter for v7 volumes (usedby = aerolab7)
	filter := fmt.Sprintf(`labels.%s="%s"`, V7_LABEL_VOLUME_USED_BY, V7_LABEL_VOLUME_MARKER)

	it := client.AggregatedList(ctx, &computepb.AggregatedListDisksRequest{
		Project: s.credentials.Project,
		Filter:  proto.String(filter),
	})

	for {
		disk, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			errs = errors.Join(errs, err)
			break
		}

		for _, d := range disk.Value.Disks {
			zone := getValueFromURL(d.GetZone())
			region := zoneToRegion(zone)

			// Skip if not in enabled regions
			found := false
			for _, r := range enabledRegions {
				if r == region {
					found = true
					break
				}
			}
			if !found {
				continue
			}

			labels := d.GetLabels()

			// Skip already migrated (unless --force)
			if !force && labels[LABEL_V7_MIGRATED] == "true" {
				continue
			}

			lock.Lock()
			volumes = append(volumes, backends.OldVolume{
				VolumeID:   d.GetName(),
				Name:       d.GetName(),
				VolumeType: getValueFromURL(d.GetType()),
				Zone:       zone,
				Tags:       labels,
			})
			lock.Unlock()
		}
	}

	return volumes, errs
}

// discoverOldImages finds v7 template images
func (s *b) discoverOldImages(force bool) ([]backends.OldImage, error) {
	var images []backends.OldImage
	var errs error

	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Filter by name pattern: aerolab4-template-*
	// GCP doesn't support wildcard in filters, so we'll filter in code
	it := client.List(ctx, &computepb.ListImagesRequest{
		Project: s.credentials.Project,
	})

	for {
		img, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			errs = errors.Join(errs, err)
			break
		}

		name := img.GetName()
		if !strings.HasPrefix(name, "aerolab4-template-") {
			continue
		}

		labels := img.GetLabels()

		// Skip already migrated (unless --force)
		if !force && labels[LABEL_V7_MIGRATED] == "true" {
			continue
		}

		images = append(images, backends.OldImage{
			ImageID: name,
			Name:    name,
			Zone:    "", // Images are global
			Tags:    labels,
		})
	}

	return images, errs
}

// translateServerLabels converts v7 server instance labels to v8 format
func (s *b) translateServerLabels(inst backends.OldInstance, project, clusterUUID, aerolabVersion string) map[string]string {
	// Build the metadata map that will be encoded
	// Note: v7 GCP labels were sanitized (dots->dashes, colons->underscores), so we unsanitize them
	metadata := map[string]string{
		TAG_CLUSTER_NAME:    inst.Tags[V7_LABEL_CLUSTER_NAME],
		TAG_NODE_NO:         inst.Tags[V7_LABEL_NODE_NUMBER],
		TAG_OS_NAME:         inst.Tags[V7_LABEL_OPERATING_SYSTEM],
		TAG_OS_VERSION:      unsanitizeVersion(inst.Tags[V7_LABEL_OPERATING_SYS_VER]),
		TAG_AEROLAB_EXPIRES: unsanitizeExpires(inst.Tags[V7_LABEL_EXPIRES]),
		TAG_AEROLAB_OWNER:   inst.Tags[V7_LABEL_OWNER],
		TAG_COST_PPH:        unsanitizeCost(inst.Tags[V7_LABEL_COST_PPH]),
		TAG_COST_SO_FAR:     unsanitizeCost(inst.Tags[V7_LABEL_COST_SO_FAR]),
		TAG_START_TIME:      inst.Tags[V7_LABEL_COST_START_TIME],
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
		TAG_SOFT_TYPE:       "aerospike", // Server type is always "aerospike"
	}

	// Add software version if present (with edition suffix for aerospike)
	if v := inst.Tags[V7_LABEL_AEROSPIKE_VERSION]; v != "" {
		metadata[TAG_SOFT_VERSION] = normalizeAerospikeVersion(unsanitizeVersion(v))
	}

	// Migrate telemetry
	if v := inst.Tags[V7_LABEL_TELEMETRY]; v != "" {
		metadata[TAG_TELEMETRY] = v
	}

	// Encode to GCP labels
	labels := encodeToLabels(metadata)
	labels["usedby"] = "aerolab"
	labels[LABEL_V7_MIGRATED] = "true"

	// Preserve architecture with v7- prefix
	if v := inst.Tags[V7_LABEL_ARCH]; v != "" {
		labels["v7-arch"] = sanitize(v, false)
	}

	// Preserve spot flag
	if v := inst.Tags[V7_LABEL_IS_SPOT]; v != "" {
		labels["v7-isspot"] = sanitize(v, false)
	}

	return labels
}

// translateClientLabels converts v7 client instance labels to v8 format
func (s *b) translateClientLabels(inst backends.OldInstance, project, clusterUUID, aerolabVersion string) map[string]string {
	// Build the metadata map that will be encoded
	// Note: v7 GCP labels were sanitized (dots->dashes, colons->underscores), so we unsanitize them
	metadata := map[string]string{
		TAG_CLUSTER_NAME:    inst.Tags[V7_LABEL_CLIENT_NAME],
		TAG_NODE_NO:         inst.Tags[V7_LABEL_CLIENT_NODE_NUMBER],
		TAG_OS_NAME:         inst.Tags[V7_LABEL_CLIENT_OS],
		TAG_OS_VERSION:      unsanitizeVersion(inst.Tags[V7_LABEL_CLIENT_OS_VER]),
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
		"aerolab.old.type":  "client", // Required for v8 client commands to identify client instances
	}

	// Add owner and expires if present
	if v := inst.Tags[V7_LABEL_OWNER]; v != "" {
		metadata[TAG_AEROLAB_OWNER] = v
	}
	if v := inst.Tags[V7_LABEL_EXPIRES]; v != "" {
		metadata[TAG_AEROLAB_EXPIRES] = unsanitizeExpires(v)
	}

	// Set aerolab.type from client type
	if v := inst.Tags[V7_LABEL_CLIENT_TYPE]; v != "" {
		metadata[TAG_SOFT_TYPE] = v
	}

	// Add software version if present
	if v := inst.Tags[V7_LABEL_CLIENT_AS_VER]; v != "" {
		metadata[TAG_SOFT_VERSION] = unsanitizeVersion(v)
	}

	// Migrate telemetry
	if v := inst.Tags[V7_LABEL_TELEMETRY]; v != "" {
		metadata[TAG_TELEMETRY] = v
	}

	// Encode to GCP labels
	labels := encodeToLabels(metadata)
	labels["usedby"] = "aerolab"
	labels[LABEL_V7_MIGRATED] = "true"

	// Add native client type label for filtering
	if v := inst.Tags[V7_LABEL_CLIENT_TYPE]; v != "" {
		labels[TAG_CLIENT_TYPE] = sanitize(v, false)
	}

	// Preserve spot flag
	if v := inst.Tags[V7_LABEL_IS_SPOT]; v != "" {
		labels["v7-isspot"] = sanitize(v, false)
	}

	return labels
}

// translateVolumeLabels converts v7 volume labels to v8 format
func (s *b) translateVolumeLabels(vol backends.OldVolume, project, aerolabVersion string) map[string]string {
	// Build the metadata map
	metadata := map[string]string{
		TAG_START_TIME:      vol.Tags[V7_LABEL_VOLUME_LAST_USED],
		TAG_AEROLAB_OWNER:   vol.Tags[V7_LABEL_VOLUME_OWNER],
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
	}

	// Encode to GCP labels
	labels := encodeToLabels(metadata)
	labels["usedby"] = "aerolab"
	labels[LABEL_V7_MIGRATED] = "true"

	// Preserve expire duration with v7- prefix
	if v := vol.Tags[V7_LABEL_VOLUME_EXPIRE_DUR]; v != "" {
		labels["v7-expireduration"] = sanitize(v, false)
	}

	// Preserve AGI-related labels with v7- prefix
	if v := vol.Tags[V7_LABEL_AGI_INSTANCE]; v != "" {
		labels["v7-agiinstance"] = sanitize(v, false)
	}
	if v := vol.Tags[V7_LABEL_AGI_NODIM]; v != "" {
		labels["v7-aginodim"] = sanitize(v, false)
	}
	if v := vol.Tags[V7_LABEL_TERM_ON_POW]; v != "" {
		labels["v7-termonpow"] = sanitize(v, false)
	}
	if v := vol.Tags[V7_LABEL_IS_SPOT]; v != "" {
		labels["v7-isspot"] = sanitize(v, false)
	}

	// Handle chunked AGI labels (agilabel0, agilabel1, etc.)
	for key, value := range vol.Tags {
		if strings.HasPrefix(key, "agilabel") {
			labels["v7-"+key] = sanitize(value, false)
		}
	}

	return labels
}

// translateImageLabels converts v7 image to v8 format
func (s *b) translateImageLabels(img backends.OldImage, project, aerolabVersion, osName, osVersion, asVersion, arch string) map[string]string {
	// Build the metadata map
	metadata := map[string]string{
		TAG_OS_NAME:         osName,
		TAG_OS_VERSION:      osVersion,
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_IMAGE_TYPE:      "aerospike", // v7 only ever creates aerospike images
	}

	if asVersion != "" {
		// Normalize version with edition suffix (v7 only creates aerospike images)
		metadata[TAG_SOFT_VERSION] = normalizeAerospikeVersion(asVersion)
	}

	// Encode to GCP labels
	labels := encodeToLabels(metadata)
	labels["usedby"] = "aerolab"
	labels[LABEL_V7_MIGRATED] = "true"

	if arch != "" {
		labels["v7-arch"] = sanitize(arch, false)
	}

	return labels
}

// parseV7GCPImageName parses GCP v7 image name format:
// aerolab4-template-{distro}-{version}-{aerospikeVersion}-{arch}
// Note: GCP uses dashes instead of underscores, and dots are replaced with dashes
func parseV7GCPImageName(name string) (osName, osVersion, asVersion, arch string) {
	// Remove prefix
	name = strings.TrimPrefix(name, "aerolab4-template-")
	if name == "" {
		return
	}

	// GCP image names use dashes - need to intelligently parse
	// Example: ubuntu-22-04-7-0-0-amd
	parts := strings.Split(name, "-")
	if len(parts) < 1 {
		return
	}

	// First part is OS name
	osName = parts[0]

	// Try to identify where version parts start
	// OS version usually starts with a number
	versionStart := 1
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
			versionStart = i
			break
		}
	}

	// Reconstruct remaining parts
	remaining := parts[versionStart:]

	// Try to parse as: osVersion (2 parts), asVersion (3 parts), arch (1 part)
	// Example: 22-04-7-0-0-amd
	if len(remaining) >= 6 {
		osVersion = remaining[0] + "." + remaining[1]
		asVersion = remaining[2] + "." + remaining[3] + "." + remaining[4]
		arch = remaining[5]
	} else if len(remaining) >= 5 {
		osVersion = remaining[0] + "." + remaining[1]
		asVersion = remaining[2] + "." + remaining[3] + "." + remaining[4]
	} else if len(remaining) >= 2 {
		osVersion = remaining[0] + "." + remaining[1]
	}

	return
}

// applyInstanceLabels adds labels to a GCP instance
func (s *b) applyInstanceLabels(instanceName, zone string, oldLabels, newLabels map[string]string) error {
	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	// Get current instance to get label fingerprint
	inst, err := client.Get(ctx, &computepb.GetInstanceRequest{
		Instance: instanceName,
		Project:  s.credentials.Project,
		Zone:     zone,
	})
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	// Merge labels (keep old, add new)
	mergedLabels := make(map[string]string)
	for k, v := range inst.GetLabels() {
		mergedLabels[k] = v
	}
	for k, v := range newLabels {
		mergedLabels[k] = v
	}

	// Check 64-label limit
	if len(mergedLabels) > 64 {
		// Remove old v7 labels that have been translated
		mergedLabels = s.trimLabelsToLimit(mergedLabels, 64)
	}

	op, err := client.SetLabels(ctx, &computepb.SetLabelsInstanceRequest{
		Instance: instanceName,
		Project:  s.credentials.Project,
		Zone:     zone,
		InstancesSetLabelsRequestResource: &computepb.InstancesSetLabelsRequest{
			LabelFingerprint: proto.String(inst.GetLabelFingerprint()),
			Labels:           mergedLabels,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to set labels: %w", err)
	}

	return op.Wait(ctx)
}

// applyVolumeLabels adds labels to a GCP disk
func (s *b) applyVolumeLabels(diskName, zone string, oldLabels, newLabels map[string]string) error {
	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	// Get current disk to get label fingerprint
	disk, err := client.Get(ctx, &computepb.GetDiskRequest{
		Disk:    diskName,
		Project: s.credentials.Project,
		Zone:    zone,
	})
	if err != nil {
		return fmt.Errorf("failed to get disk: %w", err)
	}

	// Merge labels
	mergedLabels := make(map[string]string)
	for k, v := range disk.GetLabels() {
		mergedLabels[k] = v
	}
	for k, v := range newLabels {
		mergedLabels[k] = v
	}

	// Check 64-label limit
	if len(mergedLabels) > 64 {
		mergedLabels = s.trimLabelsToLimit(mergedLabels, 64)
	}

	op, err := client.SetLabels(ctx, &computepb.SetLabelsDiskRequest{
		Resource: diskName,
		Project:  s.credentials.Project,
		Zone:     zone,
		ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
			LabelFingerprint: proto.String(disk.GetLabelFingerprint()),
			Labels:           mergedLabels,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to set labels: %w", err)
	}

	return op.Wait(ctx)
}

// applyImageLabels adds labels to a GCP image
func (s *b) applyImageLabels(imageName string, oldLabels, newLabels map[string]string) error {
	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	// Get current image to get label fingerprint
	img, err := client.Get(ctx, &computepb.GetImageRequest{
		Image:   imageName,
		Project: s.credentials.Project,
	})
	if err != nil {
		return fmt.Errorf("failed to get image: %w", err)
	}

	// Merge labels
	mergedLabels := make(map[string]string)
	for k, v := range img.GetLabels() {
		mergedLabels[k] = v
	}
	for k, v := range newLabels {
		mergedLabels[k] = v
	}

	// Check 64-label limit
	if len(mergedLabels) > 64 {
		mergedLabels = s.trimLabelsToLimit(mergedLabels, 64)
	}

	op, err := client.SetLabels(ctx, &computepb.SetLabelsImageRequest{
		Resource: imageName,
		Project:  s.credentials.Project,
		GlobalSetLabelsRequestResource: &computepb.GlobalSetLabelsRequest{
			LabelFingerprint: proto.String(img.GetLabelFingerprint()),
			Labels:           mergedLabels,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to set labels: %w", err)
	}

	return op.Wait(ctx)
}

// trimLabelsToLimit removes old v7 labels to stay under the GCP 64-label limit
func (s *b) trimLabelsToLimit(labels map[string]string, limit int) map[string]string {
	if len(labels) <= limit {
		return labels
	}

	// Labels to remove (old v7 labels that have been translated)
	removable := []string{
		V7_LABEL_USED_BY,
		V7_LABEL_CLUSTER_NAME,
		V7_LABEL_NODE_NUMBER,
		V7_LABEL_OPERATING_SYSTEM,
		V7_LABEL_OPERATING_SYS_VER,
		V7_LABEL_AEROSPIKE_VERSION,
		V7_LABEL_EXPIRES,
		V7_LABEL_OWNER,
		V7_LABEL_COST_PPH,
		V7_LABEL_COST_SO_FAR,
		V7_LABEL_COST_START_TIME,
		V7_LABEL_CLIENT_NAME,
		V7_LABEL_CLIENT_NODE_NUMBER,
		V7_LABEL_CLIENT_OS,
		V7_LABEL_CLIENT_OS_VER,
		V7_LABEL_CLIENT_AS_VER,
		V7_LABEL_CLIENT_TYPE,
	}

	result := make(map[string]string)
	for k, v := range labels {
		result[k] = v
	}

	for _, key := range removable {
		if len(result) <= limit {
			break
		}
		delete(result, key)
	}

	return result
}

// migrateSSHKey copies SSH keys from old location to new (both private and .pub for GCP)
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

	// Copy private key
	if err := copyFile(fromPath, toPath); err != nil {
		return fmt.Errorf("failed to copy private key: %w", err)
	}

	// Copy public key (.pub file) - GCP requires both
	pubFrom := fromPath + ".pub"
	pubTo := toPath + ".pub"
	if _, err := os.Stat(pubFrom); err == nil {
		if err := copyFile(pubFrom, pubTo); err != nil {
			return fmt.Errorf("failed to copy public key: %w", err)
		}
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// discoverAttachedDisks finds persistent disks attached to a specific instance
func (s *b) discoverAttachedDisks(instanceName, zone string, force bool) ([]backends.OldAttachedVolume, error) {
	var disks []backends.OldAttachedVolume

	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	instanceClient, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer instanceClient.Close()

	// Get instance details to find attached disks
	instance, err := instanceClient.Get(ctx, &computepb.GetInstanceRequest{
		Instance: instanceName,
		Project:  s.credentials.Project,
		Zone:     zone,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	if len(instance.Disks) == 0 {
		return disks, nil
	}

	diskClient, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer diskClient.Close()

	for _, attachedDisk := range instance.Disks {
		if attachedDisk.Source == nil {
			continue
		}

		// Extract disk name from source URL
		diskName := getValueFromURL(attachedDisk.GetSource())

		// Get disk details to retrieve labels
		disk, err := diskClient.Get(ctx, &computepb.GetDiskRequest{
			Disk:    diskName,
			Project: s.credentials.Project,
			Zone:    zone,
		})
		if err != nil {
			// Skip disks we can't access
			continue
		}

		labels := disk.GetLabels()

		// Skip already migrated (unless --force)
		if !force && labels[LABEL_V7_MIGRATED] == "true" {
			continue
		}

		disks = append(disks, backends.OldAttachedVolume{
			VolumeID:            diskName,
			Name:                diskName,
			VolumeType:          getValueFromURL(disk.GetType()),
			Zone:                zone,
			DeleteOnTermination: attachedDisk.GetAutoDelete(),
			DeviceName:          attachedDisk.GetDeviceName(),
			Tags:                labels,
		})
	}

	return disks, nil
}

// translateAttachedDiskLabels creates v8 labels for a disk attached to an instance
func (s *b) translateAttachedDiskLabels(inst backends.OldInstance, disk backends.OldAttachedVolume, project, clusterUUID, aerolabVersion string) map[string]string {
	// Build the metadata map using instance info
	// Note: v7 GCP labels were sanitized (dots->dashes, colons->underscores), so we unsanitize them
	metadata := map[string]string{
		TAG_CLUSTER_NAME:    inst.ClusterName,
		TAG_NODE_NO:         strconv.Itoa(inst.NodeNo),
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
	}

	// Add owner from instance
	if v := inst.Tags[V7_LABEL_OWNER]; v != "" {
		metadata[TAG_AEROLAB_OWNER] = v
	}

	// Add OS info from instance (unsanitize version)
	if inst.IsClient {
		if v := inst.Tags[V7_LABEL_CLIENT_OS]; v != "" {
			metadata[TAG_OS_NAME] = v
		}
		if v := inst.Tags[V7_LABEL_CLIENT_OS_VER]; v != "" {
			metadata[TAG_OS_VERSION] = unsanitizeVersion(v)
		}
	} else {
		if v := inst.Tags[V7_LABEL_OPERATING_SYSTEM]; v != "" {
			metadata[TAG_OS_NAME] = v
		}
		if v := inst.Tags[V7_LABEL_OPERATING_SYS_VER]; v != "" {
			metadata[TAG_OS_VERSION] = unsanitizeVersion(v)
		}
	}

	// Add expires from instance (unsanitize to RFC3339)
	if v := inst.Tags[V7_LABEL_EXPIRES]; v != "" {
		metadata[TAG_AEROLAB_EXPIRES] = unsanitizeExpires(v)
	}

	// Add start time from instance
	if v := inst.Tags[V7_LABEL_COST_START_TIME]; v != "" {
		metadata[TAG_START_TIME] = v
	}

	// Encode to GCP labels
	labels := encodeToLabels(metadata)
	labels["usedby"] = "aerolab"
	labels[LABEL_V7_MIGRATED] = "true"

	return labels
}

// getOldSSHKeyPath returns the path to an old v7 SSH key for a migrated instance
func (s *b) getOldSSHKeyPath(inst *backends.Instance) string {
	if inst.Tags[LABEL_V7_MIGRATED] != "true" {
		return ""
	}
	return filepath.Join(s.sshKeysDir, "old",
		fmt.Sprintf("aerolab-gcp-%s", inst.ClusterName))
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

// unsanitizeVersion converts v7 GCP sanitized version strings back to proper format
// v7 GCP replaced dots with dashes: "24-04" -> "24.04", "8-0-0-5" -> "8.0.0.5"
func unsanitizeVersion(s string) string {
	if s == "" {
		return s
	}
	return strings.ReplaceAll(s, "-", ".")
}

// unsanitizeCost converts v7 GCP sanitized cost strings back to proper format
// v7 GCP replaced dots with dashes: "0-13402284" -> "0.13402284"
func unsanitizeCost(s string) string {
	if s == "" {
		return s
	}
	return strings.ReplaceAll(s, "-", ".")
}

// unsanitizeExpires converts v7 GCP sanitized expiry timestamp back to RFC3339 format
// v7 format: "2025-12-06t15_11_15-07_00"
// v8 format: "2025-12-06T15:11:15-07:00"
// Conversion: lowercase 't' -> 'T', '_' -> ':'
func unsanitizeExpires(s string) string {
	if s == "" {
		return s
	}
	// Replace lowercase 't' with 'T' for RFC3339 format
	s = strings.Replace(s, "t", "T", 1)
	// Replace underscores with colons for time component
	s = strings.ReplaceAll(s, "_", ":")
	return s
}

// translateAdoptedFirewallTags creates tags for an adopted firewall
func (s *b) translateAdoptedFirewallTags(project, aerolabVersion string) map[string]string {
	return map[string]string{
		TAG_AEROLAB_PROJECT: project,
		TAG_AEROLAB_VERSION: aerolabVersion,
		TAG_AEROLAB_OWNER:   "", // Owner not known for adopted firewalls
	}
}

// applyFirewallTags updates a GCP firewall's description field with the given tags
func (s *b) applyFirewallTags(firewallName string, tags map[string]string) error {
	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	// Get current firewall to get the existing resource
	fw, err := client.Get(ctx, &computepb.GetFirewallRequest{
		Firewall: firewallName,
		Project:  s.credentials.Project,
	})
	if err != nil {
		return fmt.Errorf("failed to get firewall %s: %w", firewallName, err)
	}

	// Encode tags to description JSON
	description := encodeToDescriptionField(tags)

	// Update the firewall with the new description
	op, err := client.Update(ctx, &computepb.UpdateFirewallRequest{
		Firewall: firewallName,
		Project:  s.credentials.Project,
		FirewallResource: &computepb.Firewall{
			Name:              fw.Name,
			Network:           fw.Network,
			Description:       &description,
			Allowed:           fw.Allowed,
			Denied:            fw.Denied,
			DestinationRanges: fw.DestinationRanges,
			Direction:         fw.Direction,
			Disabled:          fw.Disabled,
			Priority:          fw.Priority,
			SourceRanges:      fw.SourceRanges,
			SourceTags:        fw.SourceTags,
			TargetTags:        fw.TargetTags,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update firewall %s: %w", firewallName, err)
	}

	return op.Wait(ctx)
}

// getInstanceNetworkTags retrieves the network tags for a GCP instance
func (s *b) getInstanceNetworkTags(instanceName, zone string) ([]string, error) {
	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	inst, err := client.Get(ctx, &computepb.GetInstanceRequest{
		Instance: instanceName,
		Project:  s.credentials.Project,
		Zone:     zone,
	})
	if err != nil {
		return nil, err
	}

	if inst.GetTags() != nil {
		return inst.GetTags().GetItems(), nil
	}
	return nil, nil
}

// discoverFirewallsByTags finds firewalls that target any of the given network tags
// Returns firewall names that are not already tagged by aerolab
func (s *b) discoverFirewallsByTags(networkTags map[string]bool, force bool) ([]string, error) {
	cli, err := connect.GetClient(s.credentials, s.log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	var firewallNames []string
	seen := make(map[string]bool)

	it := client.List(ctx, &computepb.ListFirewallsRequest{
		Project: s.credentials.Project,
	})

	for {
		fw, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		fwName := fw.GetName()

		// Skip if already seen
		if seen[fwName] {
			continue
		}

		// Skip if already tagged by aerolab (check description field)
		if !force {
			desc := fw.GetDescription()
			if desc != "" {
				m, err := decodeFromDescriptionField(desc)
				if err == nil && m[TAG_AEROLAB_PROJECT] != "" {
					// Already managed by aerolab
					continue
				}
			}
		}

		// Check if any of the firewall's target tags match our instance tags
		for _, targetTag := range fw.GetTargetTags() {
			if networkTags[targetTag] {
				seen[fwName] = true
				firewallNames = append(firewallNames, fwName)
				break
			}
		}
	}

	return firewallNames, nil
}
