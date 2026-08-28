package httpapi

import (
	"net/http"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
)

// Live state for a project someone else may also be in (FR-48, FR-49).
//
// Two endpoints, both deliberately cheap: the workspace polls the first every
// few seconds per open tab, so it answers from PostgreSQL and a map in memory
// and never touches the inference sidecar or the disk.

// GetState is what changes while you are not the one changing it.
//
// Deliberately not a BankSummary: `classes` and `model` only change when *you*
// label, and the response to your own POST /api/label already refreshes them.
// Including them here would mean a sidecar round trip every poll, per open tab,
// to re-send something that cannot have changed.
func (s *Server) GetState(w http.ResponseWriter, r *http.Request) error {
	inputDir, _, err := s.stateDirFor(r.URL.Query().Get("input_dir"))
	if err != nil {
		return err
	}
	status, err := s.Store.ListByStatus(r.Context(), inputDir, store.KindPool)
	if err != nil {
		return err
	}
	tsLabeled, err := s.Store.LabeledStems(r.Context(), inputDir)
	if err != nil {
		return err
	}
	sortStrings(tsLabeled)
	held, err := s.namedClaims(r, inputDir)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"labeled":         status.Labeled,
		"auto":            status.Auto,
		"testset_labeled": tsLabeled,
		"claims":          held,
	})
	return nil
}

// namedClaims turns the tracker's subjects into names.
//
// The tracker holds whatever currentUser returns -- the OIDC `sub` -- because
// that is the identity that stays put when someone is renamed. It is also a
// UUID, so this is the boundary where it stops being one: nothing leaves the
// API as a raw subject where a person is meant to be read.
func (s *Server) namedClaims(r *http.Request, inputDir string) (map[string]string, error) {
	held := s.Claims.Held(inputDir)
	if len(held) == 0 {
		return held, nil
	}
	oids := make([]string, 0, len(held))
	for _, oid := range held {
		oids = append(oids, oid)
	}
	names, err := s.Store.UserNames(r.Context(), oids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(held))
	for image, oid := range held {
		out[image] = names[oid]
	}
	return out, nil
}

// Claim says "I am working on this image" so the other person's queue points
// somewhere else.
//
// Advice, not a lock: POST /api/label never consults this, because refusing a
// save would throw away boxes that are already drawn. All a conflict does here
// is tell the client to move on to the next image.
//
// Re-claiming your own image renews it, which is what lets the frontend call
// this on a timer as a heartbeat instead of tracking whether it already holds
// the image.
func (s *Server) Claim(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string `json:"input_dir"`
		Image    string `json:"image"`
		/** Set when the caller is done with the image (it just saved), so the
		 *  other person is offered it now rather than after the TTL. */
		Release bool `json:"release"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	inputDir, _, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	image, err := s.checkedPath(req.Image)
	if err != nil {
		return err
	}
	user := s.currentUser(r)

	if req.Release {
		s.Claims.Release(inputDir, image, user)
	} else if holder := s.Claims.Claim(inputDir, image, user); holder != "" {
		// 409 rather than 403: nothing is forbidden, the image is simply taken.
		names, err := s.Store.UserNames(r.Context(), []string{holder})
		if err != nil {
			return err
		}
		return errStatus(http.StatusConflict, names[holder]+" is working on this image")
	}
	// The whole project's claims, not just this one -- the caller is about to
	// re-render its queue anyway, and this saves it a follow-up GET /api/state.
	held, err := s.namedClaims(r, inputDir)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"claims": held})
	return nil
}
