package baws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	etypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
)

type volumePrice struct {
	Product struct {
		Attributes struct {
			RegionCode    string `json:"regionCode"` // only if exact match
			VolumeApiName string `json:"volumeApiName"`
			StorageClass  string `json:"storageClass"`
		} `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				Unit         string            `json:"unit"`
				PricePerUnit map[string]string `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

type volumePriceList struct {
	Prices []*volumePrice
	Ts     time.Time
}

func (s *b) GetVolumePrice(region string, volumeType string) (*backends.VolumePrice, error) {
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
		log.Detail("Cache miss (%s), getting from AWS", err)
		prices, err = s.refreshVolumePrices(log)
		if err != nil {
			return nil, err
		}
	}

	// translate to backends.VolumePriceList
	backendPrices := backends.VolumePriceList{}
NEXTPRICE:
	for _, price := range prices {
		punit := ""
		pprice := float64(0)
		for _, onDemand := range price.Terms.OnDemand {
			for _, priceDimension := range onDemand.PriceDimensions {
				for unit, price := range priceDimension.PricePerUnit {
					punit = unit
					pprice, err = strconv.ParseFloat(price, 64)
					if err != nil {
						log.Detail("Error parsing price: %s", err)
						continue NEXTPRICE
					}
					break
				}
				break
			}
			break
		}
		backendPrices = append(backendPrices, &backends.VolumePrice{
			Type:           price.Product.Attributes.VolumeApiName,
			Region:         price.Product.Attributes.RegionCode,
			PricePerGBHour: pprice / 30.5 / 24,
			Currency:       punit,
		})
	}
	return backendPrices, nil
}

// refreshVolumePrices repopulates the volume price cache from AWS. Only one
// caller fetches; the rest wait and then read the cache that the winner wrote,
// so a burst of cache misses costs a single trip to the Pricing API.
func (s *b) refreshVolumePrices(log *logger.Logger) ([]*volumePrice, error) {
	s.volumePricesFetch.Lock()
	defer s.volumePricesFetch.Unlock()
	if prices, err := s.getVolumePricesFromCache(); err == nil {
		log.Detail("Volume prices were fetched by a concurrent caller, using cache")
		return prices, nil
	}
	prices, err := s.getVolumePricesFromAWS()
	if err != nil {
		return nil, err
	}
	log.Detail("Storing in cache")
	if err := s.putVolumePricesToCache(prices); err != nil {
		return nil, err
	}
	return prices, nil
}

func (s *b) putVolumePricesToCache(prices []*volumePrice) error {
	if err := os.MkdirAll(s.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
	f := path.Join(s.workDir, "volume_prices.json")
	fd, err := os.Create(f)
	if err != nil {
		return err
	}
	defer fd.Close()
	vp := &volumePriceList{
		Prices: prices,
		Ts:     time.Now(),
	}
	return json.NewEncoder(fd).Encode(vp)
}

func (s *b) volumePriceCacheInvalidate() {
	f := path.Join(s.workDir, "volume_prices.json")
	os.Remove(f)
}

func (s *b) getVolumePricesFromCache() ([]*volumePrice, error) {
	f := path.Join(s.workDir, "volume_prices.json")
	fd, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	vp := &volumePriceList{}
	err = json.NewDecoder(fd).Decode(&vp)
	if err != nil {
		return nil, err
	}
	if time.Since(vp.Ts) > 24*time.Hour {
		return nil, errors.New("cache expired")
	}
	return vp.Prices, nil
}

func (s *b) getVolumePricesFromAWS() ([]*volumePrice, error) {
	log := s.log.WithPrefix("getVolumePricesFromAWS")
	if s.credentials != nil && s.credentials.SkipPricing {
		log.Detail("Pricing disabled (skip-pricing); returning empty volume price list")
		return []*volumePrice{}, nil
	}
	log.Detail("Start")
	defer log.Detail("End")
	cli, err := getPricingClient(s.credentials, aws.String("us-east-1"))
	if err != nil {
		return nil, err
	}
	// get volume prices
	log.Detail("Getting EC2 volume prices")
	paginator := pricing.NewGetProductsPaginator(cli, &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Storage"),
			},
		},
	})
	prices := []*volumePrice{}
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}
		// parse prices
		for _, price := range out.PriceList {
			p := &volumePrice{}
			json.Unmarshal([]byte(price), p) //nolint:errcheck
			prices = append(prices, p)
		}
	}

	// get EFS volume prices, fill in the prices, with volume type being SharedDisk_OneZone and SharedDisk_GeneralPurpose
	log.Detail("Getting EFS volume prices")
	paginator = pricing.NewGetProductsPaginator(cli, &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEFS"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Storage"),
			},
		},
	})
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}
		// parse prices
		for _, price := range out.PriceList {
			p := &volumePrice{}
			json.Unmarshal([]byte(price), p) //nolint:errcheck
			switch p.Product.Attributes.StorageClass {
			case "One Zone-General Purpose":
				p.Product.Attributes.VolumeApiName = "SharedDisk_OneZone"
			case "General Purpose":
				p.Product.Attributes.VolumeApiName = "SharedDisk_GeneralPurpose"
			default:
				continue
			}
			prices = append(prices, p)
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
		log.Detail("Cache miss (%s), getting from AWS", err)
		prices, err = s.refreshInstanceTypes(log)
		if err != nil {
			return nil, err
		}
	}

	// translate to backends.InstanceTypeList
	log.Detail("Responding")
	backendTypes := backends.InstanceTypeList{}
	for _, t := range prices {
		onDemandPrice, currency, _ := t.Price.onDemand()
		backendTypes = append(backendTypes, &backends.InstanceType{
			Region:           t.Region,
			Name:             t.Name,
			CPUs:             t.CPUs,
			MemoryGiB:        t.MemoryGiB,
			Arch:             t.Arch,
			NvmeCount:        t.NvmeCount,
			NvmeTotalSizeGiB: t.NvmeTotalSizeGiB,
			PricePerHour: backends.InstanceTypePrice{
				OnDemand: onDemandPrice,
				Spot:     t.SpotPrice,
				Currency: currency,
			},
		})
	}
	return backendTypes, nil
}

