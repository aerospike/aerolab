package bgcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
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
		prices, err = s.refreshVolumePrices(log)
		if err != nil {
			return nil, err
		}
	}
	return prices, nil
}

// refreshVolumePrices repopulates the volume price cache from GCP. Only one
// caller fetches; the rest wait and then read the cache that the winner wrote,
// so a burst of cache misses costs a single walk of the billing catalog.
func (s *b) refreshVolumePrices(log *logger.Logger) (backends.VolumePriceList, error) {
	s.volumePricesFetch.Lock()
	defer s.volumePricesFetch.Unlock()
	if prices, err := s.getVolumePricesFromCache(); err == nil {
		log.Detail("Volume prices were fetched by a concurrent caller, using cache")
		return prices, nil
	}
	prices, err := s.getVolumePricesFromGCP()
	if err != nil {
		return nil, err
	}
	log.Detail("Storing in cache")
	if err := s.putVolumePricesToCache(prices); err != nil {
		return nil, err
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
		prices, err = s.refreshInstanceTypes(log)
		if err != nil {
			return nil, err
		}
	}

	// translate to backends.InstanceTypeList
	log.Detail("Responding")
	return prices, nil
}

// refreshInstanceTypes repopulates the instance type cache from GCP. Only one
// caller fetches; the rest wait and then read the cache that the winner wrote,
// so a burst of cache misses costs a single walk of the billing catalog.
func (s *b) refreshInstanceTypes(log *logger.Logger) (backends.InstanceTypeList, error) {
	s.instanceTypesFetch.Lock()
	defer s.instanceTypesFetch.Unlock()
	if types, err := s.getInstanceTypesFromCache(); err == nil {
		log.Detail("Instance types were fetched by a concurrent caller, using cache")
		return types, nil
	}
	types, err := s.getInstanceTypesFromGCP()
	if err != nil {
		return nil, err
	}
	// store in cache only if we got results (avoid caching empty results)
	if len(types) == 0 {
		log.Detail("Not caching empty instance types list")
		return types, nil
	}
	log.Detail("Storing in cache")
	if err := s.putInstanceTypesToCache(types); err != nil {
		return nil, err
	}
	return types, nil
}

