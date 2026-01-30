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

import "github.com/aerospike/aerospike-client-go/v8/types"

// TxnError implements Error interface for aerospike multi-record transaction specific errors.
type TxnError struct {
	AerospikeError

	// Error status of the attempted commit.
	CommitError CommitError

	// Verify result for each read key in the Transaction. May be nil if failure occurred before verify.
	VerifyRecords []*BatchRecord

	// Roll forward/backward result for each write key in the Transaction. May be nil if failure occurred before
	// roll forward/backward.
	RollRecords []*BatchRecord
}

var _ error = &TxnError{}
var _ Error = &TxnError{}

func NewTxnCommitError(err CommitError, verifyRecords, rollRecords []*BatchRecord, cause Error) Error {
	if cause == nil {
		res := newError(types.TXN_FAILED, string(err))
		return &TxnError{
			AerospikeError: *(res.(*AerospikeError)),
			CommitError:    err,
			VerifyRecords:  verifyRecords,
			RollRecords:    rollRecords,
		}
	}

	embed := newError(types.TXN_FAILED, string(err)).wrap(cause)
	return &TxnError{
		AerospikeError: *(embed.(*AerospikeError)),
		CommitError:    err,
		VerifyRecords:  verifyRecords,
		RollRecords:    rollRecords,
	}
}
