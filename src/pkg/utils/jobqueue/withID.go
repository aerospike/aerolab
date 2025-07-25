package jobqueue

import (
	"errors"
	"maps"
	"sync"
)

type QueueWithIDs struct {
	sync.Mutex
	jobs    chan struct{}
	maxJobs int
	queue   map[string]JobStatus // map[jobID]isRunning
	err     error
}

type JobStatus int

const (
	JobCreated JobStatus = iota
	JobRunning
	JobFinished
)

// new simple job queue
// max queue size will be concurrent + queued
// queued == how many jobs can be running concurrently and queued up to run
// concurrent == how many jobs can be running concurrently
func NewQueueWithIDs(concurrent int, queued int) *QueueWithIDs {
	if queued < concurrent {
		queued = concurrent
	}
	return &QueueWithIDs{
		jobs:    make(chan struct{}, concurrent),
		maxJobs: concurrent + queued,
		queue:   make(map[string]JobStatus),
	}
}

// add a job to the queue
func (q *QueueWithIDs) Add(jobID string) error {
	q.Lock()
	if q.err != nil {
		defer q.Unlock()
		return q.err
	}
	if len(q.queue) >= q.maxJobs {
		q.Unlock()
		return ErrQueueFull
	}
	if _, ok := q.queue[jobID]; ok {
		q.Unlock()
		return ErrJobAlreadyQueued
	}
	q.queue[jobID] = JobCreated
	q.Unlock()
	return nil
}

// prevent being able to add items to queue, will output error specified in parameter
func (q *QueueWithIDs) SetNoAccept(err error) {
	q.Lock()
	q.err = err
	q.Unlock()
}

// Start a job (add a job to concurrent jobs).
// This will block if the concurrency is reached.
func (q *QueueWithIDs) Start(jobID string) error {
	q.Lock()
	if d, ok := q.queue[jobID]; ok && d == JobRunning {
		q.Unlock()
		return ErrJobAlreadyStarted
	} else if !ok {
		q.Unlock()
		return ErrJobNotFound
	}
	q.jobs <- struct{}{}
	q.queue[jobID] = JobRunning
	q.Unlock()
	return nil
}

var ErrJobAlreadyStarted = errors.New("job already started")
var ErrJobAlreadyQueued = errors.New("job already queued")
var ErrJobNotFound = errors.New("job not found")
var ErrJobNotRunning = errors.New("job not running")
var ErrJobRunning = errors.New("job running")

// End a job (remove a job from concurrent jobs).
// This may block if the queue becomes empty from another caller.
func (q *QueueWithIDs) End(jobID string) error {
	q.Lock()
	defer q.Unlock()
	if d, ok := q.queue[jobID]; !ok {
		return ErrJobNotFound
	} else if d != JobRunning {
		return ErrJobNotRunning
	}
	q.queue[jobID] = JobFinished
	<-q.jobs
	return nil
}

// Remove a job from the queue.
func (q *QueueWithIDs) Remove(jobID string) error {
	q.Lock()
	defer q.Unlock()
	if d, ok := q.queue[jobID]; !ok {
		return ErrJobNotFound
	} else if d == JobRunning {
		return ErrJobRunning
	}
	delete(q.queue, jobID)
	return nil
}

// get the number of items waiting to be processed (queue) and number of jobs currently running (concurrent)
func (q *QueueWithIDs) GetSize() (concurrent int, queued int) {
	q.Lock()
	concurrent = len(q.jobs)
	queued = len(q.queue) - concurrent
	q.Unlock()
	return
}

// returns isRunning, exists
func (q *QueueWithIDs) GetJobStatus(jobID string) (status JobStatus, exists bool) {
	q.Lock()
	status, exists = q.queue[jobID]
	q.Unlock()
	return
}

// returns map[jobID]isRunning
func (q *QueueWithIDs) GetJobs() map[string]JobStatus {
	q.Lock()
	jobs := make(map[string]JobStatus, len(q.queue))
	maps.Copy(jobs, q.queue)
	q.Unlock()
	return jobs
}
