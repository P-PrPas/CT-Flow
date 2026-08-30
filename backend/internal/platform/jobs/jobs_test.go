package jobs

import (
	"sync"
	"testing"
)

func TestLifecycle(t *testing.T) {
	tr := NewTracker()
	id := tr.Create(10)

	j, ok := tr.Get(id)
	if !ok {
		t.Fatal("a just-created job is not there")
	}
	if j.Total != 10 || j.Done != 0 || j.Finished || j.Error != nil {
		t.Fatalf("fresh job = %+v", j)
	}
	if j.StartedAt <= 0 {
		t.Errorf("StartedAt = %v, want the server clock", j.StartedAt)
	}

	tr.Tick(id, 4)
	if j, _ := tr.Get(id); j.Done != 4 || j.Finished {
		t.Fatalf("after Tick(4) = %+v", j)
	}

	tr.Finish(id, map[string]any{"written": 3})
	j, _ = tr.Get(id)
	if !j.Finished || j.Error != nil {
		t.Fatalf("after Finish = %+v", j)
	}
	if m, ok := j.Result.(map[string]any); !ok || m["written"] != 3 {
		t.Fatalf("Result = %#v", j.Result)
	}
}

func TestFailSetsErrorAndFinishes(t *testing.T) {
	tr := NewTracker()
	id := tr.Create(1)
	tr.Fail(id, "sidecar said no")

	j, _ := tr.Get(id)
	if !j.Finished || j.Error == nil || *j.Error != "sidecar said no" {
		t.Fatalf("after Fail = %+v", j)
	}
}

// Get hands back a copy: a poller reading while another goroutine updates the
// job must never see a torn value, and must not be able to mutate the tracker's
// copy by holding onto what it got.
func TestGetReturnsACopy(t *testing.T) {
	tr := NewTracker()
	id := tr.Create(5)

	j, _ := tr.Get(id)
	j.Done = 99
	j.Result = "tampered"

	if again, _ := tr.Get(id); again.Done != 0 || again.Result != nil {
		t.Fatalf("caller mutated the stored job: %+v", again)
	}
}

func TestUnknownID(t *testing.T) {
	tr := NewTracker()
	if _, ok := tr.Get("0000000000000000"); ok {
		t.Error("Get on an unknown id returned ok")
	}
	// Updates to a job that never existed are silently dropped, not a panic:
	// a late Tick from a cancelled goroutine must not take the process down.
	tr.Tick("nope", 1)
	tr.Finish("nope", nil)
	tr.Fail("nope", "x")
}

func TestIDsAreUniqueAndOpaque(t *testing.T) {
	tr := NewTracker()
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := tr.Create(0)
		if len(id) != 32 {
			t.Fatalf("id %q is %d hex chars, want 32", id, len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

// The whole reason the tracker holds a mutex: many goroutines report progress
// on the same job while pollers read it. `go test -race` is what makes this
// assertion mean something.
func TestConcurrentAccessIsRaceFree(t *testing.T) {
	tr := NewTracker()
	ids := make([]string, 20)
	for i := range ids {
		ids[i] = tr.Create(100)
	}

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(2)
		go func(id string) {
			defer wg.Done()
			for d := 1; d <= 100; d++ {
				tr.Tick(id, d)
			}
			tr.Finish(id, "done")
		}(id)
		go func(id string) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				tr.Get(id)
			}
		}(id)
	}
	wg.Wait()

	for _, id := range ids {
		if j, _ := tr.Get(id); !j.Finished || j.Done != 100 {
			t.Fatalf("job %s ended at %+v", id, j)
		}
	}
}
