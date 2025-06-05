package counters

import (
	"sync"
)

type Int struct {
	mu    sync.Mutex
	count int
}

func (c *Int) Inc() {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

func (c *Int) Dec() {
	c.mu.Lock()
	c.count--
	c.mu.Unlock()
}

func (c *Int) Get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func (c *Int) Set(count int) {
	c.mu.Lock()
	c.count = count
	c.mu.Unlock()
}

func NewInt(val int) *Int {
	return &Int{
		mu:    sync.Mutex{},
		count: val,
	}
}
