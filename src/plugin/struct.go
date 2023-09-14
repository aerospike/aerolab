package plugin

import (
	"net/http"
	"os"
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
	srv          *http.Server
}

type Config struct {
	Service struct {
		ListenAddress string `yaml:"listenAddress" default:"127.0.0.1" envconfig:"PLUGIN_LISTEN_ADDR"`
		ListenPort    int    `yaml:"listenPort" default:"8851" envconfig:"PLUGIN_LISTEN_PORT"`
	} `yaml:"service"`
	LogLevel  int `yaml:"logLevel" default:"4" envconfig:"PLUGIN_LOGLEVEL"` // 0=NO_LOGGING 1=CRITICAL, 2=ERROR, 3=WARNING, 4=INFO, 5=DEBUG, 6=DETAIL
	Aerospike struct {
		Host             string `yaml:"host" default:"127.0.0.1"`
		Port             int    `yaml:"port" default:"3000"`
		Namespace        string `yaml:"namespace" default:"agi"`
		TimestampBinName string `yaml:"timestampBinName" default:"timestamp"`
		Timeouts         struct {
			Connect     time.Duration `yaml:"connect" default:"60s"`
			Idle        time.Duration `yaml:"idle" default:"60s"`
			RWSocket    time.Duration `yaml:"rwSocket" default:"10s"`
			RWTotal     time.Duration `yaml:"rwTimeout" default:"30s"`
			QuerySocket time.Duration `yaml:"querySocket" default:"30s"`
			QueryTotal  time.Duration `yaml:"queryTimeout" default:"60s"`
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
