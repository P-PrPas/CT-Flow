package api

import (
	"net/http"
	"os"

	"github.com/P-PrPas/CT-Flow/internal/images"
	"github.com/P-PrPas/CT-Flow/internal/store"
	"github.com/P-PrPas/CT-Flow/internal/vpe"
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

func (s *Server) bankSummary(r *http.Request, inputDir, stateDir string) (bankSummary, error) {
	bank, err := s.VPE.Bank(r.Context(), stateDir)
	if err != nil {
		return bankSummary{}, err
	}
	status, err := s.Store.ListByStatus(r.Context(), inputDir, store.KindPool)
	if err != nil {
		return bankSummary{}, err
	}
	return bankSummary{
		Classes: bank.Classes, Model: bank.Model,
		Labeled: status.Labeled, Auto: status.Auto,
	}, nil
}

// OpenSession opens the one folder a project needs: the pool.
//
// The prompt bank lives in a fixed subfolder of it; labels and test-set
// membership live in PostgreSQL keyed by the same input_dir. Nothing else to
// browse for -- and the test-set state comes back in this same response, so the
// UI never has to make a second "did you forget the test set" round trip.
func (s *Server) OpenSession(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string `json:"input_dir"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(inputDir); err != nil || !info.IsDir() {
		return errStatus(http.StatusBadRequest, "input dir not found: "+inputDir)
	}
	pool := images.List(inputDir)
	if len(pool) == 0 {
		return errStatus(http.StatusBadRequest, "no images in "+inputDir)
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
