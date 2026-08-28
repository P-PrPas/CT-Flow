// Package claims is who is working on which image right now (FR-49).
//
// The problem it solves: the pool queue is ordered identically for everyone --
// lowest confidence first, then spread by thumbnail distance -- so two people
// opening the same project are handed the same image, both draw on it, and the
// second save replaces the first with no error and no warning. Moving label
// storage into PostgreSQL fixed concurrent *writes*; it never addressed two
// people being pointed at the same work.
//
// A claim is advice, not a lock. Nothing in the label path consults it: a save
// is never refused because someone else holds the image, because a refusal
// there would throw away work that is already drawn. All it does is keep the
// two queues from pointing at the same place.
//
// Deliberately in memory rather than in the database:
//
//   - it expires in minutes and *should* vanish on restart -- a restart means
//     nobody is holding anything;
//   - images rows are created lazily, only when something is labeled. Storing
//     claims there would grow a row for every image anybody merely opened;
//   - and it would need a sweeper for expired rows.
//
// ponytail: one API process, same as internal/platform/jobs and for the same
// reason. Two replicas would not see each other's claims -- which degrades to
// exactly today's behaviour rather than breaking anything. Move both to Redis
// together when NFR-06 is real.
package claims

import (
	"sync"
	"time"
)

// TTL is how long a claim survives without being renewed.
//
// Long enough to read an image, think, and draw a few boxes; short enough that
// someone who wandered off does not park a hard image for the afternoon. The
// cost of it being wrong is small in both directions: too short and two people
// might briefly get the same image, which is where we are today anyway; too
// long and one image sits unoffered until it expires.
//
// ponytail: a constant, not a setting. Make it configurable when someone has a
// deployment that needs a different number, not before.
const TTL = 10 * time.Minute

type claim struct {
	user string
	at   time.Time
}

// Tracker holds one claim per (project, image), and at most one image per user
// per project.
type Tracker struct {
	mu sync.Mutex
	// keyed by project (input_dir) -> image path
	byProject map[string]map[string]claim
	now       func() time.Time
}

func NewTracker() *Tracker {
	return &Tracker{byProject: map[string]map[string]claim{}, now: time.Now}
}

// Claim gives image to user, releasing whatever else they held in this project.
//
// One image per person per project is what makes a release endpoint
// unnecessary: moving to the next image is itself the release, and the only
// other way to let go -- closing the tab -- is what the TTL is for.
//
// Returns held="" on success, or the name of whoever holds it. Re-claiming your
// own image succeeds and renews it, which is what makes the frontend's periodic
// re-claim a heartbeat rather than a special case.
func (t *Tracker) Claim(project, image, user string) (held string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	images := t.byProject[project]
	if images == nil {
		images = map[string]claim{}
		t.byProject[project] = images
	}
	t.expire(images)

	if c, taken := images[image]; taken && c.user != user {
		return c.user
	}
	for path, c := range images {
		if c.user == user && path != image {
			delete(images, path)
		}
	}
	images[image] = claim{user: user, at: t.now()}
	return ""
}

// Release drops a user's claim on one image. Not needed to move between images
// -- Claim does that -- but saving an image ends the work on it, and holding it
// for another ten minutes would keep it out of the other person's queue for no
// reason.
func (t *Tracker) Release(project, image, user string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	images := t.byProject[project]
	if images == nil {
		return
	}
	if c, ok := images[image]; ok && c.user == user {
		delete(images, image)
	}
	if len(images) == 0 {
		delete(t.byProject, project)
	}
}

// Held is the live claims for one project: image path -> who holds it.
//
// Expired entries are dropped on the way past, which is the whole of the
// cleanup story: a project nobody polls holds a few stale strings until someone
// does, and a project nobody opens again holds them until the process restarts.
func (t *Tracker) Held(project string) map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	images := t.byProject[project]
	if images == nil {
		return map[string]string{}
	}
	t.expire(images)
	out := make(map[string]string, len(images))
	for path, c := range images {
		out[path] = c.user
	}
	if len(images) == 0 {
		delete(t.byProject, project)
	}
	return out
}

// expire drops timed-out claims. Callers hold the lock.
func (t *Tracker) expire(images map[string]claim) {
	cutoff := t.now().Add(-TTL)
	for path, c := range images {
		if c.at.Before(cutoff) {
			delete(images, path)
		}
	}
}
