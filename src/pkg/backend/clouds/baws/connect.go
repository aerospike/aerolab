package baws

import (
	"context"
	"errors"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

func GetEc2Client(credentials *clouds.Credentials, region *string) (client *ec2.Client, err error) {
	var creds *clouds.AWS
	if credentials != nil {
		creds = &credentials.AWS
	}
	return getEc2Client(creds, region)
}

func GetEksClient(credentials *clouds.Credentials, region *string) (client *eks.Client, err error) {
	var creds *clouds.AWS
	if credentials != nil {
		creds = &credentials.AWS
	}
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = eks.NewFromConfig(*cfg)
	return client, nil
}

func GetCloudformationClient(credentials *clouds.Credentials, region *string) (client *cloudformation.Client, err error) {
	var creds *clouds.AWS
	if credentials != nil {
		creds = &credentials.AWS
	}
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = cloudformation.NewFromConfig(*cfg)
	return client, nil
}

func GetIamClient(credentials *clouds.Credentials, region *string) (client *iam.Client, err error) {
	var creds *clouds.AWS
	if credentials != nil {
		creds = &credentials.AWS
	}
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = iam.NewFromConfig(*cfg)
	return client, nil
}

func getRoute53Client(creds *clouds.AWS, region *string) (client *route53.Client, err error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = route53.NewFromConfig(*cfg)
	return client, nil
}

func GetRoute53Client(credentials *clouds.Credentials, region *string) (client *route53.Client, err error) {
	var creds *clouds.AWS
	if credentials != nil {
		creds = &credentials.AWS
	}
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = route53.NewFromConfig(*cfg)
	return client, nil
}

func getSchedulerClient(creds *clouds.AWS, region *string) (client *scheduler.Client, err error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = scheduler.NewFromConfig(*cfg)
	return client, nil
}

func getIamClient(creds *clouds.AWS, region *string) (client *iam.Client, err error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = iam.NewFromConfig(*cfg)
	return client, nil
}

func getLambdaClient(creds *clouds.AWS, region *string) (client *lambda.Client, err error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = lambda.NewFromConfig(*cfg)
	return client, nil
}

func getStsClient(creds *clouds.AWS, region *string) (client *sts.Client, err error) {
	cfg, err := getCfgForClient(creds, region)
	if err != nil {
		return nil, err
	}
	client = sts.NewFromConfig(*cfg)
	return client, nil
}

func GetStsClient(credentials *clouds.Credentials, region *string) (client *sts.Client, err error) {
	var creds *clouds.AWS
	if credentials != nil {
		creds = &credentials.AWS
	}
	return getStsClient(creds, region)
}
