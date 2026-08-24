package httpapi

import (
	"net/http"

	"github.com/P-PrPas/CT-Flow/backend/internal/platform/auth"
)

// Public is the set of paths reachable without a session. The UI needs both
// before it can even draw a login box. Everything else is gated, so a route
// added later is protected by default rather than by remembering to protect it.
var Public = map[string]bool{
	"/api/config":      true,
	"/api/auth/me":     true,
	"/api/auth/login":  true,
	"/api/auth/logout": true,
}

// RequireLogin is T-12 / FR-30. Inert until LABEL_TOOL_USERS is set.
//
// It wraps the whole mux rather than sitting on each route, so the gate is on
// by default and Public above is the only way out of it -- a route added later
// is protected by having been forgotten, not exposed by it.
func (s *Server) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || Public[r.URL.Path] || !auth.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		if s.currentUser(r) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"detail": "not signed in"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// currentUser is FR-31: who to record as having taught a prompt. "" means auth
// is off, not "unauthenticated" -- RequireLogin already rejected that case.
func (s *Server) currentUser(r *http.Request) string {
	c, err := r.Cookie(auth.Cookie)
	if err != nil {
		return ""
	}
	return s.Auth.Identify(c.Value)
}

type authState struct {
	Enabled bool    `json:"enabled"`
	User    *string `json:"user"`
}

func state(enabled bool, user string) authState {
	if user == "" {
		return authState{Enabled: enabled, User: nil}
	}
	return authState{Enabled: enabled, User: &user}
}

// AuthMe is what the UI calls on load to decide whether to draw a login screen.
// enabled:false means the server has no users configured, i.e. every other
// endpoint is open to anyone who can reach it -- not that you are signed out.
func (s *Server) AuthMe(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, state(auth.Enabled(), s.currentUser(r)))
	return nil
}

// AuthLogin sets an httponly session cookie on success.
func (s *Server) AuthLogin(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if !auth.Enabled() {
		return errStatus(http.StatusBadRequest, "auth is not configured on this server")
	}
	if !auth.Check(req.Username, req.Password) {
		// One message for both wrong-user and wrong-password: which of the two
		// it was is exactly what someone probing usernames wants to learn.
		return errStatus(http.StatusUnauthorized, "wrong username or password")
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.Cookie,
		Value:    s.Auth.Issue(req.Username),
		Path:     "/",
		MaxAge:   auth.TTLSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Only when the browser already reached us over TLS -- a hard-coded
		// Secure would silently drop the cookie on a plain-http LAN
		// deployment, which is how this gets run today (NFR-08).
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	writeJSON(w, http.StatusOK, state(true, req.Username))
	return nil
}

// AuthLogout clears the cookie. Always 200, signed in or not.
func (s *Server) AuthLogout(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, &http.Cookie{
		Name: auth.Cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, state(auth.Enabled(), ""))
	return nil
}
