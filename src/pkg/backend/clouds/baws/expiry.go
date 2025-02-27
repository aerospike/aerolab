package baws

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	ltypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	stypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
)

type ExpiryDetail struct {
	SchedulerArn       string            `json:"schedulerArn" yaml:"schedulerArn"`
	SchedulerTargetArn string            `json:"schedulerTargetArn" yaml:"schedulerTargetArn"`
	Schedule           string            `json:"schedule" yaml:"schedule"`
	FunctionArn        string            `json:"functionArn" yaml:"functionArn"`
	IAMScheduler       string            `json:"iamScheduler" yaml:"iamScheduler"`
	IAMFunction        string            `json:"iamFunction" yaml:"iamFunction"`
	ExpireEksctl       bool              `json:"expireEksctl" yaml:"expireEksctl"`
	CleanupDNS         bool              `json:"cleanupDNS" yaml:"cleanupDNS"`
	LogLevel           int               `json:"logLevel" yaml:"logLevel"`
	Environment        map[string]string `json:"environment" yaml:"environment"`
	SchedulerState     string            `json:"schedulerState" yaml:"schedulerState"`
	FunctionState      string            `json:"functionState" yaml:"functionState"`
}

func (s *b) ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeConfiguration: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	for _, zone := range zones {
		if !slices.Contains(s.regions, zone) {
			return fmt.Errorf("zone %s is not enabled", zone)
		}
	}

	log.Detail("Getting expiry list")
	esys, err := s.ExpiryList()
	if err != nil {
		return err
	}

	for _, zone := range zones {
		found := false
		for _, esys := range esys {
			if esys.Zone == zone {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("zone %s not found in expiry list", zone)
		}
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(zones))
	var reterr error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log := log.WithPrefix("zone=" + zone + " ")
			log.Detail("Start")
			defer log.Detail("End")
			err := s.expiryChangeConfiguration(zone, log, logLevel, expireEksctl, cleanupDNS)
			if err != nil {
				reterr = errors.Join(reterr, err)
			}
		}(zone)
	}
	wg.Wait()
	if reterr != nil {
		return reterr
	}
	return nil
}

func (s *b) expiryChangeConfiguration(zone string, log *logger.Logger, logLevel int, expireEksctl bool, cleanupDNS bool) error {
	log.Detail("Connecting to AWS")
	lclient, err := getLambdaClient(s.credentials, &zone)
	if err != nil {
		return err
	}

	log.Detail("Updating lambda function configuration")
	_, err = lclient.UpdateFunctionConfiguration(context.TODO(), &lambda.UpdateFunctionConfigurationInput{
		FunctionName: aws.String("aerolab-expiries-" + zone),
		Environment: &ltypes.Environment{
			Variables: map[string]string{
				"AEROLAB_EXPIRE_EKSCTL": strconv.FormatBool(expireEksctl),
				"AEROLAB_CLEANUP_DNS":   strconv.FormatBool(cleanupDNS),
				"AEROLAB_LOG_LEVEL":     strconv.Itoa(logLevel),
			},
		},
	})
	if err != nil {
		return err
	}
	return nil
}

