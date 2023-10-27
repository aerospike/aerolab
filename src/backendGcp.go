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
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

func (d *backendGcp) Arch() TypeArch {
	return TypeArchUndef
}

type backendGcp struct {
	server bool
	client bool
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
	perCoreHour  float64
	perRamGBHour float64
	gpuPriceHour float64
	gpuUltraHour float64
}

type gcpClusterExpiryInstances struct {
	labelFingerprint string
	labels           map[string]string
}

func (d *backendGcp) GetAZName(subnetId string) (string, error) {
	return "", nil
}

func (d *backendGcp) CreateMountTarget(volume *inventoryVolume, subnet string, secGroups []string) (inventoryMountTarget, error) {
	return inventoryMountTarget{}, nil
}

func (d *backendGcp) MountTargetAddSecurityGroup(mountTarget *inventoryMountTarget, volume *inventoryVolume, addGroups []string) error {
	return nil
}

func (d *backendGcp) DeleteVolume(name string) error {
	return nil
}

func (d *backendGcp) CreateVolume(name string, zone string, tags []string) error {
	return nil
}

func (d *backendGcp) SetLabel(clusterName string, key string, value string, gcpZone string) error {
	instances := make(map[string]gcpClusterExpiryInstances)
	if d.server {
		j, err := d.Inventory("", []int{InventoryItemClusters})
		d.WorkOnServers()
		if err != nil {
			return err
		}
		for _, jj := range j.Clusters {
			if jj.ClusterName == clusterName {
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.gcpMetadataFingerprint, jj.gcpMeta}
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
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.gcpMetadataFingerprint, jj.gcpMeta}
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
			Zone:     gcpZone,
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
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.gcpLabelFingerprint, jj.gcpLabels}
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
				instances[jj.InstanceId] = gcpClusterExpiryInstances{jj.gcpLabelFingerprint, jj.gcpLabels}
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
			Zone:     zone,
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

func (d *backendGcp) expiriesSystemInstall(intervalMinutes int, deployRegion string, wg *sync.WaitGroup) {
	defer wg.Done()
	err := d.ExpiriesSystemInstall(intervalMinutes, deployRegion)
	if err != nil && err.Error() != "EXISTS" {
		log.Printf("WARNING: Failed to install the expiry system, clusters will not expire: %s", err)
	}
}

func (d *backendGcp) ExpiriesSystemInstall(intervalMinutes int, deployRegion string) error {
	rd, err := a.aerolabRootDir()
	if err != nil {
		return fmt.Errorf("error getting aerolab home dir: %s", err)
	}
	log.Println("Expiries: checking if job exists already")
	if lastRegion, err := os.ReadFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project)); err == nil {
		if _, err = exec.Command("gcloud", "scheduler", "jobs", "describe", "aerolab-expiries", "--location", string(lastRegion), "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err == nil {
			log.Println("Expiries: done")
			return errors.New("EXISTS")
		}
	}

	log.Println("Expiries: cleaning up old jobs")
	d.expiriesSystemRemove(false) // cleanup

	err = os.WriteFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project), []byte(deployRegion), 0600)
	if err != nil {
		return fmt.Errorf("could not note the region where scheduler got deployed: %s", err)
	}

	log.Println("Expiries: preparing commands")
	cron := "*/" + strconv.Itoa(intervalMinutes) + " * * * *"
	if intervalMinutes >= 60 {
		if intervalMinutes%60 != 0 || intervalMinutes > 1440 {
			return errors.New("frequency can be 0-60 in 1-minute increments, or 60-1440 at 60-minute increments")
		}
		if intervalMinutes == 1440 {
			cron = "0 1 * * *"
		} else {
			if intervalMinutes == 60 {
				cron = "0 * * * *"
			} else {
				cron = "0 */" + strconv.Itoa(intervalMinutes/60) + " * * *"
			}
		}
	}

	tmpDirPath, err := os.MkdirTemp("", "aerolabexpiries")
	if err != nil {
		return fmt.Errorf("mkdir-temp: %s", err)
	}
	defer os.RemoveAll(tmpDirPath)
	err = os.WriteFile(path.Join(tmpDirPath, "go.mod"), expiriesCodeGcpMod, 0644)
	if err != nil {
		return fmt.Errorf("write go.mod: %s", err)
	}
	err = os.WriteFile(path.Join(tmpDirPath, "function.go"), expiriesCodeGcpFunction, 0644)
	if err != nil {
		return fmt.Errorf("write function.go: %s", err)
	}
	token := uuid.New().String()

	log.Println("Expiries: running gcloud functions deploy ...")
	deploy := []string{"functions", "deploy", "aerolab-expiries"}
	deploy = append(deploy, "--region="+deployRegion)
	deploy = append(deploy, "--allow-unauthenticated")
	deploy = append(deploy, "--entry-point=AerolabExpire")
	deploy = append(deploy, "--gen2")
	deploy = append(deploy, "--runtime=go120")
	deploy = append(deploy, "--serve-all-traffic-latest-revision")
	deploy = append(deploy, "--source="+tmpDirPath)
	deploy = append(deploy, "--memory=256M")
	deploy = append(deploy, "--timeout=60s")
	deploy = append(deploy, "--trigger-location=deployRegion")
	deploy = append(deploy, "--set-env-vars=TOKEN="+token)
	deploy = append(deploy, "--max-instances=2")
	deploy = append(deploy, "--min-instances=0")
	deploy = append(deploy, "--trigger-http")
	deploy = append(deploy, "--project="+a.opts.Config.Backend.Project)
	//--stage-bucket=STAGE_BUCKET
	out, err := exec.Command("gcloud", deploy...).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		return fmt.Errorf("run gcloud functions deploy: %s", err)
	}

	log.Println("Expiries: running gcloud scheduler create ...")
	sched := []string{"scheduler", "jobs", "create", "http", "aerolab-expiries"}
	sched = append(sched, "--location="+deployRegion)
	sched = append(sched, "--schedule="+cron)
	uri := "https://" + deployRegion + "-" + a.opts.Config.Backend.Project + ".cloudfunctions.net/aerolab-expiries"
	sched = append(sched, "--uri="+uri)
	sched = append(sched, "--max-backoff=15s")
	sched = append(sched, "--min-backoff=5s")
	sched = append(sched, "--max-doublings=2")
	sched = append(sched, "--max-retry-attempts=0")
	sched = append(sched, "--time-zone=Etc/UTC")
	mBody := "{\"token\":\"" + token + "\"}"
	sched = append(sched, "--message-body="+mBody)
	sched = append(sched, "--project="+a.opts.Config.Backend.Project)
	out, err = exec.Command("gcloud", sched...).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		return fmt.Errorf("run gcloud scheduler jobs create http: %s", err)
	}

	log.Println("Expiries: done")
	return nil
}

