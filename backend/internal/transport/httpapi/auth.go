package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/P-PrPas/CT-Flow/backend/internal/platform/auth"
)

// Public is the set of paths reachable without a session. The UI needs both
// before it can even draw a login box. Everything else is gated, so a route
// added later is protected by default rather than by remembering to protect it.
var Public = map[string]bool{
	"/api/config":                true,
	"/api/auth/me":               true,
	"/api/auth/login":            true,
	"/api/auth/logout":           true,
	"/api/public/login/redirect": true,
	"/api/public/login/callback": true,
}

const oidcStateCookie = "labeltool_oidc_state"

// RequireLogin is inert until OIDC or legacy local users are configured.
//
// It wraps the whole mux rather than sitting on each route, so the gate is on
// by default and Public above is the only way out of it -- a route added later
// is protected by having been forgotten, not exposed by it.
func (s *Server) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || Public[r.URL.Path] || !s.authEnabled() {
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
	user, _ := s.currentIdentity(r)
	return user
}

func (s *Server) currentDisplayUser(r *http.Request) string {
	_, display := s.currentIdentity(r)
	return display
}

func (s *Server) currentIdentity(r *http.Request) (string, string) {
	c, err := r.Cookie(auth.Cookie)
	if err != nil {
		return "", ""
	}
	attribution, display, oidcSession := auth.SessionIdentity(s.Auth.Identify(c.Value))
	mode := s.authMode()
	if mode == "none" || (mode == "oidc") != oidcSession {
		return "", ""
	}
	return attribution, display
}

type authState struct {
	Enabled bool    `json:"enabled"`
	User    *string `json:"user"`
	Mode    string  `json:"mode"`
}

func state(mode, user string) authState {
	enabled := mode != "none"
	if user == "" {
		return authState{Enabled: enabled, User: nil, Mode: mode}
	}
	return authState{Enabled: enabled, User: &user, Mode: mode}
}

func (s *Server) authMode() string {
	if s.OIDC != nil {
		return "oidc"
	}
	if auth.Enabled() {
		return "local"
	}
	return "none"
}

func (s *Server) authEnabled() bool { return s.authMode() != "none" }

// AuthMe is what the UI calls on load to decide whether to draw a login screen.
// enabled:false means the server has no users configured, i.e. every other
// endpoint is open to anyone who can reach it -- not that you are signed out.
func (s *Server) AuthMe(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, state(s.authMode(), s.currentDisplayUser(r)))
	return nil
}

// AuthLogin sets an httponly session cookie on success.
func (s *Server) AuthLogin(w http.ResponseWriter, r *http.Request) error {
	if s.OIDC != nil {
		return errStatus(http.StatusBadRequest, "local login is disabled while OIDC is configured")
	}
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
	s.setSessionCookie(w, r, req.Username)
	writeJSON(w, http.StatusOK, state(s.authMode(), req.Username))
	return nil
}

func (s *Server) OIDCRedirect(w http.ResponseWriter, r *http.Request) error {
	if s.OIDC == nil {
		return errStatus(http.StatusBadRequest, "OIDC is not configured on this server")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	state := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: state, Path: "/", MaxAge: 300,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(r),
	})
	writeJSON(w, http.StatusOK, map[string]string{"redirectUrl": s.OIDC.AuthCodeURL(state)})
	return nil
}

func (s *Server) OIDCCallback(w http.ResponseWriter, r *http.Request) error {
	if s.OIDC == nil {
		return errStatus(http.StatusBadRequest, "OIDC is not configured on this server")
	}
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	cookie, err := r.Cookie(oidcStateCookie)
	if err != nil || req.State == "" || len(cookie.Value) != len(req.State) ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(req.State)) != 1 {
		return errStatus(http.StatusUnauthorized, "invalid login state")
	}
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(r),
	})
	identity, err := s.OIDC.Identity(r.Context(), req.Code)
	if err != nil {
		return errStatus(http.StatusUnauthorized, "OIDC login failed")
	}
	s.setSessionCookie(w, r, auth.OIDCSessionIdentity(identity))
	writeJSON(w, http.StatusOK, state("oidc", identity.Display))
	return nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, user string) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.Cookie, Value: s.Auth.Issue(user), Path: "/", MaxAge: auth.TTLSeconds,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(r),
	})
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// AuthLogout clears the cookie. Always 200, signed in or not.
func (s *Server) AuthLogout(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, &http.Cookie{
		Name: auth.Cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: secureRequest(r),
	})
	writeJSON(w, http.StatusOK, state(s.authMode(), ""))
	return nil
}
