package ingest

import (
	_ "embed"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/aerospike/aerospike-client-go/v6"
	"github.com/gabriel-vasile/mimetype"
)

type Ingest struct {
	config       *Config
	patterns     *patterns
	cpuProfile   *os.File
	pprofRunning bool
	progress     *progress
	db           *aerospike.Client
	wp           *aerospike.WritePolicy
}
type Config struct {
	LogLevel  int `yaml:"logLevel" default:"4" envconfig:"LOGINGEST_LOGLEVEL"` // 0=NO_LOGGING 1=CRITICAL, 2=ERROR, 3=WARNING, 4=INFO, 5=DEBUG, 6=DETAIL
	Aerospike struct {
		Host               string `yaml:"host" default:"127.0.0.1"`
		Port               int    `yaml:"port" default:"3100"`
		Namespace          string `yaml:"namespace" default:"agi"`
		DefaultSetName     string `yaml:"defaultSetName" default:"default"`
		TimestampBinName   string `yaml:"timestampBinName" default:"timestamp"`
		TimestampIndexName string `yaml:"timestampIndexName" default:"timestamp_idx"`
		Timeouts           struct {
			Connect time.Duration `yaml:"connect" default:"60s"`
			Idle    time.Duration `yaml:"idle" default:"60s"`
			Socket  time.Duration `yaml:"socket" default:"10s"`
			Total   time.Duration `yaml:"timeout" default:"30s"`
		} `yaml:"timeouts"`
		Retries struct {
			Connect      int           `yaml:"connect" default:"-1"` // set to -1 to retry forever
			ConnectSleep time.Duration `yaml:"connectSleep" default:"1s"`
			Read         int           `yaml:"read" default:"50"`
			Write        int           `yaml:"write" default:"50"`
		} `yaml:"retries"`
		MaxPutThreads int `yaml:"maxPutThreads" default:"1024"`
		Security      struct {
			Username         string `yaml:"username" envconfig:"LOGINGEST_AEROSPIKE_USER"`
			Password         string `yaml:"password" envconfig:"LOGINGEST_AEROSPIKE_PASSWORD"`
			AuthModeExternal bool   `yaml:"authModeExternal"`
		} `yaml:"security"`
		TLS struct {
			CaFile     string `yaml:"caFile"`
			CertFile   string `yaml:"certFile"`
			KeyFile    string `yaml:"keyFile"`
			ServerName string `yaml:"serverName"`
		} `yaml:"tls"`
	} `yaml:"aerospike"`
	Dedup struct {
		Enabled   bool `yaml:"enabled" default:"true"`
		ReadBytes int  `yaml:"readBytesCount" default:"1048576"`
	} `yaml:"dedup"`
	Processor struct {
		MaxConcurrentFiles  int `yaml:"maxConcurrentFiles" default:"4"`
		LogReadBufferSizeKb int `yaml:"logReadBufferSizeKb" default:"1024"`
	} `yaml:"processor"`
	PreProcess struct {
		FileThreads         int `yaml:"fileThreads" default:"6"`
		UnpackerFileThreads int `yaml:"unpackerFileThreads" default:"4"`
	} `yaml:"preProcessor"`
	ProgressFile struct {
		DisableWrite   bool          `yaml:"disableWrite" default:"false"`
		OutputFilePath string        `yaml:"outputFilePath" default:"ingest/progress/"`
		WriteInterval  time.Duration `yaml:"writeInterval" default:"10s"`
		Compress       bool          `yaml:"compress" default:"true"`
	} `yaml:"progressFile"`
	ProgressPrint struct {
		Enable               bool          `yaml:"enable" default:"true"`
		UpdateInterval       time.Duration `yaml:"updateInterval" default:"10s"`
		PrintOverallProgress bool          `yaml:"printOverallProgress" default:"true"`
		PrintDetailProgress  bool          `yaml:"printDetailProgress" default:"true"`
	} `yaml:"progressPrint"`
	PatternsFile     string `yaml:"patternsFile"`
	IngestTimeRanges struct {
		Enabled bool      `yaml:"enabled" envconfig:"LOGINGEST_TIMERANGE_ENABLE" default:"false"`
		From    time.Time `yaml:"from" envconfig:"LOGINGEST_TIMERANGE_FROM"`
		To      time.Time `yaml:"to" envconfig:"LOGINGEST_TIMERANGE_TO"`
	} `yaml:"ingestTimeRanges"`
	CollectInfoAsadmTimeout time.Duration `yaml:"collectInfoCommandTimeout" default:"150s"`
	Directories             struct {
		CollectInfo string `yaml:"collectInfo" default:"ingest/files/collectinfo"`
		Logs        string `yaml:"logs" default:"ingest/files/logs"`
		DirtyTmp    string `yaml:"dirtyTemp" default:"ingest/files/input"`
		NoStatLogs  string `yaml:"noStatOut" default:"ingest/files/logs-cut"`
		OtherFiles  string `yaml:"otherFiles" default:"ingest/files/other"`
	} `yaml:"directories"`
	Downloader struct {
		ConcurrentSources bool        `yaml:"concurrentSources" default:"true"`
		S3Source          *S3Source   `yaml:"s3Source"`
		SftpSource        *SftpSource `yaml:"sftpSource"`
	} `yaml:"downloader"`
	FindClusterNameNodeIdRegex string `yaml:"findClusterNameNodeIdRegex" default:"NODE-ID (?P<NodeId>[^ ]+) CLUSTER-SIZE (?P<ClusterSize>\\d+)( CLUSTER-NAME (?P<ClusterName>[^$]+))*"`
	findClusterNameNodeIdRegex *regexp.Regexp
	CPUProfilingOutputFile     string `yaml:"cpuProfilingOutputFile" envconfig:"LOGINGEST_CPUPROFILE_FILE"`
}