// refreshInstanceTypes repopulates the instance type cache from AWS. Only one
// caller fetches; the rest wait and then read the cache that the winner wrote,
// so a burst of cache misses costs a single trip to the Pricing API.
func (s *b) refreshInstanceTypes(log *logger.Logger) ([]*instanceType, error) {
	s.instanceTypesFetch.Lock()
	defer s.instanceTypesFetch.Unlock()
	if types, err := s.getInstanceTypesFromCache(); err == nil {
		log.Detail("Instance types were fetched by a concurrent caller, using cache")
		return types, nil
	}
	types, err := s.getInstanceTypesFromAWS()
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

func (s *b) putInstanceTypesToCache(types []*instanceType) error {
	if err := os.MkdirAll(s.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
	f := path.Join(s.workDir, "instance_types.json")
	fd, err := os.Create(f)
	if err != nil {
		return err
	}
	defer fd.Close()
	itp := &instanceTypePriceList{
		Types: types,
		Ts:    time.Now(),
	}
	return json.NewEncoder(fd).Encode(itp)
}

func (s *b) instanceTypeCacheInvalidate() {
	f := path.Join(s.workDir, "instance_types.json")
	os.Remove(f)
}

func (s *b) getInstanceTypesFromCache() ([]*instanceType, error) {
	f := path.Join(s.workDir, "instance_types.json")
	fd, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	itp := &instanceTypePriceList{}
	err = json.NewDecoder(fd).Decode(&itp)
	if err != nil {
		return nil, err
	}
	if time.Since(itp.Ts) > 24*time.Hour {
		return nil, errors.New("cache expired")
	}
	return itp.Types, nil
}

func (s *b) getSpotPricesFromAWS(cli *ec2.Client) (map[string]float64, error) {
	log := s.log.WithPrefix("getSpotPricesFromAWS: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	prices := make(map[string]float64)
	utc, _ := time.LoadLocation("UTC")
	endTime := time.Now().In(utc)
	startTime := endTime.Add(-1 * time.Minute) // 1 minute ago
	input := &ec2.DescribeSpotPriceHistoryInput{
		StartTime:           &startTime,
		EndTime:             &endTime,
		MaxResults:          aws.Int32(1000),
		ProductDescriptions: []string{"Linux/UNIX"},
	}
	pages := ec2.NewDescribeSpotPriceHistoryPaginator(cli, input)
	for pages.HasMorePages() {
		out, err := pages.NextPage(context.TODO())
		if err != nil {
			return nil, err
		}
		for _, hist := range out.SpotPriceHistory {
			newPrice, err := strconv.ParseFloat(*hist.SpotPrice, 64)
			if err != nil {
				log.Detail("Error parsing spot price: %s", err)
				continue
			}
			if price, ok := prices[string(hist.InstanceType)]; !ok || price < newPrice {
				prices[string(hist.InstanceType)] = newPrice
			}
		}
	}
	return prices, nil
}

func (s *b) getInstanceTypesFromAWS() ([]*instanceType, error) {
	log := s.log.WithPrefix("getInstanceTypesFromAWS: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// When pricing is disabled we still fetch the instance-type catalog from
	// EC2, but skip the spot-price history and the AWS Pricing API on-demand
	// lookups, returning instance types without prices.
	skipPricing := s.credentials != nil && s.credentials.SkipPricing
	if skipPricing {
		log.Detail("Pricing disabled (skip-pricing); fetching instance types without prices")
	}
	// get instance types from AWS
	itypes := []*instanceType{}
	ilock := new(sync.Mutex)
	// map[region][instanceType]spotPrice
	spotPrices := make(map[string]map[string]float64)
	wg := new(sync.WaitGroup)
	var errs error
	errLock := new(sync.Mutex)
	addErr := func(err error) {
		errLock.Lock()
		errs = errors.Join(errs, err)
		errLock.Unlock()
	}
	for _, region := range s.regions {
		wg.Add(1)
		log.Detail("Getting instance types for region %s", region)
		go func(region string) {
			defer wg.Done()
			defer log.Detail("Done getting instance types for region %s", region)
			err := func() error {
				ec2cli, err := getEc2Client(s.credentials, aws.String(region))
				if err != nil {
					return err
				}
				ec2paginator := ec2.NewDescribeInstanceTypesPaginator(ec2cli, &ec2.DescribeInstanceTypesInput{})
				for ec2paginator.HasMorePages() {
					out, err := ec2paginator.NextPage(context.Background())
					if err != nil {
						return err
					}
					for _, itype := range out.InstanceTypes {
						// Skip Mac instances - aerolab only supports Linux
						if strings.HasPrefix(string(itype.InstanceType), "mac") {
							continue
						}
						archs := []backends.Architecture{}
						for _, arch := range itype.ProcessorInfo.SupportedArchitectures {
							switch arch {
							case etypes.ArchitectureTypeX8664:
								archs = append(archs, backends.ArchitectureX8664)
							case etypes.ArchitectureTypeArm64:
								archs = append(archs, backends.ArchitectureARM64)
							}
						}
						nvmeCount := int32(0)
						nvmeTotalSizeGiB := int64(0)
						if itype.InstanceStorageInfo != nil {
							for _, a := range itype.InstanceStorageInfo.Disks {
								nvmeCount += aws.ToInt32(a.Count)
							}
							nvmeTotalSizeGiB = aws.ToInt64(itype.InstanceStorageInfo.TotalSizeInGB)
						}
						ilock.Lock()
						itypes = append(itypes, &instanceType{
							Region:           region,
							Name:             string(itype.InstanceType),
							CPUs:             int(aws.ToInt32(itype.VCpuInfo.DefaultVCpus)),
							MemoryGiB:        float64(aws.ToInt64(itype.MemoryInfo.SizeInMiB)) / 1024,
							Arch:             archs,
							NvmeCount:        int(nvmeCount),
							NvmeTotalSizeGiB: int(nvmeTotalSizeGiB),
						})
						ilock.Unlock()
					}
				}
				return nil
			}()
			if err != nil {
				addErr(err)
			}
		}(region)
		if !skipPricing {
			wg.Add(1)
			log.Detail("Getting spot prices for region %s", region)
			go func(region string) {
				defer wg.Done()
				defer log.Detail("Done getting spot prices for region %s", region)
				ec2cli, err := getEc2Client(s.credentials, aws.String(region))
				if err != nil {
					addErr(err)
					return
				}
				prices, err := s.getSpotPricesFromAWS(ec2cli)
				if err != nil {
					addErr(err)
					return
				}
				ilock.Lock()
				spotPrices[region] = prices
				ilock.Unlock()
			}(region)
		}
	}

	// get instance prices from AWS (skipped when pricing is disabled)
	prices := make(map[instancePriceKey]*instanceTypePrice)
	plock := new(sync.Mutex)
	var cli *pricing.Client
	if !skipPricing {
		log.Detail("Getting instance prices from AWS")
		var err error
		cli, err = getPricingClient(s.credentials, aws.String("us-east-1"))
		if err != nil {
			return nil, err
		}
	}
	mergePrices := func(regionPrices map[instancePriceKey]*instanceTypePrice) {
		plock.Lock()
		defer plock.Unlock()
		for k, p := range regionPrices {
			if existing, ok := prices[k]; ok && existing.rank() >= p.rank() {
				continue
			}
			prices[k] = p
		}
	}
	for _, region := range s.regions {
		if skipPricing {
			break
		}
		wg.Add(1)
		go func(region string) {
			log.Detail("Getting instance prices from AWS for region %s", region)
			defer wg.Done()
			defer log.Detail("Done getting instance prices from AWS for region %s", region)
			regionPrices, err := getInstancePricesFromAWS(cli, region, true)
			if err != nil {
				addErr(err)
				return
			}
			mergePrices(regionPrices)
		}(region)
	}
	wg.Wait()
	if errs != nil {
		return nil, errs
	}

	// Some families are never sold with the default tenancy/capacity-status
	// combination (high-memory bare metal is host-tenancy only), so anything
	// left unpriced gets a second, wider lookup in the regions that need it.
	if !skipPricing {
		widenRegions := make(map[string]bool)
		for _, itype := range itypes {
			if _, ok := prices[instancePriceKey{region: itype.Region, instanceType: itype.Name}]; !ok {
				widenRegions[itype.Region] = true
			}
		}
		for region := range widenRegions {
			wg.Add(1)
			go func(region string) {
				log.Detail("Getting fallback instance prices from AWS for region %s", region)
				defer wg.Done()
				defer log.Detail("Done getting fallback instance prices from AWS for region %s", region)
				regionPrices, err := getInstancePricesFromAWS(cli, region, false)
				if err != nil {
					addErr(err)
					return
				}
				mergePrices(regionPrices)
			}(region)
		}
		wg.Wait()
		if errs != nil {
			return nil, errs
		}
	}

	// merge prices into types
	log.Detail("Merging prices into types")
	for _, itype := range itypes {
		price, ok := prices[instancePriceKey{region: itype.Region, instanceType: itype.Name}]
		if !ok {
			log.Detail("OnDemand price not found for instance type %s in region %s", itype.Name, itype.Region)
		} else {
			itype.Price = price
		}
		regionSpot, ok := spotPrices[itype.Region]
		if !ok {
			log.Detail("Spot price not found for region %s", itype.Region)
			continue
		}
		itype.SpotPrice, ok = regionSpot[itype.Name]
		if !ok {
			log.Detail("Spot price not found for instance type %s in region %s", itype.Name, itype.Region)
		}
	}
	return itypes, nil
}

type instanceTypePrice struct {
	Product struct {
		ProductFamily string `json:"productFamily"`
		Attributes    struct {
			RegionCode     string `json:"regionCode"` // only if exact match
			InstanceType   string `json:"instanceType"`
			Tenancy        string `json:"tenancy"`
			CapacityStatus string `json:"capacitystatus"`
			UsageType      string `json:"usagetype"`
		} `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				Unit         string            `json:"unit"`
				PricePerUnit map[string]string `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// onDemand returns the hourly on-demand price and its currency. ok is false
// when the product carries no usable on-demand term.
func (p *instanceTypePrice) onDemand() (price float64, currency string, ok bool) {
	if p == nil {
		return 0, "", false
	}
	for _, onDemand := range p.Terms.OnDemand {
		for _, dimension := range onDemand.PriceDimensions {
			for unit, value := range dimension.PricePerUnit {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					continue
				}
				return parsed, unit, true
			}
		}
	}
	return 0, "", false
}

// rank scores how well a pricing product represents "what aerolab would be
// billed for running this instance type". The AWS Price List holds a separate
// product for every tenancy/capacity-status combination, and some families are
// only sold under one of the non-default combinations: high-memory bare metal
// (u-6tb1.metal and friends) for instance only ever appears with
// tenancy=Host, so a plain tenancy=Shared lookup finds nothing for them.
// Higher is better; negative means unusable.
func (p *instanceTypePrice) rank() int {
	if _, _, ok := p.onDemand(); !ok {
		return -1
	}
	score := 0
	switch p.Product.Attributes.CapacityStatus {
	case "Used", "":
		score += 16
	case "AllocatedCapacityReservation":
		score += 4
	}
	switch p.Product.Attributes.Tenancy {
	case "Shared", "":
		score += 8
	case "Dedicated":
		score += 2
	case "Host":
		score += 1
	}
	if !strings.Contains(p.Product.Attributes.UsageType, "Unused") {
		score++
	}
	return score
}

type instancePriceKey struct {
	region       string
	instanceType string
}

func (p *instanceTypePrice) key() instancePriceKey {
	return instancePriceKey{
		region:       p.Product.Attributes.RegionCode,
		instanceType: p.Product.Attributes.InstanceType,
	}
}

// instancePricingFilters builds the Pricing API filter set for Linux EC2
// instances in a region. The narrow variant pins tenancy/capacity-status to
// the values that cover the vast majority of instance types; the wide variant
// drops those (and marketoption, which some products omit entirely) so that
// families sold only as dedicated or host tenancy can still be priced.
func instancePricingFilters(region string, narrow bool) []types.Filter {
	filters := []types.Filter{
		{
			Type:  types.FilterTypeTermMatch,
			Field: aws.String("regionCode"),
			Value: aws.String(region),
		},
		// NOTE: no productFamily filter - it excludes bare metal instances
		// which have "Compute Instance (bare metal)" instead of "Compute Instance"
		{
			Type:  types.FilterTypeTermMatch,
			Field: aws.String("preInstalledSw"),
			Value: aws.String("NA"),
		},
		{
			Type:  types.FilterTypeTermMatch,
			Field: aws.String("operatingSystem"),
			Value: aws.String("Linux"),
		},
	}
	if !narrow {
		return filters
	}
	return append(filters,
		types.Filter{
			Type:  types.FilterTypeTermMatch,
			Field: aws.String("marketoption"),
			Value: aws.String("OnDemand"),
		},
		types.Filter{
			Type:  types.FilterTypeTermMatch,
			Field: aws.String("tenancy"),
			Value: aws.String("Shared"), // other values: Dedicated, Host
		},
		types.Filter{
			Type:  types.FilterTypeTermMatch,
			Field: aws.String("capacitystatus"),
			Value: aws.String("Used"), // other values: AllocatedCapacityReservation, UnusedCapacityReservation
		},
	)
}

// getInstancePricesFromAWS pages through the Pricing API for one region and
// keeps, per instance type, the best-ranked product.
func getInstancePricesFromAWS(cli *pricing.Client, region string, narrow bool) (map[instancePriceKey]*instanceTypePrice, error) {
	paginator := pricing.NewGetProductsPaginator(cli, &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		Filters:     instancePricingFilters(region, narrow),
	})
	best := make(map[instancePriceKey]*instanceTypePrice)
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}
		for _, price := range out.PriceList {
			p := &instanceTypePrice{}
			if err := json.Unmarshal([]byte(price), p); err != nil {
				continue
			}
			if p.Product.Attributes.InstanceType == "" {
				continue
			}
			score := p.rank()
			if score < 0 {
				continue
			}
			k := p.key()
			if existing, ok := best[k]; ok && existing.rank() >= score {
				continue
			}
			best[k] = p
		}
	}
	return best, nil
}

type instanceTypePriceList struct {
	Types []*instanceType
	Ts    time.Time
}

type instanceType struct {
	Region           string
	Name             string
	CPUs             int
	MemoryGiB        float64
	Arch             []backends.Architecture
	NvmeCount        int
	NvmeTotalSizeGiB int
	Price            *instanceTypePrice
	SpotPrice        float64
}
