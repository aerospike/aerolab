package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/bestmethod/inslice"
	"golang.org/x/crypto/ssh"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"

	"github.com/aerospike/aerolab/gcplabels"
)

func (d *backendGcp) Arch() TypeArch {
	return TypeArchUndef
}

type backendGcp struct {
	server               bool
	client               bool
	disablePricing       bool
	disableExpiryInstall bool
}

func init() {
	addBackend("gcp", &backendGcp{})
}

const (
	// server
	gcpServerTagUsedBy           = "used_by"
	gcpServerTagUsedByValue      = "aerolab4"
	gcpServerTagClusterName      = "aerolab4cluster_name"
	gcpServerTagNodeNumber       = "aerolab4node_number"
	gcpServerTagOperatingSystem  = "aerolab4operating_system"
	gcpServerTagOSVersion        = "aerolab4operating_system_version"
	gcpServerTagAerospikeVersion = "aerolab4aerospike_version"
	gcpServerFirewallTag         = "aerolab-server"

	// client
	gcpClientTagUsedBy           = "used_by"
	gcpClientTagUsedByValue      = "aerolab4client"
	gcpClientTagClusterName      = "aerolab4client_name"
	gcpClientTagNodeNumber       = "aerolab4client_node_number"
	gcpClientTagOperatingSystem  = "aerolab4client_operating_system"
	gcpClientTagOSVersion        = "aerolab4client_operating_system_version"
	gcpClientTagAerospikeVersion = "aerolab4client_aerospike_version"
	gcpClientTagClientType       = "aerolab4client_type"
	gcpClientFirewallTag         = "aerolab-client"
)

var (
	gcpTagUsedBy           = gcpServerTagUsedBy
	gcpTagUsedByValue      = gcpServerTagUsedByValue
	gcpTagClusterName      = gcpServerTagClusterName
	gcpTagNodeNumber       = gcpServerTagNodeNumber
	gcpTagOperatingSystem  = gcpServerTagOperatingSystem
	gcpTagOSVersion        = gcpServerTagOSVersion
	gcpTagAerospikeVersion = gcpServerTagAerospikeVersion
	gcpTagCostPerHour      = "aerolab_cost_ph"
	gcpTagCostLastRun      = "aerolab_cost_sofar"
	gcpTagCostStartTime    = "aerolab_cost_starttime"
)

func (d *backendGcp) WorkOnClients() {
	d.server = false
	d.client = true
	gcpTagUsedBy = gcpClientTagUsedBy
	gcpTagUsedByValue = gcpClientTagUsedByValue
	gcpTagClusterName = gcpClientTagClusterName
	gcpTagNodeNumber = gcpClientTagNodeNumber
	gcpTagOperatingSystem = gcpClientTagOperatingSystem
	gcpTagOSVersion = gcpClientTagOSVersion
	gcpTagAerospikeVersion = gcpClientTagAerospikeVersion
}

func (d *backendGcp) WorkOnServers() {
	d.server = true
	d.client = false
	gcpTagUsedBy = gcpServerTagUsedBy
	gcpTagUsedByValue = gcpServerTagUsedByValue
	gcpTagClusterName = gcpServerTagClusterName
	gcpTagNodeNumber = gcpServerTagNodeNumber
	gcpTagOperatingSystem = gcpServerTagOperatingSystem
	gcpTagOSVersion = gcpServerTagOSVersion
	gcpTagAerospikeVersion = gcpServerTagAerospikeVersion
}

type gcpInstancePricing struct {
	onDemand struct {
		perCoreHour  float64
		perRamGBHour float64
		gpuPriceHour float64
		gpuUltraHour float64
	}
	spot struct {
		perCoreHour  float64
		perRamGBHour float64
		gpuPriceHour float64
		gpuUltraHour float64
	}
}

type gcpClusterExpiryInstances struct {
	labelFingerprint string
	labels           map[string]string
	zone             string
}

func (d *backendGcp) GetAZName(subnetId string) (string, error) {
	return "", nil
}

func (d *backendGcp) DisablePricingAPI() {
	d.disablePricing = true
}

func (d *backendGcp) DisableExpiryInstall() {
	d.disableExpiryInstall = true
}

func (d *backendGcp) TagVolume(fsId string, tagName string, tagValue string, zone string) error {
	tagValue = strings.ToLower(tagValue)
	ctx := context.Background()
	client, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewDisksRESTClient: %w", err)
	}
	defer client.Close()
	it := client.List(ctx, &computepb.ListDisksRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels.usedby=" + gcpTagEnclose("aerolab7")),
		Zone:    zone,
	})
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if *pair.Name != fsId {
			continue
		}
		labels := pair.Labels
		labelFinger := pair.LabelFingerprint
		labels[strings.ToLower(tagName)] = tagValue
		op, err := client.SetLabels(ctx, &computepb.SetLabelsDiskRequest{
			Project:  a.opts.Config.Backend.Project,
			Resource: fsId,
			Zone:     zone,
			ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
				LabelFingerprint: labelFinger,
				Labels:           labels,
			},
		})
		if err != nil {
			return err
		}
		return op.Wait(ctx)
	}
	return errors.New("disk not found")
}

func (d *backendGcp) CreateMountTarget(volume *inventoryVolume, subnet string, secGroups []string) (inventoryMountTarget, error) {
	return inventoryMountTarget{}, nil
}

func (d *backendGcp) MountTargetAddSecurityGroup(mountTarget *inventoryMountTarget, volume *inventoryVolume, addGroups []string) error {
	return nil
}

func (d *backendGcp) DeleteVolume(name string, zone string) error {
	ctx := context.Background()
	client, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewDisksRESTClient: %w", err)
	}
	defer client.Close()
	op, err := client.Delete(ctx, &computepb.DeleteDiskRequest{
		Zone:    zone,
		Project: a.opts.Config.Backend.Project,
		Disk:    name,
	})
	if err != nil {
		return err
	}
	err = op.Wait(ctx)
	return err
}

func (d *backendGcp) AttachVolume(name string, zone string, clusterName string, node int) error {
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer client.Close()
	op, err := client.AttachDisk(ctx, &computepb.AttachDiskInstanceRequest{
		Zone:     zone,
		Instance: fmt.Sprintf("aerolab4-%s-%d", clusterName, node),
		Project:  a.opts.Config.Backend.Project,
		AttachedDiskResource: &computepb.AttachedDisk{
			AutoDelete: proto.Bool(false),
			Boot:       proto.Bool(false),
			DeviceName: proto.String(name),
			Mode:       proto.String("READ_WRITE"),
			Type:       proto.String("PERSISTENT"),
			Source:     proto.String(fmt.Sprintf("zones/%s/disks/%s", zone, name)),
		},
	})
	if err != nil {
		return err
	}
	return op.Wait(ctx)
}

func (d *backendGcp) ResizeVolume(name string, zone string, newSize int64) error {
	ctx := context.Background()
	client, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewDisksRESTClient: %w", err)
	}
	defer client.Close()
	op, err := client.Resize(ctx, &computepb.ResizeDiskRequest{
		Zone:    zone,
		Project: a.opts.Config.Backend.Project,
		Disk:    name,
		DisksResizeRequestResource: &computepb.DisksResizeRequest{
			SizeGb: proto.Int64(newSize),
		},
	})
	if err != nil {
		return err
	}
	return op.Wait(ctx)
}

func (d *backendGcp) ExpiriesUpdateZoneID(zoneId string) error {
	return nil
}

func (d *backendGcp) GetInstanceTags(name string) (map[string]map[string]string, error) {
	return nil, nil
}

func (d *backendGcp) Tag(name string, key string, value string) error {
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer client.Close()

	// get instances
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := client.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						// tag instances
						instance.Labels[key] = value
						op, err := client.SetLabels(ctx, &computepb.SetLabelsInstanceRequest{
							Zone:     *instance.Zone,
							Project:  a.opts.Config.Backend.Project,
							Instance: *instance.Name,
							InstancesSetLabelsRequestResource: &computepb.InstancesSetLabelsRequest{
								LabelFingerprint: instance.Fingerprint,
								Labels:           instance.Labels,
							},
						})
						if err != nil {
							return err
						}
						if err := op.Wait(ctx); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

func (d *backendGcp) DetachVolume(name string, clusterName string, node int, zone string) error {
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer client.Close()
	op, err := client.DetachDisk(ctx, &computepb.DetachDiskInstanceRequest{
		Project:    a.opts.Config.Backend.Project,
		Zone:       zone,
		Instance:   fmt.Sprintf("aerolab4-%s-%d", clusterName, node),
		DeviceName: name,
	})
	if err != nil {
		return err
	}
	return op.Wait(ctx)
}

func (d *backendGcp) CreateVolume(name string, zone string, tags []string, expires time.Duration, size int64, desc string) error {
	ctx := context.Background()
	client, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewDisksRESTClient: %w", err)
	}
	defer client.Close()
	labels := make(map[string]string)
	for _, tag := range tags {
		ts := strings.Split(tag, "=")
		if len(ts) < 2 {
			return fmt.Errorf("incorrect label format, must be key=value")
		}
		labels[strings.ToLower(ts[0])] = strings.ToLower(strings.Join(ts[1:], "="))
	}
	labels["usedby"] = "aerolab7"
	if expires != 0 {
		deployRegion := strings.Split(zone, "-")
		if len(deployRegion) > 2 {
			deployRegion = deployRegion[:len(deployRegion)-1]
		}
		err := d.ExpiriesSystemInstall(10, strings.Join(deployRegion, "-"), "")
		if err != nil && err.Error() != "EXISTS" {
			log.Printf("WARNING: Failed to install the expiry system, EFS will not expire: %s", err)
		}
		labels["lastused"] = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(time.Now().Format(time.RFC3339), ":", "_"), "+", "-"))
		labels["expireduration"] = strings.ToLower(strings.ReplaceAll(expires.String(), ".", "_"))
	}
	op, err := client.Insert(ctx, &computepb.InsertDiskRequest{
		Zone:    zone,
		Project: a.opts.Config.Backend.Project,
		DiskResource: &computepb.Disk{
			Description: proto.String(desc),
			Labels:      labels,
			Name:        proto.String(name),
			SizeGb:      proto.Int64(int64(size)),
			Type:        proto.String(fmt.Sprintf("/zones/%s/diskTypes/pd-ssd", zone)),
		},
	})
	if err != nil {
		return err
	}
	err = op.Wait(ctx)
	return err
}

