package ingest

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"log"

	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/gabriel-vasile/mimetype"
)

func (i *Ingest) ProcessCollectInfo() error {
	i.progress.Lock()
	i.progress.CollectinfoProcessor.Finished = false
	i.progress.CollectinfoProcessor.running = true
	i.progress.CollectinfoProcessor.wasRunning = true
	i.progress.Unlock()
	defer func() {
		i.progress.Lock()
		i.progress.CollectinfoProcessor.running = false
		i.progress.Unlock()
	}()
	// find node prefix->nodeID from log files
	log.Printf("DEBUG: ProcessCollectInfo: enumerating log files for node prefixes")
	foundLogs := make(map[string]map[string]string) //cluster,nodeid,prefix
	err := filepath.Walk(i.config.Directories.Logs, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fn := strings.Split(info.Name(), "_")
		if len(fn) != 3 {
			return nil
		}
		fdir, _ := path.Split(filePath)
		_, fcluster := path.Split(strings.TrimSuffix(fdir, "/"))
		if _, ok := foundLogs[fcluster]; !ok {
			foundLogs[fcluster] = make(map[string]string)
		}
		foundLogs[fcluster][strings.ToLower(fn[1])] = fn[0]
		return nil
	})
	if err != nil {
		return fmt.Errorf("listing collectinfos: %s", err)
	}
	// find files
	log.Printf("DEBUG: ProcessCollectInfo: enumerating files")
	foundFiles := map[string]*CfFile{}
	err = filepath.Walk(i.config.Directories.CollectInfo, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		foundFiles[filePath] = &CfFile{
			Size: info.Size(),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("listing collectinfos: %s", err)
	}

	// merge list
	log.Printf("DEBUG: ProcessCollectInfo: merging lists")
	i.progress.Lock()
	maps.Copy(foundFiles, i.progress.CollectinfoProcessor.Files)
	i.progress.CollectinfoProcessor.Files = make(map[string]*CfFile)
	maps.Copy(i.progress.CollectinfoProcessor.Files, foundFiles)
	i.progress.CollectinfoProcessor.changed = true
	i.progress.Unlock()

	// preload meta entries for collectinfo files
	log.Printf("DEBUG: ProcessCollectInfo: db.Get existing CF filename metadata")
	cfNames := []string{}
	row, err := i.db.Get(i.patterns.LabelsSetName, "cfName", labelsValueCol)
	if err != nil {
		log.Printf("ERROR: CF Processor: could not get CF filename metadata: %s", err)
	} else if row != nil {
		if s, ok := row[labelsValueCol].AsString(); ok {
			metaItem := &metaEntries{}
			if uerr := json.Unmarshal([]byte(s), metaItem); uerr != nil {
				log.Printf("WARN: CF Processor: failed to unmarshal existing cf filename data: %s", uerr)
			}
			cfNames = append(cfNames, metaItem.Entries...)
		}
	}
	// process
	log.Printf("DEBUG: ProcessCollectInfo: processing new files")
	for filePath, cf := range foundFiles {
		if cf.ProcessingAttempted && cf.RenameAttempted {
			log.Printf("DETAIL: ProcessCollectInfo: already attempted, skipping: %s", filePath)
			continue
		}
		log.Printf("DETAIL: ProcessCollectInfo: processing %s", filePath)
		newName, err := i.processCollectInfoFile(filePath, cf, foundLogs)
		if err != nil {
			log.Printf("ERROR: ProcessCollectInfo: Could not process %s: %s", filePath, err)
			i.progress.Lock()
			cf.Errors = append(cf.Errors, err.Error())
			i.progress.Unlock()
		} else {
			fnamex := filePath
			if newName != "" && newName != filePath {
				fnamex = newName
			}
			_, fname := path.Split(fnamex)
			if !slices.Contains(cfNames, fname) {
				cfNames = append(cfNames, fname)
			}
		}
		if newName != "" && newName != filePath {
			i.progress.Lock()
			// Move the persisted progress entry to the new key. The
			// previous code only deleted from the local foundFiles
			// map and left i.progress.CollectinfoProcessor.Files
			// holding both the old and new keys, growing
			// progress.json across every ingest run.
			i.progress.CollectinfoProcessor.Files[newName] = i.progress.CollectinfoProcessor.Files[filePath]
			delete(i.progress.CollectinfoProcessor.Files, filePath)
			delete(foundFiles, filePath)
			i.progress.Unlock()
		}
		log.Printf("DETAIL: ProcessCollectInfo: result (nodeId:%s) (processAttempt:%t processed:%t renameAttempt:%t renamed:%t) (originalName:%s) (name:%s)", cf.NodeID, cf.ProcessingAttempted, cf.Processed, cf.RenameAttempted, cf.Renamed, cf.OriginalName, newName)
	}
	meta := &metaEntries{}
	meta.Entries = append(meta.Entries, cfNames...)
	// store meta entries
	metajson, err := json.Marshal(meta)
	if err != nil {
		log.Printf("ERROR: CF Processor: could not jsonify for metadata: %s", err)
	} else if perr := i.db.Put(i.patterns.LabelsSetName, "cfName", db.Row{labelsValueCol: db.Str(string(metajson))}); perr != nil {
		log.Printf("ERROR: CF Processor: could not store metadata: %s", perr)
	}
	i.progress.Lock()
	i.progress.CollectinfoProcessor.changed = true
	i.progress.CollectinfoProcessor.Finished = true
	i.progress.Unlock()
	log.Printf("DEBUG: ProcessCollectInfo: done")
	return nil
}

