package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// A write against a folder nobody created a project for must fail, and fail
// with something the transport layer can turn into a 404.
//
// This is the invariant the whole Phase 2 ownership story rests on: before it,
// every write get-or-created the project on the way past, so a typo'd path
// silently became a nameless, ownerless project that then showed up on the home
// page as work nobody could account for.
func TestWritesRequireAProject(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	orphan := fmt.Sprintf("/tmp/gostore-test/%s-never-created", t.Name())

	if _, err := s.WriteBoxes(ctx, orphan, KindPool, "/img/a.jpg",
		[]Box{{Cls: "widget", Box: [4]float64{1, 1, 2, 2}}}, nil, false); !errors.Is(err, ErrNoProject) {
		t.Errorf("WriteBoxes err = %v, want ErrNoProject", err)
	}
	if err := s.MarkLabeled(ctx, orphan, KindPool, "/img/a.jpg"); !errors.Is(err, ErrNoProject) {
		t.Errorf("MarkLabeled err = %v, want ErrNoProject", err)
	}
	if err := s.MarkAuto(ctx, orphan, []string{"/img/a.jpg"}); !errors.Is(err, ErrNoProject) {
		t.Errorf("MarkAuto err = %v, want ErrNoProject", err)
	}
	if _, err := s.MarkTest(ctx, orphan, []string{"/img/a.jpg"}); !errors.Is(err, ErrNoProject) {
		t.Errorf("MarkTest err = %v, want ErrNoProject", err)
	}

	// Reads stay forgiving: a folder with no project is empty, not an error.
	// Every reader answered that way before Phase 2 and still has to, or the
	// home page cannot render a project that has never been labeled.
	if names, err := s.Classes(ctx, orphan, KindPool); err != nil || len(names) != 0 {
		t.Errorf("Classes on a folder with no project = %v, %v; want empty, nil", names, err)
	}
}

// EnsureProject is create-or-adopt, and the caller has to be able to tell which
// happened: POST /api/projects refuses a duplicate, POST /api/session adopts it.
func TestEnsureProjectReportsWhetherItCreated(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	dir := fmt.Sprintf("/tmp/gostore-test/%s-dir", t.Name())
	t.Cleanup(func() { _ = s.DeleteProject(context.Background(), dir) })

	p, created, err := s.EnsureProject(ctx, dir, "First name", "sub-alice", TaskDetection)
	if err != nil || !created {
		t.Fatalf("first EnsureProject: created=%v err=%v", created, err)
	}
	if p.Name != "First name" || p.TaskType != TaskDetection {
		t.Errorf("project = %+v, want name/task from the create call", p)
	}
	if p.Owner == nil || p.Owner.OID != "sub-alice" {
		t.Errorf("owner = %+v, want sub-alice", p.Owner)
	}

	// Adopting must not rewrite the name or hand the project to whoever opened
	// it second -- that would silently take work away from its owner.
	again, created, err := s.EnsureProject(ctx, dir, "Second name", "sub-bob", TaskDetection)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("second EnsureProject reported created=true for an existing folder")
	}
	if again.ID != p.ID || again.Name != "First name" {
		t.Errorf("adopted project = %+v, want the original id and name", again)
	}
	if again.Owner == nil || again.Owner.OID != "sub-alice" {
		t.Errorf("adopted owner = %+v, want the original owner", again.Owner)
	}
}

// The home page's numbers come from the database, and "who worked on this" is
// derived from annotations rather than from a membership list -- so it has to
// count what was actually written, by whom.
func TestListProjectsCountsAndContributors(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()
	alice, bob := "sub-alice", "sub-bob"

	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/a.jpg",
		[]Box{{Cls: "widget", Box: [4]float64{1, 1, 2, 2}}}, &alice, false); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkLabeled(ctx, dir, KindPool, "/img/a.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/b.jpg", []Box{
		{Cls: "widget", Box: [4]float64{1, 1, 2, 2}},
		{Cls: "widget", Box: [4]float64{3, 3, 4, 4}},
	}, &bob, false); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkAuto(ctx, dir, []string{"/img/c.jpg"}); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got *Project
	for i := range list {
		if list[i].InputDir == dir {
			got = &list[i]
		}
	}
	if got == nil {
		t.Fatalf("project %s missing from ListProjects", dir)
	}
	if got.Labeled != 1 || got.Auto != 1 {
		t.Errorf("labeled=%d auto=%d, want 1 and 1", got.Labeled, got.Auto)
	}
	// Ordered by how much each person wrote: bob's two boxes outrank alice's one.
	if len(got.Members) != 2 {
		t.Fatalf("contributors = %+v, want two", got.Members)
	}
	if got.Members[0].OID != bob || got.Members[0].Boxes != 2 {
		t.Errorf("top contributor = %+v, want bob with 2 boxes", got.Members[0])
	}
	if got.Members[1].OID != alice || got.Members[1].Boxes != 1 {
		t.Errorf("second contributor = %+v, want alice with 1 box", got.Members[1])
	}
	// No users row for these subjects, so the `sub` itself is the best name
	// available -- an unreadable contributor still beats a missing one.
	if got.Members[0].Name != bob {
		t.Errorf("name for a sub with no users row = %q, want the sub", got.Members[0].Name)
	}
}

