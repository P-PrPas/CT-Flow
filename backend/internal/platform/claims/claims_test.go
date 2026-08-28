package claims

import (
	"sync"
	"testing"
	"time"
)

// A fake clock, so the TTL is tested by moving time rather than by waiting ten
// minutes -- and so the boundary is checked exactly rather than approximately.
func at(t *testing.T, start time.Time) (*Tracker, func(time.Duration)) {
	t.Helper()
	tr := NewTracker()
	now := start
	tr.now = func() time.Time { return now }
	return tr, func(d time.Duration) { now = now.Add(d) }
}

// The point of the whole package: two people in one project must not be handed
// the same image.
func TestASecondPersonCannotTakeAHeldImage(t *testing.T) {
	tr, _ := at(t, time.Now())

	if held := tr.Claim("/data/pool", "/data/pool/a.jpg", "alice"); held != "" {
		t.Fatalf("alice could not claim a free image: held by %q", held)
	}
	if held := tr.Claim("/data/pool", "/data/pool/a.jpg", "bob"); held != "alice" {
		t.Errorf("bob claiming alice's image = %q, want alice", held)
	}
	// And bob is not blocked from the rest of the project.
	if held := tr.Claim("/data/pool", "/data/pool/b.jpg", "bob"); held != "" {
		t.Errorf("bob could not claim a different image: held by %q", held)
	}
	if got := tr.Held("/data/pool"); len(got) != 2 ||
		got["/data/pool/a.jpg"] != "alice" || got["/data/pool/b.jpg"] != "bob" {
		t.Errorf("held = %v", got)
	}
}

// Re-claiming your own image has to succeed, or the frontend's periodic
// re-claim would start reporting a conflict with itself.
func TestReclaimingYourOwnImageRenewsIt(t *testing.T) {
	tr, advance := at(t, time.Now())
	tr.Claim("/data/pool", "/data/pool/a.jpg", "alice")

	advance(TTL - time.Minute)
	if held := tr.Claim("/data/pool", "/data/pool/a.jpg", "alice"); held != "" {
		t.Fatalf("alice could not renew her own claim: held by %q", held)
	}
	// Renewed, so the original deadline no longer applies.
	advance(2 * time.Minute)
	if got := tr.Held("/data/pool"); got["/data/pool/a.jpg"] != "alice" {
		t.Errorf("claim expired despite being renewed: %v", got)
	}
}

// One image per person per project is what makes a release endpoint
// unnecessary: moving on is the release.
func TestClaimingAnotherImageReleasesTheFirst(t *testing.T) {
	tr, _ := at(t, time.Now())
	tr.Claim("/data/pool", "/data/pool/a.jpg", "alice")
	tr.Claim("/data/pool", "/data/pool/b.jpg", "alice")

	got := tr.Held("/data/pool")
	if len(got) != 1 || got["/data/pool/b.jpg"] != "alice" {
		t.Errorf("held = %v, want only b.jpg", got)
	}
	// So the image she left is available to someone else immediately.
	if held := tr.Claim("/data/pool", "/data/pool/a.jpg", "bob"); held != "" {
		t.Errorf("bob could not take the image alice moved off: held by %q", held)
	}
}

// Without expiry, one closed tab parks an image forever.
func TestAClaimExpires(t *testing.T) {
	tr, advance := at(t, time.Now())
	tr.Claim("/data/pool", "/data/pool/a.jpg", "alice")

	advance(TTL - time.Second)
	if got := tr.Held("/data/pool"); got["/data/pool/a.jpg"] != "alice" {
		t.Fatalf("claim vanished before the TTL: %v", got)
	}
	advance(2 * time.Second)
	if got := tr.Held("/data/pool"); len(got) != 0 {
		t.Errorf("held after the TTL = %v, want empty", got)
	}
	if held := tr.Claim("/data/pool", "/data/pool/a.jpg", "bob"); held != "" {
		t.Errorf("bob could not take an expired claim: held by %q", held)
	}
}

// Two projects are two independent queues -- a shared map keyed only by image
// path would have them stepping on each other.
func TestProjectsDoNotShareClaims(t *testing.T) {
	tr, _ := at(t, time.Now())
	tr.Claim("/data/one", "/img/a.jpg", "alice")

	if held := tr.Claim("/data/two", "/img/a.jpg", "bob"); held != "" {
		t.Errorf("the same path in another project was held: %q", held)
	}
	if got := tr.Held("/data/one"); got["/img/a.jpg"] != "alice" {
		t.Errorf("project one = %v", got)
	}
	if got := tr.Held("/data/two"); got["/img/a.jpg"] != "bob" {
		t.Errorf("project two = %v", got)
	}
}

// Releasing is what a save does, so the image is offered to the other person
// straight away instead of sitting out the rest of the TTL.
func TestReleaseFreesTheImageAndOnlyForItsHolder(t *testing.T) {
	tr, _ := at(t, time.Now())
	tr.Claim("/data/pool", "/img/a.jpg", "alice")

	tr.Release("/data/pool", "/img/a.jpg", "bob") // not his to release
	if got := tr.Held("/data/pool"); got["/img/a.jpg"] != "alice" {
		t.Errorf("bob released alice's claim: %v", got)
	}
	tr.Release("/data/pool", "/img/a.jpg", "alice")
	if got := tr.Held("/data/pool"); len(got) != 0 {
		t.Errorf("held after release = %v, want empty", got)
	}
}

// Every exported method takes the lock; this fails under -race if one stops.
func TestConcurrentUse(t *testing.T) {
	tr := NewTracker()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				tr.Claim("/data/pool", "/img/a.jpg", string(rune('a'+n)))
				tr.Held("/data/pool")
				tr.Release("/data/pool", "/img/a.jpg", string(rune('a'+n)))
			}
		}(i)
	}
	wg.Wait()
}