func (d *backendGcp) SetLabel(clusterName string, key string, value string, gcpZone string) error {
	if key == "agiLabel" {
		ctx := context.Background()
		client, err := compute.NewDisksRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewDisksRESTClient: %w", err)
		}
		defer client.Close()
		disk, err := client.Get(ctx, &computepb.GetDiskRequest{
			Disk:    clusterName,
			Project: a.opts.Config.Backend.Project,
			Zone:    gcpZone,
		})
		if err == nil {
			vals := gcplabels.PackToMap("agilabel", value)
			vals["agilabel"] = "set"
			for k, v := range disk.Labels {
				if strings.HasPrefix(k, "agilabel") {
					continue
				}
				vals[k] = v
			}
			_, err := client.SetLabels(ctx, &computepb.SetLabelsDiskRequest{
				Resource: clusterName,
				Project:  a.opts.Config.Backend.Project,
				Zone:     gcpZone,
				ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
					LabelFingerprint: disk.LabelFingerprint,
					Labels:           vals,
				},
			})
			if err != nil {
				return err
			}
		}
		client.Update(ctx, &computepb.UpdateDiskRequest{
			Disk:    clusterName,
			Project: a.opts.Config.Backend.Project,
			Zone:    gcpZone,
			DiskResource: &computepb.Disk{
				Description: proto.String(value),
				Name:        proto.String(clusterName),
			},
			UpdateMask: proto.String("labels"),
		})
	}
	instances := make(map[string]gcpClusterExpiryInstances)
	if d.server {
		j, err := d.Inventory("", []int{InventoryItemClusters})
		d.WorkOnServers()
		if err != nil {
			return err
		}
		for _, jj := range j.Clusters {
			if jj.ClusterName == clusterName {
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.GcpMetadataFingerprint, jj.GcpMeta, jj.Zone}
			}
		}
	} else {
		j, err := d.Inventory("", []int{InventoryItemClients})
		d.WorkOnClients()
		if err != nil {
			return err
		}
		for _, jj := range j.Clients {
			if jj.ClientName == clusterName {
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.GcpMetadataFingerprint, jj.GcpMeta, jj.Zone}
			}
		}
	}
	if len(instances) == 0 {
		return errors.New("not found any instances for the given name")
	}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()
	for iName, iMeta := range instances {
		iMeta.labels[key] = value
		items := []*computepb.Items{}
		for k, v := range iMeta.labels {
			items = append(items, &computepb.Items{Key: proto.String(k), Value: proto.String(v)})
		}
		_, err = instancesClient.SetMetadata(ctx, &computepb.SetMetadataInstanceRequest{
			Instance: iName,
			Project:  a.opts.Config.Backend.Project,
			Zone:     iMeta.zone,
			MetadataResource: &computepb.Metadata{
				Fingerprint: proto.String(iMeta.labelFingerprint),
				Items:       items,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *backendGcp) ClusterExpiry(zone string, clusterName string, expiry time.Duration, nodes []int) error {
	instances := make(map[string]gcpClusterExpiryInstances)
	if d.server {
		j, err := d.Inventory("", []int{InventoryItemClusters})
		d.WorkOnServers()
		if err != nil {
			return err
		}
		for _, jj := range j.Clusters {
			nodeNo, _ := strconv.Atoi(jj.NodeNo)
			if jj.ClusterName == clusterName && (len(nodes) == 0 || inslice.HasInt(nodes, nodeNo)) {
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.GcpLabelFingerprint, jj.GcpLabels, jj.Zone}
			}
		}
	} else {
		j, err := d.Inventory("", []int{InventoryItemClients})
		d.WorkOnClients()
		if err != nil {
			return err
		}
		for _, jj := range j.Clients {
			nodeNo, _ := strconv.Atoi(jj.NodeNo)
			if jj.ClientName == clusterName && (len(nodes) == 0 || inslice.HasInt(nodes, nodeNo)) {
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.GcpLabelFingerprint, jj.GcpLabels, jj.Zone}
			}
		}
	}
	if len(instances) == 0 {
		return errors.New("not found any instances for the given name")
	}
	var expiresTime time.Time
	if expiry != 0 {
		expiresTime = time.Now().Add(expiry)
	}
	newExpiry := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(expiresTime.Format(time.RFC3339), ":", "_"), "+", "-"))
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()
	for instanceName, labelData := range instances {
		newLabels := labelData.labels
		newLabels["aerolab4expires"] = newExpiry
		_, err = instancesClient.SetLabels(ctx, &computepb.SetLabelsInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     labelData.zone,
			Instance: instanceName,
			InstancesSetLabelsRequestResource: &computepb.InstancesSetLabelsRequest{
				LabelFingerprint: proto.String(labelData.labelFingerprint),
				Labels:           newLabels,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *backendGcp) getInstancePricePerHourEnableService(zone string) (map[string]*gcpInstancePricing, error) {
	log.Println("WARN: pricing API is not enabled, attempting to enable")
	out, err := exec.Command("gcloud", "services", "enable", "--project", a.opts.Config.Backend.Project, "cloudbilling.googleapis.com").CombinedOutput()
	if err != nil {
		return make(map[string]*gcpInstancePricing), fmt.Errorf("WARN: failed to enable cloud-billing API: %s, %s", err, string(out))
	}
	log.Println("Pricing API enabled, continuing")
	return d.getInstancePricesPerHour(zone)
}

// map[instanceTypePrefix]gcpInstancePricing
func (d *backendGcp) getInstancePricesPerHour(zone string) (map[string]*gcpInstancePricing, error) {
	zoneTest := strings.Split(zone, "-")
	if len(zoneTest) > 2 {
		zone = zoneTest[0] + "-" + zoneTest[1]
	}
	prices := make(map[string]*gcpInstancePricing)
	ctx := context.Background()
	svc, err := cloudbilling.NewService(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") {
			return d.getInstancePricePerHourEnableService(zone)
		} else {
			return prices, err
		}
	}
	srv := cloudbilling.NewServicesSkusService(svc)
	call := srv.List("services/6F81-5844-456A").CurrencyCode("USD")
	err = call.Pages(ctx, func(resp *cloudbilling.ListSkusResponse) error {
		for _, i := range resp.Skus {
			if i.Category.ResourceFamily != "Compute" {
				continue
			}
			onDemand := true
			if i.Category.UsageType == "Preemptible" {
				onDemand = false
			} else if i.Category.UsageType != "OnDemand" {
				continue
			}
			if !inslice.HasString(i.ServiceRegions, zone) {
				continue
			}
			iGroup := strings.Split(i.Description, " ")
			if len(iGroup) < 6 {
				continue
			}
			if !onDemand {
				iGroup = iGroup[2:]
			}
			if i.Category.ResourceGroup == "GPU" && iGroup[0] == "Nvidia" {
				grp := ""
				if strings.HasPrefix(i.Description, "Nvidia Tesla A100 GPU ") {
					grp = "a2"
				} else if strings.HasPrefix(i.Description, "Nvidia Tesla A100 80GB GPU ") {
					grp = "a2"
				} else if strings.HasPrefix(i.Description, "Nvidia L4 GPU ") {
					grp = "g2"
				} else {
					continue
				}
				if _, ok := prices[grp]; !ok {
					prices[grp] = &gcpInstancePricing{}
				}
				if strings.HasPrefix(i.Description, "Nvidia Tesla A100 GPU ") {
					for _, j := range i.PricingInfo {
						if j.PricingExpression == nil {
							continue
						}
						if j.PricingExpression.UsageUnit != "h" {
							continue
						}
						for _, x := range j.PricingExpression.TieredRates {
							if x.UnitPrice == nil {
								continue
							}
							if x.UnitPrice.CurrencyCode != "USD" {
								continue
							}
							if onDemand {
								prices[grp].onDemand.gpuPriceHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							} else {
								prices[grp].spot.gpuPriceHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							}
						}
					}
				} else if strings.HasPrefix(i.Description, "Nvidia Tesla A100 80GB GPU ") {
					for _, j := range i.PricingInfo {
						if j.PricingExpression == nil {
							continue
						}
						if j.PricingExpression.UsageUnit != "h" {
							continue
						}
						for _, x := range j.PricingExpression.TieredRates {
							if x.UnitPrice == nil {
								continue
							}
							if x.UnitPrice.CurrencyCode != "USD" {
								continue
							}
							if onDemand {
								prices[grp].onDemand.gpuUltraHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							} else {
								prices[grp].spot.gpuUltraHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							}
						}
					}
				} else if strings.HasPrefix(i.Description, "Nvidia L4 GPU ") {
					for _, j := range i.PricingInfo {
						if j.PricingExpression == nil {
							continue
						}
						if j.PricingExpression.UsageUnit != "h" {
							continue
						}
						for _, x := range j.PricingExpression.TieredRates {
							if x.UnitPrice == nil {
								continue
							}
							if x.UnitPrice.CurrencyCode != "USD" {
								continue
							}
							if onDemand {
								prices[grp].onDemand.gpuPriceHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							} else {
								prices[grp].spot.gpuPriceHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							}
						}
					}
				}
				continue
			}
			if len(i.PricingInfo) == 0 || i.PricingInfo[0].PricingExpression == nil || len(i.PricingInfo[0].PricingExpression.TieredRates) == 0 {
				continue
			}
			if i.Category.ResourceGroup == "G1Small" {
				for _, j := range i.PricingInfo {
					if j.PricingExpression == nil {
						continue
					}
					if j.PricingExpression.UsageUnit != "h" {
						continue
					}
					for _, x := range j.PricingExpression.TieredRates {
						if x.UnitPrice == nil {
							continue
						}
						if x.UnitPrice.CurrencyCode != "USD" {
							continue
						}
						grp := "g1"
						if _, ok := prices[grp]; !ok {
							prices[grp] = &gcpInstancePricing{}
						}
						if onDemand {
							prices["g1"].onDemand.perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							prices["g1"].onDemand.perRamGBHour = 0.0000000000001
						} else {
							prices["g1"].spot.perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							prices["g1"].spot.perRamGBHour = 0.0000000000001
						}
					}
				}
				continue
			} else if i.Category.ResourceGroup == "F1Micro" {
				for _, j := range i.PricingInfo {
					if j.PricingExpression == nil {
						continue
					}
					if j.PricingExpression.UsageUnit != "h" {
						continue
					}
					for _, x := range j.PricingExpression.TieredRates {
						if x.UnitPrice == nil {
							continue
						}
						if x.UnitPrice.CurrencyCode != "USD" {
							continue
						}
						grp := "f1"
						if _, ok := prices[grp]; !ok {
							prices[grp] = &gcpInstancePricing{}
						}
						if onDemand {
							prices["f1"].onDemand.perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							prices["f1"].onDemand.perRamGBHour = 0.0000000000001
						} else {
							prices["f1"].spot.perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							prices["f1"].spot.perRamGBHour = 0.0000000000001
						}
					}
				}
				continue
			} else if (iGroup[1] != "Instance" || (iGroup[2] != "Core" && iGroup[2] != "Ram") || iGroup[3] != "running" || iGroup[4] != "in") && ((iGroup[1] != "Predefined" && iGroup[1] != "AMD" && iGroup[1] != "Arm" && iGroup[1] != "Memory-optimized") || iGroup[2] != "Instance" || (iGroup[3] != "Core" && iGroup[3] != "Ram") || iGroup[4] != "running" || iGroup[5] != "in") {
				continue
			}
			grp := strings.ToLower(iGroup[0])
			if _, ok := prices[grp]; !ok {
				prices[grp] = &gcpInstancePricing{}
			}
			if i.Category.ResourceGroup != "CPU" && i.Category.ResourceGroup != "RAM" && iGroup[1] == "Predefined" {
				if iGroup[3] == "Core" {
					i.Category.ResourceGroup = "CPU"
				} else if iGroup[3] == "Ram" {
					i.Category.ResourceGroup = "RAM"
				}
			}
			switch i.Category.ResourceGroup {
			case "CPU":
				for _, j := range i.PricingInfo {
					if j.PricingExpression == nil {
						continue
					}
					if j.PricingExpression.UsageUnit != "h" {
						continue
					}
					for _, x := range j.PricingExpression.TieredRates {
						if x.UnitPrice == nil {
							continue
						}
						if x.UnitPrice.CurrencyCode != "USD" {
							continue
						}
						if onDemand {
							prices[grp].onDemand.perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
						} else {
							prices[grp].spot.perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
						}
					}
				}
			case "RAM":
				for _, j := range i.PricingInfo {
					if j.PricingExpression == nil {
						continue
					}
					if j.PricingExpression.UsageUnit != "GiBy.h" {
						continue
					}
					for _, x := range j.PricingExpression.TieredRates {
						if x.UnitPrice == nil {
							continue
						}
						if x.UnitPrice.CurrencyCode != "USD" {
							continue
						}
						if onDemand {
							prices[grp].onDemand.perRamGBHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
						} else {
							prices[grp].spot.perRamGBHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") {
			return d.getInstancePricePerHourEnableService(zone)
		} else {
			return prices, err
		}
	}
	return prices, nil
}

func (d *backendGcp) getInstanceTypesFromCache(zone string) ([]instanceType, error) {
	cacheFile, err := a.aerolabRootDir()
	if err != nil {
		return nil, err
	}
	cacheFile = path.Join(cacheFile, "cache", "gcp.instance-types."+zone+".json")
	f, err := os.Open(cacheFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	ita := &instanceTypesCache{}
	err = dec.Decode(ita)
	if err != nil {
		return nil, err
	}
	if ita.Expires.Before(time.Now()) {
		return ita.InstanceTypes, errors.New("cache expired")
	}
	return ita.InstanceTypes, nil
}

func (d *backendGcp) saveInstanceTypesToCache(it []instanceType, zone string) error {
	cacheDir, err := a.aerolabRootDir()
	if err != nil {
		return err
	}
	cacheDir = path.Join(cacheDir, "cache")
	cacheFile := path.Join(cacheDir, "gcp.instance-types."+zone+".json")
	if _, err := os.Stat(cacheDir); err != nil {
		if err = os.Mkdir(cacheDir, 0700); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(cacheFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	ita := &instanceTypesCache{
		Expires:       time.Now().Add(24 * time.Hour),
		InstanceTypes: it,
	}
	err = enc.Encode(ita)
	if err != nil {
		os.Remove(cacheFile)
		return err
	}
	return nil
}

func (d *backendGcp) getInstanceTypes(zone string) ([]instanceType, error) {
	it, err := d.getInstanceTypesFromCache(zone)
	if err == nil {
		return it, err
	}
	saveCache := true
	prices := make(map[string]*gcpInstancePricing)
	if !d.disablePricing {
		prices, err = d.getInstancePricesPerHour(zone)
		if err != nil {
			log.Printf("WARN: pricing error: %s", err)
			saveCache = false
		}
	}
	it = []instanceType{}
	// find instance types
	ctx := context.Background()
	instancesClient, err := compute.NewMachineTypesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()
	req := &computepb.AggregatedListMachineTypesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("zone=" + zone),
	}
	ita := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := ita.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		isArm := false
		isX86 := false
		for _, t := range pair.Value.MachineTypes {
			if strings.HasPrefix(*t.Name, "ct5l-") || strings.HasPrefix(*t.Name, "ct5lp-") {
				continue
			}
			if strings.HasPrefix(*t.Name, "t2") {
				isArm = true
			} else {
				isX86 = true
			}
			eph := 0
			ephs := float64(0)
			if !*t.IsSharedCpu && !strings.HasPrefix(*t.Name, "e2-") && !strings.HasPrefix(*t.Name, "t2d-") && !strings.HasPrefix(*t.Name, "t2a-") && !strings.HasPrefix(*t.Name, "m2-") && !strings.HasPrefix(*t.Name, "h3-") {
				if strings.HasPrefix(*t.Name, "c2-") || strings.HasPrefix(*t.Name, "c2d-") || strings.HasPrefix(*t.Name, "a2-") || strings.HasPrefix(*t.Name, "m1-") || strings.HasPrefix(*t.Name, "m3-") || strings.HasPrefix(*t.Name, "g2-") {
					eph = 8
					ephs = 3 * 1000
				} else if strings.HasPrefix(*t.Name, "a3-") {
					eph = 16
					ephs = 6 * 1000
				} else if strings.HasPrefix(*t.Name, "n1-") || strings.HasPrefix(*t.Name, "n2-") || strings.HasPrefix(*t.Name, "n2d-") {
					eph = 24
					ephs = 9 * 1000
				} else if strings.HasPrefix(*t.Name, "c3-") || strings.HasPrefix(*t.Name, "c3d-") {
					eph = 32
					ephs = 12 * 1000
				} else if strings.HasPrefix(*t.Name, "n4-") && (strings.HasSuffix(*t.Name, "-2") || strings.HasSuffix(*t.Name, "-4") || strings.HasSuffix(*t.Name, "-8")) {
					eph = 16
					ephs = 257 * 1024
				} else if strings.HasPrefix(*t.Name, "n4-") {
					eph = 32
					ephs = 512 * 1024
				} else {
					eph = -1
					ephs = -1
				}
			}
			prices := prices[strings.Split(*t.Name, "-")[0]]
			price := float64(-1)
			spotPrice := float64(-1)
			accels := 0
			for _, acc := range t.Accelerators {
				accels += int(*acc.GuestAcceleratorCount)
			}
			if prices != nil && prices.onDemand.perCoreHour > 0 && prices.onDemand.perRamGBHour > 0 && prices.onDemand.gpuPriceHour >= 0 {
				price = prices.onDemand.perCoreHour*float64(*t.GuestCpus) + prices.onDemand.perRamGBHour*float64(*t.MemoryMb/1024)
				if strings.Contains(*t.Name, "-ultragpu") {
					price = price + prices.onDemand.gpuUltraHour*float64(accels)
				} else {
					price = price + prices.onDemand.gpuPriceHour*float64(accels)
				}
			}
			if prices != nil && prices.spot.perCoreHour > 0 && prices.spot.perRamGBHour > 0 && prices.spot.gpuPriceHour >= 0 {
				spotPrice = prices.spot.perCoreHour*float64(*t.GuestCpus) + prices.spot.perRamGBHour*float64(*t.MemoryMb/1024)
				if strings.Contains(*t.Name, "-ultragpu") {
					spotPrice = spotPrice + prices.spot.gpuUltraHour*float64(accels)
				} else {
					spotPrice = spotPrice + prices.spot.gpuPriceHour*float64(accels)
				}
			}
			it = append(it, instanceType{
				InstanceName:             *t.Name,
				CPUs:                     int(t.GetGuestCpus()),
				RamGB:                    float64(t.GetMemoryMb()) / 1024,
				EphemeralDisks:           eph,
				EphemeralDiskTotalSizeGB: ephs,
				PriceUSD:                 price,
				SpotPriceUSD:             spotPrice,
				IsArm:                    isArm,
				IsX86:                    isX86,
			})
		}
	}
	// end search
	if saveCache {
		err = d.saveInstanceTypesToCache(it, zone)
		if err != nil {
			log.Printf("WARN: failed to save instance types to cache: %s", err)
		}
	}
	return it, nil
}

func (d *backendGcp) GetInstanceTypes(minCpu int, maxCpu int, minRam float64, maxRam float64, minDisks int, maxDisks int, findArm bool, gcpZone string) ([]instanceType, error) {
	if gcpZone == "" {
		return nil, errors.New("GCP Zone is required, specify with --zone")
	}
	ita, err := d.getInstanceTypes(gcpZone)
	if err != nil {
		return ita, err
	}
	it := []instanceType{}
	for _, i := range ita {
		if findArm && !i.IsArm {
			continue
		}
		if !findArm && !i.IsX86 {
			continue
		}
		if (minCpu > 0 && i.CPUs < minCpu) || (maxCpu > 0 && i.CPUs > maxCpu) {
			continue
		}
		if (minRam > 0 && i.RamGB < minRam) || (maxRam > 0 && i.RamGB > maxRam) {
			continue
		}
		if (minDisks > 0 && i.EphemeralDisks < minDisks) || (maxDisks > 0 && i.EphemeralDisks > maxDisks) {
			continue
		}
		it = append(it, i)
	}
	return it, nil
}

func (d *backendGcp) Inventory(filterOwner string, inventoryItems []int) (inventoryJson, error) {
	ij := inventoryJson{}

	if inslice.HasInt(inventoryItems, InventoryItemExpirySystem) {
		ij.ExpirySystem = append(ij.ExpirySystem, inventoryExpiry{})
		rd, err := a.aerolabRootDir()
		if err != nil {
			return ij, err
		}
		nWait := new(sync.WaitGroup)
		nWait.Add(1)
		defer nWait.Wait()
		go func() {
			defer nWait.Done()
			if lastRegion, err := os.ReadFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project)); err == nil {
				if out, err := exec.Command("gcloud", "scheduler", "jobs", "describe", "aerolab-expiries", "--location", string(lastRegion), "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err == nil {
					scanner := bufio.NewScanner(bytes.NewReader(out))
					for scanner.Scan() {
						line := strings.TrimRight(scanner.Text(), "\r\n\t ")
						if strings.HasPrefix(line, "name: projects/") && strings.HasSuffix(line, "/jobs/aerolab-expiries") {
							ij.ExpirySystem[0].Scheduler = strings.TrimPrefix(line, "name: ")
						} else if strings.HasPrefix(line, "schedule: '") && strings.HasSuffix(line, "'") {
							ij.ExpirySystem[0].Schedule = strings.TrimSuffix(strings.TrimPrefix(line, "schedule: '"), "'")
						}
					}
				}
				if out, err := exec.Command("gcloud", "functions", "describe", "aerolab-expiries", "--region", string(lastRegion), "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err == nil {
					scanner := bufio.NewScanner(bytes.NewReader(out))
					bucket := ""
					object := ""
					inSource := false
					inStorageSource := false
					for scanner.Scan() {
						line := strings.TrimRight(scanner.Text(), "\r\n\t ")
						if strings.HasPrefix(line, "name: projects/") && strings.HasSuffix(line, "/functions/aerolab-expiries") {
							ij.ExpirySystem[0].Function = strings.TrimPrefix(line, "name: ")
						} else if strings.HasPrefix(line, "  source:") {
							inSource = true
						} else if inSource && strings.HasPrefix(line, "    storageSource:") {
							inStorageSource = true
						} else if inStorageSource && strings.HasPrefix(line, "      bucket:") {
							bucket = strings.TrimPrefix(line, "      bucket: ")
						} else if inStorageSource && strings.HasPrefix(line, "      object:") {
							object = strings.TrimPrefix(line, "      object: ")
						}
						if bucket != "" && object != "" && ij.ExpirySystem[0].SourceBucket == "" {
							ij.ExpirySystem[0].SourceBucket = bucket + "/" + object
						}
					}
				}
			}
		}()
	}

	if inslice.HasInt(inventoryItems, InventoryItemTemplates) {
		tmpl, err := d.ListTemplates()
		if err != nil {
			return ij, err
		}
		for _, d := range tmpl {
			arch := "amd64"
			if d.isArm {
				arch = "arm64"
			}
			ij.Templates = append(ij.Templates, inventoryTemplate{
				AerospikeVersion: d.aerospikeVersion,
				Distribution:     d.distroName,
				OSVersion:        d.distroVersion,
				Arch:             arch,
			})
		}
	}

	if inslice.HasInt(inventoryItems, InventoryItemFirewalls) {
		ctx := context.Background()
		firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
		if err != nil {
			return ij, fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer firewallsClient.Close()
		req := &computepb.ListFirewallsRequest{
			Project: a.opts.Config.Backend.Project,
		}
		it := firewallsClient.List(ctx, req)
		for {
			firewallRule, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return ij, err
			}
			allow := []string{}
			for _, all := range firewallRule.Allowed {
				allow = append(allow, all.Ports...)
			}
			deny := []string{}
			for _, all := range firewallRule.Denied {
				deny = append(deny, all.Ports...)
			}
			ij.FirewallRules = append(ij.FirewallRules, inventoryFirewallRule{
				GCP: &inventoryFirewallRuleGCP{
					FirewallName: *firewallRule.Name,
					TargetTags:   firewallRule.TargetTags,
					SourceTags:   firewallRule.SourceTags,
					SourceRanges: firewallRule.SourceRanges,
					AllowPorts:   allow,
					DenyPorts:    deny,
				},
			})
		}
	}

	nCheckList := []int{}
	if inslice.HasInt(inventoryItems, InventoryItemClusters) {
		nCheckList = []int{1}
	}
	if inslice.HasInt(inventoryItems, InventoryItemClients) {
		nCheckList = append(nCheckList, 2)
	}
	for _, i := range nCheckList {
		if i == 1 {
			d.WorkOnServers()
		} else {
			d.WorkOnClients()
		}
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return ij, fmt.Errorf("newInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()
		// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
		reqi := &computepb.AggregatedListInstancesRequest{
			Project: a.opts.Config.Backend.Project,
			Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
		}
		iti := instancesClient.AggregatedList(ctx, reqi)
		for {
			pair, err := iti.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return ij, err
			}
			instances := pair.Value.Instances
			if len(instances) > 0 {
				for _, instance := range instances {
					if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
						if filterOwner != "" {
							if instance.Labels["owner"] != filterOwner {
								continue
							}
						}
						sysArch := "x86_64"
						if arm, _ := d.IsSystemArm(*instance.MachineType); arm {
							sysArch = "aarch64"
						}
						pubIp := ""
						privIp := ""
						if len(instance.NetworkInterfaces) > 0 {
							if instance.NetworkInterfaces[0].NetworkIP != nil && *instance.NetworkInterfaces[0].NetworkIP != "" {
								privIp = *instance.NetworkInterfaces[0].NetworkIP
							}
							if len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
								if instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != nil && *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != "" {
									pubIp = *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
								}
							}
						}
						zoneSplit := strings.Split(*instance.Zone, "/zones/")
						zone := ""
						if len(zoneSplit) > 1 {
							zone = zoneSplit[1]
						}
						startTime := int(0)
						lastRunCost := float64(0)
						pricePerHour := float64(0)
						lastRunCost, err = strconv.ParseFloat(gcplabelDecode(instance.Labels[gcpTagCostLastRun]), 64)
						if err != nil {
							lastRunCost = 0
						}
						pricePerHour, err = strconv.ParseFloat(gcplabelDecode(instance.Labels[gcpTagCostPerHour]), 64)
						if err != nil {
							pricePerHour = 0
						}
						startTime, err = strconv.Atoi(gcplabelDecode(instance.Labels[gcpTagCostStartTime]))
						if err != nil {
							if instance.LastStartTimestamp != nil {
								startTimeT, err := time.Parse(time.RFC3339, *instance.LastStartTimestamp)
								if err == nil && !startTimeT.IsZero() {
									startTime = int(startTimeT.Unix())
								}
							}
							if startTime == 0 && instance.CreationTimestamp != nil {
								startTimeT, err := time.Parse(time.RFC3339, *instance.CreationTimestamp)
								if err == nil && !startTimeT.IsZero() {
									startTime = int(startTimeT.Unix())
								}
							}
						}
						currentCost := lastRunCost
						if startTime != 0 {
							now := int(time.Now().Unix())
							delta := now - startTime
							deltaH := float64(delta) / 3600
							currentCost = lastRunCost + (pricePerHour * deltaH)
						}
						expires := strings.ToUpper(strings.ReplaceAll(instance.Labels["aerolab4expires"], "_", ":"))
						if strings.HasPrefix(expires, "0001-01-01") {
							expires = ""
						}
						features, _ := strconv.Atoi(instance.Labels["aerolab4features"])
						meta := make(map[string]string)
						if instance.Metadata != nil {
							for _, metaItem := range instance.Metadata.Items {
								meta[metaItem.GetKey()] = metaItem.GetValue()
							}
						}
						srcimg := ""
						for _, dd := range instance.Disks {
							if dd.GetBoot() {
								srcimg, err = func() (string, error) {
									clt := strings.Split(*dd.Source, "/")
									clz := strings.Split(*instance.Zone, "/")
									ctx := context.Background()
									client, err := compute.NewDisksRESTClient(ctx)
									if err != nil {
										return "", fmt.Errorf("NewDisksRESTClient: %w", err)
									}
									defer client.Close()
									disk, err := client.Get(ctx, &computepb.GetDiskRequest{
										Project: a.opts.Config.Backend.Project,
										Zone:    clz[len(clz)-1],
										Disk:    clt[len(clt)-1],
									})
									if err != nil {
										return "", err
									}
									return *disk.SourceImage, nil
								}()
								if err != nil {
									log.Printf("WARN: could not get boot disk information: %s", err)
								}
								break
							}
						}
						isSpot := false
						if val, ok := instance.Labels["isspot"]; ok && val == "true" {
							isSpot = true
						}
						if currentCost < 0 {
							currentCost = 0
						}
						sshKeyPath, err := d.GetKeyPath(instance.Labels[gcpTagClusterName])
						if err != nil {
							sshKeyPath = ""
						}
						if i == 1 {
							ij.Clusters = append(ij.Clusters, inventoryCluster{
								ClusterName:            instance.Labels[gcpTagClusterName],
								NodeNo:                 instance.Labels[gcpTagNodeNumber],
								InstanceId:             *instance.Name,
								ImageId:                srcimg,
								State:                  *instance.Status,
								Arch:                   sysArch,
								Distribution:           instance.Labels[gcpTagOperatingSystem],
								OSVersion:              instance.Labels[gcpTagOSVersion],
								AerospikeVersion:       instance.Labels[gcpTagAerospikeVersion],
								PrivateIp:              privIp,
								PublicIp:               pubIp,
								Firewalls:              instance.Tags.Items,
								Zone:                   zone,
								InstanceRunningCost:    currentCost,
								Owner:                  instance.Labels["owner"],
								GcpLabelFingerprint:    *instance.LabelFingerprint,
								Expires:                expires,
								AGILabel:               meta["agiLabel"],
								GcpLabels:              instance.Labels,
								GcpMeta:                meta,
								GcpMetadataFingerprint: *instance.Metadata.Fingerprint,
								Features:               FeatureSystem(features),
								InstanceType:           *instance.MachineType,
								GcpIsSpot:              isSpot,
								SSHKeyPath:             sshKeyPath,
							})
						} else {
							ij.Clients = append(ij.Clients, inventoryClient{
								InstanceType:           *instance.MachineType,
								ClientName:             instance.Labels[gcpTagClusterName],
								NodeNo:                 instance.Labels[gcpTagNodeNumber],
								InstanceId:             *instance.Name,
								ImageId:                srcimg,
								State:                  *instance.Status,
								Arch:                   sysArch,
								Distribution:           instance.Labels[gcpTagOperatingSystem],
								OSVersion:              instance.Labels[gcpTagOSVersion],
								AerospikeVersion:       instance.Labels[gcpTagAerospikeVersion],
								PrivateIp:              privIp,
								PublicIp:               pubIp,
								ClientType:             instance.Labels[gcpClientTagClientType],
								Firewalls:              instance.Tags.Items,
								Zone:                   zone,
								InstanceRunningCost:    currentCost,
								Owner:                  instance.Labels["owner"],
								GcpLabelFingerprint:    *instance.LabelFingerprint,
								Expires:                expires,
								GcpLabels:              instance.Labels,
								GcpMeta:                meta,
								GcpMetadataFingerprint: *instance.Metadata.Fingerprint,
								GcpIsSpot:              isSpot,
								SSHKeyPath:             sshKeyPath,
							})
						}
					}
				}
			}
		}
	}

	if inslice.HasInt(inventoryItems, InventoryItemVolumes) {
		zones, err := listZones()
		if err != nil {
			return ij, err
		}
		wait := new(sync.WaitGroup)
		lock := new(sync.Mutex)
		for _, zone := range zones {
			wait.Add(1)
			go func(zone string) {
				defer wait.Done()
				ctx := context.Background()
				client, err := compute.NewDisksRESTClient(ctx)
				if err != nil {
					log.Printf("zone=%s NewDisksRESTClient: %s", zone, err)
					return
				}
				defer client.Close()
				it := client.List(ctx, &computepb.ListDisksRequest{
					Project: a.opts.Config.Backend.Project,
					Filter:  proto.String("labels.usedby=" + gcpTagEnclose("aerolab7")),
					Zone:    zone,
				})
				for {
					pair, err := it.Next()
					if err == iterator.Done {
						break
					}
					if err != nil {
						log.Printf("zone=%s iterator: %s", zone, err)
						return
					}
					ts, _ := time.Parse(time.RFC3339, *pair.CreationTimestamp)
					lock.Lock()
					nzone := strings.Split(pair.GetZone(), "/")
					attachedTo := []string{}
					for _, user := range pair.Users {
						userx := strings.Split(user, "/")
						attachedTo = append(attachedTo, strings.TrimPrefix(userx[len(userx)-1], "aerolab4-"))
					}
					agiVolume := false
					if _, ok := pair.Labels["aginodim"]; ok {
						agiVolume = true
					}
					// expiry
					expiry := ""
					if lastUsed, ok := pair.Labels["lastused"]; ok {
						lastUsed = strings.ToUpper(strings.ReplaceAll(lastUsed, "_", ":"))
						if expireDuration, ok := pair.Labels["expireduration"]; ok {
							expireDuration = strings.ReplaceAll(expireDuration, "_", ".")
							lu, err := time.Parse(time.RFC3339, lastUsed)
							if err == nil {
								ed, err := time.ParseDuration(expireDuration)
								if err == nil {
									expiresTime := lu.Add(ed)
									expiresIn := expiresTime.Sub(time.Now().In(expiresTime.Location()))
									expiry = expiresIn.Round(time.Minute).String()
								}
							}
						}
					}
					// expiry end
					ij.Volumes = append(ij.Volumes, inventoryVolume{
						AvailabilityZoneId:   *pair.Zone,
						AvailabilityZoneName: nzone[len(nzone)-1],
						CreationTime:         ts,
						LifeCycleState:       *pair.Status,
						Name:                 *pair.Name,
						FileSystemId:         *pair.Name,
						SizeBytes:            int(*pair.SizeGb) * 1024 * 1024 * 1024,
						SizeString:           convSize(*pair.SizeGb * 1024 * 1024 * 1024),
						AgiLabel:             gcplabels.UnpackNoErr(pair.Labels, "agilabel"),
						Tags:                 pair.Labels,
						Owner:                pair.Labels["aerolab7owner"],
						AGIVolume:            agiVolume,
						GCP: inventoryVolumeGcp{
							AttachedTo:       attachedTo,
							AttachedToString: strings.Join(attachedTo, ", "),
							Description:      pair.GetDescription(),
						},
						ExpiresIn: expiry,
					})
					lock.Unlock()
				}
			}(zone)
		}
		wait.Wait()
	}
	return ij, nil
}

func (d *backendGcp) GetInstanceIpMap(name string, internalIPs bool) (map[string]string, error) {
	return nil, nil
}

func (d *backendGcp) DomainCreate(zoneId string, fqdn string, IP string, wait bool) (err error) {
	return nil
}

func listZones() ([]string, error) {
	ctx := context.Background()
	client, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	it := client.List(ctx, &computepb.ListZonesRequest{
		Project: a.opts.Config.Backend.Project,
	})
	zones := []string{}
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		zones = append(zones, pair.GetName())
	}
	sort.Strings(zones)
	return zones, nil
}

func gcplabelDecode(a string) string {
	return strings.ReplaceAll(a, "-", ".")
}

func gcplabelEncodeToSave(a string) string {
	return strings.ReplaceAll(a, ".", "-")
}

func (d *backendGcp) CreateNetwork(name string, driver string, subnet string, mtu string) error {
	return nil
}
func (d *backendGcp) DeleteNetwork(name string) error {
	return nil
}
func (d *backendGcp) PruneNetworks() error {
	return nil
}
func (d *backendGcp) ListNetworks(csv bool, writer io.Writer) error {
	return nil
}

func (d *backendGcp) Init() error {
	return nil
}

func gcpTagEnclose(str string) string {
	return "\"" + str + "\""
}

func (d *backendGcp) IsSystemArm(systemType string) (bool, error) {
	stypes := strings.Split(systemType, "/")
	systemType = stypes[len(stypes)-1]
	return strings.HasPrefix(systemType, "t2a"), nil
}

func (d *backendGcp) ClusterList() ([]string, error) {
	clist := []string{}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if !inslice.HasString(clist, instance.Labels[gcpTagClusterName]) {
						clist = append(clist, instance.Labels[gcpTagClusterName])
					}
				}
			}
		}
	}
	return clist, nil
}

