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

//go:build cockroach_go

package crsync

import "runtime"

// CPUBiasedInt returns an arbitrary non-negative integer that has a best-effort
// association with the current CPU.
//
// Specifically, in the common case, the same value is returned on the same CPU;
// and different CPUs return different values.
//
// When the CockroachDB go runtime is used, the returned value is simply the
// index of the current P (between 0 and GOMAXPROCS-1).
func CPUBiasedInt() int {
	return runtime.CurrentP()
}

// NumShards returns the recommended number of shards when CPUBiasedInt is
// used to select a shard.
func NumShards() int {
	return runtime.GOMAXPROCS(0)
}

// UsingCockroachGo is true if the CockroachDB go runtime is in use.
const UsingCockroachGo = true
