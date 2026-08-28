package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/P-PrPas/CT-Flow/backend/internal/testsupport"
)

// These run against a real PostgreSQL, not a fake: the invariant this package
// exists to protect (see getOrCreateClass) is a property of row locking, and a
// mock cannot have it.
func open(t *testing.T) (*Store, string) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set -- these tests need a real PostgreSQL")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := os.Getenv("SCHEMA_PATH")
	if schema == "" {
		schema = testsupport.MustBackendFile("db/schema.sql")
	}
	if err := s.InitSchema(ctx, schema); err != nil {
		t.Fatal(err)
	}
	// A project key unique to this test, so a failed run cannot poison the next.
	dir := fmt.Sprintf("/tmp/gostore-test/%s", t.Name())
	if err := s.DeleteProject(ctx, dir); err != nil {
		t.Fatal(err)
	}
	// Writes require a project that already exists (ErrNoProject) -- creating
	// one here is what a real request does before it labels anything, through
	// POST /api/projects or POST /api/session.
	if _, _, err := s.EnsureProject(ctx, dir, t.Name(), "", TaskDetection); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.DeleteProject(context.Background(), dir)
		s.Close()
	})
	return s, dir
}

// The invariant the whole move to PostgreSQL was for: a label's class column is
// a position in this list, so an index must never shift once assigned -- not
// even when a new name sorts before an existing one.
func TestClassIndexIsAppendOnly(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/a.jpg",
		[]Box{{Cls: "test_item", Box: [4]float64{1, 1, 2, 2}}}, nil, false); err != nil {
		t.Fatal(err)
	}
	// "aaa_new_class" sorts first alphabetically and must still land second.
	names, err := s.WriteBoxes(ctx, dir, KindPool, "/img/b.jpg",
		[]Box{{Cls: "aaa_new_class", Box: [4]float64{1, 1, 2, 2}}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"test_item", "aaa_new_class"}
	if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
		t.Fatalf("classes = %v, want %v (insertion order, never sorted)", names, want)
	}
}