type cfContents struct {
	sysinfo     []byte
	confFile    []byte
	health      string
	summary     string
	infoNetJson *cfInfoNetwork
	ipToNode    map[string][]string
	asdBuild    string
}

func (i *Ingest) sendClusterInfo(ct *cfContents) {
	type clusterInfo struct {
		AsdBuild     string `json:"asd-build"`
		S3Source     string `json:"s3-source"`
		SftpSource   string `json:"sftp-source"`
		CustomSource string `json:"custom-source"`
	}
	ci := clusterInfo{
		AsdBuild:     ct.asdBuild,
		CustomSource: i.config.CustomSourceName,
	}
	if i.config.Downloader.S3Source != nil && i.config.Downloader.S3Source.Enabled {
		ci.S3Source = i.config.Downloader.S3Source.BucketName + ":" + i.config.Downloader.S3Source.PathPrefix
	}
	if i.config.Downloader.SftpSource != nil && i.config.Downloader.SftpSource.Enabled {
		ci.SftpSource = i.config.Downloader.SftpSource.Host + ":" + i.config.Downloader.SftpSource.PathPrefix
	}
	json, err := json.Marshal(ci)
	if err != nil {
		log.Printf("ERROR: failed to marshal cluster info: %s", err)
		return
	}
	resp, err := http.Post(i.config.SendClusterInfo, "application/json", bytes.NewReader(json))
	if err != nil {
		log.Printf("ERROR: failed to send cluster info: %s", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("ERROR: failed to send cluster info: %s", resp.Status)
		return
	}
	log.Printf("DETAIL: sent cluster info to %s", i.config.SendClusterInfo)
}

func (i *Ingest) processCollectInfoFile(filePath string, cf *CfFile, logs map[string]map[string]string) (string, error) {
	i.progress.Lock()
	i.progress.CollectinfoProcessor.changed = true
	cf.StartTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
	i.progress.Unlock()
	defer func() {
		i.progress.Lock()
		i.progress.CollectinfoProcessor.changed = true
		cf.FinishTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
		i.progress.Unlock()
	}()

	ct := &cfContents{
		ipToNode: make(map[string][]string),
	}
	log.Printf("DETAIL: processCollectInfoFile: Reading tgz contents of %s", filePath)
	err := i.processCollectInfoFileRead(filePath, cf, ct)
	if err != nil {
		i.progress.Lock()
		cf.ProcessingAttempted = true
		cf.RenameAttempted = true
		i.progress.CollectinfoProcessor.changed = true
		i.progress.Unlock()
		return "", err
	}
	if i.config.SendClusterInfo != "" {
		log.Printf("DETAIL: processCollectInfoFile: sending cluster info to %s", i.config.SendClusterInfo)
		i.sendClusterInfo(ct)
	}
	log.Printf("DETAIL: processCollectInfoFile: processing sysinfo of %s", filePath)
	s := bufio.NewScanner(bytes.NewReader(ct.sysinfo))
	s1 := false
	s2 := false
	ips := []string{}
	for s.Scan() {
		if err = s.Err(); err != nil {
			return "", fmt.Errorf("scanner error: %s", err)
		}
		line := s.Text()
		if strings.HasPrefix(line, "====ASCOLLECTINFO") {
			s1 = true
		} else if s1 && strings.HasPrefix(line, "['hostname") || strings.Trim(line, "\r\n\t ") == "hostname -I" {
			s2 = true
		} else if s1 && s2 {
			ips = strings.Split(strings.Trim(line, "\r\n\t "), " ")
			break
		} else {
			s1 = false
		}
	}
	log.Printf("DETAIL: processCollectInfoFile: found sysinfo IPs:%v of %s", ips, filePath)
	found := false
	newName := ""
	nodeid := ""
	for _, ip := range ips {
		if clusterNodeId, ok := ct.ipToNode[ip]; ok {
			cluster := clusterNodeId[0]
			nodeId := clusterNodeId[1]
			if nnodes, ok := logs[cluster]; ok {
				if prefix, ok := nnodes[strings.ToLower(nodeId)]; ok {
					fdir, ffile := path.Split(filePath)
					ffile = prefix + "_" + ffile
					newName = path.Join(fdir, ffile)
					nodeid = nodeId
					found = true
				}
			}
			if !found && cluster == "null" {
				cluster = "unset"
				if nnodes, ok := logs[cluster]; ok {
					if prefix, ok := nnodes[strings.ToLower(nodeId)]; ok {
						fdir, ffile := path.Split(filePath)
						ffile = prefix + "_" + ffile
						newName = path.Join(fdir, ffile)
						nodeid = nodeId
						found = true
					}
				}
			}
			break
		}
	}
	log.Printf("DETAIL: processCollectInfoFile: handling rename for %s", filePath)
	i.progress.Lock()
	resolvedPath := filePath
	cf.RenameAttempted = true
	cf.NodeID = nodeid
	i.progress.CollectinfoProcessor.changed = true
	if found {
		err = os.Rename(filePath, newName)
		if err != nil {
			log.Printf("ERROR: ProcessCollectInfo: failed to rename %s to %s", filePath, newName)
		} else {
			resolvedPath = newName
			cf.Renamed = true
			cf.OriginalName = filePath
		}
	} else {
		log.Printf("DETAIL: ProcessCollectInfo: nodeID for collectinfo source not found for %s", filePath)
	}
	i.progress.Unlock()
	log.Printf("DETAIL: processCollectInfoFile: starting asadm for %s", resolvedPath)
	err = i.processCollectInfoFileAsadm(resolvedPath, ct, logs)
	if err != nil {
		i.progress.Lock()
		cf.ProcessingAttempted = true
		i.progress.CollectinfoProcessor.changed = true
		i.progress.Unlock()
		return newName, err
	}

	log.Printf("DETAIL: processCollectInfoFile: creating DB entry for %s", resolvedPath)
	_, fname := path.Split(resolvedPath)
	cfRow := db.Row{
		"sysinfo":  db.Str(string(ct.sysinfo)),
		"conffile": db.Str(string(ct.confFile)),
		"health":   db.Str(ct.health),
		"summary":  db.Str(ct.summary),
		"cfName":   db.Str(fname),
	}
	// Pebble has no hard upper bound on row size like Aerospike's 8 MiB
	// limit, but very large blobs still hurt: they bloat the memtable,
	// blow up WAL replays, and stall flushes. Warn so an operator
	// chasing slow ingest has a single grep to find the cause.
	const cfSoftWarnBytes = 16 << 20
	cfTotalBytes := len(ct.sysinfo) + len(ct.confFile) + len(ct.health) + len(ct.summary) + len(fname)
	if cfTotalBytes > cfSoftWarnBytes {
		log.Printf("WARN: ProcessCollectInfo: collectinfo record for %s is %d bytes (> %d soft cap); ingest will continue but flushes may be slow", resolvedPath, cfTotalBytes, cfSoftWarnBytes)
	}
	if err := i.db.Put(i.config.CollectInfoSetName, resolvedPath, cfRow); err != nil {
		log.Printf("DETAIL: ProcessCollectInfo: could not insert record for %s: %s", resolvedPath, err)
		return newName, fmt.Errorf("db.Put: %s", err)
	}
	if ct.infoNetJson != nil && len(ct.infoNetJson.Groups) > 0 {
		for _, record := range ct.infoNetJson.Groups[0].Records {
			infoRow := db.Row{
				"cfName":      db.Str(fname),
				"build":       db.Str(record.Build.Converted),
				"clientConns": db.Str(record.ClientConns.Converted),
				"ip":          db.Str(record.IP.Converted),
				"migrations":  db.Str(record.Migrations.Converted),
				"nodeId":      db.Str(strings.Trim(record.NodeID.Converted, "*")),
				"uptime":      db.Str(record.Uptime.Converted),
				"integrity":   db.Str(record.Cluster.Integrity.Converted),
				"clusterKey":  db.Str(record.Cluster.Key.Converted),
				"principal":   db.Str(record.Cluster.Principal.Converted),
				"clusterSize": db.Str(record.Cluster.Size.Converted),
				"clusterName": db.Str(record.ClusterName),
			}
			pk := fmt.Sprintf("%s::%s::%s", resolvedPath, record.IP.Converted, record.NodeID.Converted)
			if perr := i.db.Put(i.config.CollectInfoSetName, pk, infoRow); perr != nil {
				log.Printf("WARN: Failed to store item in db: %s", perr.Error())
			}
		}
	}
	log.Printf("DETAIL: processCollectInfoFile: done %s", resolvedPath)
	i.progress.Lock()
	i.progress.CollectinfoProcessor.changed = true
	cf.Processed = true
	i.progress.Unlock()
	return newName, nil
}

func (i *Ingest) processCollectInfoFileAsadm(filePath string, ct *cfContents, logs map[string]map[string]string) error {
	for _, comm := range []string{"health", "summary"} {
		log.Printf("DETAIL: processCollectInfoFileAsadm: run file:%s comm:%s", filePath, comm)
		ctx, cancelFunc := context.WithTimeout(context.Background(), i.config.CollectInfoAsadmTimeout)
		out, err := exec.CommandContext(ctx, "asadm", "-cf", filePath, "-e", comm).CombinedOutput()
		nstr := string(out)
		if err != nil {
			nstr = err.Error() + "\n" + nstr
		}
		switch comm {
		case "health":
			ct.health = nstr
		case "summary":
			ct.summary = nstr
		}
		cancelFunc()
		log.Printf("DETAIL: processCollectInfoFileAsadm: finish file:%s comm:%s", filePath, comm)
	}
	// process info network as a json
	infoNet, err := i.processCollectInfoInfoNetwork(filePath, logs)
	ct.infoNetJson = infoNet
	if err != nil {
		log.Printf("WARN: Failed to get 'info network' for %s: %s", filePath, err)
		return nil
	}
	return nil
}

func (i *Ingest) processCollectInfoInfoNetwork(path string, logs map[string]map[string]string) (infoNet *cfInfoNetwork, err error) {
	infoNet = new(cfInfoNetwork)
	log.Printf("DETAIL: processCollectInfoFileAsadm: get info network from %s", path)
	ctx, ctxCancel := context.WithTimeout(context.Background(), i.config.CollectInfoAsadmTimeout)
	defer ctxCancel()
	out, err := exec.CommandContext(ctx, "asadm", "-cf", path, "-e", "info network", "-j").CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return infoNet, fmt.Errorf("failed processing on ctx %s: %s", path, ctxErr)
	}
	if err != nil {
		return infoNet, fmt.Errorf("failed processing %s: %s", path, err)
	}
	jsonString := ""
	jsonStart := false
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.Trim(line, "\n\r")
		if jsonStart {
			jsonString = jsonString + "\n" + line
			continue
		}
		if strings.Trim(line, "\t ") == "{" {
			jsonStart = true
			jsonString = line
		}
	}
	err = json.Unmarshal([]byte(jsonString), infoNet)
	if err != nil {
		return infoNet, fmt.Errorf("failed decompressing %s: %s", path, err)
	}
	log.Printf("DETAIL: processCollectInfoFileAsadm: get info cluster-name from %s", path)
	cname, ccname, err := i.processCollectInfoClusterName(path, logs)
	if err != nil {
		return infoNet, fmt.Errorf("get cluster name %s: %s", path, err)
	}
	if len(infoNet.Groups) > 0 && len(cname.Groups) > 0 {
		for x, n := range infoNet.Groups[0].Records {
			for _, c := range cname.Groups[0].Records {
				if n.IP.Converted == c.IP.Converted {
					infoNet.Groups[0].Records[x].ClusterName = c.ClusterName.Converted
				}
			}
		}
	}
	if ccname != "" {
		for x := range infoNet.Groups[0].Records {
			infoNet.Groups[0].Records[x].ClusterName = ccname
		}
	}
	return infoNet, nil
}