func (d *backendGcp) IsNodeArm(clusterName string, nodeNumber int) (bool, error) {
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return false, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == clusterName && instance.Labels[gcpTagNodeNumber] == strconv.Itoa(nodeNumber) {
						return d.IsSystemArm(*instance.MachineType)
					}
				}
			}
		}
	}
	return false, fmt.Errorf("cluster %s node %v not found", clusterName, nodeNumber)
}

func (d *backendGcp) NodeListInCluster(name string) ([]int, error) {
	nlist := []int{}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						nodeNo, err := strconv.Atoi(instance.Labels[gcpTagNodeNumber])
						if err != nil {
							return nil, fmt.Errorf("found aerolab instance without valid tag format: %v", *instance.Name)
						}
						nlist = append(nlist, nodeNo)
					}
				}
			}
		}
	}
	return nlist, nil
}

func (d *backendGcp) GetClusterNodeIps(name string) ([]string, error) {
	nlist := []string{}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						if len(instance.NetworkInterfaces) > 0 && instance.NetworkInterfaces[0].NetworkIP != nil && *instance.NetworkInterfaces[0].NetworkIP != "" {
							nlist = append(nlist, *instance.NetworkInterfaces[0].NetworkIP)
						}
					}
				}
			}
		}
	}
	return nlist, nil
}

