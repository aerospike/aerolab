package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/bestmethod/inslice"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

func (d *backendGcp) getBucketName(projectID string) string {
	projID := fmt.Sprintf("%x", sha1.Sum([]byte(projectID)))
	return "aerolab-" + projID
}

func (d *backendGcp) isExpiryInstalled(projectID string) (installed bool, region string, err error) {
	//bucket
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return false, "", fmt.Errorf("storage.NewClient: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	var buckets []string
	it := client.Buckets(ctx, projectID)
	for {
		battrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false, "", err
		}
		buckets = append(buckets, battrs.Name)
	}
	if !inslice.HasString(buckets, d.getBucketName(projectID)) {
		return false, "", nil
	}

	// expiry detail json
	type expiryDetail struct {
		Version        int
		Region         string
		InstallSuccess bool
	}

	// object
	ctxo, cancelo := context.WithTimeout(context.Background(), time.Second*50)
	defer cancelo()

	rc, err := client.Bucket(d.getBucketName(projectID)).Object("expiry-system.json").NewReader(ctxo)
	if err != nil {
		return false, "", nil
	}
	defer rc.Close()

	exp := &expiryDetail{}
	err = json.NewDecoder(rc).Decode(&exp)
	if err != nil {
		return false, "", fmt.Errorf("json: %w", err)
	}
	if !exp.InstallSuccess {
		return false, exp.Region, nil
	}
	return exp.Version >= gcpExpiryVersion, exp.Region, nil
}

func (d *backendGcp) storeExpiryRemoved() error {
	//bucket
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %w", err)
	}
	defer client.Close()
	ctxo, cancelo := context.WithTimeout(context.Background(), time.Second*50)
	defer cancelo()

	return client.Bucket(d.getBucketName(a.opts.Config.Backend.Project)).Object("expiry-system.json").Delete(ctxo)
}

