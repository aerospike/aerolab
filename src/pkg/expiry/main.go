package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/expiry/expire"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/jessevdk/go-flags"
	"github.com/rglonek/logger"
)

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		awsLambdaMain()
	} else if os.Getenv("K_SERVICE") != "" {
		gcpCloudRunMain()
	} else if os.Getenv("FUNCTION_TARGET") != "" {
		gcpCloudFunctionMain()
	} else {
		serverMain()
	}
}

// env vars:
// AWS_LAMBDA_FUNCTION_NAME not empty
// AEROLAB_LOG_LEVEL: log level
// AEROLAB_VERSION: version
// AWS_REGION: region
// AEROLAB_EXPIRE_EKSCTL: expire eksctl
// AEROLAB_CLEANUP_DNS: cleanup dns
func awsLambdaMain() {
	h := &expire.ExpiryHandler{
		ExpireEksctl: os.Getenv("AEROLAB_EXPIRE_EKSCTL") == "true",
		CleanupDNS:   os.Getenv("AEROLAB_CLEANUP_DNS") == "true",
		Credentials:  nil,
	}
	var err error
	logLevel, err := strconv.Atoi(os.Getenv("AEROLAB_LOG_LEVEL"))
	if err != nil {
		logLevel = 4
	}
	h.Backend, err = backend.Init("unused_project_name", &backend.Config{
		RootDir:         "/tmp/aerolab",
		Cache:           false,
		LogLevel:        logger.LogLevel(logLevel),
		AerolabVersion:  os.Getenv("AEROLAB_VERSION"),
		ListAllProjects: true,
		Credentials:     h.Credentials,
	}, false)
	if err != nil {
		log.Fatalf("Failed to initialize backend: %v", err)
	}
	err = h.Backend.AddRegion(backend.BackendTypeAWS, os.Getenv("AWS_REGION"))
	if err != nil {
		log.Fatalf("Failed to add region: %v", err)
	}
	lambda.Start(func(ctx context.Context, event *string) (*string, error) {
		return event, h.Expire()
	})
}

func gcpCloudRunMain() {
	// TODO: init backend and expire from gcp
}

func gcpCloudFunctionMain() {
	// TODO: init backend and expire from gcp
}

type params struct {
	LogLevel     int      `short:"l" long:"log-level" default:"4" description:"log level, 1=critical, 2=error, 3=warning, 4=info, 5=debug, 6=detail"`
	ExpireEksctl bool     `short:"e" long:"expire-eksctl" description:"enable eksctl expiry"`
	TmpDir       string   `short:"t" long:"tmp-dir" default:"./tmp-expiry" description:"temporary directory to use while running; this directory will be deleted on exit"`
	Region       []string `short:"r" long:"region" description:"AWS region, may be specified multiple times"`
	AwsProfile   string   `short:"p" long:"aws-profile" description:"AWS profile to use; this parameter is ignored if aws-key-id and aws-secret-key are provided"`
	AwsKeyID     string   `short:"k" long:"aws-key-id" description:"AWS key id to use"`
	AwsSecretKey string   `short:"s" long:"aws-secret-key" description:"AWS secret key to use"`
	CleanupDNS   bool     `short:"d" long:"cleanup-dns" description:"enable dns cleanup"`
}

func (p *params) getCredentials() *clouds.Credentials {
	if p.AwsKeyID != "" && p.AwsSecretKey != "" {
		return &clouds.Credentials{
			AWS: clouds.AWS{
				AuthMethod: clouds.AWSAuthMethodStatic,
				Static: clouds.StaticAWSConfig{
					KeyID:     p.AwsKeyID,
					SecretKey: p.AwsSecretKey,
				},
			},
		}
	} else if p.AwsProfile != "" {
		return &clouds.Credentials{
			AWS: clouds.AWS{
				AuthMethod: clouds.AWSAuthMethodShared,
				Shared: clouds.SharedAWSConfig{
					Profile: p.AwsProfile,
				},
			},
		}
	}
	return nil
}

func serverMain() {
	p := &params{}
	_, err := flags.Parse(p)
	if err != nil {
		log.Fatalf("Failed to parse flags: %v", err)
	}
	if p.Region == nil {
		log.Fatalf("Region is required")
	}
	log.Printf("Using temporary directory: %s", p.TmpDir)
	os.RemoveAll(p.TmpDir)
	err = os.MkdirAll(p.TmpDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(p.TmpDir)
	log.Printf("Running aerolab version")
	aver, err := exec.Command("aerolab", "version").CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to run aerolab version: %v", err)
	}
	log.Printf("Aerolab version: %s", strings.Trim(string(aver), "\n"))
	log.Printf("Initializing backend")
	h := &expire.ExpiryHandler{
		ExpireEksctl: p.ExpireEksctl,
		CleanupDNS:   p.CleanupDNS,
		Credentials:  p.getCredentials(),
	}
	h.Backend, err = backend.Init("unused_project_name", &backend.Config{
		RootDir:         p.TmpDir,
		Cache:           false,
		LogLevel:        logger.LogLevel(p.LogLevel),
		AerolabVersion:  strings.Trim(string(aver), "\n"),
		ListAllProjects: true,
		Credentials:     h.Credentials,
	}, false)
	if err != nil {
		log.Fatalf("Failed to initialize backend: %v", err)
	}
	log.Printf("Adding regions")
	for _, region := range p.Region {
		err = h.Backend.AddRegion(backend.BackendTypeAWS, region)
		if err != nil {
			log.Fatalf("Failed to add region: %v", err)
		}
	}
	log.Printf("Expiring")
	err = h.Expire()
	if err != nil {
		log.Fatalf("Failed to expire: %v", err)
	}
	log.Print("Expired successfully")
}
