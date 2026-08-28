package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/P-PrPas/CT-Flow/backend/internal/platform/auth"
)

// A user entry the tests can sign in as. Generated rather than pasted so the
// iteration count stays whatever auth.Iterations is.
func withUser(t *testing.T, name, password string) {
	t.Helper()
	t.Setenv("LABEL_TOOL_USERS", name+":"+auth.HashPassword(password))
}

// flipLast changes the last character to a different one. Appending or
// overwriting with a fixed byte is not enough: the signature is hex, so a fixed
// replacement is a no-op whenever it happens to match, and the test then passes
// for the wrong reason on about one run in sixteen.
func flipLast(s string) string {
	repl := byte('a')
	if s[len(s)-1] == 'a' {
		repl = 'b'
	}
	return s[:len(s)-1] + string(repl)
}

// sentinel next handler: reaching it means the gate let the request through.
func gated() (http.Handler, *bool) {
	reached := false
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}), &reached
}

// A server with no login configured must refuse requests, not serve them.
//
// This inverts what it used to assert. Signing in is mandatory (T-27) and
// main() will not start such a server at all -- but the gate is the last thing
// standing between a misconfiguration and an open API, so it fails closed on
// its own rather than trusting the startup check to have run.
func TestRequireLoginFailsClosedWithoutUsers(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	s := localServer(t)
	next, reached := gated()

	w := httptest.NewRecorder()
	s.RequireLogin(next).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/label", nil))

	if *reached || w.Code != http.StatusUnauthorized {
		t.Errorf("no login configured: status %d, reached=%v -- want 401 and no handler", w.Code, *reached)
	}
}

// The gate wraps the whole mux, so a route added later is protected by having
// been forgotten rather than exposed by it. Public is the only way out.
func TestRequireLoginGatesEverythingButPublic(t *testing.T) {
	withUser(t, "alice", "correct horse")
	s := localServer(t)

	for _, path := range []string{
		"/api/label", "/api/relabel", "/api/predict", "/api/upload", "/api/session",
		"/api/boxes", "/api/score", "/api/evaluate", "/api/autolabel", "/api/reembed",
		"/api/export", "/api/history", "/api/events", "/api/jobs/abc",
		"/api/testset/import", "/api/testset/remove", "/api/testset/label",
		// A route nobody has written yet. It must be gated too.
		"/api/something-invented-next-quarter",
	} {
		t.Run(path, func(t *testing.T) {
			next, reached := gated()
			w := httptest.NewRecorder()
			s.RequireLogin(next).ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))

			if *reached {
				t.Fatalf("%s was reachable signed out", path)
			}
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
			if got := detail(t, w); got != "not signed in" {
				t.Errorf("detail = %q, want `not signed in`", got)
			}
		})
	}

	// The UI needs these before it can even draw a login box.
	for path := range Public {
		t.Run("public "+path, func(t *testing.T) {
			next, reached := gated()
			w := httptest.NewRecorder()
			s.RequireLogin(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if !*reached {
				t.Errorf("%s is in Public but was gated", path)
			}
		})
	}
}

// The cookie is the whole gate, so each way of not having a valid one has to
// land on the same 401 -- a different answer for "expired" than for "forged"
// tells an attacker which half to work on.
func TestRequireLoginRejectsEveryBadCookie(t *testing.T) {
	withUser(t, "alice", "correct horse")
	s := localServer(t)
	valid := s.Auth.Issue("alice")

	for _, tc := range []struct {
		name  string
		token string
		want  bool // reaches the handler
	}{
		{"valid", valid, true},
		{"no cookie", "", false},
		{"tampered signature", flipLast(valid), false},
		{"tampered user", "bob" + valid[strings.Index(valid, "|"):], false},
		{"not a token at all", "garbage", false},
		{"empty fields", "||", false},
		// A validly-signed but expired token cannot be built from out here --
		// sign() is unexported, which is the point. auth.TestIdentifyRejectsExpired
		// covers that case from inside the package.
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, reached := gated()
			r := httptest.NewRequest(http.MethodPost, "/api/label", nil)
			if tc.token != "" {
				r.AddCookie(&http.Cookie{Name: auth.Cookie, Value: tc.token})
			}
			w := httptest.NewRecorder()
			s.RequireLogin(next).ServeHTTP(w, r)

			if *reached != tc.want {
				t.Errorf("reached = %v, want %v (status %d)", *reached, tc.want, w.Code)
			}
		})
	}
}