func (d *backendGcp) storeExpiryInstalled(projectID string, region string, isSuccess bool) error {
	//bucket
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	var buckets []string
	it := client.Buckets(ctx, projectID)
	for {
		battrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		buckets = append(buckets, battrs.Name)
	}
	if !inslice.HasString(buckets, d.getBucketName(projectID)) {
		ctxb, cancelb := context.WithTimeout(context.Background(), time.Second*30)
		defer cancelb()
		storageClassAndLocation := &storage.BucketAttrs{
			StorageClass: "STANDARD",
			Location:     region,
		}
		bucket := client.Bucket(d.getBucketName(projectID))
		if err := bucket.Create(ctxb, projectID, storageClassAndLocation); err != nil {
			return fmt.Errorf("Bucket(%q).Create: %w", d.getBucketName(projectID), err)
		}
	}

	// expiry detail json
	type expiryDetail struct {
		Version        int
		Region         string
		InstallSuccess bool
	}

	// object
	ctxo, cancelo := context.WithTimeout(context.Background(), time.Second*50)
	defer cancelo()

	wc := client.Bucket(d.getBucketName(projectID)).Object("expiry-system.json").NewWriter(ctxo)
	wc.ChunkSize = 0

	exp := &expiryDetail{
		Version:        gcpExpiryVersion,
		Region:         region,
		InstallSuccess: isSuccess,
	}
	err = json.NewEncoder(wc).Encode(exp)
	if err != nil {
		return fmt.Errorf("json: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("Writer.Close: %w", err)
	}
	return nil
}

func (d *backendGcp) expiriesSystemInstall(intervalMinutes int, deployRegion string, wg *sync.WaitGroup) {
	defer wg.Done()
	err := d.ExpiriesSystemInstall(intervalMinutes, deployRegion, "")
	if err != nil && err.Error() != "EXISTS" {
		log.Printf("WARNING: Failed to install the expiry system, clusters will not expire: %s", err)
	}
}

func (d *backendGcp) ExpiriesSystemInstall(intervalMinutes int, deployRegion string, awsDnsZoneId string) error {
	if d.disableExpiryInstall {
		return nil
	}
	if deployRegion == "" {
		return errors.New("to install the expriries system, region must be specified")
	}
	if len(strings.Split(deployRegion, "-")) != 2 {
		return errors.New("wrong region format, example: us-central1")
	}
	rd, err := a.aerolabRootDir()
	if err != nil {
		return fmt.Errorf("error getting aerolab home dir: %s", err)
	}
	log.Println("Expiries: checking if job exists already")
	isInstalled, bucketRegion, err := d.isExpiryInstalled(a.opts.Config.Backend.Project)
	if err != nil {
		log.Printf("Expiry: WARN: could not access bucket: %s", err)
	} else if isInstalled {
		log.Println("Expiries: done")
		return errors.New("EXISTS")
	}
	prevRegion := ""
	if lastRegion, err := os.ReadFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project)); err == nil {
		prevRegion = string(lastRegion)
		if out, err := exec.Command("gcloud", "scheduler", "jobs", "describe", "aerolab-expiries", "--location", string(lastRegion), "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err == nil {
			outx := strings.Split(string(out), "\n")
			for _, line := range outx {
				if !strings.HasPrefix(line, "description:") {
					continue
				}
				linex := strings.Split(line, ":")
				if len(linex) == 2 {
					ever, err := strconv.Atoi(strings.Trim(linex[1], " \t\r\n'"))
					if err == nil && ever >= gcpExpiryVersion {
						d.storeExpiryInstalled(a.opts.Config.Backend.Project, deployRegion, true)
						log.Println("Expiries: done")
						return errors.New("EXISTS")
					}
					break
				}
			}
		}
	}
	if prevRegion != deployRegion {
		if out, err := exec.Command("gcloud", "scheduler", "jobs", "describe", "aerolab-expiries", "--location", string(deployRegion), "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err == nil {
			outx := strings.Split(string(out), "\n")
			for _, line := range outx {
				if !strings.HasPrefix(line, "description:") {
					continue
				}
				linex := strings.Split(line, ":")
				if len(linex) == 2 {
					ever, err := strconv.Atoi(strings.Trim(linex[1], " \t\r\n'"))
					if err == nil && ever >= gcpExpiryVersion {
						d.storeExpiryInstalled(a.opts.Config.Backend.Project, deployRegion, true)
						log.Println("Expiries: done")
						return errors.New("EXISTS")
					}
					break
				}
			}
		}
	}

	log.Println("Expiries: enabling required services")
	err = d.enableServices(true)
	if err != nil {
		log.Printf("Expiries: WARNING: Some services failed to enable, expiry system installation could fail: %s", err)
	}
	log.Println("Expiries: cleaning up old jobs")
	if prevRegion != "" {
		d.expiriesSystemRemove(false, prevRegion) // cleanup
	}
	if prevRegion != deployRegion {
		d.expiriesSystemRemove(false, deployRegion)
	}
	if bucketRegion != "" && bucketRegion != prevRegion && bucketRegion != deployRegion {
		d.expiriesSystemRemove(false, bucketRegion)
	}

	err = os.WriteFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project), []byte(deployRegion), 0600)
	if err != nil {
		return fmt.Errorf("could not note the region where scheduler got deployed: %s", err)
	}
	d.storeExpiryInstalled(a.opts.Config.Backend.Project, deployRegion, false)

	log.Println("Expiries: preparing commands")
	cron := "*/" + strconv.Itoa(intervalMinutes) + " * * * *"
	if intervalMinutes >= 60 {
		if intervalMinutes%60 != 0 || intervalMinutes > 1440 {
			return errors.New("frequency can be 0-60 in 1-minute increments, or 60-1440 at 60-minute increments")
		}
		if intervalMinutes == 1440 {
			cron = "0 1 * * *"
		} else {
			if intervalMinutes == 60 {
				cron = "0 * * * *"
			} else {
				cron = "0 */" + strconv.Itoa(intervalMinutes/60) + " * * *"
			}
		}
	}

	tmpDirPath, err := os.MkdirTemp("", "aerolabexpiries")
	if err != nil {
		return fmt.Errorf("mkdir-temp: %s", err)
	}
	defer os.RemoveAll(tmpDirPath)
	err = os.WriteFile(path.Join(tmpDirPath, "go.mod"), expiriesCodeGcpMod, 0644)
	if err != nil {
		return fmt.Errorf("write go.mod: %s", err)
	}
	err = os.WriteFile(path.Join(tmpDirPath, "function.go"), expiriesCodeGcpFunction, 0644)
	if err != nil {
		return fmt.Errorf("write function.go: %s", err)
	}
	token := uuid.New().String()

	log.Println("Expiries: running gcloud functions deploy ...")
	deploy := []string{"functions", "deploy", "aerolab-expiries"}
	deploy = append(deploy, "--region="+deployRegion)
	deploy = append(deploy, "--allow-unauthenticated")
	deploy = append(deploy, "--entry-point=AerolabExpire")
	deploy = append(deploy, "--gen2")
	deploy = append(deploy, "--runtime=go120")
	deploy = append(deploy, "--serve-all-traffic-latest-revision")
	deploy = append(deploy, "--source="+tmpDirPath)
	deploy = append(deploy, "--memory=256M")
	deploy = append(deploy, "--timeout=60s")
	deploy = append(deploy, "--trigger-location=deployRegion")
	deploy = append(deploy, "--set-env-vars=TOKEN="+token)
	deploy = append(deploy, "--max-instances=2")
	deploy = append(deploy, "--min-instances=0")
	deploy = append(deploy, "--trigger-http")
	deploy = append(deploy, "--project="+a.opts.Config.Backend.Project)
	//--stage-bucket=STAGE_BUCKET
	out, err := exec.Command("gcloud", deploy...).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		return fmt.Errorf("run gcloud functions deploy: %s", err)
	}

	log.Println("Expiries: running gcloud scheduler create ...")
	sched := []string{"scheduler", "jobs", "create", "http", "aerolab-expiries"}
	sched = append(sched, "--location="+deployRegion)
	sched = append(sched, "--schedule="+cron)
	uri := "https://" + deployRegion + "-" + a.opts.Config.Backend.Project + ".cloudfunctions.net/aerolab-expiries"
	sched = append(sched, "--uri="+uri)
	sched = append(sched, "--description="+strconv.Itoa(gcpExpiryVersion))
	sched = append(sched, "--max-backoff=15s")
	sched = append(sched, "--min-backoff=5s")
	sched = append(sched, "--max-doublings=2")
	sched = append(sched, "--max-retry-attempts=0")
	sched = append(sched, "--time-zone=Etc/UTC")
	mBody := "{\"token\":\"" + token + "\"}"
	sched = append(sched, "--message-body="+mBody)
	sched = append(sched, "--project="+a.opts.Config.Backend.Project)
	out, err = exec.Command("gcloud", sched...).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		return fmt.Errorf("run gcloud scheduler jobs create http: %s", err)
	}

	err = d.storeExpiryInstalled(a.opts.Config.Backend.Project, deployRegion, true)
	if err != nil {
		log.Printf("WARN: could not store expiry system information in GCP bucket: %s", err)
	}
	log.Println("Expiries: done")
	return nil
}

