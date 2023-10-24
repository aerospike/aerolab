package plugin

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aerospike/aerospike-client-go/v6"
)

type Plugin struct {
	config       *Config
	cpuProfile   *os.File
	pprofRunning bool
	db           *aerospike.Client
	wp           *aerospike.WritePolicy
	rp           *aerospike.BasePolicy
	ip           *aerospike.InfoPolicy
	srv          *http.Server
	cache        struct {
		lock     *sync.RWMutex
		setNames []string
		binNames []string
		metadata map[string]*metaEntries
	}
	requests chan bool
	jobs     chan bool
}

type Config struct {
	Service struct {
		ListenAddress string `yaml:"listenAddress" default:"127.0.0.1" envconfig:"PLUGIN_LISTEN_ADDR"`
		ListenPort    int    `yaml:"listenPort" default:"8851" envconfig:"PLUGIN_LISTEN_PORT"`
	} `yaml:"service"`
	AddNoneToLabels            []string      `yaml:"addNoneToLabels"`
	TimeseriesLegendSeparator  string        `yaml:"timeseriesLegendSeparator" default:" : " envconfig:"PLUGIN_SEPARATOR"`
	TimeseriesDisplayNameFirst bool          `yaml:"timeseriesDisplayNameFirst" default:"false" envconfig:"PLUGIN_DISPLAYNAME_FIRST"` // should the display name come first in the legend
	MaxSeriesPerGraph          int           `yaml:"maxSeriesPerGraph" default:"1000" envconfig:"PLUGIN_MAX_SERIES"`
	MaxDataPointsReceived      int           `yaml:"maxDataPointsReceived" default:"34560000" envconfig:"PLUGIN_MAX_DP_RECV"` // 8640000 is about 1 GiB for concurrent 4 graphs, covering 1000 series in each graph, for a day; default max 4 GiB before reduction
	MaxConcurrentRequests      int           `yaml:"maxConcurrentRequests" default:"4" envconfig:"PLUGIN_MAX_REQUESTS"`
	MaxConcurrentJobs          int           `yaml:"maxConcurrentJobs" default:"4" envconfig:"PLUGIN_MAX_JOBS"`
	CacheRefreshInterval       time.Duration `yaml:"cacheRefreshInterval" default:"30s" envconfig:"PLUGIN_CACHE_REFRESH"`
	LabelsSetName              string        `yaml:"labelsSetName" default:"labels" envconfig:"PLUGIN_LABELS_SETNAME"`
	LogLevel                   int           `yaml:"logLevel" default:"4" envconfig:"PLUGIN_LOGLEVEL"` // 0=NO_LOGGING 1=CRITICAL, 2=ERROR, 3=WARNING, 4=INFO, 5=DEBUG, 6=DETAIL
	Aerospike                  struct {
		Host             string `yaml:"host" default:"127.0.0.1"`
		Port             int    `yaml:"port" default:"3000"`
		Namespace        string `yaml:"namespace" default:"agi"`
		TimestampBinName string `yaml:"timestampBinName" default:"timestamp"`
		Timeouts         struct {
			Connect     time.Duration `yaml:"connect" default:"10s"`
			Idle        time.Duration `yaml:"idle" default:"0"`
			RWSocket    time.Duration `yaml:"rwSocket" default:"10s"`
			RWTotal     time.Duration `yaml:"rwTimeout" default:"30s"`
			QuerySocket time.Duration `yaml:"querySocket" default:"30s"`
			QueryTotal  time.Duration `yaml:"queryTimeout" default:"60s"`
			ScanSocket  time.Duration `yaml:"scanSocket" default:"30s"`
			ScanTotal   time.Duration `yaml:"scanTimeout" default:"60s"`
			InfoTimeout time.Duration `yaml:"infoTimeout" default:"60s"`
		} `yaml:"timeouts"`
		Retries struct {
			Connect      int           `yaml:"connect" default:"-1"` // set to -1 to retry forever
			ConnectSleep time.Duration `yaml:"connectSleep" default:"1s"`
			Read         int           `yaml:"read" default:"50"`
			Write        int           `yaml:"write" default:"50"`
		} `yaml:"retries"`
		Security struct {
			Username         string `yaml:"username" envconfig:"PLUGIN_AEROSPIKE_USER"`
			Password         string `yaml:"password" envconfig:"PLUGIN_AEROSPIKE_PASSWORD"`
			AuthModeExternal bool   `yaml:"authModeExternal"`
		} `yaml:"security"`
		TLS struct {
			CaFile     string `yaml:"caFile"`
			CertFile   string `yaml:"certFile"`
			KeyFile    string `yaml:"keyFile"`
			ServerName string `yaml:"serverName"`
		} `yaml:"tls"`
		ConnectionQueueSize int `yaml:"connectionQueueSize" default:"8192"`
	} `yaml:"aerospike"`
	CPUProfilingOutputFile string `yaml:"cpuProfilingOutputFile" envconfig:"PLUGIN_CPUPROFILE_FILE"`
}
