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
}

func New(concurrent int, queued int) *Queue {
	return &Queue{
		jobs:    make(chan struct{}, concurrent),
		maxJobs: concurrent + queued,
	}
}

func (q *Queue) Add() error {
	q.Lock()
	if q.qsize >= q.maxJobs {
		q.Unlock()
		return ErrQueueFull
	}
	q.qsize++
	q.Unlock()
	return nil
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

var ErrQueueFull = errors.New("queue full")
var ErrQueueEmpty = errors.New("queue empty")

func (q *Queue) GetSize() (concurrent int, queued int) {
	q.Lock()
	concurrent = len(q.jobs)
	queued = q.qsize - concurrent
	q.Unlock()
	return
}