func (d *backendGcp) GetNodeIpMap(name string, internalIPs bool) (map[int]string, error) {
	nlist := make(map[int]string)
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if strings.HasPrefix(*instance.Name, "aerolab4-template-") {
					continue
				}
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						nodeNo, err := strconv.Atoi(instance.Labels[gcpTagNodeNumber])
						if err != nil {
							return nil, fmt.Errorf("found aerolab instance with incorrect labels: %v", *instance.Name)
						}
						ip := "N/A"
						if len(instance.NetworkInterfaces) > 0 {
							if internalIPs {
								if instance.NetworkInterfaces[0].NetworkIP != nil && *instance.NetworkInterfaces[0].NetworkIP != "" {
									ip = *instance.NetworkInterfaces[0].NetworkIP
								}
							} else {
								if len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
									if instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != nil && *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != "" {
										ip = *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
									}
								}
							}
						}
						nlist[nodeNo] = ip
					}
				}
			}
		}
	}
	return nlist, nil
}

func (d *backendGcp) ClusterListFull(isJson bool, owner string, pager bool, isPretty bool, sort []string, renderer string, theme string, noNotes bool) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.Owner = owner
	a.opts.Inventory.List.Pager = pager
	a.opts.Inventory.List.JsonPretty = isPretty
	a.opts.Inventory.List.SortBy = sort
	a.opts.Inventory.List.RenderType = renderer
	a.opts.Inventory.List.Theme = theme
	a.opts.Inventory.List.NoNotes = noNotes
	return "", a.opts.Inventory.List.run(d.server, d.client, false, false, false)
}

type instanceDetail struct {
	instanceZone       string
	instanceName       string
	labels             map[string]string
	labelFingerprint   string
	LastStartTimestamp *string
	CreationTimestamp  *string
	instanceState      string
}

func (d *backendGcp) getInstanceDetails(name string, nodes []int) (zones map[int]instanceDetail, err error) {
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return nil, fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones = make(map[int]instanceDetail)
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						for _, node := range nodes {
							if strconv.Itoa(node) == instance.Labels[gcpTagNodeNumber] {
								zone := strings.Split(*instance.Zone, "/")
								var lrt, ct string
								var lrtp, ctp *string
								if instance.LastStartTimestamp != nil {
									lrt = *instance.LastStartTimestamp
									lrtp = &lrt
								}
								if instance.CreationTimestamp != nil {
									ct = *instance.CreationTimestamp
									ctp = &ct
								}
								zones[node] = instanceDetail{
									instanceZone:       zone[len(zone)-1],
									instanceName:       *instance.Name,
									labels:             instance.Labels,
									labelFingerprint:   *instance.LabelFingerprint,
									LastStartTimestamp: lrtp,
									CreationTimestamp:  ctp,
									instanceState:      *instance.Status,
								}
							}
						}
					}
				}
			}
		}
	}
	return zones, nil
}

