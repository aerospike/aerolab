module github.com/aerospike/aerolab

go 1.24

require (
	cloud.google.com/go/compute v1.36.0
	cloud.google.com/go/storage v1.50.0
	github.com/aerospike/aerospike-client-go v4.5.2+incompatible
	github.com/aerospike/aerospike-client-go/v5 v5.11.0
	github.com/aerospike/aerospike-client-go/v7 v7.7.3
	github.com/aws/aws-lambda-go v1.47.0
	github.com/aws/aws-sdk-go v1.55.5
	github.com/aws/aws-sdk-go-v2 v1.36.1
	github.com/aws/aws-sdk-go-v2/config v1.29.2
	github.com/aws/aws-sdk-go-v2/credentials v1.17.55
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.57.0
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.201.1
	github.com/aws/aws-sdk-go-v2/service/efs v1.34.7
	github.com/aws/aws-sdk-go-v2/service/eks v1.58.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.39.1
	github.com/aws/aws-sdk-go-v2/service/lambda v1.69.13
	github.com/aws/aws-sdk-go-v2/service/pricing v1.32.16
	github.com/aws/aws-sdk-go-v2/service/route53 v1.48.7
	github.com/aws/aws-sdk-go-v2/service/scheduler v1.12.17
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.10
	github.com/bestmethod/inslice v0.0.0-20210212091431-146fa4d769bf
	github.com/bestmethod/logger v0.0.0-20210319152012-2c63bbe98d5a
	github.com/containerd/console v1.0.4
	github.com/creasty/defaults v1.8.0
	github.com/fsnotify/fsnotify v1.8.0
	github.com/gabemarshall/pty v0.0.0-20220927143247-d84f0bb0c17e
	github.com/gabriel-vasile/mimetype v1.4.7
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/jedib0t/go-pretty/v6 v6.6.4
	github.com/jessevdk/go-flags v1.6.1
	github.com/jroimartin/gocui v0.5.0
	github.com/lithammer/shortuuid v3.0.0+incompatible
	github.com/mattn/go-isatty v0.0.20
	github.com/mitchellh/go-ps v1.0.0
	github.com/nwaples/rardecode v1.1.3
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/pkg/sftp v1.13.7
	github.com/rglonek/aerospike-config-file-parser v1.0.4
	github.com/rglonek/envconfig v0.0.0-20230911195903-c4c689bf1744
	github.com/rglonek/jeddevdk-goflags v2.0.0+incompatible
	github.com/rglonek/logger v0.2.2
	github.com/rglonek/sbs v1.0.1
	github.com/stretchr/testify v1.10.0
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8
	golang.org/x/crypto v0.37.0
	golang.org/x/exp v0.0.0-20241204233417-43b7b7cde48d
	golang.org/x/sys v0.32.0
	golang.org/x/term v0.31.0
	golang.org/x/text v0.24.0
	google.golang.org/api v0.229.0
	google.golang.org/protobuf v1.36.6
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cel.dev/expr v0.19.2 // indirect
	cloud.google.com/go v0.120.0 // indirect
	cloud.google.com/go/auth v0.16.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	cloud.google.com/go/iam v1.5.0 // indirect
	cloud.google.com/go/longrunning v0.6.6 // indirect
	cloud.google.com/go/monitoring v1.24.1 // indirect
	cloud.google.com/go/serviceusage v1.9.6 // indirect
	github.com/GoogleCloudPlatform/functions-framework-go v1.9.2 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.25.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.50.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.50.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.9 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.11 // indirect
	github.com/aws/smithy-go v1.22.2 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudevents/sdk-go/v2 v2.15.2 // indirect
	github.com/cncf/xds/go v0.0.0-20250121191232-2f005788dc42 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/envoyproxy/go-control-plane v0.13.4 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.10 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v0.0.0-20180701023420-4b7aa43c6742 // indirect
	github.com/nsf/termbox-go v1.1.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.34.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.60.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.uber.org/atomic v1.4.0 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.10.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/oauth2 v0.29.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	google.golang.org/genproto v0.0.0-20250409194420-de1ac958c67a // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250414145226-207652e42e2e // indirect
	google.golang.org/genproto/googleapis/api/serviceusage v0.0.0-20250414145226-207652e42e2e // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250414145226-207652e42e2e // indirect
	google.golang.org/grpc v1.71.1 // indirect
	google.golang.org/grpc/stats/opentelemetry v0.0.0-20241028142157-ada6787961b3 // indirect
)

// tool github.com/onsi/ginkgo/v2/ginkgo
