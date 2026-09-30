package projectlock

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The point of the whole package: two requests racing to teach the same
// project must run their critical sections one at a time, not interleaved.
func TestSameProjectSerializes(t *testing.T) {
	tr := NewTracker()
	var active int32
	var maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := tr.Lock("/data/pool")
			defer unlock()
			n := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxActive)
				if n <= old || atomic.CompareAndSwapInt32(&maxActive, old, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()
	if maxActive != 1 {
		t.Errorf("max concurrent holders of one project's lock = %d, want 1", maxActive)
	}
}

// A slow request on one project must not block a different project -- the
// lock is per-project, not global.
func TestDifferentProjectsDoNotBlockEachOther(t *testing.T) {
	tr := NewTracker()
	unlockA := tr.Lock("/data/a")
	defer unlockA()

	done := make(chan struct{})
	go func() {
		unlockB := tr.Lock("/data/b")
		defer unlockB()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("locking project b blocked on project a's lock")
	}
}