// force true means remove previous expiry systems and install new ones
// force false means install only if previous installation was failed or version is different
// onUpdateKeepOriginalSettings true means keep original settings on update, and only apply specified settings on reinstall
func (s *b) ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryInstall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	for _, zone := range zones {
		if !slices.Contains(s.regions, zone) {
			return fmt.Errorf("zone %s is not enabled", zone)
		}
	}

	log.Detail("Getting expiry list")
	esys, err := s.ExpiryList()
	if err != nil {
		return err
	}

	expiryVersion, err := strconv.Atoi(strings.Trim(backends.ExpiryVersion, "\n \t\r"))
	if err != nil {
		return err
	}

	delZones := []string{}
	if force {
		for _, zone := range zones {
			for _, esys := range esys {
				if esys.Zone == zone {
					delZones = append(delZones, zone)
					break
				}
			}
		}
	} else {
		newZones := []string{}
		for _, zone := range zones {
			found := false
			for _, esys := range esys {
				if esys.Zone == zone {
					found = true
					esysVersion, _ := strconv.Atoi(strings.Trim(esys.Version, "\n \t\r"))
					if !esys.InstallationSuccess || esysVersion < expiryVersion {
						delZones = append(delZones, zone)
						newZones = append(newZones, zone)
					}
					break
				}
			}
			if !found {
				newZones = append(newZones, zone)
			}
		}
		zones = newZones
	}

	if len(delZones) > 0 {
		log.Detail("Removing previous expiry systems from zones: " + strings.Join(delZones, ", "))
		err := s.ExpiryRemove(delZones...)
		if err != nil {
			return err
		}
	}
	log.Detail("Installing new expiry systems in zones: " + strings.Join(zones, ", "))

	wg := new(sync.WaitGroup)
	wg.Add(len(zones))
	var reterr error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log := log.WithPrefix("zone=" + zone + " ")
			log.Detail("Start")
			defer log.Detail("End")
			err := s.expiryInstall(zone, log, intervalMinutes, expireEksctl, cleanupDNS, logLevel, onUpdateKeepOriginalSettings, esys, slices.Contains(delZones, zone))
			if err != nil {
				reterr = errors.Join(reterr, err)
			}
		}(zone)
	}
	wg.Wait()
	if reterr != nil {
		return reterr
	}
	return nil
}