func (d *backendGcp) ExpiriesSystemRemove(region string) error {
	return d.expiriesSystemRemove(true, region)
}

func (d *backendGcp) expiriesSystemRemove(printErr bool, region string) error {
	if d.disableExpiryInstall {
		return nil
	}
	rd, err := a.aerolabRootDir()
	if err != nil {
		return fmt.Errorf("error getting aerolab home dir: %s", err)
	}
	lastRegion := []byte(region)
	if region == "" {
		lastRegion, err = os.ReadFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project))
		if err != nil {
			return fmt.Errorf("could not read job region from %s: %s", path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project), err)
		}
	}
	err = d.storeExpiryInstalled(a.opts.Config.Backend.Project, region, false)
	if err != nil {
		return fmt.Errorf("WARN: cannot store expiry system status, not removing: %s", err)
	}
	var nerr error
	if out, err := exec.Command("gcloud", "scheduler", "jobs", "delete", "aerolab-expiries", "--location", string(lastRegion), "--quiet", "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err != nil {
		if printErr {
			fmt.Println(string(out))
		}
		nerr = errors.New("some deletions failed")
	}
	if out, err := exec.Command("gcloud", "functions", "delete", "aerolab-expiries", "--region", string(lastRegion), "--gen2", "--quiet", "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err != nil {
		if printErr {
			fmt.Println(string(out))
		}
		nerr = errors.New("some deletions failed")
	}
	if nerr == nil {
		d.storeExpiryRemoved()
	}
	return nerr
}

func (d *backendGcp) ExpiriesSystemFrequency(intervalMinutes int) error {
	if d.disableExpiryInstall {
		return nil
	}
	cron := "*/" + strconv.Itoa(intervalMinutes) + " * * * *"
	if intervalMinutes >= 60 {
		if intervalMinutes%60 != 0 || intervalMinutes > 1440 {
			return errors.New("frequency can be 0-60 in 1-minute increments, or 60-1440 at 60-minute increments")
		}
		if intervalMinutes == 1440 {
			cron = "0 1 * * *"
		} else {
			if intervalMinutes == 60 {
				cron = "0 * * * *"
			} else {
				cron = "0 */" + strconv.Itoa(intervalMinutes/60) + " * * *"
			}
		}
	}
	rd, err := a.aerolabRootDir()
	if err != nil {
		return fmt.Errorf("error getting aerolab home dir: %s", err)
	}
	lastRegion, err := os.ReadFile(path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project))
	if err != nil {
		return fmt.Errorf("could not read job region from %s: %s", path.Join(rd, "gcp-expiries.region."+a.opts.Config.Backend.Project), err)
	}
	if out, err := exec.Command("gcloud", "scheduler", "jobs", "update", "http", "aerolab-expiries", "--location", string(lastRegion), "--schedule", cron, "--project="+a.opts.Config.Backend.Project).CombinedOutput(); err != nil {
		fmt.Println(string(out))
		return err
	}
	return nil
}

func (d *backendGcp) EnableServices() error {
	return d.enableServices(false)
}

func (d *backendGcp) enableServices(quiet bool) error {
	gcloudServices := []string{"logging.googleapis.com", "cloudfunctions.googleapis.com", "cloudbuild.googleapis.com", "pubsub.googleapis.com", "cloudscheduler.googleapis.com", "compute.googleapis.com", "run.googleapis.com", "artifactregistry.googleapis.com", "storage.googleapis.com"}
	for _, gs := range gcloudServices {
		if !quiet {
			log.Printf("===== Running: gcloud services enable --project %s %s =====", a.opts.Config.Backend.Project, gs)
		}
		out, err := exec.Command("gcloud", "services", "enable", "--project", a.opts.Config.Backend.Project, gs).CombinedOutput()
		if err != nil {
			log.Printf("ERROR: %s", err)
			log.Println(string(out))
		}
	}
	log.Println("Done")
	return nil
}
