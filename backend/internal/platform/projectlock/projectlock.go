// Package projectlock serializes one project's Teach-then-write critical
// section across concurrent requests in this process, so the bank
// (ordered by /vpe/teach arrival) and the database (ordered by DB commit)
// can never observe a new class in a different order (CLAUDE.md invariant
// #1). Two people teaching different new classes to the same project at
// the same moment is exactly the case this exists for.
//
// ponytail: one API process, in-memory, same tradeoff as
// internal/platform/claims -- move together if NFR-06 is ever real. Never
// evicts an entry once a project has been locked once; bounded by the
// number of distinct projects ever opened in this process's lifetime, not
// a per-request leak.
package projectlock

import "sync"

type Tracker struct {
	mu        sync.Mutex
	byProject map[string]*sync.Mutex
}

func NewTracker() *Tracker { return &Tracker{byProject: map[string]*sync.Mutex{}} }

// Lock blocks until this project's critical section is free, and returns
// the func that releases it.
func (t *Tracker) Lock(project string) func() {
	t.mu.Lock()
	m, ok := t.byProject[project]
	if !ok {
		m = &sync.Mutex{}
		t.byProject[project] = m
	}
	t.mu.Unlock()
	m.Lock()
	return m.Unlock
}
