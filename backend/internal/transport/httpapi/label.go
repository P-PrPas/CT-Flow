package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/vpe"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/config"
)

// testSetRefusal is the same sentence for /api/label and /api/relabel. Teaching
// the bank from a held-out image would make /api/evaluate report memorization
// instead of generalization, so it is refused at the endpoint rather than left
// to the UI.
const testSetRefusal = "this image is in the benchmark set -- it can never be taught to the model"

// maxClassNameLen is a sanity cap, not a real limit anyone should hit.
// ponytail: 100 chars, raise it the day a real class name needs more.
const maxClassNameLen = 100

// validateBoxes is every write path's trust boundary for box content, called
// before a box reaches the bank or the database. A bad class name is
// permanent: class indexes are append-only (CLAUDE.md invariant #1), so an
// empty or newline-containing name silently shifts every index after it in a
// classes.txt-style reader (B-02). A malformed box shape is silent too, just
// downstream instead: a reversed or negative box turns into a YOLO normalized
// value outside [0,1] or a negative COCO width with no error anywhere (M-05).
func validateBoxes(boxes []store.Box) error {
	for _, b := range boxes {
		if strings.TrimSpace(b.Cls) == "" {
			return errStatus(http.StatusBadRequest, "class name cannot be empty")
		}
		if strings.ContainsAny(b.Cls, "\n\r\x00") {
			return errStatus(http.StatusBadRequest, "class name cannot contain a newline or a NUL byte")
		}
		if utf8.RuneCountInString(b.Cls) > maxClassNameLen {
			return errStatus(http.StatusBadRequest,
				fmt.Sprintf("class name longer than %d characters", maxClassNameLen))
		}
		x1, y1, x2, y2 := b.Box[0], b.Box[1], b.Box[2], b.Box[3]
		if x2 <= x1 || y2 <= y1 {
			return errStatus(http.StatusBadRequest, "box has zero or negative area")
		}
		if x1 < 0 || y1 < 0 {
			return errStatus(http.StatusBadRequest, "box coordinates cannot be negative")
		}
	}
	return nil
}

// boxesWithinImage is the bounds check validateBoxes cannot do on its own --
// it needs the image's actual pixel size, which only the caller can read.
func boxesWithinImage(boxes []store.Box, w, h int) error {
	for _, b := range boxes {
		if b.Box[2] > float64(w) || b.Box[3] > float64(h) {
			return errStatus(http.StatusBadRequest, "box extends outside the image")
		}
	}
	return nil
}