func (d *backendGcp) ExpiriesSystemRemove() error {
	return d.expiriesSystemRemove(true)
}

func (d *backendGcp) expiriesSystemRemove(printErr bool) error {
	rd, err := a.aerolabRootDir()
	if err != nil {
		return fmt.Errorf("error getting aerolab home dir: %s", err)
	}
	lastRegion, err := os.ReadFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project))
	if err != nil {
		return fmt.Errorf("could not read job region from %s: %s", path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project), err)
	}
	var nerr error
	if out, err := exec.Command("gcloud", "scheduler", "jobs", "delete", "aerolab-expiries", "--location", string(lastRegion), "--quiet", "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err != nil {
		if printErr {
			fmt.Println(string(out))
		}
		nerr = errors.New("some deletions failed")
	}
	if out, err := exec.Command("gcloud", "functions", "delete", "aerolab-expiries", "--region", string(lastRegion), "--gen2", "--quiet", "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err != nil {
		if printErr {
			fmt.Println(string(out))
		}
		nerr = errors.New("some deletions failed")
	}
	return nerr
}

func (d *backendGcp) ExpiriesSystemFrequency(intervalMinutes int) error {
	cron := "*/" + strconv.Itoa(intervalMinutes) + " * * * *"
	if intervalMinutes >= 60 {
		if intervalMinutes%60 != 0 || intervalMinutes > 1440 {
			return errors.New("frequency can be 0-60 in 1-minute increments, or 60-1440 at 60-minute increments")
		}
		if intervalMinutes == 1440 {
			cron = "0 1 * * *"
		} else {
			if intervalMinutes == 60 {
				cron = "0 * * * *"
			} else {
				cron = "0 */" + strconv.Itoa(intervalMinutes/60) + " * * *"
			}
		}
	}
	rd, err := a.aerolabRootDir()
	if err != nil {
		return fmt.Errorf("error getting aerolab home dir: %s", err)
	}
	lastRegion, err := os.ReadFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project))
	if err != nil {
		return fmt.Errorf("could not read job region from %s: %s", path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project), err)
	}
	if out, err := exec.Command("gcloud", "scheduler", "jobs", "update", "http", "aerolab-expiries", "--location", string(lastRegion), "--schedule", cron, "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}
	return nil
}

