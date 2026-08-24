package httpapi

import (
	"net/http"
	"net/http/httptest"
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

// With no users configured there is no login at all and every endpoint is open,
// which is the "one person, own PC" case the tool started as. Adding a login
// screen to that would be pure friction.
func TestRequireLoginIsInertWithoutUsers(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	s := localServer(t)
	next, reached := gated()

	w := httptest.NewRecorder()
	s.RequireLogin(next).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/label", nil))

	if !*reached || w.Code != http.StatusOK {
		t.Errorf("auth off: status %d, reached=%v -- want the request through", w.Code, *reached)
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

func TestAuthMeReportsWhetherThereIsALoginAtAll(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	s := localServer(t)

	// enabled:false means every other endpoint is open to anyone who can reach
	// it -- not that you are signed out. The UI branches on exactly this.
	w := do(s, s.AuthMe, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	body := decode(t, w)
	if body["enabled"] != false || body["user"] != nil {
		t.Errorf("auth off: %v, want {enabled:false, user:null}", body)
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
