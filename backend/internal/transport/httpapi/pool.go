package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/images"
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

	// FR-51 -- state for this project lives in two places and one command wipes
	// only half of it. A bank holding embeddings while the database holds no
	// images at all is what `docker compose down -v` leaves behind when the
	// dataset's .ctflow folder is not deleted with it: the model still knows
	// what it was taught, and nothing knows which images taught it. Silent
	// today, because predictions come back as class names rather than indexes.
	hasImages, err := s.Store.HasImages(r.Context(), inputDir)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"images":        pool,
		"bank":          bank,
		"bank_orphaned": len(bank.Classes) > 0 && !hasImages,
		"testset": map[string]any{
			"images": testImages, "labeled": labeled, "classes": classes,
		},
		// So a client that opened a folder directly can still link to it, and
		// so the UI has the name and owner without a second call.
		"project": project,
	})
	return nil
}

// poolItem is one row of the gallery: a path, how it was labeled, and whoever
// is on it right now.
type poolItem struct {
	Path   string  `json:"path"`
	Status string  `json:"status"` // "labeled" | "auto" | "unlabeled"
	HeldBy *string `json:"held_by"`
}

// GetPool is the gallery's backing query: the pool's images, filtered by status
// and paged, so a folder of 50,000 never ships as one array (the shape
// POST /api/session still sends, kept until the labeling loop stops depending
// on the full list -- see docs/GALLERY_PLAN.md T-36).
//
// The image set comes from a cached readdir; which of them are labeled/auto
// comes from PostgreSQL. "unlabeled" is the difference -- an image with no row
// yet -- so it costs no query of its own.
func (s *Server) GetPool(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	inputDir, _, err := s.stateDirFor(q.Get("input_dir"))
	if err != nil {
		return err
	}

	want := q.Get("status")
	if want == "" {
		want = "all"
	}
	switch want {
	case "all", "labeled", "auto", "unlabeled":
	default:
		return errStatus(http.StatusBadRequest,
			"status must be one of all, labeled, auto, unlabeled")
	}
	offset := max(queryInt(q.Get("offset"), 0), 0)
	limit := queryInt(q.Get("limit"), 200)
	if limit < 1 || limit > 500 {
		limit = 500
	}

	all := images.ListCached(inputDir)
	if len(all) == 0 {
		return errStatus(http.StatusBadRequest, "no images in "+inputDir)
	}
	st, err := s.Store.ListByStatus(r.Context(), inputDir, store.KindPool)
	if err != nil {
		return err
	}
	held, err := s.namedClaims(r, inputDir)
	if err != nil {
		return err
	}
	labeled := sliceSet(st.Labeled)
	auto := sliceSet(st.Auto)

	counts := map[string]int{
		"labeled":   len(st.Labeled),
		"auto":      len(st.Auto),
		"unlabeled": max(len(all)-len(st.Labeled)-len(st.Auto), 0),
	}
	total := len(all)
	if want != "all" {
		total = counts[want]
	}

	items := make([]poolItem, 0, min(limit, total))
	skipped := 0
	for _, p := range all {
		status := "unlabeled"
		switch {
		case labeled[p]:
			status = "labeled"
		case auto[p]:
			status = "auto"
		}
		if want != "all" && want != status {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		if len(items) >= limit {
			break
		}
		var by *string
		if n, ok := held[p]; ok {
			by = &n
		}
		items = append(items, poolItem{Path: p, Status: status, HeldBy: by})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":  total,
		"counts": counts,
		"items":  items,
	})
	return nil
}

func queryInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func sliceSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
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
	// FR-50. Per image, not per box: "who labeled this" is the question, and
	// hanging created_by on Box would change a shape the client sends back on
	// every save. Names, never subjects -- see store.UserNames.
	authors, err := s.Store.ImageAuthors(r.Context(), inputDir, kind, image)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"boxes": boxes, "labeled_by": authors})
	return nil
}
