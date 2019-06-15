package gceGCEDriver

import (
	"github.com/golang/glog"
	"sync"
)

type lockWithWaiters struct {
	mux     sync.Locker
	waiters uint32
}

type LockManager struct {
	mux       sync.Mutex
	newLocker func(...interface{}) sync.Locker
	locks     map[string]*lockWithWaiters
}

func NewLockManager(f func(...interface{}) sync.Locker) *LockManager {
	return &LockManager{
		newLocker: f,
		locks:     make(map[string]*lockWithWaiters),
	}
}

func NewSyncMutex(lockerParams ...interface{}) sync.Locker {
	return &sync.Mutex{}
}

// Acquires the lock corresponding to a key, and allocates that lock if one does not exist.
func (lm *LockManager) Acquire(key string, lockerParams ...interface{}) {
	lm.mux.Lock()
	lockForKey, ok := lm.locks[key]
	if !ok {
		lockForKey = &lockWithWaiters{
			mux:     lm.newLocker(lockerParams...),
			waiters: 0,
		}
		lm.locks[key] = lockForKey
	}
	lockForKey.waiters += 1
	lm.mux.Unlock()
	lockForKey.mux.Lock()
}

// Releases the lock corresponding to a key, and deallocates that lock if no other thread
// is waiting to acquire it. Logs an error and returns if the lock for a key does not exist.
func (lm *LockManager) Release(key string) {
	lm.mux.Lock()
	lockForKey, ok := lm.locks[key]
	if !ok {
		// This should not happen, but if it does the only thing to do is to log the error
		glog.Errorf("the key being released does not correspond to an existing lock")
		return
	}
	lockForKey.waiters -= 1
	lockForKey.mux.Unlock()
	if lockForKey.waiters == 0 {
		delete(lm.locks, key)
	}
	lm.mux.Unlock()
}
