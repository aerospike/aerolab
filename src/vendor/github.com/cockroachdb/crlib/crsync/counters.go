// Copyright 2025 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package crsync

import (
	"iter"
	"sync/atomic"
	"unsafe"
)

// Counter is a single logical counter backed by a sharded implementation
// (Counters) under the hood.
//
// Properties:
//   - Thread-safe increments: Add() can be called concurrently from many
//     goroutines.
//   - Low write contention: Writes are sharded to minimize cache-line
//     ping‑pong.
//   - Simple reads: Get() aggregates across shards to return the current value.
//   - Construction: Use MakeCounter(). The zero value is NOT ready to use.
//   - Performance: Add is O(1) with low contention; Get is O(NumShards()).
//   - Consistency: Reads are best-effort snapshots without global locking. Each
//     shard is read atomically, but the aggregation is not linearizable with
//     respect to concurrent Add calls. This is typically acceptable for metrics
//     and counters.
//
// Example:
//
//	c := MakeCounter()
//	c.Add(1)
//	c.Add(41)
//	fmt.Println(c.Get()) // 42
type Counter struct {
	c Counters
}

// MakeCounter initializes a new Counter.
func MakeCounter() Counter {
	return Counter{
		c: MakeCounters(1),
	}
}

// Add atomically adds delta to the counter. It is safe for concurrent use by
// multiple goroutines; delta may be negative (decrement).
//
// Add is very efficient: a single atomic increment on a mostly uncontended
// cache line.
func (c *Counter) Add(delta int64) {
	c.c.Add(0, delta)
}

// Get the current value of the counter.
//
// It safe to call Get() while there are concurrent Add() calls (but there is no
// guarantee wrt which of those are reflected).
//
// Get is O(NumShards()) so it is more expensive than Add().
func (c *Counter) Get() int64 {
	return c.c.Get(0)
}

// Counters is a sharded set of logical counters that can be incremented
// concurrently with low contention.
//
// Use when you need N independent counters that are updated from many
// goroutines (e.g., metrics like hits/misses/errors, per-state tallies).
//
// Properties:
//   - Thread-safe increments: Add() can be called concurrently from many
//     goroutines.
//   - Low write contention: Writes are sharded to minimize cache-line
//     ping‑pong.
//   - Simple reads: Get() aggregates across shards to return the current value.
//   - Construction: Use MakeCounter(). The zero value is NOT ready to use.
//   - Performance: Add is O(1) with low contention; Get is O(NumShards());
//   - Consistency: Reads are best-effort snapshots without global locking. Each
//     shard is read atomically, but the aggregation is not linearizable with
//     respect to concurrent Add calls. This is typically acceptable for metrics
//     and counters.
type Counters struct {
	numShards uint32
	// shardSize is the number of counters per shard.
	shardSize uint32
	// counters contains numShards * shardSize counters; shardSize is a multiple
	// of countersPerCacheLine to avoid false sharing at shard boundaries. Note
	// that there is a high correlation between the current CPU and the chosen
	// shard, so different counters inside a shard can share cache lines.
	//
	// We linearize the array instead of using [][]atomic.Int64 to avoid an extra
	// pointer load in the fast path.
	counters    []atomic.Int64
	numCounters int
}

// Number of counters per cache line. We assume the typical 64-byte cache line.
// Must be a power of 2.
const countersPerCacheLine = 8

// MakeCounters creates a new Counters with the specified number of counters.
func MakeCounters(numCounters int) Counters {
	return makeCounters(NumShards(), numCounters)
}

func makeCounters(numShards, numCounters int) Counters {
	// shardSize is the number of counters, rounded up to fill the last cache line
	// (to avoid false sharing).
	shardSize := (numCounters + countersPerCacheLine - 1) / countersPerCacheLine * countersPerCacheLine
	// Allocate all the counters and align the slice to start at a cache line. We
	// allocate countersPerCacheLine-1 extra values to allow realignment.
	counters := make([]atomic.Int64, shardSize*numShards+countersPerCacheLine-1)
	if r := (uintptr(unsafe.Pointer(&counters[0])) / unsafe.Sizeof(atomic.Int64{})) % countersPerCacheLine; r != 0 {
		counters = counters[countersPerCacheLine-r:]
	}
	return Counters{
		numShards:   uint32(numShards),
		shardSize:   uint32(shardSize),
		counters:    counters,
		numCounters: numCounters,
	}
}

// Add atomically adds delta to the specified counter. It is safe for concurrent
// use by multiple goroutines; delta may be negative (decrement).
//
// Add is very efficient: a single atomic increment on a mostly uncontended
// cache line.
func (c *Counters) Add(counter int, delta int64) {
	shard := uint32(CPUBiasedInt()) % c.numShards
	c.counters[shard*c.shardSize+uint32(counter)].Add(delta)
}

// Get the current value of the specified counter.
//
// It safe to call Get() while there are concurrent Add() calls (but there is no
// guarantee wrt which of those are reflected).
//
// Get is O(NumShards()) so it is more expensive than Add().
func (c *Counters) Get(counter int) int64 {
	var res int64
	for shard := range c.numShards {
		res += c.counters[shard*c.shardSize+uint32(counter)].Load()
	}
	return res
}

// All iterates through the current values of all counters (in order).
//
// Complexity is O(NumShards() * numCounters). All is safe for concurrent use,
// but there are no ordering guarantees w.r.t. concurrent updates.
//
// All is designed to minimize disruption to concurrent Add() calls and is
// preferable to multiple Get() calls when all counter values are needed.
func (c *Counters) All() iter.Seq[int64] {
	return func(yield func(int64) bool) {
		// To access each cache line only once, we calculate countersPerCacheLine
		// counters at a time.
		var vals [countersPerCacheLine]int64
		for i := 0; i < c.numCounters; i += countersPerCacheLine {
			// We have a total of c.numCounters logical counters and we are processing
			// countersPerCacheLine logical counters at a time. So the last iteration
			// of this loop will have fewer than countersPerCacheLine logical counters
			// to read. n is the number of logical counters to read in this iteration.
			n := min(c.numCounters-i, countersPerCacheLine)
			// Iterate over all the shards and for this logical cache line, read the
			// physical cache line from each shard and aggregate into vals.
			vals = [countersPerCacheLine]int64{}
			for s := range c.numShards {
				start := int(s*c.shardSize) + i
				counters := c.counters[start : start+n]
				// Avoid bound checks inside the loop.
				_ = vals[len(counters)-1]
				for j := range counters {
					vals[j] += counters[j].Load()
				}
			}
			// Yield the values for the next set of n logical counters. Across the
			// iterations of the outer loop we will in total yield c.numCounters
			// values.
			for j := range n {
				if !yield(vals[j]) {
					return
				}
			}
		}
	}
}