// A preflight carries no cookie by definition; gating it would break the
// request that follows before it ever authenticates.
func TestRequireLoginLetsPreflightThrough(t *testing.T) {
	withUser(t, "alice", "pw")
	s := localServer(t)
	next, reached := gated()

	w := httptest.NewRecorder()
	s.RequireLogin(next).ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/api/label", nil))
	if !*reached {
		t.Errorf("OPTIONS was gated (status %d)", w.Code)
	}
}

// ---------------------------------------------------------------- /api/auth/*

func TestAuthMeReportsWhoIsSignedIn(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	s := localServer(t)

	// user:null is "signed out". enabled stays true: since T-27 there is always
	// a login on this server, so the UI's only question is whether to draw the
	// login screen or the app.
	w := do(s, s.AuthMe, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	body := decode(t, w)
	if body["enabled"] != true || body["user"] != nil {
		t.Errorf("signed out: %v, want {enabled:true, user:null}", body)
	}

	withUser(t, "alice", "pw")
	r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	r.AddCookie(&http.Cookie{Name: auth.Cookie, Value: s.Auth.Issue("alice")})
	body = decode(t, do(s, s.AuthMe, r))
	if body["enabled"] != true || body["user"] != "alice" {
		t.Errorf("signed in: %v, want {enabled:true, user:alice}", body)
	}
}

func TestAuthLoginSetsAnHttpOnlySessionCookie(t *testing.T) {
	withUser(t, "alice", "correct horse")
	s := localServer(t)

	w := do(s, s.AuthLogin, jsonReq(http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "correct horse"}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	if body := decode(t, w); body["user"] != "alice" || body["enabled"] != true {
		t.Errorf("body = %v", body)
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != auth.Cookie {
		t.Errorf("cookie name = %q", c.Name)
	}
	if !c.HttpOnly {
		t.Error("cookie is not HttpOnly -- script on the page could read the session")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax so a cross-site POST carries no session", c.SameSite)
	}
	if s.Auth.Identify(c.Value) != "alice" {
		t.Error("the issued cookie does not identify the user it was issued to")
	}
}

// One message for wrong-user and wrong-password both: which of the two it was is
// exactly what someone probing usernames wants to learn.
func TestAuthLoginSaysTheSameThingForBothFailures(t *testing.T) {
	withUser(t, "alice", "correct horse")
	s := localServer(t)

	for _, creds := range []map[string]string{
		{"username": "alice", "password": "wrong"},
		{"username": "nobody", "password": "wrong"},
		{"username": "", "password": ""},
	} {
		w := do(s, s.AuthLogin, jsonReq(http.MethodPost, "/api/auth/login", creds))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%v: status = %d, want 401", creds, w.Code)
		}
		if got := detail(t, w); got != "wrong username or password" {
			t.Errorf("%v: detail = %q", creds, got)
		}
		if len(w.Result().Cookies()) != 0 {
			t.Errorf("%v: a failed login set a cookie", creds)
		}
	}
}

func TestAuthLoginRefusesWhenAuthIsNotConfigured(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	s := localServer(t)
	w := do(s, s.AuthLogin, jsonReq(http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "pw"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := detail(t, w); got != "auth is not configured on this server" {
		t.Errorf("detail = %q", got)
	}
}

// Secure only when the browser already reached us over TLS. Hard-coding it would
// silently drop the cookie on the plain-http LAN deployment this actually runs
// as today (NFR-08).
func TestAuthLoginSecureFlagFollowsTheScheme(t *testing.T) {
	withUser(t, "alice", "pw")
	s := localServer(t)

	plain := do(s, s.AuthLogin, jsonReq(http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "pw"}))
	if plain.Result().Cookies()[0].Secure {
		t.Error("Secure set on a plain-http login -- the browser would discard the cookie")
	}

	r := jsonReq(http.MethodPost, "/api/auth/login", map[string]string{"username": "alice", "password": "pw"})
	r.Header.Set("X-Forwarded-Proto", "https")
	behindTLS := do(s, s.AuthLogin, r)
	if !behindTLS.Result().Cookies()[0].Secure {
		t.Error("Secure not set behind an https proxy")
	}
}

// Always 200, signed in or not: the UI's sign-out must not be able to fail.
func TestAuthLogoutClearsTheCookie(t *testing.T) {
	withUser(t, "alice", "pw")
	s := localServer(t)

	w := do(s, s.AuthLogout, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := decode(t, w); body["user"] != nil {
		t.Errorf("user = %v, want null", body["user"])
	}
	c := w.Result().Cookies()[0]
	if c.Value != "" || c.MaxAge >= 0 {
		t.Errorf("cookie = %q MaxAge=%d, want empty and expired", c.Value, c.MaxAge)
	}
}

// FR-31: the signed-in name lands on what it taught. "" means auth is off, not
// "unauthenticated" -- RequireLogin already rejected that case.
func TestCurrentUserIsEmptyWithoutAValidCookie(t *testing.T) {
	withUser(t, "alice", "pw")
	s := localServer(t)

	if got := s.currentUser(httptest.NewRequest(http.MethodGet, "/api/boxes", nil)); got != "" {
		t.Errorf("no cookie -> %q, want empty", got)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/boxes", nil)
	r.AddCookie(&http.Cookie{Name: auth.Cookie, Value: s.Auth.Issue("alice")})
	if got := s.currentUser(r); got != "alice" {
		t.Errorf("valid cookie -> %q, want alice", got)
	}
}

func TestOIDCLoginFlow(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	if unconfigured, err := auth.NewOIDC(context.Background(), "", "", "", "https://ctflow.example"); err != nil || unconfigured != nil {
		t.Fatalf("FRONTEND_URL alone enabled OIDC: oidc=%v err=%v", unconfigured, err)
	}
	tokenHits, sentVerifier := 0, ""
	var provider *httptest.Server
	provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"userinfo_endpoint":%q,"jwks_uri":%q,"end_session_endpoint":%q,"code_challenge_methods_supported":["S256"],"response_types_supported":["code"],"subject_types_supported":["public"],"id_token_signing_alg_values_supported":["RS256"]}`,
				provider.URL, provider.URL+"/authorize", provider.URL+"/token", provider.URL+"/userinfo", provider.URL+"/jwks", provider.URL+"/logout")
		case "/token":
			tokenHits++
			sentVerifier = r.FormValue("code_verifier")
			if err := r.ParseForm(); err != nil || r.Form.Get("code") != "company-code" {
				http.Error(w, `{"error":"bad code"}`, http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"access_token":"provider-token","token_type":"Bearer"}`)
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer provider-token" {
				http.Error(w, `{"error":"bad token"}`, http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"sub":"company-user-1","preferred_username":"alice","email":"alice@example.com"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	oidcAuth, err := auth.NewOIDC(context.Background(), "client", "secret", provider.URL, "https://ctflow.example")
	if err != nil {
		t.Fatal(err)
	}
	s := localServer(t)
	s.OIDC = oidcAuth
	legacySession := s.Auth.Issue("legacy")

	withUser(t, "legacy", "password")
	local := do(s, s.AuthLogin, jsonReq(http.MethodPost, "/api/auth/login", map[string]string{
		"username": "legacy", "password": "password",
	}))
	if local.Code != http.StatusBadRequest || len(local.Result().Cookies()) != 0 {
		t.Fatalf("local login bypassed OIDC: status=%d cookies=%v", local.Code, local.Result().Cookies())
	}

	redirect := do(s, s.OIDCRedirect, httptest.NewRequest(http.MethodGet, "/api/public/login/redirect", nil))
	if redirect.Code != http.StatusOK {
		t.Fatalf("redirect status = %d: %s", redirect.Code, redirect.Body)
	}
	stateCookie := redirect.Result().Cookies()[0]
	redirectURL, err := url.Parse(decode(t, redirect)["redirectUrl"].(string))
	if err != nil {
		t.Fatal(err)
	}
	state := redirectURL.Query().Get("state")
	cookieState, verifier, split := strings.Cut(stateCookie.Value, oidcStateSep)
	if state == "" || stateCookie.Name != oidcStateCookie || cookieState != state || !stateCookie.HttpOnly {
		t.Fatalf("state cookie and redirect do not match: cookie=%+v url=%s", stateCookie, redirectURL)
	}
	// PKCE is on because this provider's discovery document advertises S256.
	// The challenge on the wire must be the hash, never the verifier itself.
	challenge := redirectURL.Query().Get("code_challenge")
	if !oidcAuth.PKCE || !split || verifier == "" || challenge == "" ||
		redirectURL.Query().Get("code_challenge_method") != "S256" || challenge == verifier {
		t.Fatalf("no S256 PKCE challenge on the authorize URL: %s", redirectURL)
	}

	callback := jsonReq(http.MethodPost, "/api/public/login/callback", map[string]string{
		"code": "company-code", "state": state,
	})
	callback.AddCookie(stateCookie)
	w := do(s, s.OIDCCallback, callback)
	if w.Code != http.StatusOK {
		t.Fatalf("callback status = %d: %s", w.Code, w.Body)
	}
	if sentVerifier != verifier {
		t.Errorf("token exchange sent code_verifier %q, want the one from the state cookie %q", sentVerifier, verifier)
	}
	if body := decode(t, w); body["user"] != "alice" || body["mode"] != "oidc" {
		t.Errorf("callback body = %v", body)
	}
	var session *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == auth.Cookie {
			session = cookie
		}
	}
	rawIdentity := ""
	if session != nil {
		rawIdentity = s.Auth.Identify(session.Value)
	}
	attribution, display, oidcSession := auth.SessionIdentity(rawIdentity)
	if session == nil || !session.HttpOnly || !oidcSession || attribution != "company-user-1" || display != "alice" {
		t.Fatalf("valid HttpOnly application session was not issued: %+v", session)
	}
	me := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	me.AddCookie(session)
	if body := decode(t, do(s, s.AuthMe, me)); body["user"] != "alice" {
		t.Errorf("auth state displays %v, want alice", body["user"])
	}

	// Signing out has to end the provider session too, or the next "sign in" on
	// a shared labelling machine is silent and lands on the previous person.
	out := do(s, s.AuthLogout, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
	if got := decode(t, out)["logoutUrl"]; got != provider.URL+"/logout" {
		t.Errorf("logout returned logoutUrl %v, want the provider end_session_endpoint", got)
	}

	bad := jsonReq(http.MethodPost, "/api/public/login/callback", map[string]string{
		"code": "company-code", "state": "wrong",
	})
	bad.AddCookie(stateCookie)
	if rejected := do(s, s.OIDCCallback, bad); rejected.Code != http.StatusUnauthorized {
		t.Fatalf("mismatched state status = %d, want 401", rejected.Code)
	}
	if tokenHits != 1 {
		t.Errorf("provider token endpoint called %d times; mismatched state must be rejected before exchange", tokenHits)
	}

	oldLocal := httptest.NewRequest(http.MethodGet, "/api/boxes", nil)
	oldLocal.AddCookie(&http.Cookie{Name: auth.Cookie, Value: legacySession})
	if got := do(s, func(w http.ResponseWriter, r *http.Request) error {
		s.RequireLogin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
		return nil
	}, oldLocal); got.Code != http.StatusUnauthorized {
		t.Errorf("legacy local session reached OIDC mode: status %d", got.Code)
	}

	s.OIDC = nil
	oldOIDC := httptest.NewRequest(http.MethodGet, "/api/boxes", nil)
	oldOIDC.AddCookie(session)
	if got := do(s, func(w http.ResponseWriter, r *http.Request) error {
		s.RequireLogin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
		return nil
	}, oldOIDC); got.Code != http.StatusUnauthorized {
		t.Errorf("OIDC session reached local mode: status %d", got.Code)
	}
}