type S3Source struct {
	Enabled     bool   `yaml:"enabled" envconfig:"LOGINGEST_S3SOURCE_ENABLED"`
	Threads     int    `yaml:"threads" envconfig:"LOGINGEST_S3SOURCE_THREADS" default:"4"`
	Region      string `yaml:"region" envconfig:"LOGINGEST_S3SOURCE_REGION"`
	BucketName  string `yaml:"bucketName" envconfig:"LOGINGEST_S3SOURCE_BUCKET"`
	KeyID       string `yaml:"keyID" envconfig:"LOGINGEST_S3SOURCE_KEYID"`
	SecretKey   string `yaml:"secretKey" envconfig:"LOGINGEST_S3SOURCE_SECRET"`
	PathPrefix  string `yaml:"pathPrefix" envconfig:"LOGINGEST_S3SOURCE_PATH"`
	SearchRegex string `yaml:"searchRegex" envconfig:"LOGINGEST_S3SOURCE_REGEX"`
	searchRegex *regexp.Regexp
}

type SftpSource struct {
	Enabled     bool   `yaml:"enabled" envconfig:"LOGINGEST_SFTPSOURCE_ENABLED"`
	Threads     int    `yaml:"threads" envconfig:"LOGINGEST_SFTPSOURCE_THREADS" default:"4"`
	Host        string `yaml:"host" envconfig:"LOGINGEST_SFTPSOURCE_HOST"`
	Port        int    `yaml:"port" envconfig:"LOGINGEST_SFTPSOURCE_PORT"`
	Username    string `yaml:"username" envconfig:"LOGINGEST_SFTPSOURCE_USER"`
	Password    string `yaml:"password" envconfig:"LOGINGEST_SFTPSOURCE_PASSWORD"`
	KeyFile     string `yaml:"keyFile" envconfig:"LOGINGEST_SFTPSOURCE_KEYFILE"`
	PathPrefix  string `yaml:"pathPrefix" envconfig:"LOGINGEST_SFTPSOURCE_PATH"`
	SearchRegex string `yaml:"searchRegex" envconfig:"LOGINGEST_SFTPSOURCE_REGEX"`
	searchRegex *regexp.Regexp
}

//go:embed patterns.yml
var patternEmbed []byte

type patterns struct {
	Timestamps []*struct {
		Definition string `yaml:"definition"`
		Regex      string `yaml:"regex"`
		regex      *regexp.Regexp
	} `yaml:"timestamps"`
	Multiline []*struct {
		StartLineSearch string `yaml:"startLineSearch"`
		ReMatchLines    string `yaml:"reMatchLines"`
		reMatchLines    *regexp.Regexp
		ReMatchJoin     []struct {
			Re       string `yaml:"re"`
			re       *regexp.Regexp
			MatchSeq int `yaml:"matchSeq"`
		} `yaml:"reMatchJoin"`
	} `yaml:"multilineJoins"`
	LabelAddStaticValue []*struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"labelAddStaticValue"`
	LabelsSetName      string `yaml:"labelsSetName"`
	NodeIdentBinName   string `yaml:"nodeIdentBinName"`
	ClusterNameBinName string `yaml:"clusterNameBinName"`
	LogFileNameBinName string `yaml:"logFileNameBinName"`
	Patterns           []*struct {
		Name    string `yaml:"setName"`
		Search  string `yaml:"search"`
		Replace []*struct {
			Regex string `yaml:"regex"`
			regex *regexp.Regexp
			Sub   string `yaml:"sub"`
		} `yaml:"replace"`
		Regex               []string `yaml:"export"`
		regex               []*regexp.Regexp
		Labels              []string          `yaml:"labels"` // used to define which regex matches are labels (to be stuck in metadata)
		StoreLogFileName    bool              `yaml:"storeLogFileName"`
		DefaultValuePadding map[string]string `yaml:"defaultValuePadding"`
		Histogram           *struct {
			Buckets       []string `yaml:"buckets"`
			GenCumulative bool     `yaml:"generateCumulative"`
		} `yaml:"histogram"`
	} `yaml:"patterns"`
}

type progress struct {
	sync.RWMutex
	Downloader           *progressDownloader
	Unpacker             *progressUnpacker
	PreProcessor         *progressPreProcessor
	LogProcessor         *progressLogProcessor
	CollectinfoProcessor *progressCollectProcessor
}

type progressDownloader struct {
	S3Files    map[string]*downloaderFile // map[key]*details
	SftpFiles  map[string]*downloaderFile // map[path]*details
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type progressUnpacker struct {
	Files      map[string]*enumFile // map[path]*details
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type progressPreProcessor struct {
	Files                     map[string]*enumFile // map[path]*details
	CollectInfoUniquePrefixes int
	Finished                  bool
	running                   bool
	wasRunning                bool
	changed                   bool
	LastUsedPrefix            int
	LastUsedSuffixForPrefix   map[int]int
	NodeToPrefix              map[string]int
}

type progressLogProcessor struct {
	changed bool
}

type progressCollectProcessor struct {
	changed bool
}

type downloaderFile struct {
	Size         int64
	LastModified time.Time
	IsDownloaded bool
	Error        string
}

type enumFile struct {
	Size                  int64
	mimeType              *mimetype.MIME
	ContentType           string
	IsCollectInfo         bool
	IsArchive             bool
	IsText                bool
	IsTarGz               bool
	IsTarBz               bool
	UnpackFailed          bool
	Errors                []string
	PreProcessDuplicateOf []string
	StartAt               int64 // workaround for log files starting at binary 000s
	PreProcessOutPaths    []string
}