func (d *backendGcp) EnableServices() error {
	gcloudServices := []string{"logging.googleapis.com", "cloudfunctions.googleapis.com", "cloudbuild.googleapis.com", "pubsub.googleapis.com", "cloudscheduler.googleapis.com", "compute.googleapis.com", "run.googleapis.com", "artifactregistry.googleapis.com"}
	for _, gs := range gcloudServices {
		log.Printf("===== Running: gcloud services enable --project %s %s =====", a.opts.Config.Backend.Project, gs)
		out, err := exec.Command("gcloud", "services", "enable", "--project", a.opts.Config.Backend.Project, gs).CombinedOutput()
		if err != nil {
			log.Printf("ERROR: %s", err)
			log.Println(string(out))
		}
	}
	log.Println("Done")
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
			if i.Category.UsageType != "OnDemand" {
				continue
			}
			if !inslice.HasString(i.ServiceRegions, zone) {
				continue
			}
			iGroup := strings.Split(i.Description, " ")
			if len(iGroup) < 6 {
				continue
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
							prices[grp].gpuPriceHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
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
							prices[grp].gpuUltraHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
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
							prices[grp].gpuPriceHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
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
						prices["g1"].perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
						prices["g1"].perRamGBHour = 0.0000000000001
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
						prices["f1"].perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
						prices["f1"].perRamGBHour = 0.0000000000001
					}
				}
				continue
			} else if (iGroup[1] != "Instance" || (iGroup[2] != "Core" && iGroup[2] != "Ram") || iGroup[3] != "running" || iGroup[4] != "in") && ((iGroup[1] != "Predefined" && iGroup[1] != "AMD" && iGroup[1] != "Memory-optimized") || iGroup[2] != "Instance" || (iGroup[3] != "Core" && iGroup[3] != "Ram") || iGroup[4] != "running" || iGroup[5] != "in") {
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
						prices[grp].perCoreHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
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
						prices[grp].perRamGBHour = float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
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
	prices, err := d.getInstancePricesPerHour(zone)
	if err != nil {
		log.Printf("WARN: pricing error: %s", err)
		saveCache = false
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
			if strings.HasPrefix(*t.Name, "t2") {
				isArm = true
			} else {
				isX86 = true
			}
			eph := 0
			ephs := float64(0)
			if !*t.IsSharedCpu && !strings.HasPrefix(*t.Name, "e2-") && !strings.HasPrefix(*t.Name, "t2d-") && !strings.HasPrefix(*t.Name, "t2a-") && !strings.HasPrefix(*t.Name, "m2-") && !strings.HasPrefix(*t.Name, "c3-") {
				if strings.HasPrefix(*t.Name, "c2-") || strings.HasPrefix(*t.Name, "c2d-") || strings.HasPrefix(*t.Name, "a2-") || strings.HasPrefix(*t.Name, "m1-") || strings.HasPrefix(*t.Name, "m3-") || strings.HasPrefix(*t.Name, "g2-") {
					eph = 8
					ephs = 3 * 1024
				} else if strings.HasPrefix(*t.Name, "n1-") || strings.HasPrefix(*t.Name, "n2-") || strings.HasPrefix(*t.Name, "n2d-") {
					eph = 24
					ephs = 9 * 1024
				} else {
					eph = -1
					ephs = -1
				}
			}
			prices := prices[strings.Split(*t.Name, "-")[0]]
			price := float64(-1)
			accels := 0
			for _, acc := range t.Accelerators {
				accels += int(*acc.GuestAcceleratorCount)
			}
			if prices != nil && prices.perCoreHour > 0 && prices.perRamGBHour > 0 && prices.gpuPriceHour >= 0 {
				price = prices.perCoreHour*float64(*t.GuestCpus) + prices.perRamGBHour*float64(*t.MemoryMb/1024)
				if strings.Contains(*t.Name, "-ultragpu") {
					price = price + prices.gpuUltraHour*float64(accels)
				} else {
					price = price + prices.gpuPriceHour*float64(accels)
				}
			}
			it = append(it, instanceType{
				InstanceName:             *t.Name,
				CPUs:                     int(t.GetGuestCpus()),
				RamGB:                    float64(t.GetMemoryMb()) / 1024,
				EphemeralDisks:           eph,
				EphemeralDiskTotalSizeGB: ephs,
				PriceUSD:                 price,
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
						if i == 1 {
							ij.Clusters = append(ij.Clusters, inventoryCluster{
								ClusterName:            instance.Labels[gcpTagClusterName],
								NodeNo:                 instance.Labels[gcpTagNodeNumber],
								InstanceId:             *instance.Name,
								ImageId:                instance.GetSourceMachineImage(),
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
								gcpLabelFingerprint:    *instance.LabelFingerprint,
								Expires:                expires,
								AGILabel:               meta["agiLabel"],
								gcpLabels:              instance.Labels,
								gcpMeta:                meta,
								gcpMetadataFingerprint: *instance.Metadata.Fingerprint,
								Features:               FeatureSystem(features),
							})
						} else {
							ij.Clients = append(ij.Clients, inventoryClient{
								ClientName:             instance.Labels[gcpTagClusterName],
								NodeNo:                 instance.Labels[gcpTagNodeNumber],
								InstanceId:             *instance.Name,
								ImageId:                instance.GetSourceMachineImage(),
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
								gcpLabelFingerprint:    *instance.LabelFingerprint,
								Expires:                expires,
								gcpLabels:              instance.Labels,
								gcpMeta:                meta,
								gcpMetadataFingerprint: *instance.Metadata.Fingerprint,
							})
						}
					}
				}
			}
		}
	}
	return ij, nil
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
					clist = append(clist, instance.Labels[gcpTagClusterName])
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

