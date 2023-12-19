package ingest

import (
	_ "embed"
	"encoding/json"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/aerospike/aerospike-client-go/v7"
	"github.com/gabriel-vasile/mimetype"
)

type Ingest struct {
	config       *Config
	patterns     *patterns
	cpuProfile   *os.File
	pprofRunning bool
	progress     *Progress
	db           *aerospike.Client
	wp           *aerospike.WritePolicy
	end          bool
	endLock      *sync.Mutex
}

type lineErrors struct {
	sync.Mutex
	errors  map[int]map[string]int
	changed bool
}

func (l *lineErrors) add(nodeIdent int, err string) {
	l.Lock()
	l.changed = true
	if l.errors == nil {
		l.errors = make(map[int]map[string]int)
	}
	if _, ok := l.errors[nodeIdent]; !ok {
		l.errors[nodeIdent] = make(map[string]int)
		l.errors[nodeIdent][err] = 1
		l.Unlock()
		return
	}
	l.errors[nodeIdent][err]++
	l.Unlock()
}

func (l *lineErrors) isChanged() bool {
	l.Lock()
	c := l.changed
	l.Unlock()
	return c
}

func (l *lineErrors) MarshalJSON() ([]byte, error) {
	l.Lock()
	defer l.Unlock()
	return json.Marshal(l.errors)
}

