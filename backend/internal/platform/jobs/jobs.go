// Package jobs tracks progress for the long inference passes (evaluate,
// autolabel, score, reembed).
//
// A request starts a job and gets an id back immediately; the UI polls
// GET /api/jobs/{id} for {done, total, ...} to drive a progress bar and an ETA.
//
// Ported from backend/inference/job_tracker.py, deliberately unchanged in
// behaviour: a single map in this process, never pruned.
//
// ponytail: that is fine for one API instance serving a handful of people, and
// it is the same limitation the Python had. Move to Redis (or add TTL eviction)
// if this ever runs with more than one replica or sees real traffic. It was
// tempting to fix that here -- a refactor that also changes behaviour is a
// refactor nobody can debug, so it stays as it is and becomes its own piece of
// work.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Job is exactly what GET /api/jobs/{id} returns, minus the server clock the
// handler adds.
type Job struct {
	Done      int     `json:"done"`
	Total     int     `json:"total"`
	StartedAt float64 `json:"started_at"`
	Finished  bool    `json:"finished"`
	Result    any     `json:"result"`
	Error     *string `json:"error"`
}

type Tracker struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewTracker() *Tracker { return &Tracker{jobs: map[string]*Job{}} }

// newID is uuid4().hex by another name: 16 random bytes, hex encoded. Job ids
// only have to be unguessable and unique, which crypto/rand already gives --
// not worth a dependency.
func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic("jobs: no entropy for a job id: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

func (t *Tracker) Create(total int) string {
	id := newID()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.jobs[id] = &Job{
		Total: total,
		// The server's clock, so ProgressBar.tsx can compute an ETA without
		// depending on the browser's being right.
		StartedAt: float64(time.Now().UnixNano()) / 1e9,
	}
	return id
}

func (t *Tracker) Tick(id string, done int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Done = done
	}
}

func (t *Tracker) Finish(id string, result any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Finished = true
		j.Result = result
	}
}

func (t *Tracker) Fail(id string, err string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Finished = true
		j.Error = &err
	}
}

// Get returns a copy, so a poller can never read a job while another goroutine
// is halfway through updating it.
func (t *Tracker) Get(id string) (Job, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	j, ok := t.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}