// Pool and test set are separate index spaces, so the same name in both is two
// rows with two different indices.
func TestPoolAndTestsetHaveSeparateClassSpaces(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/a.jpg",
		[]Box{{Cls: "widget"}, {Cls: "gadget"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteBoxes(ctx, dir, KindTestset, "/img/b.jpg",
		[]Box{{Cls: "gadget"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	pool, err := s.Classes(ctx, dir, KindPool)
	if err != nil {
		t.Fatal(err)
	}
	test, err := s.Classes(ctx, dir, KindTestset)
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 2 || len(test) != 1 || test[0] != "gadget" {
		t.Errorf("pool=%v testset=%v -- the two index spaces are not independent", pool, test)
	}
}

// Two writers teaching two different new class names at the same moment must
// get different indices: a label's class column is a position in the list, so a
// collision silently mislabels everything written afterwards.
//
// Note what this does and does not prove. It passes with the FOR UPDATE in
// getOrCreateClass removed, because requireProject's UPDATE already touches
// the projects row and holds that row lock until commit -- every writer to one
// project is serialised before it ever reaches the class lookup. The explicit
// lock is kept anyway: it is what the Python does, it is what makes the
// guarantee local to the function that needs it rather than an accident of an
// unrelated upsert, and the UNIQUE (project_id, kind, idx) constraint is the
// backstop that turns any remaining race into an error instead of corruption.
//
// So this is an end-to-end check of the invariant, not an isolation test of the
// lock.
func TestConcurrentNewClassesGetDistinctIndices(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.WriteBoxes(ctx, dir, KindPool, fmt.Sprintf("/img/%d.jpg", i),
				[]Box{{Cls: fmt.Sprintf("class_%d", i), Box: [4]float64{1, 1, 2, 2}}}, nil, false)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	names, err := s.Classes(ctx, dir, KindPool)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != writers {
		t.Fatalf("got %d classes from %d concurrent writers, want %d: %v",
			len(names), writers, writers, names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("duplicate class %q", n)
		}
		seen[n] = true
	}
}

func TestWriteBoxesReplaceAndMerge(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()
	const img = "/img/a.jpg"

	first := []Box{{Cls: "widget", Box: [4]float64{10, 10, 20, 20}}}
	if _, err := s.WriteBoxes(ctx, dir, KindPool, img, first, nil, false); err != nil {
		t.Fatal(err)
	}

	// merge=true keeps what was already there and adds to it.
	if _, err := s.WriteBoxes(ctx, dir, KindPool, img,
		[]Box{{Cls: "widget", Box: [4]float64{30, 30, 40, 40}}}, nil, true); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadBoxes(ctx, dir, KindPool, img)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Box[0] != 10 || got[1].Box[0] != 30 {
		t.Fatalf("after merge: %v, want the original box then the new one", got)
	}

	// merge=false (the default) fully replaces.
	if _, err := s.WriteBoxes(ctx, dir, KindPool, img,
		[]Box{{Cls: "widget", Box: [4]float64{5, 5, 6, 6}}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if got, err = s.ReadBoxes(ctx, dir, KindPool, img); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Box[0] != 5 {
		t.Fatalf("after replace: %v, want only the new box", got)
	}

	// An empty write is legitimate -- "the model was wrong about everything
	// here" -- and must clear the image rather than being ignored.
	if _, err := s.WriteBoxes(ctx, dir, KindPool, img, []Box{}, nil, false); err != nil {
		t.Fatal(err)
	}
	if got, err = s.ReadBoxes(ctx, dir, KindPool, img); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("after an empty write: %v, want none", got)
	}
}

// WriteBoxes must not touch status: relabel writes boxes without claiming the
// image was hand-labeled, and that distinction drives the whole review flow.
func TestStatusTransitions(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/a.jpg",
		[]Box{{Cls: "widget"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	lists, err := s.ListByStatus(ctx, dir, KindPool)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists.Labeled) != 0 || len(lists.Auto) != 0 {
		t.Errorf("writing boxes alone changed status: %+v", lists)
	}

	if err := s.MarkLabeled(ctx, dir, KindPool, "/img/a.jpg"); err != nil {
		t.Fatal(err)
	}
	// MarkAuto must never downgrade a hand-labeled image, and must set an
	// untouched one.
	if err := s.MarkAuto(ctx, dir, []string{"/img/a.jpg", "/img/b.jpg"}); err != nil {
		t.Fatal(err)
	}
	if lists, err = s.ListByStatus(ctx, dir, KindPool); err != nil {
		t.Fatal(err)
	}
	if len(lists.Labeled) != 1 || lists.Labeled[0] != "/img/a.jpg" {
		t.Errorf("labeled = %v, want the hand-labeled image only -- MarkAuto downgraded it", lists.Labeled)
	}
	if len(lists.Auto) != 1 || lists.Auto[0] != "/img/b.jpg" {
		t.Errorf("auto = %v, want the untouched image only", lists.Auto)
	}
}

func TestTestsetMembership(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()
	const img = "/img/held-out.jpg"

	if is, err := s.IsTest(ctx, dir, img); err != nil || is {
		t.Fatalf("IsTest before import = %v (err %v), want false", is, err)
	}
	added, err := s.MarkTest(ctx, dir, []string{img})
	if err != nil || len(added) != 1 {
		t.Fatalf("MarkTest = %v (err %v), want one added", added, err)
	}
	// Importing the same image again is a no-op, not a duplicate row.
	if added, err = s.MarkTest(ctx, dir, []string{img}); err != nil || len(added) != 0 {
		t.Fatalf("second MarkTest = %v (err %v), want nothing added", added, err)
	}
	if is, err := s.IsTest(ctx, dir, img); err != nil || !is {
		t.Fatalf("IsTest after import = %v (err %v), want true", is, err)
	}

	if _, err := s.WriteBoxes(ctx, dir, KindTestset, img,
		[]Box{{Cls: "widget", Box: [4]float64{1, 1, 2, 2}}}, nil, false); err != nil {
		t.Fatal(err)
	}
	stems, err := s.LabeledStems(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(stems) != 1 || stems[0] != "held-out" {
		t.Errorf("labeled stems = %v, want [held-out]", stems)
	}

	// Removing takes the ground truth with it (ON DELETE CASCADE)...
	removed, err := s.UnmarkTest(ctx, dir, []string{img})
	if err != nil || len(removed) != 1 {
		t.Fatalf("UnmarkTest = %v (err %v), want one removed", removed, err)
	}
	if boxes, err := s.ReadBoxes(ctx, dir, KindTestset, img); err != nil || len(boxes) != 0 {
		t.Errorf("ground truth survived removal: %v (err %v)", boxes, err)
	}
	// ...and removing again reports nothing, rather than failing.
	if removed, err = s.UnmarkTest(ctx, dir, []string{img}); err != nil || len(removed) != 0 {
		t.Fatalf("second UnmarkTest = %v (err %v), want nothing removed", removed, err)
	}
}

// A pool image and its test-set counterpart are two rows sharing one path, and
// writing to one must not disturb the other -- there is no file copy, so this
// separation is the only thing keeping ground truth out of the prompt bank's
// half of the project.
func TestPoolAndTestsetRowsAreIndependent(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()
	const img = "/img/shared.jpg"

	if _, err := s.WriteBoxes(ctx, dir, KindPool, img,
		[]Box{{Cls: "widget", Box: [4]float64{1, 1, 2, 2}}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkTest(ctx, dir, []string{img}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteBoxes(ctx, dir, KindTestset, img,
		[]Box{{Cls: "widget", Box: [4]float64{9, 9, 9, 9}}, {Cls: "widget", Box: [4]float64{8, 8, 8, 8}}},
		nil, false); err != nil {
		t.Fatal(err)
	}

	pool, err := s.ReadBoxes(ctx, dir, KindPool, img)
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 1 || pool[0].Box[0] != 1 {
		t.Errorf("pool boxes = %v, want the one written to the pool row", pool)
	}
	test, err := s.ReadBoxes(ctx, dir, KindTestset, img)
	if err != nil {
		t.Fatal(err)
	}
	if len(test) != 2 {
		t.Errorf("testset boxes = %v, want the two written to the testset row", test)
	}
}

func TestLoadAnnotationsSkipsEmptyImages(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/has.jpg",
		[]Box{{Cls: "widget", Box: [4]float64{1, 2, 3, 4}}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/none.jpg", []Box{}, nil, false); err != nil {
		t.Fatal(err)
	}
	all, err := s.LoadAnnotations(ctx, dir, KindPool)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("loaded %d images, want only the one with boxes: %v", len(all), all)
	}
	if got := all["/img/has.jpg"]; len(got) != 1 || got[0].Box != [4]float64{1, 2, 3, 4} {
		t.Errorf("boxes = %v, want the pixel coords as written (never normalised)", got)
	}
}

// An unknown project reads as empty rather than erroring, and reading must not
// be what creates it.
func TestUnknownProjectReadsEmpty(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	const missing = "/tmp/gostore-test/never-created"
	t.Cleanup(func() { _ = s.DeleteProject(context.Background(), missing) })

	if names, err := s.Classes(ctx, missing, KindPool); err != nil || len(names) != 0 {
		t.Errorf("classes = %v (err %v), want none", names, err)
	}
	if boxes, err := s.ReadBoxes(ctx, missing, KindPool, "/img/a.jpg"); err != nil || len(boxes) != 0 {
		t.Errorf("boxes = %v (err %v), want none", boxes, err)
	}
	if err := s.tx(ctx, func(q pgx.Tx) error {
		if _, found, err := projectID(ctx, q, missing); err != nil || found {
			t.Errorf("reading created the project row (found=%v, err=%v)", found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Reads must not queue behind a writer holding the project row.
//
// requireProject's UPDATE locks that row for the rest of its transaction,
// which is fine for the writes that need it and ruinous for the reads that do
// not: with reads going through it too, one export of a big project blocked
// every label save for that project until it finished. The Python this was
// ported from did not have the problem, because its get_or_create_project ran
// on its own connection and committed immediately.
//
// So: hold a write open, and assert a read still answers.
func TestReadsDoNotBlockOnAWriter(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/a.jpg",
		[]Box{{Cls: "widget", Box: [4]float64{1, 1, 2, 2}}}, nil, false); err != nil {
		t.Fatal(err)
	}

	// A writer mid-transaction, holding the project row exactly as a real
	// /api/label does between resolving the project and committing.
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.tx(ctx, func(q pgx.Tx) error {
			if _, err := requireProject(ctx, q, dir); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()

	// Generous enough that a slow CI box is not what fails this, short enough
	// that a genuine block does. A read that waits for the writer times out here.
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	names, err := s.Classes(readCtx, dir, KindPool)
	if err != nil {
		t.Fatalf("reading while a writer holds the project row: %v", err)
	}
	if len(names) != 1 || names[0] != "widget" {
		t.Errorf("classes = %v, want [widget]", names)
	}
}