type Config struct {
	LogLevel  int `yaml:"logLevel" default:"4" envconfig:"LOGINGEST_LOGLEVEL"` // 0=NO_LOGGING 1=CRITICAL, 2=ERROR, 3=WARNING, 4=INFO, 5=DEBUG, 6=DETAIL
	Aerospike struct {
		WaitForSindexes     bool   `yaml:"waitForSindexes" default:"true"`
		Host                string `yaml:"host" default:"127.0.0.1"`
		Port                int    `yaml:"port" default:"3000"`
		Namespace           string `yaml:"namespace" default:"agi"`
		DefaultSetName      string `yaml:"defaultSetName" default:"default"`
		LogFileRagesSetName string `yaml:"logFileRangesSetName" default:"logRanges"`
		TimestampBinName    string `yaml:"timestampBinName" default:"timestamp"`
		TimestampIndexName  string `yaml:"timestampIndexName" default:"timestamp_idx"`
		Timeouts            struct {
			Connect time.Duration `yaml:"connect" default:"10s"`
			Idle    time.Duration `yaml:"idle" default:"0"`
			Socket  time.Duration `yaml:"socket" default:"10s"`
			Total   time.Duration `yaml:"timeout" default:"30s"`
		} `yaml:"timeouts"`
		Retries struct {
			Connect      int           `yaml:"connect" default:"-1"` // set to -1 to retry forever
			ConnectSleep time.Duration `yaml:"connectSleep" default:"1s"`
			Read         int           `yaml:"read" default:"50"`
			Write        int           `yaml:"write" default:"50"`
		} `yaml:"retries"`
		MaxPutThreads int `yaml:"maxPutThreads" default:"128"`
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
		MaxConcurrentLogFiles int `yaml:"maxConcurrentLogFiles" default:"4"`
		LogReadBufferSizeKb   int `yaml:"logReadBufferSizeKb" default:"1024"`
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
	PatternsFile            string        `yaml:"patternsFile"`
	IngestTimeRanges        TimeRanges    `yaml:"ingestTimeRanges"`
	CollectInfoAsadmTimeout time.Duration `yaml:"collectInfoCommandTimeout" default:"150s"`
	CollectInfoMaxSize      int64         `yaml:"collectInfoMaxSize" default:"20971520"` // files over 20MiB will be considered not collectinfo
	CollectInfoSetName      string        `yaml:"collectInfoSetName" default:"collectinfos"`
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
	CustomSourceName           string `yaml:"customSourceName" default:"" envconfig:"LOGINGEST_CUSTOM_SRCNAME"`
	FindClusterNameNodeIdRegex string `yaml:"findClusterNameNodeIdRegex" default:"NODE-ID (?P<NodeId>[^ ]+) CLUSTER-SIZE (?P<ClusterSize>\\d+)( CLUSTER-NAME (?P<ClusterName>[^$]+))*"`
	findClusterNameNodeIdRegex *regexp.Regexp
	CPUProfilingOutputFile     string `yaml:"cpuProfilingOutputFile" envconfig:"LOGINGEST_CPUPROFILE_FILE"`
}

type TimeRanges struct {
	Enabled bool      `yaml:"enabled" envconfig:"LOGINGEST_TIMERANGE_ENABLE" default:"false"`
	From    time.Time `yaml:"from" envconfig:"LOGINGEST_TIMERANGE_FROM"`
	To      time.Time `yaml:"to" envconfig:"LOGINGEST_TIMERANGE_TO"`
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
	GlobalLabels  []string `yaml:"labels"`
	LabelsSetName string   `yaml:"labelsSetName"`
	Patterns      []*struct {
		Name    string `yaml:"setName"`
		Search  string `yaml:"search"`
		Replace []*struct {
			Regex string `yaml:"regex"`
			regex *regexp.Regexp
			Sub   string `yaml:"sub"`
		} `yaml:"replace"`
		Regex         []string `yaml:"export"`
		regex         []*regexp.Regexp
		RegexAdvanced []struct {
			Regex   string `yaml:"regex"`
			regex   *regexp.Regexp
			SetName string `yaml:"setName"`
		} `yaml:"exportAdvanced"`
		StoreNodePrefix     string            `yaml:"storeNodePrefix"`
		Labels              []string          `yaml:"labels"` // used to define which regex matches are labels (to be stuck in metadata)
		DefaultValuePadding map[string]string `yaml:"defaultValuePadding"`
		Histogram           *struct {
			Buckets       []string `yaml:"buckets"`
			GenCumulative bool     `yaml:"generateCumulative"`
		} `yaml:"histogram"`
		Aggregate *struct {
			Every time.Duration `yaml:"every"` // aggregation time window
			Field string        `yaml:"field"` // field to use for aggregation counts
			On    string        `yaml:"on"`    // aggregate on this result value being the unique value
		} `yaml:"aggregate"`
	} `yaml:"patterns"`
}

type Progress struct {
	sync.RWMutex
	Downloader           *ProgressDownloader
	Unpacker             *ProgressUnpacker
	PreProcessor         *ProgressPreProcessor
	LogProcessor         *ProgressLogProcessor
	CollectinfoProcessor *ProgressCollectProcessor
}

type ProgressDownloader struct {
	S3Files    map[string]*DownloaderFile // map[key]*details
	SftpFiles  map[string]*DownloaderFile // map[path]*details
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type ProgressUnpacker struct {
	Files      map[string]*EnumFile // map[path]*details
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type ProgressPreProcessor struct {
	Files                     map[string]*EnumFile // map[path]*details
	CollectInfoUniquePrefixes int
	Finished                  bool
	running                   bool
	wasRunning                bool
	changed                   bool
	LastUsedPrefix            int
	LastUsedSuffixForPrefix   map[int]int
	NodeToPrefix              map[string]int
}

type ProgressLogProcessor struct {
	Files      map[string]*LogFile
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
	StartTime  time.Time
	LineErrors *lineErrors
}

type LogFile struct {
	ClusterName string
	NodePrefix  string
	NodeID      string
	NodeSuffix  string
	Size        int64
	Processed   int64
	Finished    bool
}

type ProgressCollectProcessor struct {
	Files      map[string]*CfFile
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type CfFile struct {
	Size                int64
	NodeID              string
	RenameAttempted     bool
	Renamed             bool
	OriginalName        string
	ProcessingAttempted bool
	Processed           bool
	Errors              []string
}

type DownloaderFile struct {
	Size         int64
	LastModified time.Time
	IsDownloaded bool
	Error        string
}

type EnumFile struct {
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

type jsonPayload struct {
	JsonPayload struct {
		Log          string `json:"log"`           // type 1: full log line in here
		Level        string `json:"level"`         // type 2: INFO
		Module       string `json:"module"`        // type 2: info
		ModuleDetail string `json:"module_detail"` // type 2: ticker.c:497
		Message      string `json:"message"`       // type 2: {test} memory-usage: total-bytes 0 index-bytes 0 sindex-bytes 0 data-bytes 0 used-pct 0.00
	} `json:"jsonPayload"`
	Timestamp   string   `json:"timestamp"`   // type 2: 2021-06-18T10:00:00Z
	TextPayload string   `json:"textPayload"` // type 3
	Resource    struct { // type 3
		Labels struct { // type 3
			PodName string `json:"pod_name"` // type 3
		} `json:"labels"` // type 3
	} `json:"resource"` // type 3
	Log string `json:"log"` // type 4
}

type IngestStatusStruct struct {
	Ingest struct {
		Running                  bool
		CompleteSteps            *IngestSteps
		DownloaderCompletePct    int
		DownloaderTotalSize      int64
		DownloaderCompleteSize   int64
		LogProcessorCompletePct  int
		LogProcessorTotalSize    int64
		LogProcessorCompleteSize int64
		Errors                   []string
	}
	AerospikeRunning     bool
	PluginRunning        bool
	GrafanaHelperRunning bool
	System               struct {
		DiskTotalBytes   uint64
		DiskFreeBytes    uint64
		MemoryTotalBytes int
		MemoryFreeBytes  int
	}
}

type IngestSteps struct {
	Init                 bool
	Download             bool
	Unpack               bool
	PreProcess           bool
	ProcessLogs          bool
	ProcessCollectInfo   bool
	CriticalError        string
	DownloadStartTime    time.Time
	DownloadEndTime      time.Time
	ProcessLogsStartTime time.Time
	ProcessLogsEndTime   time.Time
}

type NotifyEvent struct {
	AGIName             string
	Event               string
	EventDetail         string
	IsDataInMemory      bool
	IngestStatus        *IngestStatusStruct
	DeploymentJsonGzB64 string
}
