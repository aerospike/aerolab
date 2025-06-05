package counters

import (
	"maps"
	"sync"
)

type Map struct {
	mu sync.Mutex
	m  map[string]int
}

func (c *Map) Inc(key string) {
	c.mu.Lock()
	if _, ok := c.m[key]; !ok {
		c.m[key] = 0
	}
	c.m[key]++
	c.mu.Unlock()
}

func (c *Map) Dec(key string) {
	c.mu.Lock()
	if _, ok := c.m[key]; !ok {
		c.m[key] = 0
	}
	c.m[key]--
	c.mu.Unlock()
}

func (c *Map) Get(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[key]; !ok {
		return 0
	}
	return c.m[key]
}

func (c *Map) Set(key string, value int) {
	c.mu.Lock()
	c.m[key] = value
	c.mu.Unlock()
}

func (c *Map) GetKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.m))
	for key := range c.m {
		keys = append(keys, key)
	}
	return keys
}

func (c *Map) GetValues() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	values := make([]int, 0, len(c.m))
	for _, value := range c.m {
		values = append(values, value)
	}
	return values
}

func (c *Map) GetTotal() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, value := range c.m {
		total += value
	}
	return total
}

func (c *Map) GetMapSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

func (c *Map) GetMapCopy() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	newMap := make(map[string]int, len(c.m))
	maps.Copy(newMap, c.m)
	return newMap
}

func (c *Map) Clone() *Map {
	c.mu.Lock()
	defer c.mu.Unlock()
	newMap := make(map[string]int, len(c.m))
	maps.Copy(newMap, c.m)
	return &Map{
		mu: sync.Mutex{},
		m:  newMap,
	}
}

func NewMap() *Map {
	return &Map{
		mu: sync.Mutex{},
		m:  make(map[string]int),
	}
}