func (d *backendGcp) ClusterStart(name string, nodes []int) error {
	var err error
	ops := []gcpMakeOps{}
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones, err := d.getInstanceDetails(name, nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, ok := zones[node]; !ok {
			return fmt.Errorf("node %v not found", node)
		}
	}
	for _, node := range nodes {
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()
		if zones[node].instanceState == "TERMINATED" {
			zones[node].labels[gcpTagCostStartTime] = gcplabelEncodeToSave(strconv.Itoa(int(time.Now().Unix())))
			_, err = instancesClient.SetLabels(ctx, &computepb.SetLabelsInstanceRequest{
				Project:  a.opts.Config.Backend.Project,
				Zone:     zones[node].instanceZone,
				Instance: zones[node].instanceName,
				InstancesSetLabelsRequestResource: &computepb.InstancesSetLabelsRequest{
					LabelFingerprint: proto.String(zones[node].labelFingerprint),
					Labels:           zones[node].labels,
				},
			})
			if err != nil {
				return fmt.Errorf("unable label instance: %w", err)
			}
		}
		req := &computepb.StartInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     zones[node].instanceZone,
			Instance: zones[node].instanceName,
		}
		op, err := instancesClient.Start(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to start instance: %w", err)
		}
		ops = append(ops, gcpMakeOps{
			op:  op,
			ctx: ctx,
		})
	}

	for _, o := range ops {
		if err = o.op.Wait(o.ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}

	_, keyPath, err := d.getKey(name)
	if err != nil {
		return fmt.Errorf("unable to locate keyPath for the cluster: %s", err)
	}

	nodeIps, err := d.GetNodeIpMap(name, false)
	if err != nil {
		return err
	}
	nodeCount := len(nodes)

	for {
		working := 0
		for _, node := range nodes {
			instIp := nodeIps[node]
			_, err = remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "ls", 0)
			if err == nil {
				working++
			} else {
				fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", instIp, keyPath, err)
				time.Sleep(time.Second)
			}
		}
		if working >= nodeCount {
			break
		}
	}
	return nil
}

func (d *backendGcp) ClusterStop(name string, nodes []int) error {
	var err error
	ops := []gcpMakeOps{}
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones, err := d.getInstanceDetails(name, nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, ok := zones[node]; !ok {
			return fmt.Errorf("node %v not found", node)
		}
	}
	for _, node := range nodes {
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()
		req := &computepb.StopInstanceRequest{
			Project:         a.opts.Config.Backend.Project,
			Zone:            zones[node].instanceZone,
			Instance:        zones[node].instanceName,
			DiscardLocalSsd: proto.Bool(true),
		}
		op, err := instancesClient.Stop(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to stop instance: %w", err)
		}
		// label
		if zones[node].instanceState != "TERMINATED" {
			startTime := int(0)
			lastRunCost := float64(0)
			pricePerHour := float64(0)
			lastRunCost, err = strconv.ParseFloat(gcplabelDecode(zones[node].labels[gcpTagCostLastRun]), 64)
			if err != nil {
				lastRunCost = 0
			}
			pricePerHour, err = strconv.ParseFloat(gcplabelDecode(zones[node].labels[gcpTagCostPerHour]), 64)
			if err != nil {
				pricePerHour = 0
			}
			startTime, err = strconv.Atoi(gcplabelDecode(zones[node].labels[gcpTagCostStartTime]))
			if err != nil {
				if zones[node].LastStartTimestamp != nil {
					startTimeT, err := time.Parse(time.RFC3339, *zones[node].LastStartTimestamp)
					if err == nil && !startTimeT.IsZero() {
						startTime = int(startTimeT.Unix())
					}
				}
				if startTime == 0 && zones[node].CreationTimestamp != nil {
					startTimeT, err := time.Parse(time.RFC3339, *zones[node].CreationTimestamp)
					if err == nil && !startTimeT.IsZero() {
						startTime = int(startTimeT.Unix())
					}
				}
			}
			lastRunCost = lastRunCost + (pricePerHour * (float64((int(time.Now().Unix()) - startTime)) / 3600))
			zones[node].labels[gcpTagCostStartTime] = "0"
			zones[node].labels[gcpTagCostLastRun] = gcplabelEncodeToSave(strconv.FormatFloat(lastRunCost, 'f', 8, 64))
			_, err = instancesClient.SetLabels(ctx, &computepb.SetLabelsInstanceRequest{
				Project:  a.opts.Config.Backend.Project,
				Zone:     zones[node].instanceZone,
				Instance: zones[node].instanceName,
				InstancesSetLabelsRequestResource: &computepb.InstancesSetLabelsRequest{
					LabelFingerprint: proto.String(zones[node].labelFingerprint),
					Labels:           zones[node].labels,
				},
			})
			if err != nil {
				return fmt.Errorf("unable label instance: %w", err)
			}
		}
		// label end
		ops = append(ops, gcpMakeOps{
			op:  op,
			ctx: ctx,
		})
	}
	for _, o := range ops {
		if err = o.op.Wait(o.ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) ClusterDestroy(name string, nodes []int) error {
	var err error
	ops := []gcpMakeOps{}
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones, err := d.getInstanceDetails(name, nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, ok := zones[node]; !ok {
			return fmt.Errorf("node %v not found", node)
		}
	}
	for _, node := range nodes {
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()
		req := &computepb.DeleteInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     zones[node].instanceZone,
			Instance: zones[node].instanceName,
		}
		op, err := instancesClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete instance: %w", err)
		}
		ops = append(ops, gcpMakeOps{
			op:  op,
			ctx: ctx,
		})
	}
	if len(ops) > 0 {
		log.Printf("Terminate command sent to %d instances, waiting for GCP to finish terminating", len(ops))
	}
	for _, o := range ops {
		if err = o.op.Wait(o.ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	cl, err := d.ClusterList()
	if err != nil {
		return fmt.Errorf("could not kill key as ClusterList failed: %s", err)
	}
	if !inslice.HasString(cl, name) {
		d.killKey(name)
	}
	return nil
}

func (d *backendGcp) ListTemplates() ([]backendVersion, error) {
	ctx := context.Background()
	bv := []backendVersion{}
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()
	req := computepb.ListImagesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}

	it := imagesClient.List(ctx, &req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if image.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
			isArm := false
			if strings.Contains(strings.ToLower(*image.Architecture), "arm") || strings.Contains(strings.ToLower(*image.Architecture), "aarch") {
				isArm = true
			}
			bv = append(bv, backendVersion{
				distroName:       image.Labels[gcpServerTagOperatingSystem],
				distroVersion:    gcpResourceNameBack(image.Labels[gcpServerTagOSVersion]),
				aerospikeVersion: gcpResourceNameBack(image.Labels[gcpServerTagAerospikeVersion]),
				isArm:            isArm,
			})
		}
	}
	return bv, nil
}

func (d *backendGcp) deleteImage(name string) error {
	ctx := context.Background()
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()

	req := computepb.DeleteImageRequest{
		Image:   name,
		Project: a.opts.Config.Backend.Project,
	}
	op, err := imagesClient.Delete(ctx, &req)
	if err != nil {
		return err
	}
	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) TemplateDestroy(v backendVersion) error {
	ctx := context.Background()
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()
	req := computepb.ListImagesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}

	it := imagesClient.List(ctx, &req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if image.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
			isArm := false
			if strings.Contains(strings.ToLower(*image.Architecture), "arm") || strings.Contains(strings.ToLower(*image.Architecture), "aarch") {
				isArm = true
			}
			if (image.Labels[gcpServerTagOperatingSystem] == v.distroName || v.distroName == "all") && (image.Labels[gcpServerTagOSVersion] == gcpResourceName(v.distroVersion) || v.distroVersion == "all") && (image.Labels[gcpServerTagAerospikeVersion] == gcpResourceName(v.aerospikeVersion) || v.aerospikeVersion == "all") && isArm == v.isArm {
				err = d.deleteImage(*image.Name)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (d *backendGcp) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
	fr := []fileListReader{}
	for _, f := range files {
		fr = append(fr, fileListReader{
			filePath:     f.filePath,
			fileSize:     f.fileSize,
			fileContents: strings.NewReader(f.fileContents),
		})
	}
	return d.CopyFilesToClusterReader(name, fr, nodes)
}

func (d *backendGcp) CopyFilesToClusterReader(name string, files []fileListReader, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	nodeIps, err := d.GetNodeIpMap(name, false)
	if err != nil {
		return fmt.Errorf("could not get node ip map for cluster: %s", err)
	}
	_, keypath, err := d.getKey(name)
	if err != nil {
		return fmt.Errorf("could not get key: %s", err)
	}
	for _, node := range nodes {
		err = scp("root", fmt.Sprintf("%s:22", nodeIps[node]), keypath, files)
		if err != nil {
			return fmt.Errorf("scp failed: %s", err)
		}
	}
	return err
}

func (d *backendGcp) RunCommands(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
	var fout [][]byte
	var err error
	if nodes == nil {
		nodes, err = d.NodeListInCluster(clusterName)
		if err != nil {
			return nil, err
		}
	}
	_, keypath, err := d.getKey(clusterName)
	if err != nil {
		return nil, fmt.Errorf("could not get key:%s", err)
	}
	nodeIps, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return nil, fmt.Errorf("could not get node ip map:%s", err)
	}
	for _, node := range nodes {
		var out []byte
		var err error
		for _, command := range commands {
			comm := command[0]
			for _, c := range command[1:] {
				if strings.Contains(c, " ") {
					comm = comm + " \"" + c + "\""
				} else {
					comm = comm + " " + c
				}
			}
			out, err = remoteRun("root", fmt.Sprintf("%s:22", nodeIps[node]), keypath, comm, node)
			fout = append(fout, out)
			if checkExecRetcode(err) != 0 {
				return fout, fmt.Errorf("error running `%s`: %s", comm, err)
			}
		}
	}
	return fout, nil
}

func (d *backendGcp) AttachAndRun(clusterName string, node int, command []string, isInteractive bool) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr, isInteractive, nil)
}

func (d *backendGcp) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, isInteractive bool, dockerForceUser *string) (err error) {
	clusters, err := d.ClusterList()
	if err != nil {
		return fmt.Errorf("could not get cluster list: %s", err)
	}
	if !inslice.HasString(clusters, clusterName) {
		return fmt.Errorf("cluster not found")
	}
	nodes, err := d.NodeListInCluster(clusterName)
	if err != nil {
		return fmt.Errorf("could not get node list: %s", err)
	}
	if !inslice.HasInt(nodes, node) {
		return fmt.Errorf("node not found")
	}
	nodeIp, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return fmt.Errorf("could not get node ip map: %s", err)
	}
	_, keypath, err := d.getKey(clusterName)
	if err != nil {
		return fmt.Errorf("could not get key path: %s", err)
	}
	var comm string
	if len(command) > 0 {
		comm = command[0]
		for _, c := range command[1:] {
			if strings.Contains(c, " ") {
				comm = comm + " \"" + strings.ReplaceAll(c, "\"", "\\\"") + "\""
			} else {
				comm = comm + " " + c
			}
		}
	} else {
		comm = "bash"
	}
	err = remoteAttachAndRun("root", fmt.Sprintf("%s:22", nodeIp[node]), keypath, comm, stdin, stdout, stderr, node, isInteractive)
	return err
}

func (d *backendGcp) Upload(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
	nodes, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return errors.New("found no nodes in cluster")
	}
	nodeIp, ok := nodes[node]
	if !ok {
		return errors.New("node not found in cluster")
	}
	_, key, err := d.getKey(clusterName)
	if err != nil {
		return err
	}
	return scpExecUpload("root", nodeIp, "22", key, source, destination, os.Stdout, 30*time.Second, verbose, legacy)
}

func (d *backendGcp) Download(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
	nodes, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return errors.New("found no nodes in cluster")
	}
	nodeIp, ok := nodes[node]
	if !ok {
		return errors.New("node not found in cluster")
	}
	_, key, err := d.getKey(clusterName)
	if err != nil {
		return err
	}
	return scpExecDownload("root", nodeIp, "22", key, source, destination, os.Stdout, 30*time.Second, verbose, legacy)
}

func (d *backendGcp) vacuum(v *backendVersion) error {
	isArm := "amd"
	if v != nil && v.isArm {
		isArm = "arm"
	}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	delList := []*computepb.Instance{}
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("listing instances: %s", err)
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if (v != nil && *instance.Name == fmt.Sprintf("aerolab4-template-%s-%s-%s-%s", v.distroName, gcpResourceName(v.distroVersion), gcpResourceName(v.aerospikeVersion), isArm)) || (v == nil && strings.HasPrefix(*instance.Name, "aerolab4-template-")) {
						instance := instance
						delList = append(delList, instance)
					}
				}
			}
		}
	}
	return d.deleteInstances(delList)
}

func gcpResourceName(name string) string {
	return strings.ReplaceAll(name, ".", "-")
}

func gcpResourceNameBack(name string) string {
	return strings.ReplaceAll(name, "-", ".")
}

