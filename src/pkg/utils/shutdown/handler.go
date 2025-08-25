package shutdown

import (
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
)

var waiter = new(sync.WaitGroup)
var isShuttingDown = false
var isShuttingDownMutex sync.Mutex

func IsShuttingDown() bool {
	isShuttingDownMutex.Lock()
	defer isShuttingDownMutex.Unlock()
	return isShuttingDown
}

// add a job to the waiter: should be called from any go routine that needs to be waited for
func AddJob() {
	cleanupJobsMutex.Lock()
	waiter.Add(1)
	cleanupJobsMutex.Unlock()
}

// wait for all jobs to complete: should be called from main() and any os.Exit/log.Fatal calls
func WaitJobs() {
	waitJobs(false)
}

func waitJobs(isSignal bool) {
	isShuttingDownMutex.Lock()
	isShuttingDown = true
	isShuttingDownMutex.Unlock()
	cleanupJobsMutex.Lock() // we never unlock, as new jobs cannot be added now that we are in shutdown
	for i := len(earlyCleanupJobs) - 1; i >= 0; i-- {
		earlyCleanupJobs[i].Func(isSignal)
	}
	waiter.Wait()
	for i := len(lateCleanupJobs) - 1; i >= 0; i-- {
		lateCleanupJobs[i].Func(isSignal)
	}
}

// mark a job as done: should be called from any go routine that needs to be waited for when it's done
func DoneJob() {
	waiter.Done()
}

type CleanupJob struct {
	Name string
	Func func(isSignal bool)
}

var earlyCleanupJobs = []CleanupJob{}
var lateCleanupJobs = []CleanupJob{}
var cleanupJobsMutex sync.Mutex

// add a job that will run cleanup before waiting for jobs to complete
// all jobs run in goroutines, and are waited upon by the main thread
func AddEarlyCleanupJob(name string, job func(isSignal bool)) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	earlyCleanupJobs = append(earlyCleanupJobs, CleanupJob{Name: name, Func: job})
}

// add a job that will run cleanup after waiting for jobs to complete
// all jobs run in goroutines, and are waited upon by the main thread
func AddLateCleanupJob(name string, job func(isSignal bool)) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	lateCleanupJobs = append(lateCleanupJobs, CleanupJob{Name: name, Func: job})
}

func DeleteEarlyCleanupJob(name string) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	earlyCleanupJobs = slices.DeleteFunc(earlyCleanupJobs, func(job CleanupJob) bool {
		return job.Name == name
	})
}

func DeleteLateCleanupJob(name string) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	lateCleanupJobs = slices.DeleteFunc(lateCleanupJobs, func(job CleanupJob) bool {
		return job.Name == name
	})
}

func init() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		waitJobs(true)
		os.Exit(0)
	}()
}
