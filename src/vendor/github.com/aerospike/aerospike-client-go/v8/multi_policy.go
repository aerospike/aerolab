// Copyright 2014-2022 Aerospike, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aerospike

import "time"

// MultiPolicy contains parameters for policy attributes used in
// query and scan operations.
type MultiPolicy struct {
	BasePolicy

	// Maximum number of concurrent requests to server nodes at any point in time.
	// If there are 16 nodes in the cluster and maxConcurrentNodes is 8, then queries
	// will be made to 8 nodes in parallel.  When a query completes, a new query will
	// be issued until all 16 nodes have been queried.
	// Default (0) is to issue requests to all server nodes in parallel.
	// 1 will to issue requests to server nodes one by one avoiding parallel queries.
	MaxConcurrentNodes int

	// MaxRecords approximates the number of records to return to the client. This number is divided by the
	// number of nodes involved in the query. The actual number of records returned
	// may be less than MaxRecords if node record counts are small and unbalanced across
	// nodes.
	//
	// This field is supported on server versions >= 4.9.
	//
	// Default: 0 (do not limit record count)
	MaxRecords int64

	// RecordsPerSecond limits returned records per second (rps) rate for each server.
	// It does not apply rps limit if RecordsPerSecond is zero (default).
	//
	// RecordsPerSecond is supported in all primary and secondary index
	// queries in server versions 6.0+. For background queries, RecordsPerSecond
	// is bounded by the server config background-query-max-rps.
	RecordsPerSecond int

	// Number of records to place in queue before blocking.
	// Records received from multiple server nodes will be placed in a queue.
	// A separate goroutine consumes these records in parallel.
	// If the queue is full, the producer goroutines will block until records are consumed.
	RecordQueueSize int //= 50

	// Indicates if bin data is retrieved. If false, only record digests are retrieved.
	IncludeBinData bool //= true;

	// RawCDT specifies that the value of the CDT fields (Maps and Lists) should not be unpacked/decoded.
	// This is only used internally by Aerospike for Backup purposes and should not be used by 3rd parties.
	RawCDT bool
}

// NewMultiPolicy initializes a MultiPolicy instance with default values.
// It disables TotalTimeout and sets MaxRetries.
// Set MaxRetries for non-aggregation queries with a nil filter on
// server versions >= 4.9. All other queries are not retried.
//
// The latest servers support retries on individual data partitions.
// This feature is useful when a cluster is migrating and partition(s)
// are missed or incomplete on the first query attempt.
//
// If the first query attempt misses 2 of 4096 partitions, then only
// those 2 partitions are retried in the next query attempt from the
// last key digest received for each respective partition. A higher
// default MaxRetries is used because it's wasteful to invalidate
// all query results because a single partition was missed.
func NewMultiPolicy() *MultiPolicy {
	bp := *NewPolicy()
	bp.TotalTimeout = 0 * time.Second
	bp.SocketTimeout = 30 * time.Second
	bp.MaxRetries = 5

	return &MultiPolicy{
		BasePolicy:         bp,
		MaxConcurrentNodes: 0,
		RecordsPerSecond:   0,
		RecordQueueSize:    50,
		IncludeBinData:     true,
	}
}
