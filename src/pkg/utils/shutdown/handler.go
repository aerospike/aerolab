package shutdown

import (
	"os"
	"os/signal"
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
	cleanupJobsMutex.Lock() // we never unlock, as new jobs cannot be added now that we are in shutdown
	isShuttingDownMutex.Lock()
	isShuttingDown = true
	isShuttingDownMutex.Unlock()
	for _, job := range earlyCleanupJobs {
		job(isSignal)
	}
	waiter.Wait()
	for _, job := range lateCleanupJobs {
		job(isSignal)
	}
}

// mark a job as done: should be called from any go routine that needs to be waited for when it's done
func DoneJob() {
	waiter.Done()
}

var earlyCleanupJobs = make(map[string]func(isSignal bool))
var lateCleanupJobs = make(map[string]func(isSignal bool))
var cleanupJobsMutex sync.Mutex

// add a job that will run cleanup before waiting for jobs to complete
// all jobs run in goroutines, and are waited upon by the main thread
func AddEarlyCleanupJob(name string, job func(isSignal bool)) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	earlyCleanupJobs[name] = job
}

// add a job that will run cleanup after waiting for jobs to complete
// all jobs run in goroutines, and are waited upon by the main thread
func AddLateCleanupJob(name string, job func(isSignal bool)) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	lateCleanupJobs[name] = job
}

func DeleteEarlyCleanupJob(name string) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	delete(earlyCleanupJobs, name)
}

func DeleteLateCleanupJob(name string) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	delete(lateCleanupJobs, name)
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