func (d *backendGcp) deleteInstances(list []*computepb.Instance) error {
	delOps := make(map[context.Context]*compute.Operation)
	for _, instance := range list {
		ctx := context.Background()
		deleteClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer deleteClient.Close()
		zone := strings.Split(*instance.Zone, "/")
		log.Printf("Deleting project=%s zone=%s name=%s", a.opts.Config.Backend.Project, zone[len(zone)-1], *instance.Name)
		req := &computepb.DeleteInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     zone[len(zone)-1],
			Instance: *instance.Name,
		}
		op, err := deleteClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete instance: %w", err)
		}
		delOps[ctx] = op
	}
	for ctx, op := range delOps {
		if err := op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) VacuumTemplate(v backendVersion) error {
	return d.vacuum(&v)
}

func (d *backendGcp) VacuumTemplates() error {
	return d.vacuum(nil)
}

func (d *backendGcp) TemplateListFull(isJson bool, pager bool, isPretty bool, sort []string, renderer string, theme string, noNotes bool) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.Pager = pager
	a.opts.Inventory.List.JsonPretty = isPretty
	a.opts.Inventory.List.SortBy = sort
	a.opts.Inventory.List.RenderType = renderer
	a.opts.Inventory.List.Theme = theme
	a.opts.Inventory.List.NoNotes = noNotes
	return "", a.opts.Inventory.List.run(false, false, true, false, false)
}

func (d *backendGcp) GetKeyPath(clusterName string) (keyPath string, err error) {
	_, p, e := d.getKey(clusterName)
	return p, e
}

// get KeyPair
func (d *backendGcp) getKey(clusterName string) (keyName string, keyPath string, err error) {
	homeDir, err := a.aerolabRootDir()
	if err == nil {
		if _, err := os.Stat(path.Join(homeDir, "sshkey")); err == nil {
			return "sshkey", path.Join(homeDir, "sshkey"), nil
		}
	}
	keyName = fmt.Sprintf("aerolab-gcp-%s", clusterName)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	// check keypath exists, if not, error
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		err = fmt.Errorf("key does not exist in given location: %s", keyPath)
		return keyName, keyPath, err
	}
	return
}

// get KeyPair
func (d *backendGcp) makeKey(clusterName string) (keyName string, keyPath string, err error) {
	homeDir, err := a.aerolabRootDir()
	if err == nil {
		if _, err := os.Stat(path.Join(homeDir, "sshkey")); err == nil {
			return "sshkey", path.Join(homeDir, "sshkey"), nil
		}
	}
	keyName = fmt.Sprintf("aerolab-gcp-%s", clusterName)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	_, _, err = d.getKey(clusterName)
	if err == nil {
		return
	}
	// check keypath exists, if not, make
	if _, err := os.Stat(string(a.opts.Config.Backend.SshKeyPath)); os.IsNotExist(err) {
		os.MkdirAll(string(a.opts.Config.Backend.SshKeyPath), 0700)
	}

	// generate keypair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}

	// generate and write private key as PEM
	privateKeyFile, err := os.OpenFile(keyPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer privateKeyFile.Close()
	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return keyName, keyPath, err
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return
	}
	err = os.WriteFile(keyPath+".pub", ssh.MarshalAuthorizedKey(pub), 0600)
	if err != nil {
		return
	}

	return
}

// get KeyPair
func (d *backendGcp) killKey(clusterName string) (keyName string, keyPath string, err error) {
	homeDir, err := a.aerolabRootDir()
	if err == nil {
		if _, err := os.Stat(path.Join(homeDir, "sshkey")); err == nil {
			return "sshkey", path.Join(homeDir, "sshkey"), nil
		}
	}
	keyName = fmt.Sprintf("aerolab-gcp-%s", clusterName)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	os.Remove(keyPath)
	os.Remove(keyPath + ".pub")
	return
}

var deployGcpTemplateShutdownMaking = make(chan int, 1)

func (d *backendGcp) getImage(v backendVersion) (string, error) {
	imName := ""
	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return "", fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer client.Close()
	filter := ""
	arch := "X86_64"
	if v.isArm {
		arch = "ARM64"
	}
	project := ""
	switch v.distroName {
	case "debian":
		filter = fmt.Sprintf("(family:%s) AND (architecture:%s)", gcpTagEnclose("debian-"+v.distroVersion+"*"), gcpTagEnclose(arch))
		project = "debian-cloud"
	case "centos":
		dv := v.distroVersion
		if dv != "6" && dv != "7" {
			dv = "stream-" + dv
		}
		filter = fmt.Sprintf("(family:%s) AND (architecture:%s)", gcpTagEnclose("centos-"+dv+"*"), gcpTagEnclose(arch))
		project = "centos-cloud"
	case "rocky":
		dv := v.distroVersion
		project = "rocky-linux-cloud"
		filter = fmt.Sprintf("(family:%s) AND (architecture:%s)", gcpTagEnclose("rocky-linux-"+dv+"-optimized-*"), gcpTagEnclose(arch))
	case "ubuntu":
		filter = fmt.Sprintf("(family:%s) AND (architecture:%s)", gcpTagEnclose("ubuntu-"+strings.ReplaceAll(v.distroVersion, ".", "")+"-lts*"), gcpTagEnclose(arch))
		project = "ubuntu-os-cloud"
	}
	req := &computepb.ListImagesRequest{
		Filter:  &filter,
		Project: project,
	}
	it := client.List(ctx, req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", err
		}
		if *image.Name > imName {
			imName = *image.SelfLink
		}
	}
	return imName, nil
}

func (d *backendGcp) makeLabels(extra []string, isArm string, v backendVersion) (map[string]string, error) {
	labels := make(map[string]string)
	badNames := []string{gcpTagOperatingSystem, gcpTagOSVersion, gcpTagAerospikeVersion, gcpTagUsedBy, gcpTagClusterName, gcpTagNodeNumber, "Arch", "Name"}
	for _, extraTag := range extra {
		kv := strings.Split(extraTag, "=")
		if len(kv) < 2 {
			return nil, errors.New("tags must follow key=value format")
		}
		key := kv[0]
		if inslice.HasString(badNames, key) {
			return nil, fmt.Errorf("key %s is used internally by aerolab, cannot be used", key)
		}
		val := strings.Join(kv[1:], "=")

		labels[key] = val
	}
	labels[gcpTagOperatingSystem] = v.distroName
	labels[gcpTagOSVersion] = strings.ReplaceAll(v.distroVersion, ".", "-")
	labels[gcpTagAerospikeVersion] = strings.ReplaceAll(v.aerospikeVersion, ".", "-")
	labels[gcpTagUsedBy] = gcpTagUsedByValue
	labels["arch"] = isArm
	return labels, nil
}

func (d *backendGcp) validateLabels(labels map[string]string) error {
	for key, val := range labels {
		for _, char := range val {
			// TODO: document why these characters are validated by ascii code here
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return fmt.Errorf("invalid tag value for `%s`, only the following are allowed: [a-zA-Z0-9_-]", val)
			}
		}
		for _, char := range key {
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return fmt.Errorf("invalid tag name for `%s`, only the following are allowed: [a-zA-Z0-9_-]", key)
			}
		}
	}

	return nil
}

func (d *backendGcp) DeployTemplate(v backendVersion, script string, files []fileListReader, extra *backendExtra) error {
	if extra.zone == "" {
		return errors.New("zone must be specified")
	}
	if v.distroName == "amazon" {
		return errors.New("amazon not supported on gcp backend")
	}
	addShutdownHandler("deployGcpTemplate", func(os.Signal) {
		deployGcpTemplateShutdownMaking <- 1
		d.VacuumTemplate(v)
	})
	defer delShutdownHandler("deployGcpTemplate")
	genKeyName := "template" + strconv.Itoa(int(time.Now().Unix()))
	_, keyPath, err := d.getKey(genKeyName)
	if err != nil {
		d.killKey(genKeyName)
		_, keyPath, err = d.makeKey(genKeyName)
		if err != nil {
			return fmt.Errorf("could not obtain or make 'template' key:%s", err)
		}
	}
	defer d.killKey(genKeyName)
	sshKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return err
	}
	// start VM
	var imageName string
	if extra.ami != "" {
		imageName = extra.ami
	} else {
		imageName, err = d.getImage(v)
		if err != nil {
			return err
		}
	}

	isArm := "amd"
	if v.isArm {
		isArm = "arm"
	}
	labels, err := d.makeLabels(extra.labels, isArm, v)
	if err != nil {
		return err
	}

	err = d.validateLabels(labels)
	if err != nil {
		return err
	}

	name := fmt.Sprintf("aerolab4-template-%s-%s-%s-%s", v.distroName, gcpResourceName(v.distroVersion), gcpResourceName(v.aerospikeVersion), isArm)

	for _, fnp := range extra.firewallNamePrefix {
		err = d.createSecurityGroupsIfNotExist(fnp, extra.isAgiFirewall, true, []string{}, false)
		if err != nil {
			return fmt.Errorf("firewall: %s", err)
		}
	}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	instanceType := "e2-medium"
	if v.isArm {
		instanceType = "t2a-standard-1"
	}
	fnp := append(extra.firewallNamePrefix, "aerolab-server")
	tags := append(extra.tags, fnp...)
	req := &computepb.InsertInstanceRequest{
		Project: a.opts.Config.Backend.Project,
		Zone:    extra.zone,
		InstanceResource: &computepb.Instance{
			Metadata: &computepb.Metadata{
				Items: []*computepb.Items{
					{
						Key:   proto.String("ssh-keys"),
						Value: proto.String("root:" + string(sshKey)),
					},
					{
						Key:   proto.String("startup-script"),
						Value: proto.String(d.startupScriptEnableRootLogin()),
					},
				},
			},
			Labels:      labels,
			Name:        proto.String(name),
			MachineType: proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", extra.zone, instanceType)),
			Tags: &computepb.Tags{
				Items: tags,
			},
			Scheduling: &computepb.Scheduling{
				AutomaticRestart:  proto.Bool(true),
				OnHostMaintenance: proto.String("MIGRATE"),
				ProvisioningModel: proto.String("STANDARD"),
			},
			Disks: []*computepb.AttachedDisk{
				{
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						DiskSizeGb:  proto.Int64(20),
						SourceImage: proto.String(imageName),
					},
					AutoDelete: proto.Bool(true),
					Boot:       proto.Bool(true),
					Type:       proto.String(computepb.AttachedDisk_PERSISTENT.String()),
				},
			},
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					StackType: proto.String("IPV4_ONLY"),
					AccessConfigs: []*computepb.AccessConfig{
						{
							Name:        proto.String("External NAT"),
							NetworkTier: proto.String("PREMIUM"),
						},
					},
				},
			},
		},
	}

	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	op, err := instancesClient.Insert(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to create instance: %w", err)
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	inst, err := instancesClient.Get(ctx, &computepb.GetInstanceRequest{
		Project:  a.opts.Config.Backend.Project,
		Zone:     extra.zone,
		Instance: name,
	})
	if err != nil {
		return fmt.Errorf("failed to poll new instance: %s", err)
	}
	if len(inst.NetworkInterfaces) < 1 || len(inst.NetworkInterfaces[0].AccessConfigs) < 1 || inst.NetworkInterfaces[0].AccessConfigs[0].NatIP == nil {
		return errors.New("instance failed to obtain public IP")
	}
	instIp := *inst.NetworkInterfaces[0].AccessConfigs[0].NatIP
	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}
	for {
		// wait for instances to be made available via SSH
		_, err = remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "ls", 0)
		if err == nil {
			break
		} else {
			fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", instIp, keyPath, err)
			time.Sleep(time.Second)
		}
	}

	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	fmt.Println("Connection succeeded, continuing deployment...")

	// copy files as required to VM
	files = append(files, fileListReader{"/root/installer.sh", strings.NewReader(script), len(script)})
	err = scp("root", fmt.Sprintf("%s:22", instIp), keyPath, files)
	if err != nil {
		return fmt.Errorf("scp failed: %s", err)
	}
	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// run script as required
	_, err = remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "chmod 755 /root/installer.sh", 0)
	if err != nil {
		out, err := remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "chmod 755 /root/installer.sh", 0)
		if err != nil {
			return fmt.Errorf("chmod failed: %s\n%s", string(out), err)
		}
	}
	out, err := remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "/bin/bash -c /root/installer.sh", 0)
	if err != nil {
		return fmt.Errorf("/root/installer.sh failed: %s\n%s", string(out), err)
	}
	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// shutdown VM
	reqStop := &computepb.StopInstanceRequest{
		Project:  a.opts.Config.Backend.Project,
		Zone:     extra.zone,
		Instance: *inst.Name,
	}
	op, err = instancesClient.Stop(ctx, reqStop)
	if err != nil {
		return fmt.Errorf("unable to stop instance: %w", err)
	}
	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// create ami from VM
	imgName := fmt.Sprintf("aerolab4-template-%s-%s-%s-%s", v.distroName, gcpResourceName(v.distroVersion), gcpResourceName(v.aerospikeVersion), isArm)
	err = d.makeImageFromDisk(extra.zone, *inst.Name, imgName, labels)
	if err != nil {
		return fmt.Errorf("makeImageFromDisk: %s", err)
	}

	// destroy VM
	reqd := &computepb.DeleteInstanceRequest{
		Project:  a.opts.Config.Backend.Project,
		Zone:     extra.zone,
		Instance: *inst.Name,
	}
	_, err = instancesClient.Delete(ctx, reqd)
	if err != nil {
		return fmt.Errorf("unable to destroy instance: %w", err)
	}
	//if err = op.Wait(ctx); err != nil {
	//	return fmt.Errorf("unable to wait for the operation: %w", err)
	//}
	return nil
}

