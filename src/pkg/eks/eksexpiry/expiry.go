package eksexpiry

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"gopkg.in/yaml.v3"
)

type request struct {
	ClusterName string
	Region      string
	File        string
	In          time.Duration
	At          string
	at          time.Time
	queryOnly   bool
}

func Expiry() {
	// flag request struct
	r := new(request)
	// custom flag usage function
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), "\nIf both -in and -at are specified, the one with the longest expiry will take effect.\n")
	}
	// flag parameters
	flag.StringVar(&r.File, "file", "", "eksctl cluster defintion (sets/overrides 'name' and 'region' if provided)")
	flag.StringVar(&r.ClusterName, "name", "", "EKS Cluster Name (required)")
	flag.StringVar(&r.Region, "region", "", "EKS Cluster Region (required)")
	flag.DurationVar(&r.In, "in", 0, "Expire in duration from now; ex: 30h5m10s")
	flag.StringVar(&r.At, "at", "", "Expire at this precise time; format: RFC3339 (2006-01-02T15:04:05+07:00)")
	// if nothing is specified, display help and exit
	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(1)
	}
	// parse flags
	flag.Parse()
	// parse file if provided
	if r.File != "" {
		type eksctlYaml struct {
			Metadata struct {
				Name   string `yaml:"name"`
				Region string `yaml:"region"`
			} `yaml:"metadata"`
		}
		eksctl := new(eksctlYaml)
		f, err := os.Open(r.File)
		if err != nil {
			fmt.Printf("Failed to open %s: %s\n", r.File, err)
			os.Exit(1)
		}
		err = yaml.NewDecoder(f).Decode(eksctl)
		f.Close()
		if err != nil {
			fmt.Printf("Failed to open %s: %s\n", r.File, err)
			os.Exit(1)
		}
		r.ClusterName = eksctl.Metadata.Name
		r.Region = eksctl.Metadata.Region
	}
	// flag sanity checks
	if r.ClusterName == "" {
		fmt.Println("ERROR: Cluster Name is required!")
		os.Exit(1)
	}
	if r.Region == "" {
		fmt.Println("ERROR: Cluster Region is required!")
		os.Exit(1)
	}
	if r.In == 0 && r.At == "" {
		r.queryOnly = true
	}
	if !r.queryOnly {
		// compute expiry time
		if r.In != 0 {
			r.at = time.Now().Add(r.In)
		}
		if r.At != "" {
			at, err := time.Parse(time.RFC3339, r.At)
			if err != nil {
				fmt.Printf("ERROR: Invalid value for -at, must be RFC3339 (2006-01-02T15:04:05Z07:00): %s\n", err)
				os.Exit(1)
			}
			if r.at.IsZero() || at.After(r.at) {
				r.at = at
			}
		}
		// expiry time sanity check
		if r.at.Before(time.Now()) {
			fmt.Printf("ERROR: cluster would expiry immediately (calculated expiry: %s)\n", r.at.Format(time.RFC3339))
			os.Exit(1)
		}
	}
	// check connectivity
	fmt.Println("Connecting to AWS EKS")
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(r.Region))
	if err != nil {
		fmt.Printf("ERROR: Could not create AWS Session: %s\n", err)
		os.Exit(1)
	}
	svc := eks.NewFromConfig(cfg)
	// find cluster
	fmt.Println("Looking up cluster")
	cluster, err := svc.DescribeCluster(context.Background(), &eks.DescribeClusterInput{
		Name: aws.String(r.ClusterName),
	})
	if err != nil {
		fmt.Printf("ERROR: eks.DescribeCluster API returned: %s\n", err)
		os.Exit(1)
	}
	if cluster == nil {
		fmt.Println("ERROR: eks.DescribeCluster API returned an empty value.")
		os.Exit(1)
	}
	// get old expiry
	loc := time.Local
	old := "none"
	oldAt := cluster.Cluster.Tags["ExpireAt"]
	if oldAt != "" {
		oldAtInt, err := strconv.Atoi(oldAt)
		if err == nil {
			old = time.Unix(int64(oldAtInt), 0).In(loc).Format(time.RFC3339)
		}
	} else {
		initial := cluster.Cluster.Tags["initialExpiry"]
		if initial != "" {
			createTs := aws.ToTime(cluster.Cluster.CreatedAt)
			initialDuration, err := time.ParseDuration(initial)
			if err == nil {
				old = createTs.Add(initialDuration).In(loc).Format(time.RFC3339)
			}
		}
	}
	//query only - print and exit
	if r.queryOnly {
		fmt.Printf("query:result cluster=%s region=%s at=%s\n", r.ClusterName, r.Region, old)
		fmt.Println("Done")
		return
	}
	// set new expiry
	fmt.Printf("Setting expiry cluster=%s region=%s at=%s old=%s\n", r.ClusterName, r.Region, r.at.In(loc).Format(time.RFC3339), old)
	_, err = svc.TagResource(context.Background(), &eks.TagResourceInput{
		ResourceArn: cluster.Cluster.Arn,
		Tags: map[string]string{
			"ExpireAt": strconv.Itoa(int(r.at.UTC().Unix())),
		},
	})
	if err != nil {
		fmt.Printf("ERROR: Could not tag EKS cluster, eks.TagResource API returned: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("Done")
}
