module github.com/aerospike/aerolab

go 1.23.0

toolchain go1.24.0

require (
	cloud.google.com/go/compute v1.29.0
	cloud.google.com/go/storage v1.48.0
	github.com/aerospike/aerospike-client-go v4.5.2+incompatible
	github.com/aerospike/aerospike-client-go/v5 v5.11.0
	github.com/aerospike/aerospike-client-go/v7 v7.7.3
	github.com/aerospike/aerospike-client-go/v8 v8.2.0
	github.com/aws/aws-sdk-go v1.55.5
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
	github.com/jroimartin/gocui v0.5.0
	github.com/lithammer/shortuuid v3.0.0+incompatible
	github.com/mattn/go-isatty v0.0.20
	github.com/mitchellh/go-ps v1.0.0
	github.com/nwaples/rardecode v1.1.3
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/pkg/sftp v1.13.8
	github.com/rglonek/aerospike-config-file-parser v1.0.4
	github.com/rglonek/envconfig v0.0.0-20230911195903-c4c689bf1744
	github.com/rglonek/jeddevdk-goflags v2.0.0+incompatible
	github.com/rglonek/sbs v1.0.1
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8
	golang.org/x/crypto v0.36.0
	golang.org/x/exp v0.0.0-20241204233417-43b7b7cde48d
	golang.org/x/sys v0.31.0
	golang.org/x/term v0.30.0
	golang.org/x/text v0.23.0
	google.golang.org/api v0.210.0
	google.golang.org/protobuf v1.35.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cel.dev/expr v0.19.1 // indirect
	cloud.google.com/go v0.116.0 // indirect
	cloud.google.com/go/auth v0.12.1 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.6 // indirect
	cloud.google.com/go/compute/metadata v0.5.2 // indirect
	cloud.google.com/go/iam v1.3.0 // indirect
	cloud.google.com/go/monitoring v1.22.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.25.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.49.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.49.0 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20240905190251-b4127c9b8d78 // indirect
	github.com/envoyproxy/go-control-plane v0.13.1 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/googleapis/gax-go/v2 v2.14.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/nsf/termbox-go v1.1.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/wadey/gocovmerge v0.0.0-20160331181800-b5bfa59ec0ad // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.32.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.57.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.57.0 // indirect
	go.opentelemetry.io/otel v1.32.0 // indirect
	go.opentelemetry.io/otel/metric v1.32.0 // indirect
	go.opentelemetry.io/otel/sdk v1.32.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.32.0 // indirect
	go.opentelemetry.io/otel/trace v1.32.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/oauth2 v0.28.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	golang.org/x/tools v0.28.0 // indirect
	google.golang.org/genproto v0.0.0-20241209162323-e6fa225c2576 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20241209162323-e6fa225c2576 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241209162323-e6fa225c2576 // indirect
	google.golang.org/grpc v1.68.1 // indirect
	google.golang.org/grpc/stats/opentelemetry v0.0.0-20241028142157-ada6787961b3 // indirect
)
