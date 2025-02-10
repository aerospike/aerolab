package baws

import (
	"context"
	"errors"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
)

func getPricingClient(creds *clouds.AWS, region *string) (*pricing.Client, error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	return pricing.NewFromConfig(*cfg), nil
}

func getCfgForClient(creds *clouds.AWS, region *string) (*aws.Config, error) {
	opts := []func(*config.LoadOptions) error{}
	if creds != nil {
		switch creds.AuthMethod {
		case clouds.AWSAuthMethodShared:
			if creds.Shared.Profile != "" {
				opts = append(opts, config.WithSharedConfigProfile(creds.Shared.Profile))
			}
		case clouds.AWSAuthMethodStatic:
			opts = append(opts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(creds.Static.KeyID, creds.Static.SecretKey, "")))
		default:
			return nil, errors.New("credentials auth method unsupported")
		}
	}
	if region != nil {
		opts = append(opts, config.WithRegion(*region))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func getEc2Client(creds *clouds.AWS, region *string) (client *ec2.Client, err error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = ec2.NewFromConfig(*cfg)
	return client, nil
}

func getEfsClient(creds *clouds.AWS, region *string) (client *efs.Client, err error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = efs.NewFromConfig(*cfg)
	return client, nil
}
