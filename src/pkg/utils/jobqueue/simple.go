package jobqueue

import (
	"errors"
	"sync"
)

type SimpleQueue struct {
	sync.Mutex
	jobs    chan struct{}
	maxJobs int
	qsize   int
	err     error
}

// new simple job queue
func NewSimpleQueue(concurrent int, queued int) *SimpleQueue {
	return &SimpleQueue{
		jobs:    make(chan struct{}, concurrent),
		maxJobs: concurrent + queued,
	}
}

// add a job to the queue
func (q *SimpleQueue) Add() error {
	q.Lock()
	if q.err != nil {
		defer q.Unlock()
		return q.err
	}
	if q.qsize >= q.maxJobs {
		q.Unlock()
		return ErrQueueFull
	}
	q.qsize++
	q.Unlock()
	return nil
}

// prevent being able to add items to queue, will output error specified in parameter
func (q *SimpleQueue) SetNoAccept(err error) {
	q.Lock()
	q.err = err
	q.Unlock()
}

// Start a job (add a job to concurrent jobs).
// This will block if the concurrency is reached.
func (q *SimpleQueue) Start() {
	q.jobs <- struct{}{}
}

// End a job (remove a job from concurrent jobs).
// This may block if the queue becomes empty from another caller.
func (q *SimpleQueue) End() error {
	if len(q.jobs) == 0 {
		return ErrQueueEmpty
	}
	<-q.jobs
	return nil
}

// Remove a job from the queue.
func (q *SimpleQueue) Remove() error {
	q.Lock()
	defer q.Unlock()
	if q.qsize == 0 {
		return ErrQueueEmpty
	}
	q.qsize--
	return nil
}

var ErrQueueFull = errors.New("job queue full")
var ErrQueueEmpty = errors.New("job queue empty")

// get the number of items waiting to be processed (queue) and number of jobs currently running (concurrent)
func (q *SimpleQueue) GetSize() (concurrent int, queued int) {
	q.Lock()
	concurrent = len(q.jobs)
	queued = q.qsize - concurrent
	q.Unlock()
	return
}