func (i *Ingest) processCollectInfoClusterName(npath string, logs map[string]map[string]string) (infoNet cfClusterName, clusterName string, err error) {
	_, nfile := path.Split(npath)
	if !strings.HasPrefix(nfile, "x") {
		// we can work out cluster name by checking the node cluster instead of running asadm, do that
		pref := strings.Split(nfile, "_")[0]
		for cluster, nodes := range logs {
			for _, prefix := range nodes {
				if prefix == pref {
					// insert cluster name into infoNet
					return infoNet, cluster, nil
				}
			}
		}
	}
	ctx, ctxCancel := context.WithTimeout(context.Background(), i.config.CollectInfoAsadmTimeout)
	defer ctxCancel()
	out, err := exec.CommandContext(ctx, "asadm", "-cf", npath, "-e", "show config like cluster-name", "-j").CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return infoNet, "", fmt.Errorf("failed processing on ctx %s: %s", npath, ctxErr)
	}
	if err != nil {
		return infoNet, "", fmt.Errorf("failed processing %s: %s", npath, err)
	}
	jsonString := ""
	jsonStart := false
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.Trim(line, "\n\r")
		if jsonStart {
			jsonString = jsonString + "\n" + line
			continue
		}
		if strings.Trim(line, "\t ") == "{" {
			jsonStart = true
			jsonString = line
		}
	}
	err = json.Unmarshal([]byte(jsonString), &infoNet)
	if err != nil {
		return infoNet, "", fmt.Errorf("failed decompressing %s: %s", npath, err)
	}
	return infoNet, "", nil
}

