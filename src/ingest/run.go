package ingest

import (
	"errors"
	"fmt"
	"sync"
)

// --- ENV VARS ---
//os.Setenv("LOGINGEST_CPUPROFILE_FILE", "cpu.pprof")
//os.Setenv("LOGINGEST_LOGLEVEL", "6")
//os.Setenv("LOGINGEST_S3SOURCE_ENABLED", "true")
//os.Setenv("LOGINGEST_SFTPSOURCE_ENABLED", "true")
//os.Setenv("LOGINGEST_S3SOURCE_REGION", "ca-central-1")
//os.Setenv("LOGINGEST_S3SOURCE_BUCKET", "") // set outside
//os.Setenv("LOGINGEST_S3SOURCE_KEYID", "") // set outside
//os.Setenv("LOGINGEST_S3SOURCE_SECRET", "") // set outside
//os.Setenv("LOGINGEST_S3SOURCE_PATH", "logs/")
//os.Setenv("LOGINGEST_S3SOURCE_REGEX", "^.*\\.tgz")
//os.Setenv("LOGINGEST_SFTPSOURCE_HOST", "") // set outside
//os.Setenv("LOGINGEST_SFTPSOURCE_PORT", "22")
//os.Setenv("LOGINGEST_SFTPSOURCE_USER", "") // set outside
//os.Setenv("LOGINGEST_SFTPSOURCE_PASSWORD", "") // set outside
//os.Setenv("LOGINGEST_SFTPSOURCE_PATH", "withhist.tgz")
//os.Setenv("LOGINGEST_SFTPSOURCE_REGEX", "^.*\\.tgz")
//LOGINGEST_TIMERANGE_ENABLE: true
//LOGINGEST_TIMERANGE_FROM: ...
//LOGINGEST_TIMERANGE_TO: ...
//LOGINGEST_S3SOURCE_THREADS: 4
//LOGINGEST_SFTPSOURCE_KEYFILE: /loc.f.pem
//LOGINGEST_SFTPSOURCE_THREADS: 4

func Run() error {
	config, err := MakeConfig(true, "", true)
	if err != nil {
		return fmt.Errorf("MakeConfig: %s", err)
	}
	i, err := Init(config)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	err = i.Download()
	if err != nil {
		return fmt.Errorf("Download: %s", err)
	}
	err = i.Unpack()
	if err != nil {
		return fmt.Errorf("Unpack: %s", err)
	}
	err = i.PreProcess()
	if err != nil {
		return fmt.Errorf("PreProcess: %s", err)
	}
	nerr := []error{}
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := i.ProcessLogs()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("ProcessLogs: %s", err))
		}
	}()
	go func() {
		defer wg.Done()
		err := i.ProcessCollectInfo()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("ProcessCollectInfo: %s", err))
		}
	}()
	wg.Wait()
	i.Close()
	if len(nerr) > 0 {
		errstr := ""
		for _, e := range nerr {
			if errstr != "" {
				errstr += "; "
			}
			errstr = errstr + e.Error()
		}
		return errors.New(errstr)
	}
	return nil
}
