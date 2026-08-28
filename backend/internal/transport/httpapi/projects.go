package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/images"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
)

// Projects is the home page's API (FR-43, FR-44, FR-50).
//
// Nothing else in this package changes shape for it: a project is looked up
// here to turn an id into an input_dir, and every other endpoint keeps taking
// the input_dir it always took. That is the whole reason Phase 2 costs one
// table's worth of columns instead of a rewrite of the transport layer
// (docs/PHASE2_WORKSPACE.md #2, decision 3).

// ListProjects is every project on this server, newest activity first. No
// filtering by owner: the UI splits "mine" from "all", but hiding other
// people's work behind an endpoint would be a permission boundary, and there
// isn't one -- anyone signed in can already reach any folder through
// GET /api/browse (docs/PHASE2_WORKSPACE.md #2, decision 5).
func (s *Server) ListProjects(w http.ResponseWriter, r *http.Request) error {
	list, err := s.Store.ListProjects(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": list})
	return nil
}

// CreateProject registers a folder as a project.
//
// The folder has to exist and hold images before this succeeds -- the same two
// checks POST /api/session has always made, applied at the point where someone
// picks the folder rather than at the point where they try to label in it.
func (s *Server) CreateProject(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name     string `json:"name"`
		InputDir string `json:"input_dir"`
		TaskType string `json:"task_type"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return errStatus(http.StatusBadRequest, "a project needs a name")
	}
	if req.TaskType != "" && req.TaskType != store.TaskDetection {
		// Storing a type no module answers to would make the project a
		// promise the app cannot keep. One value today; this rejects the rest
		// until there is a second module to honour them.
		return errStatus(http.StatusBadRequest, "unknown task type: "+req.TaskType)
	}
	inputDir, _, err := s.usableDir(req.InputDir)
	if err != nil {
		return err
	}

	p, created, err := s.Store.EnsureProject(r.Context(), inputDir, name, s.currentUser(r), req.TaskType)
	if err != nil {
		return err
	}
	if !created {
		// Never adopt silently: whoever asked to create this does not know they
		// are about to join someone else's work, and the name is how they find
		// out whose.
		return errStatus(http.StatusConflict,
			"this folder is already the project "+strconv.Quote(p.Name))
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p})
	return nil
}

// GetProject turns the id in /p/{id} into the input_dir the rest of the API
// speaks. The frontend calls it once on mount and then behaves exactly as it
// did before.
func (s *Server) GetProject(w http.ResponseWriter, r *http.Request) error {
	id, err := projectPathID(r)
	if err != nil {
		return err
	}
	p, found, err := s.Store.GetProject(r.Context(), id)
	if err != nil {
		return err
	}
	if !found {
		return errStatus(http.StatusNotFound, "no such project")
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p})
	return nil
}

// UpdateProject renames a project, and/or hands ownership of an unowned one to
// whoever asked.
//
// Ownership is take-it, not assign-it: there is no field for naming someone
// else, because a project nobody has claimed is the only case this exists for,
// and "reassign this to Bob" is a permission question Phase 2 deliberately does
// not answer.
func (s *Server) UpdateProject(w http.ResponseWriter, r *http.Request) error {
	id, err := projectPathID(r)
	if err != nil {
		return err
	}
	var req struct {
		Name           *string `json:"name"`
		ClaimOwnership bool    `json:"claim_ownership"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return errStatus(http.StatusBadRequest, "a project needs a name")
		}
		req.Name = &trimmed
	}
	var owner *string
	if req.ClaimOwnership {
		if user := s.currentUser(r); user != "" {
			owner = &user
		}
	}
	if req.Name == nil && owner == nil {
		return errStatus(http.StatusBadRequest, "nothing to update")
	}
	p, found, err := s.Store.UpdateProject(r.Context(), id, req.Name, owner)
	if err != nil {
		return err
	}
	if !found {
		return errStatus(http.StatusNotFound, "no such project")
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p})
	return nil
}

// DeleteProject removes the record of the work, not the work: the classes,
// images and annotations cascade, and nothing on disk is touched -- not the
// images, not the prompt bank in .ctflow/. The response says so explicitly so
// the UI can tell someone what they are about to keep.
//
// ponytail: this leaves the same DB/bank split docs/PHASE2_WORKSPACE.md #8
// describes, only reachable from one HTTP call instead of a forgotten reset.
// Delete a project and open the folder again and it is a new project with
// classes.idx back at 0, while _bank/embeddings.pt still holds the old class
// order and metadata.json still locks the old checkpoint -- invariant 1 in
// CLAUDE.md, broken quietly. It does not bite today because predictions come
// back as class names, never as indexes (#8 again). Clearing the bank here is
// not the fix: invariant 5 says only the sidecar may touch those two files, so
// it would need an endpoint over there, and this response promises the opposite
// anyway. The planned fix is bank_orphaned on POST /api/session (#4.3, FR-51),
// which makes the split visible where someone can act on it.
func (s *Server) DeleteProject(w http.ResponseWriter, r *http.Request) error {
	id, err := projectPathID(r)
	if err != nil {
		return err
	}
	p, found, err := s.Store.DeleteProjectByID(r.Context(), id)
	if err != nil {
		return err
	}
	if !found {
		return errStatus(http.StatusNotFound, "no such project")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":      p.ID,
		"kept_on_disk": p.InputDir,
	})
	return nil
}

// usableDir is the folder half of opening or creating a project: allowed by the
// path gate, present, and holding at least one image. Lifted out of OpenSession
// unchanged, messages included, so picking a bad folder fails identically
// whether you pick it on the home page or open it directly.
func (s *Server) usableDir(raw string) (string, []string, error) {
	dir, err := s.checkedPath(raw)
	if err != nil {
		return "", nil, err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", nil, errStatus(http.StatusBadRequest, "input dir not found: "+dir)
	}
	pool := images.List(dir)
	if len(pool) == 0 {
		return "", nil, errStatus(http.StatusBadRequest, "no images in "+dir)
	}
	return dir, pool, nil
}

// defaultProjectName is what a folder opened directly gets called until someone
// renames it. The folder's own name is the one thing already known to mean
// something to whoever picked it.
func defaultProjectName(inputDir string) string {
	if base := filepath.Base(strings.TrimRight(inputDir, `/\`)); base != "." && base != string(filepath.Separator) {
		return base
	}
	return inputDir
}

func projectPathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errStatus(http.StatusNotFound, "no such project")
	}
	return id, nil
}
