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

import (
	"fmt"

	"github.com/aerospike/aerospike-client-go/v8/types"
)

// BatchRead specifies the Key and bin names used in batch read commands
// where variable bins are needed for each key.
type BatchRead struct {
	BatchRecord

	// Optional read policy.
	Policy *BatchReadPolicy

	// BinNames specifies the Bins to retrieve for this key.
	// BinNames are mutually exclusive with Ops.
	BinNames []string

	// ReadAllBins defines what data should be read from the record.
	// If true, ignore binNames and read all bins.
	// If false and binNames are set, read specified binNames.
	// If false and binNames are not set, read record header (generation, expiration) only.
	ReadAllBins bool //= false

	// Ops specifies the operations to perform for every key.
	// Ops are mutually exclusive with BinNames.
	// A binName can be emulated with `GetOp(binName)`
	// Supported by server v5.6.0+.
	Ops []*Operation
}

// NewBatchRead defines a key and bins to retrieve in a batch operation.
func NewBatchRead(policy *BatchReadPolicy, key *Key, binNames []string) *BatchRead {
	return &BatchRead{
		BatchRecord: *newSimpleBatchRecord(key, false),
		Policy:      policy,
		BinNames:    binNames,
		ReadAllBins: len(binNames) == 0,
	}
}

// newBatchRead defines a key and bins to retrieve in a batch operation.
func newBatchRead(policy *BatchReadPolicy, key *Key, binNames []string) (*BatchRead, *BatchRecord) {
	res := &BatchRead{
		BatchRecord: *newSimpleBatchRecord(key, false),
		Policy:      policy,
		BinNames:    binNames,
		ReadAllBins: len(binNames) == 0,
	}

	return res, &res.BatchRecord
}

// NewBatchReadOps defines a key and bins to retrieve in a batch operation, including expressions.
func NewBatchReadOps(policy *BatchReadPolicy, key *Key, ops ...*Operation) *BatchRead {
	res := &BatchRead{
		BatchRecord: *newSimpleBatchRecord(key, false),
		Policy:      policy,
		Ops:         ops,
	}

	return res
}

// NewBatchReadOps defines a key and bins to retrieve in a batch operation, including expressions.
func newBatchReadOps(policy *BatchReadPolicy, key *Key, ops ...*Operation) (*BatchRead, *BatchRecord) {
	// since binNames is mutually exclusive with ops parameter.
	res := &BatchRead{
		BatchRecord: *newSimpleBatchRecord(key, false),
		Policy:      policy,
		Ops:         ops,
	}

	res.ReadAllBins = true

	return res, &res.BatchRecord
}

// NewBatchReadHeader defines a key to retrieve the record headers only in a batch operation.
func NewBatchReadHeader(policy *BatchReadPolicy, key *Key) *BatchRead {
	return &BatchRead{
		BatchRecord: *newSimpleBatchRecord(key, false),
		Policy:      policy,
		ReadAllBins: false,
	}
}

// Return batch command type.
func (br *BatchRead) getType() batchRecordType {
	return _BRT_BATCH_READ
}

// Optimized reference equality check to determine batch wire protocol repeat flag.
// For internal use only.
func (br *BatchRead) equals(obj BatchRecordIfc) bool {
	other, ok := obj.(*BatchRead)
	if !ok {
		return false
	}

	return &br.BinNames == &other.BinNames && &br.Ops == &other.Ops && br.Policy == other.Policy && br.ReadAllBins == other.ReadAllBins
}

// Return wire protocol size. For internal use only.
func (br *BatchRead) size(parentPolicy *BasePolicy) (int, Error) {
	size := 0

	if br.Policy != nil {
		if br.Policy.FilterExpression != nil {
			if sz, err := br.Policy.FilterExpression.size(); err != nil {
				return -1, err
			} else {
				size += sz + int(_FIELD_HEADER_SIZE)
			}
		}
	}

	for i := range br.BinNames {
		size += len(br.BinNames[i]) + int(_OPERATION_HEADER_SIZE)
	}

	for i := range br.Ops {
		if br.Ops[i].opType.isWrite {
			return -1, newError(types.PARAMETER_ERROR, "Write operations not allowed in batch read")
		}
		sz, err := br.Ops[i].size()
		if err != nil {
			return -1, err
		}
		size += sz
	}

	return size, nil
}

// String implements the Stringer interface.
func (br *BatchRead) String() string {
	return fmt.Sprintf("%s: %v", br.Key, br.BinNames)
}

func (br *BatchRead) headerOnly() bool {
	return len(br.Ops) == 0 && len(br.BinNames) == 0 && !br.ReadAllBins
}
