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

// Verify publish readers coexist and exclude exclusive conversions.
func TestVolumeLocksSharedAndExclusive(t *testing.T) {
	t.Run("shared locks coexist", func(t *testing.T) {
		locks := NewVolumeLocks()
		if !locks.TryAcquireShared("disk") {
			t.Fatal("first shared acquire failed")
		}
		if !locks.TryAcquireShared("disk") {
			t.Fatal("second shared acquire failed")
		}
	})

	t.Run("exclusive waits for every shared lock to be released", func(t *testing.T) {
		locks := NewVolumeLocks()
		locks.TryAcquireShared("disk")
		locks.TryAcquireShared("disk")

		// Both readers are held, so the exclusive lock is refused.
		if locks.TryAcquire("disk") {
			t.Fatal("exclusive acquire succeeded while shared locks were held")
		}

		// One reader is still held, so the exclusive lock is still refused.
		locks.ReleaseShared("disk")
		if locks.TryAcquire("disk") {
			t.Fatal("exclusive acquire succeeded while one shared lock remained")
		}

		// The last reader is gone, so the exclusive lock is granted.
		locks.ReleaseShared("disk")
		if !locks.TryAcquire("disk") {
			t.Fatal("exclusive acquire failed after shared locks were released")
		}
	})

	t.Run("shared waits for the exclusive lock to be released", func(t *testing.T) {
		locks := NewVolumeLocks()
		if !locks.TryAcquire("disk") {
			t.Fatal("exclusive acquire failed on an unlocked volume")
		}

		// The exclusive lock is held, so readers are refused.
		if locks.TryAcquireShared("disk") {
			t.Fatal("shared acquire succeeded while exclusive lock was held")
		}

		// The exclusive lock is released, so readers are granted again.
		locks.Release("disk")
		if !locks.TryAcquireShared("disk") {
			t.Fatal("shared acquire failed after exclusive lock was released")
		}
		locks.ReleaseShared("disk")
	})
}
