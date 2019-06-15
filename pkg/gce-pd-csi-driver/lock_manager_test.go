package gceGCEDriver

import (
	"sync"
	"testing"
	"time"
)

// Checks that the lock manager has the expected number of locks allocated.
// this function is implementation dependant! It acquires the lock and directly
// checks the map of the lock manager.
func checkAllocation(lm *LockManager, expectedNumAllocated int, t *testing.T) {
	lm.mux.Lock()
	defer lm.mux.Unlock()
	if len(lm.locks) != expectedNumAllocated {
		t.Fatalf("expected %d locks allocated, but found %d", expectedNumAllocated, len(lm.locks))
	}
}

// coinOperatedMutex is a mutex that only acquires if a "coin" is provided. Otherwise
// it sleeps until there is both a coin and the lock is free. This is used
// so a parent thread can control the execution of children's lock.
type coinOperatedMutex struct {
	mux  *sync.Mutex
	cond *sync.Cond
	held bool
	coin chan coin
	t    *testing.T
}

type coin struct{}

func (m *coinOperatedMutex) Lock() {
	m.mux.Lock()
	defer m.mux.Unlock()

	for m.held || len(m.coin) == 0 {
		m.cond.Wait()
	}
	<-m.coin
	m.held = true
}

func (m *coinOperatedMutex) Unlock() {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.held = false
	m.cond.Broadcast()
}

func (m *coinOperatedMutex) Deposit() {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.coin <- coin{}
	m.cond.Broadcast()
}

func passCoinOperatedMutex(lockerParams ...interface{}) sync.Locker {
	return lockerParams[0].(*coinOperatedMutex)
}

func TestLockManagerSingle(t *testing.T) {
	lm := NewLockManager(NewSyncMutex)
	lm.Acquire("A")
	checkAllocation(lm, 1, t)
	lm.Acquire("B")
	checkAllocation(lm, 2, t)
	lm.Release("A")
	checkAllocation(lm, 1, t)
	lm.Release("B")
	checkAllocation(lm, 0, t)
}

func TestLockManagerMultiple(t *testing.T) {
	lm := NewLockManager(passCoinOperatedMutex)
	m := &sync.Mutex{}
	com := &coinOperatedMutex{
		mux:  m,
		cond: sync.NewCond(m),
		coin: make(chan coin, 1),
		held: false,
		t:    t,
	}

	// start thread 1
	t1OperationFinished := make(chan coin, 1)
	t1OkToRelease := make(chan coin, 1)
	go func() {
		lm.Acquire("A", com)
		t1OperationFinished <- coin{}
		<-t1OkToRelease
		lm.Release("A")
		t1OperationFinished <- coin{}
	}()

	// this allows the acquire by thread 1 to acquire
	com.Deposit()
	<-t1OperationFinished

	// thread 1 should have acquired the lock, putting allocation at 1
	checkAllocation(lm, 1, t)

	// start thread 2
	// this should allow thread 2 to start the acquire for A through the
	// lock manager, but block on the acquire Lock() of the lock for A.
	t2OperationFinished := make(chan coin, 1)
	t2OkToRelease := make(chan coin, 1)
	go func() {
		lm.Acquire("A")
		t2OperationFinished <- coin{}
		<-t2OkToRelease
		lm.Release("A")
		t2OperationFinished <- coin{}
	}()

	// because now thread 2 is the only thread that can run, we must wait
	// until it runs until it is blocked on acquire. for simplicity just wait
	// 5 seconds.
	time.Sleep(time.Second * 3)

	// this allows the release by thread 1 to complete
	// only the release can run because the acquire by thread 1 can only run if
	// there is both a coin and the lock is free
	t1OkToRelease <- coin{}
	<-t1OperationFinished

	// check that the lock has not been deallocated, since thread 2 is still waiting to acquire it
	checkAllocation(lm, 1, t)

	// this allows t2 to finish its acquire
	com.Deposit()
	<-t2OperationFinished

	// check that the lock has been deallocated, since thread 2 still holds it
	checkAllocation(lm, 1, t)

	// this allows the release by thread 2 to release
	t2OkToRelease <- coin{}
	<-t2OperationFinished

	// check that the lock has been deallocated
	checkAllocation(lm, 0, t)
}
