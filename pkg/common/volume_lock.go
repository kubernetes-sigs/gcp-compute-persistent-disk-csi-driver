/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"sync"
)

// VolumeLocks implements a map with atomic operations. It stores a set of all volume IDs
// with an ongoing operation.
type VolumeLocks struct {
	locks map[string]volumeLockState
	mux   sync.Mutex
}

type volumeLockState struct {
	writer  bool
	readers int
}

func NewVolumeLocks() *VolumeLocks {
	return &VolumeLocks{
		locks: make(map[string]volumeLockState),
	}
}

// TryAcquire tries to acquire the lock for operating on volumeID and returns true if successful.
// If another operation is already using volumeID, returns false.
func (vl *VolumeLocks) TryAcquire(volumeID string) bool {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	if state := vl.locks[volumeID]; state.writer || state.readers > 0 {
		return false
	}
	vl.locks[volumeID] = volumeLockState{writer: true}
	return true
}

// TryAcquireShared acquires a read lock for volumeID. Multiple shared
// operations can coexist, but none can run while an exclusive volume
// operation, such as conversion, is in progress.
// PR2397-C2: This preserves concurrent publishes while excluding conversion.
func (vl *VolumeLocks) TryAcquireShared(volumeID string) bool {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	state := vl.locks[volumeID]
	if state.writer {
		return false
	}
	state.readers++
	vl.locks[volumeID] = state
	return true
}

func (vl *VolumeLocks) Release(volumeID string) {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	state := vl.locks[volumeID]
	state.writer = false
	if state.readers == 0 {
		delete(vl.locks, volumeID)
	} else {
		vl.locks[volumeID] = state
	}
}

// ReleaseShared releases a read lock previously acquired with
// TryAcquireShared.
func (vl *VolumeLocks) ReleaseShared(volumeID string) {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	state := vl.locks[volumeID]
	if state.readers > 0 {
		state.readers--
	}
	if !state.writer && state.readers == 0 {
		delete(vl.locks, volumeID)
	} else {
		vl.locks[volumeID] = state
	}
}
