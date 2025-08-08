module github.com/aerospike/aerolab

go 1.24.0

require (
	cloud.google.com/go/compute v1.37.0
	cloud.google.com/go/functions v1.19.6
	cloud.google.com/go/iam v1.5.2
	cloud.google.com/go/run v1.9.3
	cloud.google.com/go/scheduler v1.11.7
	cloud.google.com/go/serviceusage v1.9.6
	cloud.google.com/go/storage v1.52.0
	github.com/PuerkitoBio/goquery v1.10.3
	github.com/aerospike/aerospike-client-go/v8 v8.2.2
	github.com/aws/aws-lambda-go v1.48.0
	github.com/aws/aws-sdk-go-v2 v1.36.6
	github.com/aws/aws-sdk-go-v2/config v1.29.14
	github.com/aws/aws-sdk-go-v2/credentials v1.17.67
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.59.2
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.212.0
	github.com/aws/aws-sdk-go-v2/service/efs v1.35.3
	github.com/aws/aws-sdk-go-v2/service/eks v1.64.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.41.1
	github.com/aws/aws-sdk-go-v2/service/lambda v1.71.2
	github.com/aws/aws-sdk-go-v2/service/pricing v1.34.3
	github.com/aws/aws-sdk-go-v2/service/route53 v1.51.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.84.1
	github.com/aws/aws-sdk-go-v2/service/scheduler v1.13.3
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.19
	github.com/aws/smithy-go v1.22.4
	github.com/bestmethod/inslice v0.0.0-20210212091431-146fa4d769bf
	github.com/charmbracelet/bubbles v0.21.0
	github.com/charmbracelet/bubbletea v1.3.6
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/charmbracelet/x/term v0.2.1
	github.com/creasty/defaults v1.8.0
	github.com/docker/docker v28.1.1+incompatible
	github.com/docker/go-connections v0.5.0
	github.com/gabriel-vasile/mimetype v1.4.9
	github.com/google/uuid v1.6.0
	github.com/jedib0t/go-pretty/v6 v6.6.8
	github.com/jessevdk/go-flags v1.6.1
	github.com/jroimartin/gocui v0.5.0
	github.com/lithammer/shortuuid v3.0.0+incompatible
	github.com/mattn/go-isatty v0.0.20
	github.com/mitchellh/go-ps v1.0.0
	github.com/nwaples/rardecode v1.1.3
	github.com/opencontainers/image-spec v1.1.1
	github.com/pkg/sftp v1.13.9
	github.com/rglonek/aerospike-config-file-parser v1.0.4
	github.com/rglonek/envconfig v0.0.0-20230911195903-c4c689bf1744
	github.com/rglonek/go-flags v1.6.3
	github.com/rglonek/go-wget v0.0.2
	github.com/rglonek/logger v0.2.4
	github.com/rglonek/sbs v1.0.1
	github.com/stretchr/testify v1.10.0
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8
	golang.org/x/crypto v0.37.0
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0
	golang.org/x/oauth2 v0.29.0
	golang.org/x/sys v0.34.0
	golang.org/x/term v0.31.0
	google.golang.org/api v0.231.0
	google.golang.org/grpc v1.72.0
	google.golang.org/protobuf v1.36.6
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cel.dev/expr v0.23.1 // indirect
	cloud.google.com/go v0.121.0 // indirect
	cloud.google.com/go/auth v0.16.1 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	cloud.google.com/go/longrunning v0.6.7 // indirect
	cloud.google.com/go/monitoring v1.24.2 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.27.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.51.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.51.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.11 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.7.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.1 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc // indirect
	github.com/charmbracelet/x/ansi v0.9.3 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13-0.20250311204145-2c3ea96c31dd // indirect
	github.com/cncf/xds/go v0.0.0-20250326154945-ae57f3c0d45f // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.32.4 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-jose/go-jose/v4 v4.1.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/nsf/termbox-go v1.1.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sahilm/fuzzy v0.1.1 // indirect
	github.com/spiffe/go-spiffe/v2 v2.5.0 // indirect
	github.com/wadey/gocovmerge v0.0.0-20160331181800-b5bfa59ec0ad // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zeebo/errs v1.4.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.35.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.60.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.35.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.32.0 // indirect
	google.golang.org/genproto v0.0.0-20250428153025-10db94c68c34 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250428153025-10db94c68c34 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250428153025-10db94c68c34 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)