func (d *backendGcp) makeImageFromDisk(zone string, sourceDiskName string, imageName string, imageLabels map[string]string) error {
	ctx := context.Background()
	disksClient, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewDisksRESTClient: %w", err)
	}
	defer disksClient.Close()
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()

	// Get the source disk
	source_req := &computepb.GetDiskRequest{
		Disk:    sourceDiskName,
		Project: a.opts.Config.Backend.Project,
		Zone:    zone,
	}

	disk, err := disksClient.Get(ctx, source_req)
	if err != nil {
		return fmt.Errorf("unable to get source disk: %w", err)
	}

	// Create the image
	req := computepb.InsertImageRequest{
		ForceCreate: proto.Bool(true),
		ImageResource: &computepb.Image{
			Name:             &imageName,
			SourceDisk:       disk.SelfLink,
			StorageLocations: nil,
			Labels:           imageLabels,
		},
		Project: a.opts.Config.Backend.Project,
	}

	op, err := imagesClient.Insert(ctx, &req)
	if err != nil {
		return err
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	return nil
}

func (d *backendGcp) ListSubnets() error {
	return errors.New("not implemented")
}

func (d *backendGcp) AssignSecurityGroups(clusterName string, names []string, zone string, remove bool, performLocking bool, extraPorts []string, noDefaults bool) error {
	ctx := context.Background()
	for _, name := range names {
		if err := d.createSecurityGroupsIfNotExist(name, false, performLocking, extraPorts, noDefaults); err != nil {
			return err
		}
	}
	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer client.Close()

	nodes, err := d.NodeListInCluster(clusterName)
	if err != nil {
		return err
	}
	var ops []*compute.Operation
	for _, node := range nodes {
		inst, err := client.Get(ctx, &computepb.GetInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Instance: fmt.Sprintf("aerolab4-%s-%d", clusterName, node),
			Zone:     zone,
		})
		if err != nil {
			return err
		}
		var newNames []string
		if remove {
			newNames = []string{}
			for _, name := range inst.Tags.Items {
				if !inslice.HasString(names, name) {
					newNames = append(newNames, name)
				}
			}
		} else {
			newNames = names
			for _, name := range inst.Tags.Items {
				if !inslice.HasString(newNames, name) {
					newNames = append(newNames, name)
				}
			}
		}
		setTags := &computepb.SetTagsInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Instance: fmt.Sprintf("aerolab4-%s-%d", clusterName, node),
			TagsResource: &computepb.Tags{
				Items:       newNames,
				Fingerprint: inst.Tags.Fingerprint,
			},
			Zone: zone,
		}
		op, err := client.SetTags(ctx, setTags)
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	for opid, op := range ops {
		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation on node %d: %w", opid, err)
		}
	}
	return nil
}

func (d *backendGcp) CreateSecurityGroups(vpc string, namePrefix string, isAgi bool, extraPorts []string, noDefaults bool) error {
	return d.createSecurityGroupsIfNotExist(namePrefix, isAgi, true, extraPorts, noDefaults)
}

func (d *backendGcp) createSecurityGroupExternal(namePrefix string, isAgi bool, extraPorts []string, noDefaults bool) error {
	ports := []string{"22", "3000", "443", "80", "8080", "8888", "9200", "8182", "8998", "8081"}
	if isAgi {
		ports = []string{"22", "80", "443"}
	}
	if noDefaults {
		ports = []string{}
	}
	for _, eport := range extraPorts {
		if !inslice.HasString(ports, eport) {
			ports = append(ports, eport)
		}
	}
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	firewallRule := &computepb.Firewall{
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: proto.String("tcp"),
				Ports:      ports,
			},
		},
		Direction: proto.String(computepb.Firewall_INGRESS.String()),
		Name:      proto.String(namePrefix),
		TargetTags: []string{
			namePrefix,
		},
		Description: proto.String("Allowing external access to aerolab-managed instance services"),
		SourceRanges: []string{
			"0.0.0.0/0",
		},
	}

	req := &computepb.InsertFirewallRequest{
		Project:          a.opts.Config.Backend.Project,
		FirewallResource: firewallRule,
	}

	op, err := firewallsClient.Insert(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to create firewall rule: %w", err)
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) createSecurityGroupInternal() error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	firewallRule2 := &computepb.Firewall{
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: proto.String("all"),
			},
		},
		Direction: proto.String(computepb.Firewall_INGRESS.String()),
		Name:      proto.String("aerolab-managed-internal"),
		TargetTags: []string{
			"aerolab-server",
			"aerolab-client",
		},
		SourceTags: []string{
			"aerolab-server",
			"aerolab-client",
		},
		Description: proto.String("Allowing internal access to aerolab-managed instance services"),
	}

	req2 := &computepb.InsertFirewallRequest{
		Project:          a.opts.Config.Backend.Project,
		FirewallResource: firewallRule2,
	}

	op2, err := firewallsClient.Insert(ctx, req2)
	if err != nil {
		return fmt.Errorf("unable to create firewall rule: %w", err)
	}

	if err = op2.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) LockSecurityGroups(ip string, lockSSH bool, vpc string, namePrefix string, isAgi bool, extraPorts []string, noDefaults bool) error {
	ports := []string{"22", "3000", "443", "80", "8080", "8888", "9200", "8182", "8998", "8081"}
	if isAgi {
		ports = []string{"22", "80", "443"}
	}
	if noDefaults {
		ports = []string{}
	}
	for _, eport := range extraPorts {
		if !inslice.HasString(ports, eport) {
			ports = append(ports, eport)
		}
	}
	if isAgi {
		ip = "0.0.0.0/0"
	} else {
		if ip == "discover-caller-ip" {
			ip = getip2()
		}
		if !strings.Contains(ip, "/") {
			ip = ip + "/32"
		}
	}
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	firewallRule := &computepb.Firewall{
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: proto.String("tcp"),
				Ports:      ports,
			},
		},
		Direction: proto.String(computepb.Firewall_INGRESS.String()),
		Name:      proto.String(namePrefix),
		TargetTags: []string{
			namePrefix,
		},
		Description: proto.String("Allowing external access to aerolab-managed instance services"),
		SourceRanges: []string{
			ip,
		},
	}

	req := &computepb.UpdateFirewallRequest{
		Project:          a.opts.Config.Backend.Project,
		Firewall:         namePrefix,
		FirewallResource: firewallRule,
	}

	op, err := firewallsClient.Update(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to update firewall rule: %w", err)
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) DeleteSecurityGroups(vpc string, namePrefix string, internal bool) error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	req := &computepb.ListFirewallsRequest{
		Project: a.opts.Config.Backend.Project,
	}
	it := firewallsClient.List(ctx, req)
	existInternal := false
	existExternal := false
	for {
		firewallRule, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if *firewallRule.Name == "aerolab-managed-internal" && internal {
			existInternal = true
		}
		if *firewallRule.Name == namePrefix {
			existExternal = true
		}
	}

	if existExternal {
		req := &computepb.DeleteFirewallRequest{
			Project:  a.opts.Config.Backend.Project,
			Firewall: namePrefix,
		}

		op, err := firewallsClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete firewall rule: %w", err)
		}

		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}

	if existInternal {
		req := &computepb.DeleteFirewallRequest{
			Project:  a.opts.Config.Backend.Project,
			Firewall: "aerolab-managed-internal",
		}

		op, err := firewallsClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete firewall rule: %w", err)
		}

		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) ListSecurityGroups() error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	req := &computepb.ListFirewallsRequest{
		Project: a.opts.Config.Backend.Project,
	}

	it := firewallsClient.List(ctx, req)
	output := []string{}
	for {
		firewallRule, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		allow := []string{}
		for _, all := range firewallRule.Allowed {
			allow = append(allow, all.Ports...)
		}
		deny := []string{}
		for _, all := range firewallRule.Denied {
			deny = append(deny, all.Ports...)
		}
		output = append(output, fmt.Sprintf("%s\t%v\t%v\t%v\t%v\t%v", *firewallRule.Name, firewallRule.TargetTags, firewallRule.SourceTags, firewallRule.SourceRanges, allow, deny))
	}
	sort.Strings(output)
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 4, ' ', 0)
	fmt.Fprintln(w, "FirewallName\tTargetTags\tSourceTags\tSourceRanges\tAllowPorts\tDenyPorts")
	fmt.Fprintln(w, "------------\t----------\t----------\t------------\t----------\t---------")
	for _, line := range output {
		fmt.Fprint(w, line+"\n")
	}
	w.Flush()
	return nil
}

func (d *backendGcp) createSecurityGroupsIfNotExist(namePrefix string, isAgi bool, performLocking bool, extraPorts []string, noDefaults bool) error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	req := &computepb.ListFirewallsRequest{
		Project: a.opts.Config.Backend.Project,
	}

	it := firewallsClient.List(ctx, req)
	existInternal := false
	existExternal := false
	needsLock := performLocking
	for {
		firewallRule, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if *firewallRule.Name == "aerolab-managed-internal" {
			existInternal = true
		}
		if *firewallRule.Name == namePrefix {
			existExternal = true
			myIp := getip2()
			parsedIp := net.ParseIP(myIp)
			for _, allowed := range firewallRule.Allowed {
				extraPorts = append(extraPorts, allowed.Ports...)
			}
			for _, iprange := range firewallRule.SourceRanges {
				if strings.Contains(iprange, "/") {
					_, cidr, err := net.ParseCIDR(iprange)
					if err != nil || cidr == nil {
						log.Printf("WARNING: failed to parse Source Range CIDR (%s): %s", iprange, err)
					} else if cidr.Contains(parsedIp) {
						needsLock = false
						break
					}
				} else {
					nip := net.ParseIP(iprange)
					if nip.Equal(parsedIp) {
						needsLock = false
						break
					}
				}
			}
		}
	}
	if !existInternal {
		err = d.createSecurityGroupInternal()
		if err != nil {
			return err
		}
	}
	if !existExternal {
		err = d.createSecurityGroupExternal(namePrefix, isAgi, extraPorts, noDefaults)
		if err != nil {
			return err
		}
		if !isAgi {
			err = d.LockSecurityGroups("discover-caller-ip", true, "", namePrefix, isAgi, extraPorts, noDefaults)
			if err != nil {
				return err
			}
		}
	} else if needsLock {
		log.Println("Security group CIDR doesn't allow this command to complete, re-locking security groups with the caller's IP")
		err = d.LockSecurityGroups("discover-caller-ip", true, "", namePrefix, isAgi, extraPorts, noDefaults)
		if err != nil {
			return err
		}
	}
	return nil
}

type gcpMakeOps struct {
	ctx context.Context
	op  *compute.Operation
}