func (i *Ingest) processCollectInfoFileRead(filePath string, cf *CfFile, ct *cfContents) error {
	fd, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("could not open file: %s", err)
	}
	defer fd.Close()
	buffer := make([]byte, 4096)
	rdCnt, err := fd.Read(buffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("could not read file: %s", err)
	}
	contentType := mimetype.Detect(buffer[0:rdCnt])
	if !contentType.Is("application/gzip") {
		return errors.New("file not gzip")
	}
	if cf.Size >= i.config.CollectInfoMaxSize {
		return errors.New("file size too large")
	}

	//nolint:errcheck
	fd.Seek(0, 0)
	f, err := gzip.NewReader(fd)
	if err != nil {
		return fmt.Errorf("could not open gzip reader: %s", err)
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		header, err := tr.Next()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return fmt.Errorf("error reading tar archive: %s", err)
		case header == nil:
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if strings.HasSuffix(header.Name, "_sysinfo.log") || strings.HasSuffix(header.Name, "_sysinfo.log.zip") {
			fr, err := getFdFromZip(tr, header.Name, "_sysinfo.log")
			if err != nil {
				return fmt.Errorf("could not read file from tar archive: %s", err)
			}
			defer fr.Close()
			ct.sysinfo, err = io.ReadAll(fr)
			if err != nil {
				return fmt.Errorf("could not read sysinfo file: %s", err)
			}
			continue
		}
		if strings.HasSuffix(header.Name, "_aerospike.conf") || strings.HasSuffix(header.Name, "_aerospike.conf.zip") {
			fr, err := getFdFromZip(tr, header.Name, "_aerospike.conf")
			if err != nil {
				return fmt.Errorf("could not read file from tar archive: %s", err)
			}
			defer fr.Close()
			ct.confFile, err = io.ReadAll(fr)
			if err != nil {
				return fmt.Errorf("could not read sysinfo file: %s", err)
			}
			continue
		}
		if strings.HasSuffix(header.Name, "_ascinfo.json") || strings.HasSuffix(header.Name, "_ascinfo.json.zip") {
			fr, err := getFdFromZip(tr, header.Name, "_ascinfo.json")
			if err != nil {
				return fmt.Errorf("could not read file from tar archive: %s", err)
			}
			defer fr.Close()
			ret := make(map[string]any)
			err = json.NewDecoder(fr).Decode(&ret)
			if err != nil {
				return fmt.Errorf("could not decode ascinfo.json: %s", err)
			}
			for _, inDtVal := range ret {
				if _, ok := inDtVal.(map[string]any); !ok {
					continue
				}
				for clusterName, inClusterNameVal := range inDtVal.(map[string]any) {
					if _, ok := inClusterNameVal.(map[string]any); !ok {
						continue
					}
					for _, inIPVal := range inClusterNameVal.(map[string]any) {
						if _, ok := inIPVal.(map[string]any); !ok {
							continue
						}
						inAsStat := inIPVal.(map[string]any)["as_stat"]
						if _, ok := inAsStat.(map[string]any); !ok {
							continue
						}
						meta := inAsStat.(map[string]any)["meta_data"]
						if _, ok := meta.(map[string]any); !ok {
							continue
						}
						nodeId := meta.(map[string]any)["node_id"]
						if _, ok := nodeId.(string); !ok {
							continue
						}
						ip := meta.(map[string]any)["ip"]
						if _, ok := ip.(string); !ok {
							continue
						}
						asdBuild := meta.(map[string]any)["asd_build"]
						if _, ok := asdBuild.(string); ok {
							ct.asdBuild = asdBuild.(string)
						}
						ct.ipToNode[strings.Split(ip.(string), ":")[0]] = []string{clusterName, nodeId.(string)}
					}
				}
			}
			continue
		}
	}
}

