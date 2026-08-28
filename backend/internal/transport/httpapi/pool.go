package httpapi

import (
	"context"
	"net/http"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/vpe"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/config"
)

// bankSummary is BankSummary as docs/API_REFERENCE.md defines it, assembled
// from two stores because neither can see both halves: what the bank was taught
// comes from the sidecar, which images are labeled/auto comes from PostgreSQL.
type bankSummary struct {
	Classes []vpe.ClassCount `json:"classes"`
	Model   *string          `json:"model"`
	Labeled []string         `json:"labeled"`
	Auto    []string         `json:"auto"`
}

// bankSummaryCtx takes a context rather than a request because background jobs
// build a summary after the response has gone out, when the request's context is
// already cancelled.
func (s *Server) bankSummaryCtx(ctx context.Context, inputDir, stateDir string) (bankSummary, error) {
	bank, err := s.VPE.Bank(ctx, stateDir)
	if err != nil {
		return bankSummary{}, err
	}
	status, err := s.Store.ListByStatus(ctx, inputDir, store.KindPool)
	if err != nil {
		return bankSummary{}, err
	}
	return bankSummary{
		Classes: bank.Classes, Model: bank.Model,
		Labeled: status.Labeled, Auto: status.Auto,
	}, nil
}

func (s *Server) bankSummary(r *http.Request, inputDir, stateDir string) (bankSummary, error) {
	return s.bankSummaryCtx(r.Context(), inputDir, stateDir)
}

// OpenSession opens the one folder a project needs: the pool.
//
// The prompt bank lives in a fixed subfolder of it; labels and test-set
// membership live in PostgreSQL keyed by the same input_dir. Nothing else to
// browse for -- and the test-set state comes back in this same response, so the
// UI never has to make a second "did you forget the test set" round trip.
//
// It is also where a project row comes from for a folder opened directly rather
// than created on the home page. Every other write path now requires a project
// that exists (store.ErrNoProject), and this is the honest place to create one:
// opening a folder is someone saying "this is work I am doing", and it is the
// call the frontend already makes first. The row gets the folder's name and the
// opener as owner, so it is never nameless or ownerless the way get-or-create
// on every write used to leave it.
func (s *Server) OpenSession(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string `json:"input_dir"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, pool, err := s.usableDir(req.InputDir)
	if err != nil {
		return err
	}
	stateDir := config.StateDir(inputDir)
	project, _, err := s.Store.EnsureProject(r.Context(), inputDir,
		defaultProjectName(inputDir), s.currentUser(r), store.TaskDetection)
	if err != nil {
		return err
	}

	bank, err := s.bankSummary(r, inputDir, stateDir)
	if err != nil {
		return err
	}
	testImages, err := s.Store.ListTestImages(r.Context(), inputDir)
	if err != nil {
		return err
	}
	labeled, err := s.Store.LabeledStems(r.Context(), inputDir)
	if err != nil {
		return err
	}
	sortStrings(labeled)
	classes, err := s.Store.Classes(r.Context(), inputDir, store.KindTestset)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"images": pool,
		"bank":   bank,
		"testset": map[string]any{
			"images": testImages, "labeled": labeled, "classes": classes,
		},
		// So a client that opened a folder directly can still link to it, and
		// so the UI has the name and owner without a second call.
		"project": project,
	})
	return nil
}

// GetBoxes returns the boxes already saved for one image, so revisiting it
// shows what is there instead of a blank canvas.
func (s *Server) GetBoxes(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	inputDir, _, err := s.stateDirFor(q.Get("input_dir"))
	if err != nil {
		return err
	}
	image, err := s.checkedPath(q.Get("image"))
	if err != nil {
		return err
	}
	// The wire name is "test", the storage name is "testset" -- the frontend has
	// always sent the former.
	kind := store.KindPool
	if q.Get("kind") == "test" {
		kind = store.KindTestset
	}
	boxes, err := s.Store.ReadBoxes(r.Context(), inputDir, kind, image)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"boxes": boxes})
	return nil
}
