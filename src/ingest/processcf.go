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
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/aerospike/aerospike-client-go/v6"
	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
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
	logger.Debug("ProcessCollectInfo: enumerating log files for node prefixes")
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
		foundLogs[strings.ToLower(fcluster)][strings.ToLower(fn[1])] = fn[0]
		return nil
	})
	if err != nil {
		return fmt.Errorf("listing collectinfos: %s", err)
	}
	// find files
	logger.Debug("ProcessCollectInfo: enumerating files")
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
	logger.Debug("ProcessCollectInfo: merging lists")
	i.progress.Lock()
	for n, f := range i.progress.CollectinfoProcessor.Files {
		foundFiles[n] = f
	}
	i.progress.CollectinfoProcessor.Files = make(map[string]*CfFile)
	for n, f := range foundFiles {
		i.progress.CollectinfoProcessor.Files[n] = f
	}
	i.progress.CollectinfoProcessor.changed = true
	i.progress.Unlock()

	// preload meta entries for collectinfo files
	logger.Debug("ProcessCollectInfo: db.Get existing CF filename metadata")
	cfNames := []string{}
	key, aerr := aerospike.NewKey(i.config.Aerospike.Namespace, i.patterns.LabelsSetName, "cfName")
	if aerr != nil {
		logger.Error("CF Processor: could not create key for metadata fetch: %s", aerr)
	} else {
		rec, err := i.db.Get(nil, key)
		if err != nil {
			logger.Error("CF Processor: could not get CF filename metadata: %s", err)
		} else {
			metaItem := &metaEntries{}
			err := json.Unmarshal([]byte(rec.Bins["cfName"].(string)), &metaItem)
			if err != nil {
				logger.Warn("CF Processor: failed to unmarshal existing cf filename data: %s", err)
			}
			cfNames = append(cfNames, metaItem.Entries...)
		}
	}
	// process
	logger.Debug("ProcessCollectInfo: processing new files")
	for filePath, cf := range foundFiles {
		if cf.ProcessingAttempted && cf.RenameAttempted {
			logger.Detail("ProcessCollectInfo: already attempted, skipping: %s", filePath)
			continue
		}
		logger.Detail("ProcessCollectInfo: processing %s", filePath)
		newName, err := i.processCollectInfoFile(filePath, cf, foundLogs)
		if err != nil {
			logger.Error("ProcessCollectInfo: Could not process %s: %s", filePath, err)
			i.progress.Lock()
			cf.Errors = append(cf.Errors, err.Error())
			i.progress.Unlock()
		} else {
			fnamex := filePath
			if newName != "" && newName != filePath {
				fnamex = newName
			}
			_, fname := path.Split(fnamex)
			if !inslice.HasString(cfNames, fname) {
				cfNames = append(cfNames, fname)
			}
		}
		if newName != "" && newName != filePath {
			i.progress.Lock()
			i.progress.CollectinfoProcessor.Files[newName] = i.progress.CollectinfoProcessor.Files[filePath]
			delete(foundFiles, filePath)
			i.progress.Unlock()
		}
		logger.Detail("ProcessCollectInfo: result (nodeId:%s) (processAttempt:%t processed:%t renameAttempt:%t renamed:%t) (originalName:%s) (name:%s)", cf.NodeID, cf.ProcessingAttempted, cf.Processed, cf.RenameAttempted, cf.Renamed, cf.OriginalName, newName)
	}
	meta := &metaEntries{}
	meta.Entries = append(meta.Entries, cfNames...)
	// store meta entries
	metajson, err := json.Marshal(meta)
	if err != nil {
		logger.Error("CF Processor: could not jsonify for metadata: %s", err)
	} else {
		key, aerr := aerospike.NewKey(i.config.Aerospike.Namespace, i.patterns.LabelsSetName, "cfName")
		if aerr != nil {
			logger.Error("CF Processor: could not create key for metadata: %s", aerr)
		} else {
			bin := aerospike.NewBin("cfName", string(metajson))
			aerr = i.db.PutBins(i.wp, key, bin)
			if aerr != nil {
				logger.Error("CF Processor: could not store metadata: %s", aerr)
			}
		}
	}
	i.progress.Lock()
	i.progress.CollectinfoProcessor.changed = true
	i.progress.CollectinfoProcessor.Finished = true
	i.progress.Unlock()
	logger.Debug("ProcessCollectInfo: done")
	return nil
}

