package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/events"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/history"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/config"
)

// stateDirFor validates an input_dir from the browser and returns the project's
// state directory. Both come back because handlers usually need each: the
// database is keyed by input_dir, the files live under the state dir.
func (s *Server) stateDirFor(inputDir string) (string, string, error) {
	p, err := s.checkedPath(inputDir)
	if err != nil {
		return "", "", err
	}
	return p, config.StateDir(p), nil
}

// GetHistory is T-07: every Evaluate run this project has recorded, for the
// accuracy-over-time chart.
func (s *Server) GetHistory(w http.ResponseWriter, r *http.Request) error {
	_, state, err := s.stateDirFor(r.URL.Query().Get("input_dir"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history.Read(state)})
	return nil
}

// AddHistory appends one point and returns the history as it now stands.
func (s *Server) AddHistory(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string          `json:"input_dir"`
		Point    json.RawMessage `json:"point"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	_, state, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	points, err := history.Append(state, req.Point)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": points})
	return nil
}

func (s *Server) DeleteHistory(w http.ResponseWriter, r *http.Request) error {
	_, state, err := s.stateDirFor(r.URL.Query().Get("input_dir"))
	if err != nil {
		return err
	}
	if err := history.Clear(state); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": []json.RawMessage{}})
	return nil
}

// AddEvent records one thing that happened, so "does this tool save time" has
// an answer that outlives the tab.
//
// Fire-and-forget: the UI never waits on it and never shows an error for it,
// which is also why a write failure here is reported rather than swallowed --
// nobody is watching, so it needs to reach the log.
func (s *Server) AddEvent(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		InputDir string   `json:"input_dir"`
		Kind     string   `json:"kind"`
		Session  string   `json:"session"`
		Secs     *float64 `json:"secs"`
		Written  int      `json:"written"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	_, state, err := s.stateDirFor(req.InputDir)
	if err != nil {
		return err
	}
	e := events.Event{Kind: req.Kind, Session: req.Session, Secs: req.Secs, Written: req.Written}
	// FR-31: who did it, when there is a who. Nil when auth is off.
	if u := s.currentUser(r); u != "" {
		e.User = &u
	}
	if err := events.Append(state, e); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) GetEvents(w http.ResponseWriter, r *http.Request) error {
	_, state, err := s.stateDirFor(r.URL.Query().Get("input_dir"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": events.Summarize(state)})
	return nil
}
