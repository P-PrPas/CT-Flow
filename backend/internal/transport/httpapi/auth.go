package httpapi

import (
	"crypto/rand"
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

const loginStateCookie = "labeltool_oidc_state"

// RequireLogin gates every request that is not in Public.
//
// It wraps the whole mux rather than sitting on each route, so the gate is on
// by default and Public above is the only way out of it -- a route added later
// is protected by having been forgotten, not exposed by it.
//
// It used to be inert when no login was configured, for the "one person, own
// PC" deployment the tool started as. That deployment is gone (T-27) and the
// process now refuses to start without Directory or LABEL_TOOL_USERS, so the
// bypass went with it: an unconfigured server fails closed here rather than
// serving everything to anyone who can reach it.
func (s *Server) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || Public[r.URL.Path] {
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

// currentUser is FR-31: who to record as having taught a prompt, and who owns a
// project they create. Empty only on a request RequireLogin would have rejected
// -- every gated handler is reached with a signed-in caller.
func (s *Server) currentUser(r *http.Request) string {
	user, _, _ := s.currentIdentity(r)
	return user
}

func (s *Server) currentIdentity(r *http.Request) (oid, display, picture string) {
	c, err := r.Cookie(auth.Cookie)
	if err != nil {
		return "", "", ""
	}
	attribution, display, picture, isDirectory := auth.SessionIdentity(s.Auth.Identify(c.Value))
	// A session issued under the other login mode is not a session here: a
	// directory cookie must not survive a switch to local accounts, or the
	// reverse.
	if (s.authMode() == "directory") != isDirectory {
		return "", "", ""
	}
	return attribution, display, picture
}

type authState struct {
	Enabled bool    `json:"enabled"`
	User    *string `json:"user"`
	// OID is the caller's own attribution key -- the same value that lands in
	// projects.owner_oid and annotations.created_by. User is the display name
	// beside it, and under Directory the two are different strings for the same
	// person: the subject is an opaque id, the display name is what the
	// directory happens to call them today. The UI needs the subject to answer
	// "is this mine", because comparing display names is comparing labels that
	// can collide and can be renamed out from under a project.
	OID  *string `json:"oid"`
	Mode string  `json:"mode"`
	// Picture is the avatar Directory has on file for this person, or empty
	// for a local account (there is nothing to fetch it from). Omitted
	// entirely rather than sent as "" so the frontend's fallback-to-initials
	// check is a plain truthiness test.
	Picture string `json:"picture,omitempty"`
	// LogoutURL is always empty now: the Directory SDK has no RP-initiated
	// logout endpoint to send the browser to (see AuthLogout). The field stays
	// on the wire because the frontend still reads it.
	LogoutURL string `json:"logoutUrl,omitempty"`
}

// Enabled is always true now that signing in is mandatory. The field stays on
// the wire because the frontend and the smoke test both read it, and because
// "is there a login on this server" is still the question the UI is asking --
// the answer just cannot be no any more.
//
// oid and user travel together: either both are set or the caller is signed out
// and both are null. Nothing on the wire should ever carry one without the
// other, because a client with a name and no subject cannot tell its own work
// apart from anyone else's.
func state(mode, oid, user, picture string) authState {
	if user == "" {
		return authState{Enabled: true, User: nil, OID: nil, Mode: mode}
	}
	return authState{Enabled: true, User: &user, OID: &oid, Mode: mode, Picture: picture}
}

// authMode picks which credential the session cookie is expected to carry.
// Directory wins where both are configured; local accounts are the CI and
// development path (docs/PHASE2_WORKSPACE.md #2, decision 8). There is no
// third value: a server with neither does not get past main().
func (s *Server) authMode() string {
	if s.Directory != nil {
		return "directory"
	}
	return "local"
}

// AuthMe is what the UI calls on load to decide whether to draw a login screen
// or the app, and to tell its own projects from everyone else's. user:null
// means signed out.
func (s *Server) AuthMe(w http.ResponseWriter, r *http.Request) error {
	oid, display, picture := s.currentIdentity(r)
	writeJSON(w, http.StatusOK, state(s.authMode(), oid, display, picture))
	return nil
}

// AuthLogin sets an httponly session cookie on success.
func (s *Server) AuthLogin(w http.ResponseWriter, r *http.Request) error {
	if s.Directory != nil {
		return errStatus(http.StatusBadRequest, "local login is disabled while Directory is configured")
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
	// A local account has no separate subject: the username is both.
	writeJSON(w, http.StatusOK, state(s.authMode(), req.Username, req.Username, ""))
	return nil
}

func (s *Server) OIDCRedirect(w http.ResponseWriter, r *http.Request) error {
	if s.Directory == nil {
		return errStatus(http.StatusBadRequest, "Directory login is not configured on this server")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	loginState := base64.RawURLEncoding.EncodeToString(raw)
	s.setStateCookie(w, r, loginState, 300)
	redirectURL, err := s.Directory.AuthorizeURL()
	if err != nil {
		// Most likely ErrNotConnected: the redial loop hasn't completed its
		// first handshake yet. Transient, not a misconfiguration -- 503 says so.
		return errStatus(http.StatusServiceUnavailable, "directory login is temporarily unavailable")
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirectUrl": redirectURL})
	return nil
}

func (s *Server) OIDCCallback(w http.ResponseWriter, r *http.Request) error {
	if s.Directory == nil {
		return errStatus(http.StatusBadRequest, "Directory login is not configured on this server")
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	// The directory hands back no state of its own (see AuthorizeURL), so this
	// cookie's mere presence is the CSRF proof instead: only a browser that hit
	// /api/public/login/redirect on this origin has it, and it is consumed here
	// so a captured callback URL cannot be replayed.
	cookie, err := r.Cookie(loginStateCookie)
	if err != nil || cookie.Value == "" {
		return errStatus(http.StatusUnauthorized, "invalid login state")
	}
	s.setStateCookie(w, r, "", -1)
	identity, err := s.Directory.Identity(r.Context(), req.Code)
	if err != nil {
		return errStatus(http.StatusUnauthorized, "directory login failed")
	}
	s.recordUser(r, identity)
	s.setSessionCookie(w, r, auth.OIDCSessionIdentity(identity))
	writeJSON(w, http.StatusOK, state("directory", identity.Subject, identity.Display, identity.Picture))
	return nil
}

// recordUser is what keeps FR-31 answerable. Attribution stores the
// directory's user id -- the only claim that survives someone being renamed
// -- and an id on its own belongs to no other table. The users row is where
// it becomes a person again.
//
// Best-effort on purpose: an identity ledger that cannot be written is a
// reporting problem, not a reason to refuse an otherwise valid login.
func (s *Server) recordUser(r *http.Request, identity auth.OIDCIdentity) {
	if s.Store == nil {
		return
	}
	if err := s.Store.UpsertUser(r.Context(), identity.Subject, identity.Display, identity.Email); err != nil {
		s.Log.Warn("cannot record the directory user", "err", err)
	}
}

func (s *Server) setStateCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: loginStateCookie, Value: value, Path: "/", MaxAge: maxAge,
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
		(s.Directory != nil && s.Directory.Secure())
}

// AuthLogout clears the cookie. Always 200, signed in or not.
//
// The Directory SDK has no RP-initiated logout endpoint, so unlike the old
// OIDC flow this cannot also end the provider's own session: "sign out" only
// ever clears CT-Flow's cookie. On the shared labelling machine this tool is
// deployed on, that means the next "sign in" can be silent if the directory
// itself still has the browser signed in -- accepted for v1, not fixable from
// this side.
func (s *Server) AuthLogout(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, &http.Cookie{
		Name: auth.Cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.secureCookie(r),
	})
	writeJSON(w, http.StatusOK, state(s.authMode(), "", "", ""))
	return nil
}