func (s *b) expiryInstall(zone string, log *logger.Logger, intervalMinutes int, expireEksctl bool, cleanupDNS bool, logLevel int, onUpdateKeepOriginalSettings bool, esys []*backends.ExpirySystem, isUpdate bool) error {
	if isUpdate && onUpdateKeepOriginalSettings {
		var e *backends.ExpirySystem
		for _, esys := range esys {
			if esys.Zone == zone {
				e = esys
				break
			}
		}
		if e != nil {
			intervalMinutes = e.FrequencyMinutes
			expireEksctl = e.BackendSpecific.(*ExpiryDetail).ExpireEksctl
			cleanupDNS = e.BackendSpecific.(*ExpiryDetail).CleanupDNS
			logLevel = e.BackendSpecific.(*ExpiryDetail).LogLevel
		}
	}
	log.Detail("Connecting to AWS")
	sclient, err := getSchedulerClient(s.credentials, &zone)
	if err != nil {
		return err
	}
	lclient, err := getLambdaClient(s.credentials, &zone)
	if err != nil {
		return err
	}
	iamclient, err := getIamClient(s.credentials, &zone)
	if err != nil {
		return err
	}
	stsclient, err := getStsClient(s.credentials, &zone)
	if err != nil {
		return err
	}

	log.Detail("Getting caller identity - account ID")
	ident, err := stsclient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}
	accountId := *ident.Account

	log.Detail("Creating lambda IAM role")
	lambdaRole, err := iamclient.CreateRole(context.TODO(), &iam.CreateRoleInput{
		RoleName:                 aws.String("aerolab-expiries-lambda-" + zone),
		AssumeRolePolicyDocument: aws.String(`{"Statement":[{"Action":"sts:AssumeRole","Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"}}],"Version":"2012-10-17"}`),
	})
	if err != nil {
		return err
	}

	log.Detail("Waiting for lambda IAM role to exist")
	err = iam.NewRoleExistsWaiter(iamclient, func(o *iam.RoleExistsWaiterOptions) {
		o.MinDelay = 1 * time.Second
		o.MaxDelay = 5 * time.Second
	}).Wait(context.TODO(), &iam.GetRoleInput{
		RoleName: lambdaRole.Role.RoleName,
	}, time.Minute)
	if err != nil {
		return err
	}

	log.Detail("Creating embedded lambda IAM policy")
	_, err = iamclient.PutRolePolicy(context.TODO(), &iam.PutRolePolicyInput{
		PolicyName:     aws.String("aerolab-expiries-lambda-policy-" + zone),
		RoleName:       aws.String("aerolab-expiries-lambda-" + zone),
		PolicyDocument: aws.String(fmt.Sprintf(`{"Statement":[{"Action":"logs:CreateLogGroup","Effect":"Allow","Resource":"arn:aws:logs:%s:%s:*"},{"Action":["logs:CreateLogStream","logs:PutLogEvents"],"Effect":"Allow","Resource":["arn:aws:logs:%s:%s:log-group:/aws/lambda/aerolab-expiries:*"]},{"Action":"eks:*","Effect":"Allow","Resource":"*"},{"Action":["ssm:GetParameter","ssm:GetParameters"],"Effect":"Allow","Resource":["arn:aws:ssm:*:%s:parameter/aws/*","arn:aws:ssm:*::parameter/aws/*"]},{"Action":["kms:CreateGrant","kms:DescribeKey"],"Effect":"Allow","Resource":"*"},{"Action":["logs:PutRetentionPolicy"],"Effect":"Allow","Resource":"*"},{"Action":["iam:CreateInstanceProfile","iam:DeleteInstanceProfile","iam:GetInstanceProfile","iam:RemoveRoleFromInstanceProfile","iam:GetRole","iam:CreateRole","iam:DeleteRole","iam:AttachRolePolicy","iam:PutRolePolicy","iam:AddRoleToInstanceProfile","iam:ListInstanceProfilesForRole","iam:PassRole","iam:DetachRolePolicy","iam:DeleteRolePolicy","iam:GetRolePolicy","iam:GetOpenIDConnectProvider","iam:CreateOpenIDConnectProvider","iam:DeleteOpenIDConnectProvider","iam:TagOpenIDConnectProvider","iam:ListOpenIDConnectProviders","iam:ListOpenIDConnectProviderTags","iam:DeleteOpenIDConnectProvider","iam:ListAttachedRolePolicies","iam:TagRole","iam:GetPolicy","iam:CreatePolicy","iam:DeletePolicy","iam:ListPolicyVersions"],"Effect":"Allow","Resource":["arn:aws:iam::%s:instance-profile/eksctl-*","arn:aws:iam::%s:role/eksctl-*","arn:aws:iam::%s:policy/eksctl-*","arn:aws:iam::%s:oidc-provider/*","arn:aws:iam::%s:role/aws-service-role/eks-nodegroup.amazonaws.com/AWSServiceRoleForAmazonEKSNodegroup","arn:aws:iam::%s:role/eksctl-managed-*"]},{"Action":["iam:GetRole"],"Effect":"Allow","Resource":["arn:aws:iam::%s:role/*"]},{"Action":["iam:CreateServiceLinkedRole"],"Condition":{"StringEquals":{"iam:AWSServiceName":["eks.amazonaws.com","eks-nodegroup.amazonaws.com","eks-fargate.amazonaws.com"]}},"Effect":"Allow","Resource":"*"},{"Effect": "Allow","Action": ["route53:ChangeResourceRecordSets","route53:ListResourceRecordSets"],"Resource": ["arn:aws:route53:::hostedzone/*"]}],"Version":"2012-10-17"}`, zone, accountId, zone, accountId, accountId, accountId, accountId, accountId, accountId, accountId, accountId, accountId)),
	})
	if err != nil {
		return err
	}

	log.Detail("Attaching base lambda IAM policies")
	for _, npolicy := range awsExpiryPolicies {
		_, err = iamclient.AttachRolePolicy(context.TODO(), &iam.AttachRolePolicyInput{
			RoleName:  aws.String("aerolab-expiries-lambda-" + zone),
			PolicyArn: aws.String(npolicy),
		})
		if err != nil {
			return err
		}
	}

	log.Detail("Creating scheduler IAM role")
	schedRole, err := iamclient.CreateRole(context.TODO(), &iam.CreateRoleInput{
		RoleName:                 aws.String("aerolab-expiries-scheduler-" + zone),
		AssumeRolePolicyDocument: aws.String(fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect": "Allow","Principal":{"Service":"scheduler.amazonaws.com"},"Action":"sts:AssumeRole","Condition":{"StringEquals":{"aws:SourceAccount":"%s"}}}]}`, accountId)),
	})
	if err != nil {
		return err
	}

	log.Detail("Waiting for scheduler IAM role to exist")
	err = iam.NewRoleExistsWaiter(iamclient, func(o *iam.RoleExistsWaiterOptions) {
		o.MinDelay = 1 * time.Second
		o.MaxDelay = 5 * time.Second
	}).Wait(context.TODO(), &iam.GetRoleInput{
		RoleName: schedRole.Role.RoleName,
	}, time.Minute)
	if err != nil {
		return err
	}

	log.Detail("Attaching embedded scheduler IAM policy")
	_, err = iamclient.PutRolePolicy(context.TODO(), &iam.PutRolePolicyInput{
		PolicyName:     aws.String("aerolab-expiries-scheduler-policy-" + zone),
		RoleName:       aws.String("aerolab-expiries-scheduler-" + zone),
		PolicyDocument: aws.String(fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["lambda:InvokeFunction"],"Resource":["arn:aws:lambda:%s:%s:function:aerolab-expiries:*","arn:aws:lambda:%s:%s:function:aerolab-expiries"]}]}`, zone, accountId, zone, accountId)),
	})
	if err != nil {
		return err
	}

	log.Detail("Creating lambda function")
	function, err := lclient.CreateFunction(context.TODO(), &lambda.CreateFunctionInput{
		Code: &ltypes.FunctionCode{
			ZipFile: backends.ExpiryBinary,
		},
		FunctionName: aws.String("aerolab-expiries-" + zone),
		Handler:      aws.String("bootstrap"),
		PackageType:  ltypes.PackageTypeZip,
		Timeout:      aws.Int32(900),
		Publish:      true,
		Runtime:      ltypes.RuntimeProvidedal2023,
		Tags:         map[string]string{TAG_AEROLAB_VERSION: strings.Trim(backends.ExpiryVersion, "\n \t\r")},
		Environment: &ltypes.Environment{
			Variables: map[string]string{
				"EKS_ROLE":              aws.ToString(lambdaRole.Role.Arn),
				"AEROLAB_LOG_LEVEL":     strconv.Itoa(logLevel),
				"AEROLAB_VERSION":       s.aerolabVersion,
				"AEROLAB_EXPIRE_EKSCTL": strconv.FormatBool(expireEksctl),
				"AEROLAB_CLEANUP_DNS":   strconv.FormatBool(cleanupDNS),
			},
		},
		Role: lambdaRole.Role.Arn,
	})
	if err != nil && strings.Contains(err.Error(), "InvalidParameterValueException: The role defined for the function cannot be assumed by Lambda") {
		retries := 0
		for {
			retries++
			log.Detail("IAM not ready, waiting for IAM and retrying to create Lambda")
			time.Sleep(5 * time.Second)
			function, err = lclient.CreateFunction(context.TODO(), &lambda.CreateFunctionInput{
				Code: &ltypes.FunctionCode{
					ZipFile: backends.ExpiryBinary,
				},
				FunctionName: aws.String("aerolab-expiries-" + zone),
				Handler:      aws.String("bootstrap"),
				PackageType:  ltypes.PackageTypeZip,
				Timeout:      aws.Int32(900),
				Publish:      true,
				Runtime:      ltypes.RuntimeProvidedal2023,
				Tags:         map[string]string{TAG_AEROLAB_VERSION: strings.Trim(backends.ExpiryVersion, "\n \t\r")},
				Environment: &ltypes.Environment{
					Variables: map[string]string{
						"EKS_ROLE":              aws.ToString(lambdaRole.Role.Arn),
						"AEROLAB_LOG_LEVEL":     strconv.Itoa(logLevel),
						"AEROLAB_VERSION":       s.aerolabVersion,
						"AEROLAB_EXPIRE_EKSCTL": strconv.FormatBool(expireEksctl),
						"AEROLAB_CLEANUP_DNS":   strconv.FormatBool(cleanupDNS),
					},
				},
				Role: lambdaRole.Role.Arn,
			})
			if err != nil && !strings.Contains(err.Error(), "InvalidParameterValueException: The role defined for the function cannot be assumed by Lambda") {
				return err
			} else if err == nil {
				break
			} else if retries > 3 {
				return err
			}
		}
	} else if err != nil {
		return err
	}

	log.Detail("Creating scheduler")
	err = errors.New("execution role you provide must allow AWS EventBridge Scheduler to assume the role")
	retries := 0
	for err != nil && strings.Contains(err.Error(), "execution role you provide must allow AWS EventBridge Scheduler to assume the role") {
		retries++
		_, err = sclient.CreateSchedule(context.TODO(), &scheduler.CreateScheduleInput{
			Name:               aws.String("aerolab-expiries-" + zone),
			Description:        aws.String("Aerolab automated resource expiry system"),
			ScheduleExpression: aws.String("rate(" + strconv.Itoa(intervalMinutes) + " minutes)"),
			State:              stypes.ScheduleStateEnabled,
			ClientToken:        aws.String("aerolab-expiries-" + zone),
			FlexibleTimeWindow: &stypes.FlexibleTimeWindow{
				Mode: stypes.FlexibleTimeWindowModeOff,
			},
			Target: &stypes.Target{
				Arn:     function.FunctionArn,
				RoleArn: schedRole.Role.Arn,
			},
		})
		if err != nil {
			if !strings.Contains(err.Error(), "execution role you provide must allow AWS EventBridge Scheduler to assume the role") {
				return err
			} else {
				if retries > 3 {
					return err
				}
				log.Detail("Scheduler: IAM not ready, waiting for IAM and retrying to create Lambda")
				time.Sleep(5 * time.Second)
			}
		}
	}
	return nil
}

var awsExpiryPolicies = []string{"arn:aws:iam::aws:policy/AmazonEC2FullAccess", "arn:aws:iam::aws:policy/AmazonElasticFileSystemFullAccess", "arn:aws:iam::aws:policy/AWSCloudFormationFullAccess"}

func (s *b) ExpiryRemove(zones ...string) error {
	log := s.log.WithPrefix("ExpiryRemove: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	for _, zone := range zones {
		if !slices.Contains(s.regions, zone) {
			return fmt.Errorf("zone %s is not enabled", zone)
		}
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(zones))
	var reterr error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log := log.WithPrefix("zone=" + zone + " ")
			log.Detail("Start")
			defer log.Detail("End")

			sclient, err := getSchedulerClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			lclient, err := getLambdaClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			iamclient, err := getIamClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}

			_, err = sclient.DeleteSchedule(context.TODO(), &scheduler.DeleteScheduleInput{
				Name: aws.String("aerolab-expiries-" + zone),
			})
			if err != nil && !strings.Contains(err.Error(), "Schedule aerolab-expiries does not exist") {
				reterr = errors.Join(reterr, err)
			}

			_, err = lclient.DeleteFunction(context.TODO(), &lambda.DeleteFunctionInput{
				FunctionName: aws.String("aerolab-expiries-" + zone),
			})
			if err != nil && !strings.Contains(err.Error(), "ResourceNotFoundException: Function not found") {
				reterr = errors.Join(reterr, err)
			}
			for _, npolicy := range awsExpiryPolicies {
				_, err = iamclient.DetachRolePolicy(context.TODO(), &iam.DetachRolePolicyInput{
					PolicyArn: aws.String(npolicy),
					RoleName:  aws.String("aerolab-expiries-lambda-" + zone),
				})
				if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
					reterr = errors.Join(reterr, err)
				}
			}
			_, err = iamclient.DeleteRolePolicy(context.TODO(), &iam.DeleteRolePolicyInput{
				PolicyName: aws.String("aerolab-expiries-lambda-policy-" + zone),
				RoleName:   aws.String("aerolab-expiries-lambda-" + zone),
			})
			if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
				reterr = errors.Join(reterr, err)
			}
			_, err = iamclient.DeleteRole(context.TODO(), &iam.DeleteRoleInput{
				RoleName: aws.String("aerolab-expiries-lambda-" + zone),
			})
			if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
				reterr = errors.Join(reterr, err)
			}

			_, err = iamclient.DeleteRolePolicy(context.TODO(), &iam.DeleteRolePolicyInput{
				PolicyName: aws.String("aerolab-expiries-scheduler-policy-" + zone),
				RoleName:   aws.String("aerolab-expiries-scheduler-" + zone),
			})
			if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
				reterr = errors.Join(reterr, err)
			}
			_, err = iamclient.DeleteRole(context.TODO(), &iam.DeleteRoleInput{
				RoleName: aws.String("aerolab-expiries-scheduler-" + zone),
			})
			if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
				reterr = errors.Join(reterr, err)
			}

		}(zone)
	}
	wg.Wait()
	if reterr != nil {
		return reterr
	}
	return nil
}

func (s *b) ExpiryList() ([]*backends.ExpirySystem, error) {
	log := s.log.WithPrefix("ExpiryList: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var reterr error
	ret := []*backends.ExpirySystem{}
	retLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	wg.Add(len(s.regions))
	for _, zone := range s.regions {
		go func(zone string) {
			defer wg.Done()
			log := log.WithPrefix("zone=" + zone + " ")
			log.Detail("Start")
			defer log.Detail("End")
			success := 0
			found := false
			detail := &ExpiryDetail{}
			esys := &backends.ExpirySystem{
				BackendType: backends.BackendTypeAWS,
				Zone:        zone,
			}
			sclient, err := getSchedulerClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			lclient, err := getLambdaClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			iamclient, err := getIamClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}

			q, err := sclient.GetSchedule(context.TODO(), &scheduler.GetScheduleInput{
				Name: aws.String("aerolab-expiries-" + zone),
			})
			if err == nil {
				found = true
				detail.Schedule = aws.ToString(q.ScheduleExpression)
				detail.SchedulerArn = aws.ToString(q.Arn)
				detail.SchedulerState = string(q.State)
				detail.SchedulerTargetArn = aws.ToString(q.Target.Arn)
				if q.State == stypes.ScheduleStateEnabled {
					success++
				}
				if strings.HasPrefix(detail.Schedule, "rate(") {
					// grab just the number from the string
					re := regexp.MustCompile(`\d+`)
					matches := re.FindStringSubmatch(detail.Schedule)
					if len(matches) > 0 {
						esys.FrequencyMinutes, err = strconv.Atoi(matches[0])
						if err != nil {
							reterr = errors.Join(reterr, err)
						}
						if strings.Contains(detail.Schedule, "hour") {
							esys.FrequencyMinutes = esys.FrequencyMinutes * 60
						} else if strings.Contains(detail.Schedule, "day") {
							esys.FrequencyMinutes = esys.FrequencyMinutes * 24 * 60
						}
					}
				}
			} else {
				log.Detail("Error getting scheduler: %s", err)
			}

			q2, err := lclient.GetFunction(context.TODO(), &lambda.GetFunctionInput{
				FunctionName: aws.String("aerolab-expiries-" + zone),
			})
			if err == nil {
				found = true
				detail.FunctionArn = aws.ToString(q2.Configuration.FunctionArn)
				detail.ExpireEksctl = q2.Configuration.Environment.Variables["AEROLAB_EXPIRE_EKSCTL"] == "true"
				detail.CleanupDNS = q2.Configuration.Environment.Variables["AEROLAB_CLEANUP_DNS"] == "true"
				detail.Environment = q2.Configuration.Environment.Variables
				detail.FunctionState = string(q2.Configuration.State)
				detail.LogLevel, err = strconv.Atoi(detail.Environment["AEROLAB_LOG_LEVEL"])
				if err != nil {
					detail.LogLevel = 4
				}
				esys.Version = q2.Tags[TAG_AEROLAB_VERSION]
				if q2.Configuration.State == ltypes.StateActive {
					success++
				}
			} else {
				log.Detail("Error getting function: %s", err)
			}

			q3, err := iamclient.GetRole(context.TODO(), &iam.GetRoleInput{
				RoleName: aws.String("aerolab-expiries-scheduler-" + zone),
			})
			if err == nil {
				found = true
				detail.IAMScheduler = aws.ToString(q3.Role.Arn)
				success++
			} else {
				log.Detail("Error getting IAM scheduler: %s", err)
			}

			q4, err := iamclient.GetRole(context.TODO(), &iam.GetRoleInput{
				RoleName: aws.String("aerolab-expiries-lambda-" + zone),
			})
			if err == nil {
				found = true
				detail.IAMFunction = aws.ToString(q4.Role.Arn)
				success++
			} else {
				log.Detail("Error getting IAM function: %s", err)
			}

			if success == 4 {
				esys.InstallationSuccess = true
			}
			if found {
				esys.BackendSpecific = detail
				retLock.Lock()
				ret = append(ret, esys)
				retLock.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	if reterr != nil {
		return ret, reterr
	}
	return ret, nil
}

func (s *b) ExpiryChangeFrequency(intervalMinutes int, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeFrequency: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	for _, zone := range zones {
		if !slices.Contains(s.regions, zone) {
			return fmt.Errorf("zone %s is not enabled", zone)
		}
	}

	log.Detail("Getting expiry list")
	esys, err := s.ExpiryList()
	if err != nil {
		return err
	}

	for _, zone := range zones {
		found := false
		for _, esys := range esys {
			if esys.Zone == zone {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("zone %s does not have an expiry system", zone)
		}
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(zones))
	var reterr error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log := log.WithPrefix("zone=" + zone + " ")
			log.Detail("Start")
			defer log.Detail("End")
			var e *backends.ExpirySystem
			for _, esys := range esys {
				if esys.Zone == zone {
					e = esys
					break
				}
			}
			sclient, err := getSchedulerClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			_, err = sclient.UpdateSchedule(context.TODO(), &scheduler.UpdateScheduleInput{
				Name:               aws.String("aerolab-expiries-" + zone),
				ScheduleExpression: aws.String(fmt.Sprintf("rate(%d minutes)", intervalMinutes)),
				State:              stypes.ScheduleStateEnabled,
				FlexibleTimeWindow: &stypes.FlexibleTimeWindow{
					Mode: stypes.FlexibleTimeWindowModeOff,
				},
				Target: &stypes.Target{
					Arn:     aws.String(e.BackendSpecific.(*ExpiryDetail).SchedulerTargetArn),
					RoleArn: aws.String(e.BackendSpecific.(*ExpiryDetail).IAMScheduler),
				},
			})
			if err != nil {
				reterr = errors.Join(reterr, err)
			}
		}(zone)
	}
	wg.Wait()
	if reterr != nil {
		return reterr
	}
	return nil
}

func (s *b) InstancesChangeExpiry(instances backends.InstanceList, expiry time.Time) error {
	log := s.log.WithPrefix("InstancesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return instances.AddTags(map[string]string{TAG_EXPIRES: expiry.Format(time.RFC3339)})
}

func (s *b) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	log := s.log.WithPrefix("VolumesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return volumes.AddTags(map[string]string{TAG_EXPIRES: expiry.Format(time.RFC3339)}, 0)
}
