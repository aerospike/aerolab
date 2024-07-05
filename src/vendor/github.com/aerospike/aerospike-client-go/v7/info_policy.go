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
	"context"
	"time"

	kvs "github.com/aerospike/aerospike-client-go/v7/proto/kvs"
)

// InfoPolicy contains attributes used for info commands.
type InfoPolicy struct {

	// Info command socket timeout.
	// Default is 2 seconds.
	Timeout time.Duration
}

// NewInfoPolicy generates a new InfoPolicy with default values.
func NewInfoPolicy() *InfoPolicy {
	return &InfoPolicy{
		Timeout: _DEFAULT_TIMEOUT,
	}
}

func (p *InfoPolicy) deadline() time.Time {
	var deadline time.Time
	if p != nil && p.Timeout > 0 {
		deadline = time.Now().Add(p.Timeout)
	}

	return deadline
}

func (p *InfoPolicy) timeout() time.Duration {
	if p != nil && p.Timeout > 0 {
		return p.Timeout
	}

	return _DEFAULT_TIMEOUT
}

var simpleCancelFunc = func() {}

func (p *InfoPolicy) grpcDeadlineContext() (context.Context, context.CancelFunc) {
	timeout := p.timeout()
	if timeout <= 0 {
		return context.Background(), simpleCancelFunc

	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return ctx, cancel
}

func (p *InfoPolicy) grpc() *kvs.InfoPolicy {
	if p == nil {
		return nil
	}

	Timeout := uint32(p.Timeout / time.Millisecond)
	res := &kvs.InfoPolicy{
		Timeout: &Timeout,
	}

	return res
}