// SaveLabel extracts embeddings for these boxes into the prompt bank, then
// writes the label into PostgreSQL.
//
// The order is load-bearing. The test-set check and the model lock both refuse
// before any inference runs -- a mismatched model_id costs a 409, not a wasted
// checkpoint load -- and the bank is taught before the database is written.
// Those two stores have never shared a transaction, so a failure between them
// still leaves an embedding with no annotation row: the same exposure as
// before, deliberately not widened here.
func (s *Server) SaveLabel(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string      `json:"input_dir"`
		Image    string      `json:"image"`
		Boxes    []store.Box `json:"boxes"`
		ModelID  string      `json:"model_id"`
		Mode     string      `json:"mode"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	if len(req.Boxes) == 0 {
		return errStatus(http.StatusBadRequest, "no boxes")
	}
	if err := validateBoxes(req.Boxes); err != nil {
		return err
	}
	// The trust boundary comes before anything that uses the path, the database
	// included: a rejected path should not be able to reach a query at all, and
	// a path outside the root should answer 403 rather than whatever the test-set
	// check happens to say about it.
	image, err := s.checkedPath(req.Image)
	if err != nil {
		return err
	}
	isTest, err := s.Store.IsTest(r.Context(), inputDir, req.Image)
	if err != nil {
		return err
	}
	if isTest {
		return errStatus(http.StatusBadRequest, testSetRefusal)
	}
	if iw, ih, ok := s.imageDims(image); ok {
		if err := boxesWithinImage(req.Boxes, iw, ih); err != nil {
			return err
		}
	}

	var user *string
	if u := s.currentUser(r); u != "" {
		user = &u // FR-31: the signed-in name lands on each instance taught here
	}
	unlock := s.ProjectLock.Lock(inputDir)
	defer unlock()
	if _, err := s.VPE.Teach(r.Context(), stateDir, image, toVPEBoxes(req.Boxes),
		req.ModelID, user); err != nil {
		return err
	}

	if _, err := s.Store.WriteBoxes(r.Context(), inputDir, store.KindPool, req.Image,
		req.Boxes, user, req.Mode == "update"); err != nil {
		return err
	}
	if err := s.Store.MarkLabeled(r.Context(), inputDir, store.KindPool, req.Image); err != nil {
		return err
	}
	// Saving is finishing, so stop holding the image (FR-49). Without this the
	// claim would sit out the rest of its TTL keeping a done image out of the
	// other person's queue. Never a reason to fail the save -- the label is
	// already written, and the worst case is a claim that expires on its own.
	s.Claims.Release(inputDir, image, deref(user))
	return s.writeBankSummary(w, r, inputDir, stateDir)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Relabel rewrites this image's label directly -- no embedding extraction, no
// bank write.
//
// For fixing generated labels (delete an over-prediction, drag in a box the
// model missed) without the correction being treated as a new visual prompt.
// Empty boxes are legitimate: "the model was wrong about everything here".
//
// It does mark the image 'labeled': a human has now decided its boxes, so it is
// no longer a raw machine guess. Without this a review pass over the
// auto-labeled set never shrinks that set -- "next unreviewed" keeps landing on
// the first one -- and the gallery goes on calling a checked image "by model".
func (s *Server) Relabel(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string      `json:"input_dir"`
		Image    string      `json:"image"`
		Boxes    []store.Box `json:"boxes"`
		Mode     string      `json:"mode"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	image, err := s.checkedPath(req.Image)
	if err != nil {
		return err
	}
	isTest, err := s.Store.IsTest(r.Context(), inputDir, req.Image)
	if err != nil {
		return err
	}
	if isTest {
		return errStatus(http.StatusBadRequest, testSetRefusal)
	}

	// Only classes the bank already knows: this endpoint teaches nothing, so a
	// new name here would create a label the model has no prompt for.
	bank, err := s.VPE.Bank(r.Context(), stateDir)
	if err != nil {
		return err
	}
	taught := make(map[string]bool, len(bank.Classes))
	for _, c := range bank.Classes {
		taught[c.Name] = true
	}
	unknown := []string{}
	seen := map[string]bool{}
	for _, b := range req.Boxes {
		if !taught[b.Cls] && !seen[b.Cls] {
			seen[b.Cls] = true
			unknown = append(unknown, b.Cls)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return errStatus(http.StatusBadRequest, fmt.Sprintf(
			"unknown class(es) %s -- use Save to bank to teach a new class", pyReprList(unknown)))
	}

	if !readableImage(image) {
		return errStatus(http.StatusBadRequest, "cannot read image")
	}
	if err := validateBoxes(req.Boxes); err != nil {
		return err
	}
	if iw, ih, ok := s.imageDims(image); ok {
		if err := boxesWithinImage(req.Boxes, iw, ih); err != nil {
			return err
		}
	}

	var user *string
	if u := s.currentUser(r); u != "" {
		user = &u
	}
	if _, err := s.Store.WriteBoxes(r.Context(), inputDir, store.KindPool, req.Image,
		req.Boxes, user, req.Mode == "update"); err != nil {
		return err
	}
	if err := s.Store.MarkLabeled(r.Context(), inputDir, store.KindPool, req.Image); err != nil {
		return err
	}
	return s.writeBankSummary(w, r, inputDir, stateDir)
}

// Predict is FR-19 / T-05: the model's guesses for ONE image, so the user
// corrects instead of drawing from scratch. An empty bank costs nothing --
// no checkpoint load, no forward pass.
func (s *Server) Predict(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir    string             `json:"input_dir"`
		Image       string             `json:"image"`
		Conf        *float64           `json:"conf"`
		ConfByClass map[string]float64 `json:"conf_by_class"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	_, stateDir, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	image, err := s.checkedPath(req.Image)
	if err != nil {
		return err
	}
	boxes, err := s.VPE.Predict(r.Context(), stateDir, image, orDefaultConf(req.Conf), req.ConfByClass)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"boxes": boxes})
	return nil
}

func (s *Server) writeBankSummary(w http.ResponseWriter, r *http.Request, inputDir, stateDir string) error {
	bank, err := s.bankSummary(r, inputDir, stateDir)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"bank": bank})
	return nil
}

func toVPEBoxes(boxes []store.Box) []vpe.Box {
	out := make([]vpe.Box, len(boxes))
	for i, b := range boxes {
		out[i] = vpe.Box{Cls: b.Cls, Box: b.Box}
	}
	return out
}

// orDefaultConf distinguishes "no conf sent" from "conf: 0". The second is a
// real request -- FR-33's per-class thresholds are exercised with conf 0.0 --
// so a zero value cannot stand in for absence.
func orDefaultConf(conf *float64) float64 {
	if conf == nil {
		return config.DefaultConf
	}
	return *conf
}
