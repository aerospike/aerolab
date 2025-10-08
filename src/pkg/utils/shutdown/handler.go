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

// IsShuttingDown returns true if the application is currently in the shutdown process.
// This function is thread-safe and can be used by goroutines to check if they should
// terminate their operations gracefully.
//
// Returns:
//   - bool: true if shutdown is in progress, false otherwise
func IsShuttingDown() bool {
	isShuttingDownMutex.Lock()
	defer isShuttingDownMutex.Unlock()
	return isShuttingDown
}

// AddJob registers a new job with the shutdown handler's wait group.
// This should be called from any goroutine that needs to be waited for during shutdown.
// Each call to AddJob must be paired with a corresponding call to DoneJob when the
// goroutine completes its work.
//
// Usage:
//
//	shutdown.AddJob()
//	go func() {
//	    defer shutdown.DoneJob()
//	    // ... do work ...
//	}()
func AddJob() {
	cleanupJobsMutex.Lock()
	waiter.Add(1)
	cleanupJobsMutex.Unlock()
}

// WaitJobs waits for all registered jobs to complete and runs cleanup functions.
// This should be called from main() and before any os.Exit/log.Fatal calls to ensure
// graceful shutdown. It will:
// 1. Set the shutdown flag to prevent new jobs from being added
// 2. Run all early cleanup jobs in reverse order
// 3. Wait for all active jobs to complete
// 4. Run all late cleanup jobs in reverse order
//
// Usage:
//
//	defer shutdown.WaitJobs() // in main function
//	// or before os.Exit/log.Fatal calls
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

// DoneJob marks a job as completed in the shutdown handler's wait group.
// This should be called from any goroutine that previously called AddJob when it
// completes its work. It's typically used with defer to ensure it's always called.
//
// Usage:
//
//	shutdown.AddJob()
//	defer shutdown.DoneJob()
//	// ... do work ...
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

// AddEarlyCleanupJob registers a cleanup function that will run before waiting for jobs to complete.
// Early cleanup jobs are executed in reverse order (LIFO) during shutdown and are intended
// for operations that should happen before waiting for worker goroutines to finish.
//
// Parameters:
//   - name: A unique identifier for the cleanup job (used for deletion)
//   - job: The cleanup function to execute. The isSignal parameter indicates if shutdown
//     was triggered by a signal (SIGINT/SIGTERM) vs programmatic shutdown
//
// Usage:
//
//	shutdown.AddEarlyCleanupJob("close-listeners", func(isSignal bool) {
//	    listener.Close()
//	})
func AddEarlyCleanupJob(name string, job func(isSignal bool)) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	earlyCleanupJobs = append(earlyCleanupJobs, CleanupJob{Name: name, Func: job})
}

// AddLateCleanupJob registers a cleanup function that will run after waiting for jobs to complete.
// Late cleanup jobs are executed in reverse order (LIFO) during shutdown and are intended
// for final cleanup operations that should happen after all worker goroutines have finished.
//
// Parameters:
//   - name: A unique identifier for the cleanup job (used for deletion)
//   - job: The cleanup function to execute. The isSignal parameter indicates if shutdown
//     was triggered by a signal (SIGINT/SIGTERM) vs programmatic shutdown
//
// Usage:
//
//	shutdown.AddLateCleanupJob("cleanup-temp", func(isSignal bool) {
//	    os.RemoveAll("/tmp/myapp")
//	})
func AddLateCleanupJob(name string, job func(isSignal bool)) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	lateCleanupJobs = append(lateCleanupJobs, CleanupJob{Name: name, Func: job})
}

// DeleteEarlyCleanupJob removes a previously registered early cleanup job by name.
// This is useful when a cleanup job is no longer needed or when replacing an existing job.
//
// Parameters:
//   - name: The unique identifier of the cleanup job to remove
//
// Usage:
//
//	shutdown.DeleteEarlyCleanupJob("close-listeners")
func DeleteEarlyCleanupJob(name string) {
	cleanupJobsMutex.Lock()
	defer cleanupJobsMutex.Unlock()
	earlyCleanupJobs = slices.DeleteFunc(earlyCleanupJobs, func(job CleanupJob) bool {
		return job.Name == name
	})
}

// DeleteLateCleanupJob removes a previously registered late cleanup job by name.
// This is useful when a cleanup job is no longer needed or when replacing an existing job.
//
// Parameters:
//   - name: The unique identifier of the cleanup job to remove
//
// Usage:
//
//	shutdown.DeleteLateCleanupJob("cleanup-temp")
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
