package httpapi

import (
	"net/http"
	"os"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
)

// Ground truth for the held-out test set. Deliberately separate from the pool
// handlers: these images must never touch the prompt bank, or the F1 read back
// from /api/evaluate measures memorization instead of generalization.
//
// There is no session endpoint here -- POST /api/session already bundles this
// state, since the test set is keyed by the same input_dir as the bank.

// testsetState is what all three endpoints below return, so the UI can re-render
// from any of them without a follow-up read.
func (s *Server) testsetState(r *http.Request, inputDir string, extraKey string, extra []string) (map[string]any, error) {
	imgs, err := s.Store.ListTestImages(r.Context(), inputDir)
	if err != nil {
		return nil, err
	}
	labeled, err := s.Store.LabeledStems(r.Context(), inputDir)
	if err != nil {
		return nil, err
	}
	sortStrings(labeled)
	classes, err := s.Store.Classes(r.Context(), inputDir, store.KindTestset)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"images": imgs, "labeled": labeled, "classes": classes}
	if extraKey != "" {
		out[extraKey] = extra
	}
	return out, nil
}

// checkedPaths runs every path in a list through the trust boundary.
func (s *Server) checkedPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		checked, err := s.checkedPath(p)
		if err != nil {
			return nil, err
		}
		out = append(out, checked)
	}
	return out, nil
}

// TestsetImport flags pool images as held-out test set.
//
// No file or row copy -- the pool image is the test image, just a second images
// row sharing the same path, so nothing duplicates an image byte. Importing the
// same image twice reports nothing added rather than failing.
func (s *Server) TestsetImport(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string   `json:"input_dir"`
		Images   []string `json:"images"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, _, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	paths, err := s.checkedPaths(req.Images)
	if err != nil {
		return err
	}
	added, err := s.Store.MarkTest(r.Context(), inputDir, paths)
	if err != nil {
		return err
	}
	body, err := s.testsetState(r, inputDir, "imported", added)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, body)
	return nil
}

// TestsetRemove unflags images out of the test set: the row and its ground
// truth go, the pool image itself is untouched -- there is no copy to delete.
func (s *Server) TestsetRemove(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string   `json:"input_dir"`
		Images   []string `json:"images"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, _, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	paths, err := s.checkedPaths(req.Images)
	if err != nil {
		return err
	}
	removed, err := s.Store.UnmarkTest(r.Context(), inputDir, paths)
	if err != nil {
		return err
	}
	body, err := s.testsetState(r, inputDir, "removed", removed)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, body)
	return nil
}

// TestsetLabel writes ground truth for one test-set image.
//
// No embedding extraction, no bank interaction whatsoever -- this is the one
// write path in the whole app that is fully independent of the prompt bank.
func (s *Server) TestsetLabel(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string      `json:"input_dir"`
		Image    string      `json:"image"`
		Boxes    []store.Box `json:"boxes"`
		Mode     string      `json:"mode"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, _, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	isTest, err := s.Store.IsTest(r.Context(), inputDir, req.Image)
	if err != nil {
		return err
	}
	if !isTest {
		return errStatus(http.StatusBadRequest, "this image isn't in the test set yet -- import it first")
	}
	image, err := s.checkedPath(req.Image)
	if err != nil {
		return err
	}
	if !readableImage(image) {
		return errStatus(http.StatusBadRequest, "cannot read image")
	}
	if len(req.Boxes) == 0 {
		return errStatus(http.StatusBadRequest, "no boxes")
	}
	names, err := s.Store.WriteBoxes(r.Context(), inputDir, store.KindTestset, req.Image,
		req.Boxes, nil, req.Mode == "update")
	if err != nil {
		return err
	}
	labeled, err := s.Store.LabeledStems(r.Context(), inputDir)
	if err != nil {
		return err
	}
	sortStrings(labeled)
	writeJSON(w, http.StatusOK, map[string]any{"classes": names, "labeled": labeled})
	return nil
}

// readableImage is the "can this actually be decoded" gate the Python routers
// got from cv2.imread returning None.
//
// Only the header is decoded: every caller here wants a yes/no, and none of
// them wants the pixels. That is strictly cheaper than the original, which
// decoded the whole image to answer the same question.
func readableImage(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return decodableImage(f)
}