// Deleting a project drops its rows and nothing else. The handler promises the
// files stay; this is the half of that promise the store can be held to.
func TestDeleteProjectByIDRemovesOnlyTheRows(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()

	if _, err := s.WriteBoxes(ctx, dir, KindPool, "/img/a.jpg",
		[]Box{{Cls: "widget", Box: [4]float64{1, 1, 2, 2}}}, nil, false); err != nil {
		t.Fatal(err)
	}
	p, found, err := s.GetProjectByDir(ctx, dir)
	if err != nil || !found {
		t.Fatalf("GetProjectByDir: found=%v err=%v", found, err)
	}

	deleted, found, err := s.DeleteProjectByID(ctx, p.ID)
	if err != nil || !found {
		t.Fatalf("DeleteProjectByID: found=%v err=%v", found, err)
	}
	if deleted.InputDir != dir {
		t.Errorf("deleted.InputDir = %q, want %q -- the caller needs it to say what stayed on disk", deleted.InputDir, dir)
	}
	if _, found, err := s.GetProject(ctx, p.ID); err != nil || found {
		t.Errorf("project still resolvable after delete: found=%v err=%v", found, err)
	}
	if _, found, err := s.DeleteProjectByID(ctx, p.ID); err != nil || found {
		t.Errorf("deleting twice: found=%v err=%v, want found=false and no error", found, err)
	}
}

// Claiming fills an empty owner and never replaces one. The endpoint has no
// field for naming someone else, so if this ever regresses, "claim" quietly
// becomes "take" and one PATCH hands anyone someone else's work.
func TestClaimingCannotTakeAnOwnedProject(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	dir := fmt.Sprintf("/tmp/gostore-test/%s-dir", t.Name())
	t.Cleanup(func() { _ = s.DeleteProject(context.Background(), dir) })

	p, _, err := s.EnsureProject(ctx, dir, "Alice's work", "sub-alice", TaskDetection)
	if err != nil {
		t.Fatal(err)
	}
	bob := "sub-bob"
	got, found, err := s.UpdateProject(ctx, p.ID, nil, &bob)
	if err != nil || !found {
		t.Fatalf("UpdateProject: found=%v err=%v", found, err)
	}
	if got.Owner == nil || got.Owner.OID != "sub-alice" {
		t.Errorf("owner = %+v after bob claimed it, want sub-alice", got.Owner)
	}
}

// Renaming must not clear an owner, and claiming must not rename -- the two
// fields travel in the same request and COALESCE is what keeps them independent.
func TestUpdateProjectLeavesOmittedFieldsAlone(t *testing.T) {
	s, dir := open(t)
	ctx := context.Background()
	p, found, err := s.GetProjectByDir(ctx, dir)
	if err != nil || !found {
		t.Fatalf("GetProjectByDir: found=%v err=%v", found, err)
	}
	owner := "sub-alice"
	if _, _, err := s.UpdateProject(ctx, p.ID, nil, &owner); err != nil {
		t.Fatal(err)
	}
	renamed := "Renamed"
	got, found, err := s.UpdateProject(ctx, p.ID, &renamed, nil)
	if err != nil || !found {
		t.Fatalf("UpdateProject: found=%v err=%v", found, err)
	}
	if got.Name != renamed {
		t.Errorf("name = %q, want %q", got.Name, renamed)
	}
	if got.Owner == nil || got.Owner.OID != owner {
		t.Errorf("owner = %+v after a rename, want it untouched", got.Owner)
	}
	if _, found, err := s.UpdateProject(ctx, p.ID+1_000_000, &renamed, nil); err != nil || found {
		t.Errorf("updating a missing project: found=%v err=%v, want found=false and no error", found, err)
	}
}