func (d *backendGcp) DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error {
	name = strings.Trim(name, "\r\n\t ")
	if extra.zone == "" {
		return errors.New("zone must be specified")
	}
	if extra.instanceType == "" {
		return errors.New("instance type must be specified")
	}
	if v.distroName == "amazon" {
		return errors.New("amazon not supported on gcp backend")
	}
	isArm := "amd"
	if v.isArm {
		isArm = "arm"
	}
	labels, err := d.makeLabels(extra.labels, isArm, v)
	if err != nil {
		return err
	}
	labels[gcpTagClusterName] = name
	if extra.clientType == "" {
		extra.clientType = "not-available"
	}
	labels[gcpClientTagClientType] = extra.clientType
	price := float64(-1)
	it, err := d.getInstanceTypes(extra.zone)
	if err == nil {
		for _, iti := range it {
			if iti.InstanceName == extra.instanceType {
				price = iti.PriceUSD
			}
		}
	}
	labels[gcpTagCostPerHour] = gcplabelEncodeToSave(strconv.FormatFloat(price, 'f', 8, 64))
	labels[gcpTagCostLastRun] = gcplabelEncodeToSave(strconv.FormatFloat(0, 'f', 8, 64))
	labels[gcpTagCostStartTime] = gcplabelEncodeToSave(strconv.Itoa(int(time.Now().Unix())))

	disksInt := extra.cloudDisks
	// this whole IF block is to handle old formatting and defaults if disks are not specified
	if len(disksInt) == 0 {
		if len(extra.disks) == 0 {
			if strings.HasPrefix(extra.instanceType, "n4-") {
				extra.disks = []string{"hyperdisk-balanced:20"}
			} else {
				extra.disks = []string{"pd-balanced:20"}
			}
		}
		nDisks := extra.disks
		extra.disks = []string{}
		for _, disk := range nDisks {
			diskb := strings.Split(disk, "@")
			disk = diskb[0]
			count := 1
			if len(diskb) > 1 {
				count, err = strconv.Atoi(diskb[1])
				if err != nil {
					return fmt.Errorf("invalid definition of %s: %s", disk, err)
				}
			}
			for i := 1; i <= count; i++ {
				extra.disks = append(extra.disks, disk)
			}
		}
		for _, disk := range extra.disks {
			if disk == "local-ssd" {
				disksInt = append(disksInt, &cloudDisk{
					Type: disk,
				})
				continue
			}
			if strings.HasPrefix(disk, "hyperdisk-balanced:") {
				diska := strings.Split(disk, ":")
				if len(diska) == 2 {
					diska = append(diska, []string{"3060", "155"}...)
				}
				if len(diska) != 4 {
					return errors.New("invalid disk definition; disk must be either local-ssd[@count] or type:sizeGB[:iops:throughputMb][@count] (ex local-ssd@5 pd-balanced:50 pd-ssd:50@5 pd-standard:10 hyperdisk-balanced:20:3060:155)")
				}
				dsize, err := strconv.Atoi(diska[1])
				if err != nil {
					return fmt.Errorf("incorrect format for disk mapping: size must be an integer: %s", err)
				}
				diops, err := strconv.Atoi(diska[2])
				if err != nil {
					return fmt.Errorf("incorrect format for disk mapping: IOPs must be an integer: %s", err)
				}
				dthroughput, err := strconv.Atoi(diska[3])
				if err != nil {
					return fmt.Errorf("incorrect format for disk mapping: Throughput must be an integer: %s", err)
				}
				disksInt = append(disksInt, &cloudDisk{
					Type:                  diska[0],
					Size:                  int64(dsize),
					ProvisionedIOPS:       int64(diops),
					ProvisionedThroughput: int64(dthroughput),
				})
				continue
			}
			if !strings.HasPrefix(disk, "pd-") {
				return errors.New("invalid disk definition; disk must be either local-ssd[@count] or type:sizeGB[:iops:throughputMb][@count] (ex local-ssd@5 pd-balanced:50 pd-ssd:50@5 pd-standard:10 hyperdisk-balanced:20:3060:155)")
			}
			diska := strings.Split(disk, ":")
			if len(diska) != 2 && len(diska) != 4 {
				return errors.New("invalid disk definition; disk must be either local-ssd[@count] or type:sizeGB[:iops:throughputMb][@count] (ex local-ssd@5 pd-balanced:50 pd-ssd:50@5 pd-standard:10 hyperdisk-balanced:20:3060:155)")
			}
			dint, err := strconv.Atoi(diska[1])
			if err != nil {
				return fmt.Errorf("incorrect format for disk mapping: size must be an integer: %s", err)
			}
			var diops int
			var dthroughput int
			if len(diska) == 4 {
				diops, err = strconv.Atoi(diska[2])
				if err != nil {
					return fmt.Errorf("incorrect format for disk mapping: IOPs must be an integer: %s", err)
				}
				dthroughput, err = strconv.Atoi(diska[3])
				if err != nil {
					return fmt.Errorf("incorrect format for disk mapping: Throughput must be an integer: %s", err)
				}
			}
			disksInt = append(disksInt, &cloudDisk{
				Type:                  diska[0],
				Size:                  int64(dint),
				ProvisionedIOPS:       int64(diops),
				ProvisionedThroughput: int64(dthroughput),
			})
		}
	}

	var imageName string
	if d.client {
		if extra.ami != "" {
			imageName = extra.ami
		} else {
			imageName, err = d.getImage(v)
			if err != nil {
				return err
			}
		}
	} else {
		ctx := context.Background()
		imagesClient, err := compute.NewImagesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewImagesRESTClient: %w", err)
		}
		defer imagesClient.Close()
		req := computepb.ListImagesRequest{
			Project: a.opts.Config.Backend.Project,
			Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
		}

		it := imagesClient.List(ctx, &req)
		for {
			image, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			if image.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
				arch := "amd"
				if v.isArm {
					arch = "arm"
				}
				if image.Labels[gcpTagOperatingSystem] == gcpResourceName(v.distroName) && image.Labels[gcpTagOSVersion] == gcpResourceName(v.distroVersion) && image.Labels[gcpTagAerospikeVersion] == gcpResourceName(v.aerospikeVersion) && image.Labels["arch"] == arch {
					imageName = *image.SelfLink
				}
			}
		}
	}

	nodes, err := d.NodeListInCluster(name)
	if err != nil {
		return fmt.Errorf("could not run NodeListInCluster\n%s", err)
	}
	start := 1
	for _, node := range nodes {
		if node >= start {
			start = node + 1
		}
	}

	for _, fnp := range extra.firewallNamePrefix {
		err = d.createSecurityGroupsIfNotExist(fnp, extra.isAgiFirewall, true, []string{}, false)
		if err != nil {
			return fmt.Errorf("firewall: %s", err)
		}
	}
	var keyPath string
	ops := []gcpMakeOps{}
	expWg := new(sync.WaitGroup)
	if !extra.expiresTime.IsZero() {
		defer expWg.Wait()
		deployRegion := strings.Split(extra.zone, "-")
		if len(deployRegion) > 2 {
			deployRegion = deployRegion[:len(deployRegion)-1]
		}
		expWg.Add(1)
		go d.expiriesSystemInstall(10, strings.Join(deployRegion, "-"), expWg)
		labels["aerolab4expires"] = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(extra.expiresTime.Format(time.RFC3339), ":", "_"), "+", "-"))
	}
	expiryTelemetryLock.Lock()
	if expiryTelemetryUUID != "" {
		labels["telemetry"] = string(expiryTelemetryUUID)
	}
	expiryTelemetryLock.Unlock()
	onHostMaintenance := "MIGRATE"
	if extra.onHostMaintenance != "" {
		onHostMaintenance = extra.onHostMaintenance
	}
	autoRestart := true
	provisioning := "STANDARD"
	if extra.spotInstance {
		provisioning = "SPOT"
		autoRestart = false
		onHostMaintenance = "TERMINATE"
		labels["isspot"] = "true"
	}
	var serviceAccounts []*computepb.ServiceAccount
	if extra.terminateOnPoweroff || extra.instanceRole != "" {
		serviceAccounts = []*computepb.ServiceAccount{
			{
				Scopes: []string{
					"https://www.googleapis.com/auth/compute",
				},
			},
		}
		if !strings.HasSuffix(extra.instanceRole, "::nopricing") {
			serviceAccounts[0].Scopes = append(serviceAccounts[0].Scopes, "https://www.googleapis.com/auth/cloud-billing.readonly")
		}
	}
	for i := start; i < (nodeCount + start); i++ {
		labels[gcpTagNodeNumber] = strconv.Itoa(i)
		_, keyPath, err = d.getKey(name)
		if err != nil {
			d.killKey(name)
			_, keyPath, err = d.makeKey(name)
			if err != nil {
				return fmt.Errorf("could not run getKey\n%s", err)
			}
		}
		sshKey, err := os.ReadFile(keyPath + ".pub")
		if err != nil {
			return err
		}
		// disk mapping
		// zones/zone/diskTypes/diskType
		disksList := []*computepb.AttachedDisk{}
		for nI, nDisk := range disksInt {
			var simage *string
			boot := false
			if nI == 0 {
				if !strings.HasPrefix(nDisk.Type, "pd-") && !strings.HasPrefix(nDisk.Type, "hyperdisk-") {
					return errors.New("first (root volume) disk must be of type pd-* or hyperdisk-*")
				}
				simage = proto.String(imageName)
				boot = true
			}
			diskType := fmt.Sprintf("zones/%s/diskTypes/%s", extra.zone, nDisk.Type)
			var diskSize *int64
			var piops *int64
			var pput *int64
			attachmentType := proto.String(computepb.AttachedDisk_SCRATCH.String())
			var devIface *string
			if strings.HasPrefix(nDisk.Type, "pd-") || strings.HasPrefix(nDisk.Type, "hyperdisk-") {
				devIface = nil
				diskSize = proto.Int64(int64(nDisk.Size))
				attachmentType = proto.String(computepb.AttachedDisk_PERSISTENT.String())
				if nDisk.ProvisionedThroughput != 0 {
					put := int64(nDisk.ProvisionedThroughput)
					pput = &put
				}
				if nDisk.ProvisionedIOPS != 0 {
					iops := int64(nDisk.ProvisionedIOPS)
					piops = &iops
				}
			} else {
				devIface = proto.String(computepb.AttachedDisk_NVME.String())
			}
			disksList = append(disksList, &computepb.AttachedDisk{
				InitializeParams: &computepb.AttachedDiskInitializeParams{
					DiskSizeGb:            diskSize,
					SourceImage:           simage,
					DiskType:              proto.String(diskType),
					ProvisionedIops:       piops,
					ProvisionedThroughput: pput,
				},
				AutoDelete: proto.Bool(true),
				Boot:       proto.Bool(boot),
				Type:       attachmentType,
				Interface:  devIface,
			})
		}
		// create instance (remember name and labels and tags and extra tags)
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()

		instanceType := extra.instanceType
		fnp := append(extra.firewallNamePrefix, "aerolab-server")
		tags := append(extra.tags, fnp...)
		metaItems := []*computepb.Items{
			{
				Key:   proto.String("ssh-keys"),
				Value: proto.String("root:" + string(sshKey)),
			},
			{
				Key:   proto.String("startup-script"),
				Value: proto.String(d.startupScriptEnableRootLogin()),
			},
		}
		for mk, mv := range extra.gcpMeta {
			metaItems = append(metaItems, &computepb.Items{
				Key:   proto.String(mk),
				Value: proto.String(mv),
			})
		}

		// Validate the labels before we use them
		err = d.validateLabels(labels)
		if err != nil {
			return err
		}

		req := &computepb.InsertInstanceRequest{
			Project: a.opts.Config.Backend.Project,
			Zone:    extra.zone,
			InstanceResource: &computepb.Instance{
				MinCpuPlatform:  extra.gcpMinCpuPlatform,
				ServiceAccounts: serviceAccounts,
				Metadata: &computepb.Metadata{
					Items: metaItems,
				},
				Labels:      labels,
				Name:        proto.String(fmt.Sprintf("aerolab4-%s-%d", name, i)),
				MachineType: proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", extra.zone, instanceType)),
				Tags: &computepb.Tags{
					Items: tags,
				},
				Scheduling: &computepb.Scheduling{
					AutomaticRestart:  proto.Bool(autoRestart),
					OnHostMaintenance: proto.String(onHostMaintenance),
					ProvisioningModel: proto.String(provisioning),
				},
				Disks: disksList,
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						StackType: proto.String("IPV4_ONLY"),
						AccessConfigs: []*computepb.AccessConfig{
							{
								Name:        proto.String("External NAT"),
								NetworkTier: proto.String("PREMIUM"),
							},
						},
					},
				},
			},
		}

		op, err := instancesClient.Insert(ctx, req)
		if err != nil && (strings.Contains(err.Error(), "OnHostMaintenance must be set to TERMINATE") || strings.Contains(err.Error(), "not support live migration")) {
			req.InstanceResource.Scheduling.OnHostMaintenance = proto.String("TERMINATE")
			onHostMaintenance = "TERMINATE"
			log.Print("OnHostMaintenance mode not supported, attempting to switch to TERMINATE")
			op, err = instancesClient.Insert(ctx, req)
		}
		if err != nil {
			return fmt.Errorf("unable to create instance: %w", err)
		}
		ops = append(ops, gcpMakeOps{
			ctx: ctx,
			op:  op,
		})
	}

	for _, o := range ops {
		if err = o.op.Wait(o.ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}

	nodeIps, err := d.GetNodeIpMap(name, false)
	if err != nil {
		return err
	}
	newNodeCount := len(nodeIps)

	for {
		working := 0
		for _, instIp := range nodeIps {
			_, err = remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "ls", 0)
			if err == nil {
				working++
			} else {
				fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", instIp, keyPath, err)
				time.Sleep(time.Second)
			}
		}
		if working >= newNodeCount {
			break
		}
	}
	log.Print("All connections succeeded, continuing...")
	return nil
}

func (d *backendGcp) startupScriptEnableRootLogin() string {
	return `#!/bin/bash
CHANGED=0
grep PermitRootLogin /etc/ssh/sshd_config |grep no
if [ $? -eq 0 ]
then
sed -i.bak 's/PermitRootLogin.*no//g' /etc/ssh/sshd_config
CHANGED=1
fi
egrep '^PermitRootLogin' /etc/ssh/sshd_config |grep prohibit-password
if [ $? -ne 0 ]
then
cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak2
echo "PermitRootLogin prohibit-password" >> /etc/ssh/sshd_config
CHANGED=1
fi
if [ ${CHANGED} -eq 1 ]
then
systemctl restart ssh || systemctl restart sshd || systemctl restart openssh-server
echo "SSH Reconfigured"
fi
`
}
