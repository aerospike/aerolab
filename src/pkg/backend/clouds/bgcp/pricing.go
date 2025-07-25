package bgcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	serviceusage "cloud.google.com/go/serviceusage/apiv1"
	serviceusagepb "cloud.google.com/go/serviceusage/apiv1/serviceusagepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	_ "embed"
)

func (s *b) GetVolumePrice(region string, volumeType string) (*backends.VolumePrice, error) {
	if strings.Count(region, "-") == 2 {
		parts := strings.Split(region, "-")
		region = parts[0] + "-" + parts[1]
	}
	log := s.log.WithPrefix("GetVolumePrice: job=" + shortuuid.New() + " region=" + region + " volumeType=" + volumeType + " ")
	log.Detail("Start")
	defer log.Detail("End")
	prices, err := s.GetVolumePrices()
	if err != nil {
		return nil, err
	}
	for _, price := range prices {
		if price.Type == volumeType && price.Region == region {
			return price, nil
		}
	}
	return nil, errors.New("volume price not found")
}

func (s *b) GetVolumePrices() (backends.VolumePriceList, error) {
	log := s.log.WithPrefix("GetVolumePrices: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// get prices from cache
	prices, err := s.getVolumePricesFromCache()
	if err != nil {
		log.Detail("Cache miss (%s), getting from GCP", err)
		prices, err = s.getVolumePricesFromGCP()
		if err != nil {
			return nil, err
		}
		// store in cache
		log.Detail("Storing in cache")
		err = s.putVolumePricesToCache(prices)
		if err != nil {
			return nil, err
		}
	}
	return prices, nil
}

func (s *b) GetInstanceType(region string, instanceType string) (*backends.InstanceType, error) {
	log := s.log.WithPrefix("GetInstanceType: job=" + shortuuid.New() + " region=" + region + " instanceType=" + instanceType + " ")
	log.Detail("Start")
	defer log.Detail("End")
	types, err := s.GetInstanceTypes()
	if err != nil {
		return nil, err
	}
	for _, t := range types {
		if t.Region == region && t.Name == instanceType {
			return t, nil
		}
	}
	return nil, errors.New("instance type not found")
}

func (s *b) GetInstanceTypes() (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("GetInstanceTypes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// get prices from cache
	prices, err := s.getInstanceTypesFromCache()
	if err != nil {
		log.Detail("Cache miss (%s), getting from GCP", err)
		prices, err = s.getInstanceTypesFromGCP()
		if err != nil {
			return nil, err
		}
		// store in cache
		log.Detail("Storing in cache")
		err = s.putInstanceTypesToCache(prices)
		if err != nil {
			return nil, err
		}
	}

	// translate to backends.InstanceTypeList
	log.Detail("Responding")
	return prices, nil
}

// cache operations
func (s *b) putInstanceTypesToCache(types backends.InstanceTypeList) error {
	os.MkdirAll(s.workDir, 0755)
	f := path.Join(s.workDir, "instance_types.json")
	fd, err := os.Create(f)
	if err != nil {
		return err
	}
	defer fd.Close()
	itp := &instanceTypes{
		Types: types,
		Ts:    time.Now(),
	}
	return json.NewEncoder(fd).Encode(itp)
}

func (s *b) instanceTypeCacheInvalidate() {
	f := path.Join(s.workDir, "instance_types.json")
	os.Remove(f)
}

func (s *b) getInstanceTypesFromCache() (backends.InstanceTypeList, error) {
	f := path.Join(s.workDir, "instance_types.json")
	fd, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	itp := &instanceTypes{}
	err = json.NewDecoder(fd).Decode(&itp)
	if err != nil {
		return nil, err
	}
	if time.Since(itp.Ts) > 24*time.Hour {
		return nil, errors.New("cache expired")
	}
	return itp.Types, nil
}

func (s *b) putVolumePricesToCache(prices backends.VolumePriceList) error {
	f := path.Join(s.workDir, "volume_prices.json")
	fd, err := os.Create(f)
	if err != nil {
		return err
	}
	defer fd.Close()
	vp := &volumePrices{
		Prices: prices,
		Ts:     time.Now(),
	}
	return json.NewEncoder(fd).Encode(vp)
}

func (s *b) volumePriceCacheInvalidate() {
	f := path.Join(s.workDir, "volume_prices.json")
	os.Remove(f)
}

func (s *b) getVolumePricesFromCache() (backends.VolumePriceList, error) {
	f := path.Join(s.workDir, "volume_prices.json")
	fd, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	vp := &volumePrices{}
	err = json.NewDecoder(fd).Decode(&vp)
	if err != nil {
		return nil, err
	}
	if time.Since(vp.Ts) > 24*time.Hour {
		return nil, errors.New("cache expired")
	}
	return vp.Prices, nil
}

type instanceTypes struct {
	Types backends.InstanceTypeList
	Ts    time.Time
}

type volumePrices struct {
	Prices backends.VolumePriceList
	Ts     time.Time
}

// GCP pricing API
func (s *b) getVolumePricesFromGCP() (backends.VolumePriceList, error) {
	log := s.log.WithPrefix("getVolumePricesFromGCP: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	ctx := context.Background()
	creds, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}

	svc, err := cloudbilling.NewService(ctx,
		option.WithCredentials(creds),
		option.WithQuotaProject(s.credentials.Project),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloudbilling service: %w", err)
	}

	skus := cloudbilling.NewServicesSkusService(svc)
	call := skus.List("services/6F81-5844-456A").CurrencyCode("USD")

	out := backends.VolumePriceList{}
	err = call.Pages(ctx, func(resp *cloudbilling.ListSkusResponse) error {
		for _, sku := range resp.Skus {
			if sku.Category == nil || sku.PricingInfo == nil || len(sku.PricingInfo) == 0 {
				continue
			}
			if sku.Category.ResourceFamily != "Storage" {
				continue
			}
			if sku.Category.UsageType != "OnDemand" {
				continue
			}
			// Match descriptions like "Standard provisioned storage", "SSD backed storage", etc.
			var diskType string
			desc := strings.ToLower(sku.Description)
			switch {
			case strings.Contains(desc, "snapshot"):
				continue
			case strings.Contains(desc, "defined duration"):
				continue
			case strings.Contains(desc, "standard") && strings.Contains(desc, "storage"):
				diskType = "pd-standard"
			case strings.Contains(desc, "ssd") && strings.Contains(desc, "storage"):
				diskType = "pd-ssd"
			case strings.Contains(desc, "balanced") && strings.Contains(desc, "storage"):
				diskType = "pd-balanced"
			case strings.Contains(desc, "extreme") && strings.Contains(desc, "storage"):
				diskType = "pd-extreme"
			default:
				continue
			}
			for _, region := range sku.ServiceRegions {
				for _, pricing := range sku.PricingInfo {
					if pricing.PricingExpression == nil {
						continue
					}
					if pricing.PricingExpression.UsageUnit != "GiBy.mo" && pricing.PricingExpression.UsageUnit != "GiBy.h" {
						continue
					}
					for _, rate := range pricing.PricingExpression.TieredRates {
						if rate.UnitPrice == nil || rate.UnitPrice.CurrencyCode != "USD" {
							continue
						}
						// Normalize per GB per hour
						pricePerMonth := float64(rate.UnitPrice.Units) + float64(rate.UnitPrice.Nanos)/1e9
						var pricePerHour float64
						if pricing.PricingExpression.UsageUnit == "GiBy.mo" {
							pricePerHour = pricePerMonth / (730.0) // avg hours/month
						} else {
							pricePerHour = pricePerMonth
						}
						out = append(out, &backends.VolumePrice{
							Type:           diskType,
							Region:         region,
							Currency:       "USD",
							PricePerGBHour: pricePerHour,
						})
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list SKUs: %w", err)
	}

	return out, nil
}

//go:embed machine-type-ssd-count.json
var ssdCountStr string

var ssdCount = make(map[string]int)

func init() {
	err := json.Unmarshal([]byte(ssdCountStr), &ssdCount)
	if err != nil {
		panic(err)
	}
}

// GCP pricing API
func (s *b) getInstanceTypesFromGCP() (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("getInstanceTypesFromGCP: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewMachineTypesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	var wg sync.WaitGroup
	var out backends.InstanceTypeList
	var outLock sync.Mutex
	var errs error
	enabledZones, _ := s.ListEnabledZones()
	zones := []string{}
	for _, zone := range s.allZones {
		for _, enabledZone := range enabledZones {
			if strings.HasPrefix(zone, enabledZone) {
				zones = append(zones, zone)
				break
			}
		}
	}
	wg.Add(len(zones))
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			iter := client.List(context.Background(), &computepb.ListMachineTypesRequest{
				Project: s.credentials.Project,
				Zone:    zone,
			})
			for {
				machineType, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				arch := backends.ArchitectureX8664
				if strings.HasPrefix(*machineType.Name, "t2") {
					arch = backends.ArchitectureARM64
				}
				ssds := 0
				for k, v := range ssdCount {
					if strings.HasPrefix(*machineType.Name, k) {
						ssds = v
						break
					}
				}
				ssdGiB := ssds * 375
				if strings.HasPrefix(*machineType.Name, "z3") {
					ssdGiB = ssds * 3 * 1024
				}
				accels := 0
				for _, acc := range machineType.Accelerators {
					accels += int(*acc.GuestAcceleratorCount)
				}
				outLock.Lock()
				out = append(out, &backends.InstanceType{
					Name:             *machineType.Name,
					Region:           zone,
					CPUs:             int(*machineType.GuestCpus),
					GPUs:             accels,
					MemoryGiB:        float64(*machineType.MemoryMb) * 1000 * 1000 / 1024 / 1024 / 1024,
					NvmeCount:        ssds,
					NvmeTotalSizeGiB: ssdGiB,
					Arch:             []backends.Architecture{arch},
					PricePerHour:     backends.InstanceTypePrice{},
				})
				outLock.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	if errs != nil {
		return nil, errs
	}
	return s.getInstancePrices(out)
}

func (s *b) getInstancePricesEnableService(out backends.InstanceTypeList) (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("getInstancePricesEnableService: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	ctx := context.Background()

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	client, err := serviceusage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	name := fmt.Sprintf("projects/%s/services/cloudbilling.googleapis.com", s.credentials.Project)
	op, err := client.EnableService(ctx, &serviceusagepb.EnableServiceRequest{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to enable cloud-billing API: %w", err)
	}
	log.Detail("Waiting for cloud-billing API to be enabled")
	_, err = op.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for cloud-billing API to be enabled: %w", err)
	}
	log.Detail("Cloud-billing API enabled")
	return s.getInstancePrices(out)
}

type gcpInstancePricing struct {
	OnDemandPerCPUHour   float64
	OnDemandPerRamGBHour float64
	SpotPerCPUHour       float64
	SpotPerRamGBHour     float64
	OnDemandPerGPUHour   float64
	SpotPerGPUHour       float64
}

// map[instanceTypePrefix]gcpInstancePricing
func (s *b) getInstancePrices(out backends.InstanceTypeList) (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("getInstancePrices: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	enabledZones, err := s.ListEnabledZones()
	if err != nil {
		return nil, err
	}
	zones := []string{}
	for _, zone := range s.allZones {
		for _, enabledZone := range enabledZones {
			if strings.HasPrefix(zone, enabledZone) {
				zones = append(zones, zone)
				break
			}
		}
	}

	ctx := context.Background()
	cli, err := connect.GetCredentials(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	svc, err := cloudbilling.NewService(ctx, option.WithCredentials(cli), option.WithQuotaProject(s.credentials.Project))
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") {
			return s.getInstancePricesEnableService(out)
		} else {
			return out, err
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
			zoneFound := false
			for _, zone := range zones {
				if strings.Count(zone, "-") == 2 {
					parts := strings.Split(zone, "-")
					zone = parts[0] + "-" + parts[1]
				}
				if slices.Contains(i.ServiceRegions, zone) {
					zoneFound = true
					break
				}
			}
			if !zoneFound {
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
								pricePerUnitHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
								for _, t := range out {
									if strings.HasPrefix(t.Name, grp) {
										t.PricePerHour.Currency = "USD"
										if t.BackendSpecific == nil {
											t.BackendSpecific = &gcpInstancePricing{}
										}
										pricing := t.BackendSpecific.(*gcpInstancePricing)
										pricing.OnDemandPerGPUHour = pricePerUnitHour
									}
								}
							} else {
								pricePerUnitHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
								for _, t := range out {
									if strings.HasPrefix(t.Name, grp) {
										t.PricePerHour.Currency = "USD"
										if t.BackendSpecific == nil {
											t.BackendSpecific = &gcpInstancePricing{}
										}
										pricing := t.BackendSpecific.(*gcpInstancePricing)
										pricing.SpotPerGPUHour = pricePerUnitHour
									}
								}
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
								pricePerUnitHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
								for _, t := range out {
									if strings.HasPrefix(t.Name, grp) {
										t.PricePerHour.Currency = "USD"
										if t.BackendSpecific == nil {
											t.BackendSpecific = &gcpInstancePricing{}
										}
										pricing := t.BackendSpecific.(*gcpInstancePricing)
										pricing.OnDemandPerCPUHour = pricePerUnitHour
									}
								}
							} else {
								pricePerUnitHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
								for _, t := range out {
									if strings.HasPrefix(t.Name, grp) {
										t.PricePerHour.Currency = "USD"
										if t.BackendSpecific == nil {
											t.BackendSpecific = &gcpInstancePricing{}
										}
										pricing := t.BackendSpecific.(*gcpInstancePricing)
										pricing.SpotPerCPUHour = pricePerUnitHour
									}
								}
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
								pricePerUnitHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
								for _, t := range out {
									if strings.HasPrefix(t.Name, grp) {
										t.PricePerHour.Currency = "USD"
										if t.BackendSpecific == nil {
											t.BackendSpecific = &gcpInstancePricing{}
										}
										pricing := t.BackendSpecific.(*gcpInstancePricing)
										pricing.OnDemandPerGPUHour = pricePerUnitHour
									}
								}
							} else {
								pricePerUnitHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
								for _, t := range out {
									if strings.HasPrefix(t.Name, grp) {
										t.PricePerHour.Currency = "USD"
										if t.BackendSpecific == nil {
											t.BackendSpecific = &gcpInstancePricing{}
										}
										pricing := t.BackendSpecific.(*gcpInstancePricing)
										pricing.SpotPerGPUHour = pricePerUnitHour
									}
								}
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
						if onDemand {
							pricePerCoreHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							pricePerRamGBHour := 0.0000000000001
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.OnDemandPerCPUHour = pricePerCoreHour
									pricing.OnDemandPerRamGBHour = pricePerRamGBHour
								}
							}
						} else {
							pricePerCoreHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							pricePerRamGBHour := 0.0000000000001
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.SpotPerCPUHour = pricePerCoreHour
									pricing.SpotPerRamGBHour = pricePerRamGBHour
								}
							}
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
						if onDemand {
							pricePerCoreHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							pricePerRamGBHour := 0.0000000000001
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.OnDemandPerCPUHour = pricePerCoreHour
									pricing.OnDemandPerRamGBHour = pricePerRamGBHour
								}
							}
						} else {
							pricePerCoreHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							pricePerRamGBHour := 0.0000000000001
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.SpotPerCPUHour = pricePerCoreHour
									pricing.SpotPerRamGBHour = pricePerRamGBHour
								}
							}
						}
					}
				}
				continue
			} else if (iGroup[1] != "Instance" || (iGroup[2] != "Core" && iGroup[2] != "Ram") || iGroup[3] != "running" || iGroup[4] != "in") && ((iGroup[1] != "Predefined" && iGroup[1] != "AMD" && iGroup[1] != "Arm" && iGroup[1] != "Memory-optimized") || iGroup[2] != "Instance" || (iGroup[3] != "Core" && iGroup[3] != "Ram") || iGroup[4] != "running" || iGroup[5] != "in") {
				continue
			}
			grp := strings.ToLower(iGroup[0])
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
							pricePerCoreHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.OnDemandPerCPUHour = pricePerCoreHour
								}
							}
						} else {
							pricePerCoreHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.SpotPerCPUHour = pricePerCoreHour
								}
							}
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
							pricePerRamGBHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.OnDemandPerRamGBHour = pricePerRamGBHour
								}
							}
						} else {
							pricePerRamGBHour := float64(x.UnitPrice.Units) + float64(x.UnitPrice.Nanos)/1000000000
							for _, t := range out {
								if strings.HasPrefix(t.Name, grp) {
									t.PricePerHour.Currency = "USD"
									if t.BackendSpecific == nil {
										t.BackendSpecific = &gcpInstancePricing{}
									}
									pricing := t.BackendSpecific.(*gcpInstancePricing)
									pricing.SpotPerRamGBHour = pricePerRamGBHour
								}
							}
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") {
			return s.getInstancePricesEnableService(out)
		} else {
			return out, err
		}
	}
	// compute total instance price per hour for each instance type
	for _, t := range out {
		if t.BackendSpecific == nil {
			continue
		}
		pricing := t.BackendSpecific.(*gcpInstancePricing)
		t.PricePerHour.OnDemand = pricing.OnDemandPerCPUHour*float64(t.CPUs) + pricing.OnDemandPerRamGBHour*t.MemoryGiB + pricing.OnDemandPerGPUHour*float64(t.GPUs)
		t.PricePerHour.Spot = pricing.SpotPerCPUHour*float64(t.CPUs) + pricing.SpotPerRamGBHour*t.MemoryGiB + pricing.SpotPerGPUHour*float64(t.GPUs)
	}
	return out, nil
}
