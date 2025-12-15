package baws

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	etypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/lithammer/shortuuid"
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
		prices, err = s.getVolumePricesFromAWS()
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

func (s *b) putVolumePricesToCache(prices []*volumePrice) error {
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
			json.Unmarshal([]byte(price), p)
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
			json.Unmarshal([]byte(price), p)
			if p.Product.Attributes.StorageClass == "OneZone-General Purpose" {
				p.Product.Attributes.VolumeApiName = "SharedDisk_OneZone"
			} else if p.Product.Attributes.StorageClass == "General Purpose" {
				p.Product.Attributes.VolumeApiName = "SharedDisk_GeneralPurpose"
			} else {
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
		prices, err = s.getInstanceTypesFromAWS()
		if err != nil {
			return nil, err
		}
		// store in cache only if we got results (avoid caching empty results)
		if len(prices) > 0 {
			log.Detail("Storing in cache")
			err = s.putInstanceTypesToCache(prices)
			if err != nil {
				return nil, err
			}
		} else {
			log.Detail("Not caching empty instance types list")
		}
	}

	// translate to backends.InstanceTypeList
	log.Detail("Responding")
	backendTypes := backends.InstanceTypeList{}
	for _, t := range prices {
		onDemandPrice := float64(0)
		currency := ""
		if t.Price != nil {
			for _, onDemand := range t.Price.Terms.OnDemand {
				for _, priceDimension := range onDemand.PriceDimensions {
					for unit, price := range priceDimension.PricePerUnit {
						currency = unit
						onDemandPrice, err = strconv.ParseFloat(price, 64)
						if err != nil {
							log.Detail("Error parsing price: %s", err)
							continue
						}
						break
					}
					break
				}
				break
			}
		}
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

func (s *b) putInstanceTypesToCache(types []*instanceType) error {
	os.MkdirAll(s.workDir, 0755)
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
		MaxResults:          aws.Int32(100),
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
	// get instance types from AWS
	itypes := []*instanceType{}
	ilock := new(sync.Mutex)
	// map[region][instanceType]spotPrice
	spotPrices := make(map[string]map[string]float64)
	wg := new(sync.WaitGroup)
	var errs error
	for _, region := range s.regions {
		wg.Add(2)
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
						archs := []backends.Architecture{}
						for _, arch := range itype.ProcessorInfo.SupportedArchitectures {
							if arch == etypes.ArchitectureTypeX8664 {
								archs = append(archs, backends.ArchitectureX8664)
							} else if arch == etypes.ArchitectureTypeArm64 {
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
				errs = errors.Join(errs, err)
			}
		}(region)
		log.Detail("Getting spot prices for region %s", region)
		go func(region string) {
			defer wg.Done()
			defer log.Detail("Done getting spot prices for region %s", region)
			ec2cli, err := getEc2Client(s.credentials, aws.String(region))
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			prices, err := s.getSpotPricesFromAWS(ec2cli)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			ilock.Lock()
			spotPrices[region] = prices
			ilock.Unlock()
		}(region)
	}

	// get instance prices from AWS
	log.Detail("Getting instance prices from AWS")
	cli, err := getPricingClient(s.credentials, aws.String("us-east-1"))
	if err != nil {
		return nil, err
	}
	prices := []*instanceTypePrice{}
	for _, region := range s.regions {
		wg.Add(1)
		go func(region string) {
			log.Detail("Getting instance prices from AWS for region %s", region)
			defer wg.Done()
			defer log.Detail("Done getting instance prices from AWS for region %s", region)
			paginator := pricing.NewGetProductsPaginator(cli, &pricing.GetProductsInput{
				ServiceCode: aws.String("AmazonEC2"),
				Filters: []types.Filter{
					{
						Field: aws.String("regionCode"),
						Type:  types.FilterTypeTermMatch,
						Value: aws.String(region),
					},
					{
						Type:  types.FilterTypeTermMatch,
						Field: aws.String("productFamily"),
						Value: aws.String("Compute Instance"),
					},
					{
						Type:  types.FilterTypeTermMatch,
						Field: aws.String("marketoption"),
						Value: aws.String("OnDemand"),
					},
					{
						Type:  types.FilterTypeTermMatch,
						Field: aws.String("tenancy"),
						Value: aws.String("Shared"), // other values: Dedicated, Host
					},
					{
						Type:  types.FilterTypeTermMatch,
						Field: aws.String("capacitystatus"),
						Value: aws.String("Used"), // other values: AllocatedCapacityReservation, UnusedCapacityReservation
					},
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
				},
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.Background())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				// parse prices
				for _, price := range out.PriceList {
					p := &instanceTypePrice{}
					json.Unmarshal([]byte(price), p)
					prices = append(prices, p)
				}
			}
		}(region)
	}
	wg.Wait()
	if errs != nil {
		return nil, errs
	}
	// merge prices into types
	log.Detail("Merging prices into types")
	for _, itype := range itypes {
		found := false
		for _, price := range prices {
			if itype.Region == price.Product.Attributes.RegionCode && itype.Name == price.Product.Attributes.InstanceType {
				price := price
				itype.Price = price
				found = true
				break
			}
		}
		if !found {
			log.Detail("OnDemand price not found for instance type %s in region %s", itype.Name, itype.Region)
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
		Attributes struct {
			RegionCode   string `json:"regionCode"` // only if exact match
			InstanceType string `json:"instanceType"`
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
