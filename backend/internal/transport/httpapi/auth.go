package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

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

// The state cookie carries the PKCE verifier next to the state, separated by a
// ".". Neither half can contain one -- both are base64url -- and one cookie
// that cannot half-arrive beats two that can.
const oidcStateSep = "."

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
	// LogoutURL is set only by AuthLogout, and only when the provider offers
	// RP-initiated logout. The browser has to be sent there for a sign-out to
	// mean anything on a shared machine.
	LogoutURL string `json:"logoutUrl,omitempty"`
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
	login, verifier := base64.RawURLEncoding.EncodeToString(raw), oauth2.GenerateVerifier()
	s.setStateCookie(w, r, login+oidcStateSep+verifier, 300)
	writeJSON(w, http.StatusOK, map[string]string{"redirectUrl": s.OIDC.AuthCodeURL(login, verifier)})
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
	if err != nil {
		return errStatus(http.StatusUnauthorized, "invalid login state")
	}
	want, verifier, _ := strings.Cut(cookie.Value, oidcStateSep)
	if req.State == "" || len(want) != len(req.State) ||
		subtle.ConstantTimeCompare([]byte(want), []byte(req.State)) != 1 {
		return errStatus(http.StatusUnauthorized, "invalid login state")
	}
	s.setStateCookie(w, r, "", -1)
	identity, err := s.OIDC.Identity(r.Context(), req.Code, verifier)
	if err != nil {
		return errStatus(http.StatusUnauthorized, "OIDC login failed")
	}
	s.recordUser(r, identity)
	s.setSessionCookie(w, r, auth.OIDCSessionIdentity(identity))
	writeJSON(w, http.StatusOK, state("oidc", identity.Display))
	return nil
}

// recordUser is what keeps FR-31 answerable. Attribution stores the provider's
// `sub` -- the only claim that survives someone being renamed -- and a `sub` on
// its own is a UUID belonging to no other table. The users row is where it
// becomes a person again.
//
// Best-effort on purpose: an identity ledger that cannot be written is a
// reporting problem, not a reason to refuse an otherwise valid login.
func (s *Server) recordUser(r *http.Request, identity auth.OIDCIdentity) {
	if s.Store == nil {
		return
	}
	if err := s.Store.UpsertUser(r.Context(), identity.Subject, identity.Display, identity.Email); err != nil {
		s.Log.Warn("cannot record the OIDC user", "err", err)
	}
}

func (s *Server) setStateCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.secureCookie(r),
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, user string) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.Cookie, Value: s.Auth.Issue(user), Path: "/", MaxAge: auth.TTLSeconds,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.secureCookie(r),
	})
}

// secureCookie asks the deployment before it asks the proxy. X-Forwarded-Proto
// is only as trustworthy as whoever configured the ingress, and one that
// forgets to send it silently drops Secure from every session cookie on an
// https site -- a downgrade nothing in the app would ever report.
func (s *Server) secureCookie(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" ||
		(s.OIDC != nil && s.OIDC.Secure)
}

// AuthLogout clears the cookie. Always 200, signed in or not.
//
// Under OIDC it also hands back the provider's end-session URL, because
// clearing only CT-Flow's cookie makes "sign out" a lie: the provider session
// outlives it, so the next "sign in" is silent and the next person at a shared
// labelling machine is signed in as whoever left. No post_logout_redirect_uri
// is attached -- that parameter has to be registered with the provider first,
// and a logout rejected for an unregistered URL is worse than one that ends on
// the provider's own signed-out page.
func (s *Server) AuthLogout(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, &http.Cookie{
		Name: auth.Cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.secureCookie(r),
	})
	out := state(s.authMode(), "")
	if s.OIDC != nil {
		out.LogoutURL = s.OIDC.EndSession
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