func getFdFromZip(r io.Reader, fileName string, nameSuffix string) (io.ReadCloser, error) {
	if !strings.HasSuffix(fileName, ".zip") {
		return &zipnozip{
			r: r,
		}, nil
	}
	zipContents, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error reading tar contents for zip processor: %s", err)
	}
	zipfd, err := zip.NewReader(bytes.NewReader(zipContents), int64(len(zipContents)))
	if err != nil {
		return nil, fmt.Errorf("error opening zip for reading: %s", err)
	}
	for _, file := range zipfd.File {
		if strings.HasSuffix(file.Name, nameSuffix) {
			zipfdfile, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("error opening zip contents for reading: %s", err)
			}
			return &zipnozip{
				r: zipfdfile,
				c: zipfdfile,
			}, nil
		}
	}
	return nil, errors.New("found zip in collectinfo, but it did not contain the named file")
}

type zipnozip struct {
	r io.Reader
	c io.Closer
}

func (z *zipnozip) Read(b []byte) (n int, err error) {
	return z.r.Read(b)
}

func (z *zipnozip) Close() error {
	if z.c != nil {
		return z.c.Close()
	}
	return nil
}

type cfInfoNetwork struct {
	Groups []struct {
		Records []struct {
			IP struct {
				Converted string `json:"converted"`
			} `json:"IP"`
			NodeID struct {
				Converted string `json:"converted"`
			} `json:"Node ID"`
			Build struct {
				Converted string `json:"converted"`
			} `json:"Build"`
			Migrations struct {
				Converted string `json:"converted"`
			} `json:"Migrations"`
			ClientConns struct {
				Converted string `json:"converted"`
			} `json:"Client Conns"`
			Uptime struct {
				Converted string `json:"converted"`
			} `json:"Uptime"`
			Cluster struct {
				Size struct {
					Converted string `json:"converted"`
				} `json:"Size"`
				Key struct {
					Converted string `json:"converted"`
				} `json:"Key"`
				Integrity struct {
					Converted string `json:"converted"`
				} `json:"Integrity"`
				Principal struct {
					Converted string `json:"converted"`
				} `json:"Principal"`
			} `json:"Cluster"`
			ClusterName string
		} `json:"records"`
	} `json:"groups"`
}

type cfClusterName struct {
	Groups []cfClusterNameGroup `json:"groups"`
}

type cfClusterNameGroup struct {
	Records []cfClusterNameRecord `json:"records"`
}

type cfClusterNameRecord struct {
	ClusterName struct {
		Converted string `json:"converted"`
	} `json:"cluster-name"`
	IP struct {
		Converted string `json:"converted"`
	} `json:"Node"`
}
