// Package api is the HTTP layer: request in, service call, response out.
//
// Two conventions the frontend depends on, both inherited from the FastAPI
// original and both non-negotiable during the port (see docs/API_REFERENCE.md):
//
//   - every error body is {"detail": "<message>"}, because lib/api.ts throws
//     Error(data.detail) for any non-ok response;
//   - any field naming a filesystem path is a trust boundary, checked against
//     the deployment's allowed roots before anything touches disk.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/P-PrPas/CT-Flow/internal/auth"
	"github.com/P-PrPas/CT-Flow/internal/config"
	"github.com/P-PrPas/CT-Flow/internal/jobs"
	"github.com/P-PrPas/CT-Flow/internal/models"
	"github.com/P-PrPas/CT-Flow/internal/store"
	"github.com/P-PrPas/CT-Flow/internal/vpe"
)

type Server struct {
	Cfg     config.Config
	Catalog *models.Catalog
	Auth    *auth.Auth
	Store   *store.Store
	VPE     *vpe.Client
	Jobs    *jobs.Tracker
	Log     *slog.Logger
}

// httpError is the only way a handler reports a failure, so the {"detail": ...}
// shape cannot drift endpoint by endpoint.
type httpError struct {
	Status  int
	Message string
}

func (e *httpError) Error() string { return e.Message }

func errStatus(status int, msg string) error { return &httpError{Status: status, Message: msg} }

// forbiddenPath is the one refusal produced in enough places to be worth a
// constructor. The message is copied verbatim from backend/deps.py: the smoke
// test and the parity differ both compare it.
func (s *Server) forbiddenPath() error {
	return errStatus(http.StatusForbidden, "path outside "+s.Cfg.VMDataRoot+" (vm mode)")
}

// checkedPath is deps.checked_path: every path the browser sends goes through
// here before it reaches the filesystem.
func (s *Server) checkedPath(p string) (string, error) {
	if !s.Cfg.PathAllowed(p) {
		return "", s.forbiddenPath()
	}
	return p, nil
}

// Handler lets each endpoint return an error instead of writing one, so no
// handler can forget the response shape or write a body after an error.
type Handler func(http.ResponseWriter, *http.Request) error

// Handle adapts a Handler for the mux.
func (s *Server) Handle(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			var he *httpError
			if errors.As(err, &he) {
				writeJSON(w, he.Status, map[string]string{"detail": he.Message})
				return
			}
			// The inference sidecar's refusals are this API's refusals: a
			// mismatched model_id is still a 409 with the same message, an empty
			// bank still a 400. The browser must not be able to tell that the
			// check moved out of process.
			var ve *vpe.Error
			if errors.As(err, &ve) {
				writeJSON(w, ve.Status, map[string]string{"detail": ve.Detail})
				return
			}
			// An unexpected error is logged in full and reported as one line:
			// the browser gets something lib/api.ts can display, and the detail
			// stays server-side rather than leaking a path or a query.
			s.Log.Error("unhandled error", "path", r.URL.Path, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "internal error"})
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// Headers are already out; there is nothing left to tell the client.
		slog.Error("writing response body", "err", err)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return errStatus(http.StatusBadRequest, "malformed request body: "+err.Error())
	}
	return nil
}