type cfContents struct {
	sysinfo     []byte
	confFile    []byte
	health      string
	summary     string
	infoNetJson *cfInfoNetwork
	ipToNode    map[string][]string
}

func (i *Ingest) processCollectInfoFile(filePath string, cf *CfFile, logs map[string]map[string]string) (string, error) {
	ct := &cfContents{
		ipToNode: make(map[string][]string),
	}
	logger.Detail("processCollectInfoFile: Reading tgz contents of %s", filePath)
	err := i.processCollectInfoFileRead(filePath, cf, ct)
	if err != nil {
		i.progress.Lock()
		cf.ProcessingAttempted = true
		cf.RenameAttempted = true
		i.progress.CollectinfoProcessor.changed = true
		i.progress.Unlock()
		return "", err
	}
	logger.Detail("processCollectInfoFile: processing sysinfo of %s", filePath)
	s := bufio.NewScanner(bytes.NewReader(ct.sysinfo))
	s1 := false
	s2 := false
	ips := []string{}
	for s.Scan() {
		if err = s.Err(); err != nil {
			return "", fmt.Errorf("scanner error: %s", err)
		}
		line := s.Text()
		if strings.HasPrefix(line, "===") {
			s1 = true
		} else if s1 && strings.HasPrefix(line, "['hostname") {
			s2 = true
		} else if s1 && s2 {
			ips = strings.Split(strings.Trim(line, "\r\n\t "), " ")
			break
		} else {
			s1 = false
		}
	}
	logger.Detail("processCollectInfoFile: found sysinfo IPs:%v of %s", ips, filePath)
	found := false
	newName := ""
	nodeid := ""
	for _, ip := range ips {
		if clusterNodeId, ok := ct.ipToNode[ip]; ok {
			cluster := clusterNodeId[0]
			nodeId := clusterNodeId[1]
			if nnodes, ok := logs[strings.ToLower(cluster)]; ok {
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
				if nnodes, ok := logs[strings.ToLower(cluster)]; ok {
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
	logger.Detail("processCollectInfoFile: handling rename for %s", filePath)
	i.progress.Lock()
	new := filePath
	cf.RenameAttempted = true
	cf.NodeID = nodeid
	i.progress.CollectinfoProcessor.changed = true
	if found {
		err = os.Rename(filePath, newName)
		if err != nil {
			logger.Error("ProcessCollectInfo: failed to rename %s to %s", filePath, newName)
		} else {
			new = newName
			cf.Renamed = true
			cf.OriginalName = filePath
		}
	} else {
		logger.Detail("ProcessCollectInfo: nodeID for collectinfo source not found for %s", filePath)
	}
	i.progress.Unlock()
	logger.Detail("processCollectInfoFile: starting asadm for %s", new)
	err = i.processCollectInfoFileAsadm(new, ct, logs)
	if err != nil {
		i.progress.Lock()
		cf.ProcessingAttempted = true
		i.progress.CollectinfoProcessor.changed = true
		i.progress.Unlock()
		return newName, err
	}

	logger.Detail("processCollectInfoFile: creating DB entry for %s", new)
	_, fname := path.Split(new)
	key, err := aerospike.NewKey(i.config.Aerospike.Namespace, i.config.CollectInfoSetName, new)
	if err != nil {
		return newName, fmt.Errorf("aerospike.NewKey: %s", err)
	}
	binSysinfo := aerospike.NewBin("sysinfo", string(ct.sysinfo))
	binConfFile := aerospike.NewBin("conffile", string(ct.confFile))
	binHealth := aerospike.NewBin("health", ct.health)
	binSummary := aerospike.NewBin("summary", ct.summary)
	binFname := aerospike.NewBin("cfName", fname)
	err = i.db.PutBins(i.wp, key, binSysinfo, binConfFile, binHealth, binSummary, binFname)
	if err != nil {
		logger.Detail("ProcessCollectInfo: could not insert record for %s: %s", new, err)
		return newName, fmt.Errorf("aerospike.PutBins: %s", err)
	}
	if len(ct.infoNetJson.Groups) > 0 {
		for _, record := range ct.infoNetJson.Groups[0].Records {
			bins := make(map[string]interface{})
			bins["cfName"] = fname
			bins["build"] = record.Build.Converted
			bins["clientConns"] = record.ClientConns.Converted
			bins["ip"] = record.IP.Converted
			bins["migrations"] = record.Migrations.Converted
			bins["nodeId"] = strings.Trim(record.NodeID.Converted, "*")
			bins["uptime"] = record.Uptime.Converted
			bins["integrity"] = record.Cluster.Integrity.Converted
			bins["clusterKey"] = record.Cluster.Key.Converted
			bins["principal"] = record.Cluster.Principal.Converted
			bins["clusterSize"] = record.Cluster.Size.Converted
			bins["clusterName"] = record.ClusterName
			pk, _ := aerospike.NewKey(i.config.Aerospike.Namespace, i.config.CollectInfoSetName, fmt.Sprintf("%s::%s::%s", new, record.IP.Converted, record.NodeID.Converted))
			aerr := i.db.Put(i.wp, pk, bins)
			if aerr != nil {
				logger.Warn("Failed to store item in aerospike: %s", aerr.Error())
			}
		}
	}
	logger.Detail("processCollectInfoFile: done %s", new)
	i.progress.Lock()
	i.progress.CollectinfoProcessor.changed = true
	cf.Processed = true
	i.progress.Unlock()
	return newName, nil
}

func (i *Ingest) processCollectInfoFileAsadm(filePath string, ct *cfContents, logs map[string]map[string]string) error {
	for _, comm := range []string{"health", "summary"} {
		logger.Detail("processCollectInfoFileAsadm: run file:%s comm:%s", filePath, comm)
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
		logger.Detail("processCollectInfoFileAsadm: finish file:%s comm:%s", filePath, comm)
	}
	// process info network as a json
	infoNet, err := i.processCollectInfoInfoNetwork(filePath, logs)
	if err != nil {
		logger.Warn("Failed to get 'info network' for %s: %s", filePath, err)
		return nil
	}
	ct.infoNetJson = infoNet
	return nil
}

func (i *Ingest) processCollectInfoInfoNetwork(path string, logs map[string]map[string]string) (infoNet *cfInfoNetwork, err error) {
	infoNet = new(cfInfoNetwork)
	logger.Detail("processCollectInfoFileAsadm: get info network from %s", path)
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
	for _, line := range strings.Split(string(out), "\n") {
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
	logger.Detail("processCollectInfoFileAsadm: get info cluster-name from %s", path)
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
	for _, line := range strings.Split(string(out), "\n") {
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
	_, err = fd.Read(buffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("could not read file: %s", err)
	}
	contentType := mimetype.Detect(buffer)
	if !contentType.Is("application/gzip") {
		return errors.New("file not gzip")
	}
	if cf.Size >= i.config.CollectInfoMaxSize {
		return errors.New("file size too large")
	}

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
			ret := make(map[string]interface{})
			err = json.NewDecoder(fr).Decode(&ret)
			if err != nil {
				return fmt.Errorf("could not decode ascinfo.json: %s", err)
			}
			for _, inDtVal := range ret {
				if _, ok := inDtVal.(map[string]interface{}); !ok {
					continue
				}
				for clusterName, inClusterNameVal := range inDtVal.(map[string]interface{}) {
					if _, ok := inClusterNameVal.(map[string]interface{}); !ok {
						continue
					}
					for _, inIPVal := range inClusterNameVal.(map[string]interface{}) {
						if _, ok := inIPVal.(map[string]interface{}); !ok {
							continue
						}
						inAsStat := inIPVal.(map[string]interface{})["as_stat"]
						if _, ok := inAsStat.(map[string]interface{}); !ok {
							continue
						}
						meta := inAsStat.(map[string]interface{})["meta_data"]
						if _, ok := meta.(map[string]interface{}); !ok {
							continue
						}
						nodeId := meta.(map[string]interface{})["node_id"]
						if _, ok := nodeId.(string); !ok {
							continue
						}
						ip := meta.(map[string]interface{})["ip"]
						if _, ok := ip.(string); !ok {
							continue
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