// cache operations
func (s *b) putInstanceTypesToCache(types backends.InstanceTypeList) error {
	if err := os.MkdirAll(s.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
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
	if err := os.MkdirAll(s.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
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

	if s.credentials != nil && s.credentials.SkipPricing {
		log.Detail("Pricing disabled (skip-pricing); returning empty volume price list")
		return backends.VolumePriceList{}, nil
	}

	ctx := context.Background()
	// cloudbilling is an old-style google.golang.org/api client that does not
	// authenticate correctly with option.WithCredentials under Workload Identity
	// Federation; use the httptransport-backed client (as the compute path does).
	// GetBillingClient (not GetClient) forces the X-Goog-User-Project header: the
	// SKU catalog is a global API and federated tokens 401 without a billing project.
	cli, err := connect.GetBillingClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	svc, err := cloudbilling.NewService(ctx, option.WithHTTPClient(cli))
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
			var diskType string
			desc := strings.ToLower(sku.Description)
			switch {
			case strings.Contains(desc, "snapshot"):
				continue
			case strings.Contains(desc, "defined duration"):
				continue
			// Hyperdisk capacity SKUs: "Hyperdisk Balanced Capacity in {Region}", etc.
			case strings.Contains(desc, "hyperdisk") && strings.Contains(desc, "balanced") && strings.Contains(desc, "capacity"):
				diskType = "hyperdisk-balanced"
			case strings.Contains(desc, "hyperdisk") && strings.Contains(desc, "throughput") && strings.Contains(desc, "capacity"):
				diskType = "hyperdisk-throughput"
			case strings.Contains(desc, "hyperdisk") && strings.Contains(desc, "extreme") && strings.Contains(desc, "capacity"):
				diskType = "hyperdisk-extreme"
			case strings.Contains(desc, "hyperdisk") && strings.Contains(desc, "ml") && strings.Contains(desc, "capacity"):
				diskType = "hyperdisk-ml"
			// Skip non-capacity hyperdisk SKUs (IOPS, throughput provisioning, storage pools, replication)
			// to prevent false matches against pd-* patterns below
			case strings.Contains(desc, "hyperdisk"):
				continue
			// Persistent Disk types: "Balanced Persistent Disk Storage in {Region}", etc.
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
				// Families such as x4 and a4 are listed even though GCP
				// publishes no pay-as-you-go SKU for them; they end up priced
				// at zero rather than being hidden from the catalog.
				name := *machineType.Name
				arch := backends.ArchitectureX8664
				if armFamilies[machineFamilyToken(name)] {
					arch = backends.ArchitectureARM64
				}
				ssds := 0
				for k, v := range ssdCount {
					if strings.HasPrefix(name, k) {
						ssds = v
						break
					}
				}
				ssdGiB := ssds * 375
				if strings.HasPrefix(name, "z3") {
					ssdGiB = ssds * 3 * 1024
				}
				accels := 0
				for _, acc := range machineType.Accelerators {
					accels += int(*acc.GuestAcceleratorCount)
				}
				outLock.Lock()
				out = append(out, &backends.InstanceType{
					Name:             name,
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
	// Pricing is best-effort: never let a pricing failure (e.g. cloudbilling API
	// disabled, quota, or insufficient permissions) block operations that only
	// need the instance-type catalog, such as cluster create. On failure, return
	// the instance types without prices.
	if s.credentials != nil && s.credentials.SkipPricing {
		log.Detail("Pricing disabled (skip-pricing); returning instance types without prices")
		return out, nil
	}
	priced, err := s.getInstancePrices(out)
	if err != nil {
		log.Detail("Failed to retrieve instance pricing (continuing without prices): %s", err)
		return out, nil
	}
	return priced, nil
}

func (s *b) getInstancePricesEnableService(out backends.InstanceTypeList) (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("getInstancePricesEnableService: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// enableService checks which services are already enabled and only enables
	// the missing ones before proceeding.
	if err := s.enableService("cloudbilling.googleapis.com"); err != nil {
		return nil, fmt.Errorf("failed to enable cloud-billing API: %w", err)
	}
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

// getGcpInstancePricing safely extracts *gcpInstancePricing from BackendSpecific, initializing it if needed.
// This handles cases where BackendSpecific might be nil, a map (from JSON/YAML deserialization),
// or already the correct type.
func getGcpInstancePricing(t *backends.InstanceType) *gcpInstancePricing {
	if t.BackendSpecific == nil {
		t.BackendSpecific = &gcpInstancePricing{}
		return t.BackendSpecific.(*gcpInstancePricing)
	}
	if p, ok := t.BackendSpecific.(*gcpInstancePricing); ok {
		return p
	}
	// If it's a map (from JSON/YAML deserialization), try to convert it
	if m, ok := t.BackendSpecific.(map[string]any); ok {
		jsonBytes, err := json.Marshal(m)
		if err == nil {
			var p gcpInstancePricing
			if err := json.Unmarshal(jsonBytes, &p); err == nil {
				t.BackendSpecific = &p
				return &p
			}
		}
	}
	// If conversion failed or it's something else, create a new gcpInstancePricing
	t.BackendSpecific = &gcpInstancePricing{}
	return t.BackendSpecific.(*gcpInstancePricing)
}

// getInstancePrices fills in per-hour prices on an instance-type catalog by
// walking the Cloud Billing SKU list for Compute Engine. Rates are collected
// per region and per billing family first, then applied, so that a machine
// type is only ever priced with the rates published for its own region and
// its own family.
func (s *b) getInstancePrices(out backends.InstanceTypeList) (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("getInstancePrices: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	enabledZones, err := s.ListEnabledZones()
	if err != nil {
		return nil, err
	}
	regions := map[string]bool{}
	for _, zone := range s.allZones {
		for _, enabledZone := range enabledZones {
			if strings.HasPrefix(zone, enabledZone) {
				regions[zoneToRegion(zone)] = true
				break
			}
		}
	}

	ctx := context.Background()
	// cloudbilling is an old-style google.golang.org/api client; unlike the
	// modern cloud.google.com/go clients it does not authenticate correctly with
	// option.WithCredentials under Workload Identity Federation (the legacy
	// transport yields a token GCP rejects with 401). Use the same
	// httptransport-backed client as the compute path, which carries a
	// WIF-capable token. GetBillingClient (not GetClient) forces the
	// X-Goog-User-Project header: the SKU catalog is a global API and federated
	// tokens 401 without a billing project.
	cli, err := connect.GetBillingClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	svc, err := cloudbilling.NewService(ctx, option.WithHTTPClient(cli))
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") {
			return s.getInstancePricesEnableService(out)
		}
		return out, err
	}

	base := rateTable{}
	premium := rateTable{}
	srv := cloudbilling.NewServicesSkusService(svc)
	call := srv.List("services/6F81-5844-456A").CurrencyCode("USD")
	err = call.Pages(ctx, func(resp *cloudbilling.ListSkusResponse) error {
		for _, sku := range resp.Skus {
			if sku.Category == nil || sku.Category.ResourceFamily != "Compute" {
				continue
			}
			if sku.Category.UsageType != "OnDemand" && sku.Category.UsageType != "Preemptible" {
				continue
			}
			skuRegions := []string{}
			for _, region := range sku.ServiceRegions {
				if regions[region] {
					skuRegions = append(skuRegions, region)
				}
			}
			if len(skuRegions) == 0 {
				continue
			}
			parsed, ok := parseComputeSku(sku.Description, sku.Category.ResourceGroup, sku.Category.UsageType == "Preemptible")
			if !ok {
				continue
			}
			value, ok := skuHourlyRate(sku, parsed.resource)
			if !ok {
				continue
			}
			table := base
			if parsed.additive {
				table = premium
			}
			for _, region := range skuRegions {
				for _, family := range parsed.families {
					table.rates(region, family).set(parsed.resource, parsed.spot, value)
				}
			}
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") {
			return s.getInstancePricesEnableService(out)
		}
		return out, err
	}
	for region, byFamily := range premium {
		for family, rates := range byFamily {
			base.rates(region, family).add(rates)
		}
	}

	for _, t := range out {
		region := zoneToRegion(t.Region)
		family := billingFamilyFor(t.Name)
		pricing := getGcpInstancePricing(t)
		if rates := base.lookup(region, family.cpuRAM); rates != nil {
			pricing.OnDemandPerCPUHour = rates.onDemand[billingResourceCPU]
			pricing.OnDemandPerRamGBHour = rates.onDemand[billingResourceRAM]
			pricing.SpotPerCPUHour = rates.spot[billingResourceCPU]
			pricing.SpotPerRamGBHour = rates.spot[billingResourceRAM]
		}
		if rates := base.lookup(region, family.gpu); rates != nil {
			pricing.OnDemandPerGPUHour = rates.onDemand[billingResourceGPU]
			pricing.SpotPerGPUHour = rates.spot[billingResourceGPU]
		}
		onDemand := pricing.OnDemandPerCPUHour*float64(t.CPUs) + pricing.OnDemandPerRamGBHour*t.MemoryGiB + pricing.OnDemandPerGPUHour*float64(t.GPUs)
		spot := pricing.SpotPerCPUHour*float64(t.CPUs) + pricing.SpotPerRamGBHour*t.MemoryGiB + pricing.SpotPerGPUHour*float64(t.GPUs)
		if rates := base.lookup(region, family.tpu); rates != nil {
			chips := float64(tpuChipCount(t.Name))
			onDemand += rates.onDemand[billingResourceTPU] * chips
			spot += rates.spot[billingResourceTPU] * chips
		}
		if onDemand == 0 && spot == 0 {
			log.Detail("No price published for instance type %s in %s", t.Name, t.Region)
			continue
		}
		t.PricePerHour.Currency = "USD"
		t.PricePerHour.OnDemand = onDemand
		t.PricePerHour.Spot = spot
	}
	return out, nil
}

// skuHourlyRate pulls the hourly USD rate out of a SKU for a given resource,
// skipping any zero-priced free tier.
func skuHourlyRate(sku *cloudbilling.Sku, resource billingResource) (float64, bool) {
	for _, info := range sku.PricingInfo {
		expr := info.PricingExpression
		if expr == nil {
			continue
		}
		if resource == billingResourceRAM {
			// GCP is inconsistent about GB vs GiB across families.
			if expr.UsageUnit != "GiBy.h" && expr.UsageUnit != "GBy.h" {
				continue
			}
		} else if expr.UsageUnit != "h" {
			continue
		}
		for _, tier := range expr.TieredRates {
			if tier.UnitPrice == nil || tier.UnitPrice.CurrencyCode != "USD" {
				continue
			}
			value := float64(tier.UnitPrice.Units) + float64(tier.UnitPrice.Nanos)/1000000000
			if value > 0 {
				return value, true
			}
		}
	}
	return 0, false
}
