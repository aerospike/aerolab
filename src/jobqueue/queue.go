package jobqueue

import (
	"errors"
	"sync"
)

type Queue struct {
	sync.Mutex
	jobs    chan struct{}
	maxJobs int
	qsize   int
	err     error
}

func New(concurrent int, queued int) *Queue {
	return &Queue{
		jobs:    make(chan struct{}, concurrent),
		maxJobs: concurrent + queued,
	}
}

func (q *Queue) Add() error {
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
func (q *Queue) SetNoAccept(err error) {
	q.Lock()
	q.err = err
	q.Unlock()
}

func (q *Queue) Start() {
	q.jobs <- struct{}{}
}

func (q *Queue) End() error {
	if len(q.jobs) == 0 {
		return ErrQueueEmpty
	}
	<-q.jobs
	return nil
}

func (q *Queue) Remove() error {
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

func (q *Queue) GetSize() (concurrent int, queued int) {
	q.Lock()
	concurrent = len(q.jobs)
	queued = q.qsize - concurrent
	q.Unlock()
	return
}
