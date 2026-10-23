/*
Copyright 2026 The Kubernetes Authors.

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

import "testing"

// PR2397-C2: Verify publish readers coexist and exclude exclusive conversions.
func TestVolumeLocksSharedAndExclusive(t *testing.T) {
	locks := NewVolumeLocks()
	if !locks.TryAcquireShared("disk") {
		t.Fatal("first shared acquire failed")
	}
	if !locks.TryAcquireShared("disk") {
		t.Fatal("second shared acquire failed")
	}
	if locks.TryAcquire("disk") {
		t.Fatal("exclusive acquire succeeded while shared locks were held")
	}
	locks.ReleaseShared("disk")
	if locks.TryAcquire("disk") {
		t.Fatal("exclusive acquire succeeded while one shared lock remained")
	}
	locks.ReleaseShared("disk")
	if !locks.TryAcquire("disk") {
		t.Fatal("exclusive acquire failed after shared locks were released")
	}
	if locks.TryAcquireShared("disk") {
		t.Fatal("shared acquire succeeded while exclusive lock was held")
	}
	locks.Release("disk")
	if !locks.TryAcquireShared("disk") {
		t.Fatal("shared acquire failed after exclusive lock was released")
	}
	locks.ReleaseShared("disk")
}