func (d *backendGcp) ClusterListFull(isJson bool, owner string, noPager bool) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.Owner = owner
	a.opts.Inventory.List.NoPager = noPager
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
			if strings.Contains(*image.Architecture, "arm") || strings.Contains(*image.Architecture, "aarch") {
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
			if strings.Contains(*image.Architecture, "arm") || strings.Contains(*image.Architecture, "aarch") {
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

func (d *backendGcp) AttachAndRun(clusterName string, node int, command []string) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr)
}

func (d *backendGcp) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (err error) {
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
	isInteractive := true
	if len(command) > 0 {
		isInteractive = false
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

func (d *backendGcp) TemplateListFull(isJson bool, noPager bool) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.NoPager = noPager
	return "", a.opts.Inventory.List.run(false, false, true, false, false)
}

func (d *backendGcp) GetKeyPath(clusterName string) (keyPath string, err error) {
	_, p, e := d.getKey(clusterName)
	return p, e
}

// get KeyPair
func (d *backendGcp) getKey(clusterName string) (keyName string, keyPath string, err error) {
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
		for _, char := range val {
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return nil, fmt.Errorf("invalid tag value for `%s`, only the following are allowed: [a-zA-Z0-9_-]", val)
			}
		}
		for _, char := range key {
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return nil, fmt.Errorf("invalid tag name for `%s`, only the following are allowed: [a-zA-Z0-9_-]", key)
			}
		}
		labels[key] = val
	}
	labels[gcpTagOperatingSystem] = v.distroName
	labels[gcpTagOSVersion] = strings.ReplaceAll(v.distroVersion, ".", "-")
	labels[gcpTagAerospikeVersion] = strings.ReplaceAll(v.aerospikeVersion, ".", "-")
	labels[gcpTagUsedBy] = gcpTagUsedByValue
	labels["arch"] = isArm
	return labels, nil
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
	name := fmt.Sprintf("aerolab4-template-%s-%s-%s-%s", v.distroName, gcpResourceName(v.distroVersion), gcpResourceName(v.aerospikeVersion), isArm)

	for _, fnp := range extra.firewallNamePrefix {
		err = d.createSecurityGroupsIfNotExist(fnp)
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

func (d *backendGcp) AssignSecurityGroups(clusterName string, names []string, zone string, remove bool) error {
	ctx := context.Background()
	for _, name := range names {
		if err := d.createSecurityGroupsIfNotExist(name); err != nil {
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
		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) CreateSecurityGroups(vpc string, namePrefix string) error {
	return d.createSecurityGroupsIfNotExist(namePrefix)
}

func (d *backendGcp) createSecurityGroupExternal(namePrefix string) error {
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
				Ports:      []string{"22", "3000", "443", "80", "8080", "8888", "9200"},
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

func (d *backendGcp) createSecurityGroupInternal(namePrefix string) error {
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

func (d *backendGcp) LockSecurityGroups(ip string, lockSSH bool, vpc string, namePrefix string) error {
	if ip == "discover-caller-ip" {
		ip = getip2()
	}
	if !strings.Contains(ip, "/") {
		ip = ip + "/32"
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
				Ports:      []string{"22", "3000", "443", "80", "8080", "8888", "9200"},
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

func (d *backendGcp) createSecurityGroupsIfNotExist(namePrefix string) error {
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
	needsLock := true
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
			for _, iprange := range firewallRule.SourceRanges {
				_, cidr, _ := net.ParseCIDR(iprange)
				if cidr.Contains(parsedIp) {
					needsLock = false
					break
				}
			}
		}
	}
	if !existInternal {
		err = d.createSecurityGroupInternal(namePrefix)
		if err != nil {
			return err
		}
	}
	if !existExternal {
		err = d.createSecurityGroupExternal(namePrefix)
		if err != nil {
			return err
		}
		err = d.LockSecurityGroups("discover-caller-ip", true, "", namePrefix)
		if err != nil {
			return err
		}
	} else if needsLock {
		log.Println("Security group CIDR doesn't allow this command to complete, re-locking security groups with the caller's IP")
		err = d.LockSecurityGroups("discover-caller-ip", true, "", namePrefix)
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

type gcpDisk struct {
	diskType string
	diskSize int
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

	disksInt := []gcpDisk{}
	if len(extra.disks) == 0 {
		extra.disks = []string{"pd-balanced:20"}
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
			disksInt = append(disksInt, gcpDisk{
				diskType: disk,
			})
			continue
		}
		if !strings.HasPrefix(disk, "pd-") {
			return errors.New("invalid disk definition, disk must be local-ssd[@count], or pd-*:sizeGB[@count] (eg pd-standard:20 or pd-balanced:20 or pd-ssd:40 or pd-ssd:40@5 or local-ssd@5)")
		}
		diska := strings.Split(disk, ":")
		if len(diska) != 2 {
			return errors.New("invalid disk definition, disk must be local-ssd[@count], or pd-*:sizeGB[@count] (eg pd-standard:20 or pd-balanced:20 or pd-ssd:40 or pd-ssd:40@5 or local-ssd@5)")
		}
		dint, err := strconv.Atoi(diska[1])
		if err != nil {
			return fmt.Errorf("incorrect format for disk mapping: size must be an integer: %s", err)
		}
		disksInt = append(disksInt, gcpDisk{
			diskType: diska[0],
			diskSize: dint,
		})
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
		err = d.createSecurityGroupsIfNotExist(fnp)
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
				if !strings.HasPrefix(nDisk.diskType, "pd-") {
					return errors.New("first (root volume) disk must be of type pd-*")
				}
				simage = proto.String(imageName)
				boot = true
			}
			diskType := fmt.Sprintf("zones/%s/diskTypes/%s", extra.zone, nDisk.diskType)
			var diskSize *int64
			attachmentType := proto.String(computepb.AttachedDisk_SCRATCH.String())
			var devIface *string
			if strings.HasPrefix(nDisk.diskType, "pd-") {
				devIface = nil
				diskSize = proto.Int64(int64(nDisk.diskSize))
				attachmentType = proto.String(computepb.AttachedDisk_PERSISTENT.String())
			} else {
				devIface = proto.String(computepb.AttachedDisk_NVME.String())
			}
			disksList = append(disksList, &computepb.AttachedDisk{
				InitializeParams: &computepb.AttachedDiskInitializeParams{
					DiskSizeGb:  diskSize,
					SourceImage: simage,
					DiskType:    proto.String(diskType),
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
		req := &computepb.InsertInstanceRequest{
			Project: a.opts.Config.Backend.Project,
			Zone:    extra.zone,
			InstanceResource: &computepb.Instance{
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
					AutomaticRestart:  proto.Bool(true),
					OnHostMaintenance: proto.String("MIGRATE"),
					ProvisioningModel: proto.String("STANDARD"),
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
