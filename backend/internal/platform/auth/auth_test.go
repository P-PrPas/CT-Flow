package auth

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/P-PrPas/CT-Flow/backend/internal/testsupport"
	"time"
)

// The vectors Python produced (backend/tests/gen_testdata.py). This is the check
// that matters: not "does Go's login work" but "does Go accept exactly what
// Python accepts", because existing LABEL_TOOL_USERS entries and already-issued
// session cookies both have to survive the port.
type vectors struct {
	Iterations int    `json:"iterations"`
	CookieName string `json:"cookie_name"`
	TTLSeconds int    `json:"ttl_seconds"`
	Secret     string `json:"secret"`
	Verify     []struct {
		Password string `json:"password"`
		Stored   string `json:"stored"`
		Expect   bool   `json:"expect"`
	} `json:"verify"`
	Identify []struct {
		Token  string  `json:"token"`
		Expect *string `json:"expect"`
		Why    string  `json:"why"`
	} `json:"identify"`
}

func load(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile(testsupport.MustBackendFile("tests/testdata/auth_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestConstantsMatchPython(t *testing.T) {
	v := load(t)
	if Iterations != v.Iterations {
		t.Errorf("iterations = %d, want %d -- every existing LABEL_TOOL_USERS entry would stop working",
			Iterations, v.Iterations)
	}
	if Cookie != v.CookieName {
		t.Errorf("cookie name = %q, want %q -- every live session would be dropped", Cookie, v.CookieName)
	}
	if TTLSeconds != v.TTLSeconds {
		t.Errorf("ttl = %d, want %d", TTLSeconds, v.TTLSeconds)
	}
}

func TestVerifyPasswordAgainstPythonHashes(t *testing.T) {
	for _, c := range load(t).Verify {
		if got := VerifyPassword(c.Password, c.Stored); got != c.Expect {
			t.Errorf("VerifyPassword(%q, %.20s...) = %v, want %v", c.Password, c.Stored, got, c.Expect)
		}
	}
}

func TestIdentifyAgainstPythonCookies(t *testing.T) {
	v := load(t)
	a := NewWithSecret(v.Secret)
	for _, c := range v.Identify {
		want := ""
		if c.Expect != nil {
			want = *c.Expect
		}
		if got := a.Identify(c.Token); got != want {
			t.Errorf("Identify(%s) = %q, want %q -- %s", c.Token, got, want, c.Why)
		}
	}
}

// The other direction: a cookie Go issues has to be the shape Python read. The
// port is done, but the vectors stay bidirectional -- this is the check that
// would have caught a token format quietly drifting while both were live, and
// it is what pins the format now that only one side is left to read it.
func TestIssuedTokenShapeIsPythonReadable(t *testing.T) {
	a := NewWithSecret(load(t).Secret)
	tok := a.Issue("alice")
	parts := strings.Split(tok, "|")
	if len(parts) != 3 {
		t.Fatalf("issued token %q has %d fields, want user|exp|sig", tok, len(parts))
	}
	if parts[0] != "alice" {
		t.Errorf("user field = %q", parts[0])
	}
	if len(parts[2]) != 64 {
		t.Errorf("signature is %d chars, want 64 (hex sha256)", len(parts[2]))
	}
	if a.Identify(tok) != "alice" {
		t.Error("we cannot read back our own token")
	}
}

// A username containing the separator must round-trip. This is why Identify
// splits from the right; splitting from the left lets a name like "a|b" shift
// the expiry field, which is a forgery primitive, not a cosmetic bug.
func TestUsernameContainingSeparator(t *testing.T) {
	a := NewWithSecret("secret")
	if got := a.Identify(a.Issue("a|b")); got != "a|b" {
		t.Errorf("round trip of a name containing | = %q, want a|b", got)
	}
}

func TestIdentifyRejectsTampering(t *testing.T) {
	a := NewWithSecret("secret")
	tok := a.Issue("alice")
	flipped := tok[:len(tok)-1] + map[bool]string{true: "0", false: "1"}[tok[len(tok)-1] != '0']
	for _, tc := range []struct{ name, token string }{
		{"flipped signature byte", flipped},
		{"forged signature", "alice|9999999999|deadbeef"},
		{"no signature", "alice|9999999999"},
		{"empty", ""},
		{"nonsense", "nonsense"},
		{"signed by a different secret", NewWithSecret("other").Issue("alice")},
	} {
		if got := a.Identify(tc.token); got != "" {
			t.Errorf("%s: Identify = %q, want rejection", tc.name, got)
		}
	}
}

func TestIdentifyRejectsExpired(t *testing.T) {
	a := NewWithSecret("secret")
	past := time.Now().Unix() - 1
	body := "alice|" + strconv.FormatInt(past, 10)
	if got := a.Identify(body + "|" + a.sign(body)); got != "" {
		t.Error("a correctly signed but expired token must be rejected")
	}
}

func TestHashPasswordRoundTripAndSalting(t *testing.T) {
	h1, h2 := HashPassword("hunter2"), HashPassword("hunter2")
	if h1 == h2 {
		t.Error("two hashes of the same password are identical -- the salt is not random")
	}
	for _, h := range []string{h1, h2} {
		if !VerifyPassword("hunter2", h) {
			t.Error("a freshly generated hash does not verify")
		}
		if VerifyPassword("hunter3", h) {
			t.Error("the wrong password verified")
		}
		if !strings.HasPrefix(h, "pbkdf2$240000$") {
			t.Errorf("hash %q is not in the format LABEL_TOOL_USERS expects", h)
		}
	}
}

func TestUsersParsing(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	if Enabled() || len(Users()) != 0 {
		t.Error("no LABEL_TOOL_USERS must mean no login at all")
	}

	h := HashPassword("hunter2")
	t.Setenv("LABEL_TOOL_USERS", "alice:"+h+", bob:"+HashPassword("pw2")+",,broken,:nohash,noname:")
	u := Users()
	if len(u) != 2 {
		t.Errorf("parsed %d users from a list with blank and malformed entries, want 2: %v", len(u), u)
	}
	if u["alice"] != h {
		t.Error("a hash containing $ was truncated -- the split must be on the first colon only")
	}
	if !Enabled() || !Check("alice", "hunter2") {
		t.Error("alice should authenticate")
	}
	if Check("alice", "wrong") || Check("mallory", "hunter2") {
		t.Error("a wrong password or an unknown user must not authenticate")
	}
}

func TestOIDCSessionIdentityCannotCollideWithLocalUsername(t *testing.T) {
	value := OIDCSessionIdentity(OIDCIdentity{Subject: "company-user-1", Display: "alice"})
	if !strings.HasPrefix(value, "oidc:") {
		t.Fatalf("OIDC session identity = %q, want reserved oidc: prefix", value)
	}
	if _, parsed, found := strings.Cut(value, ":"); !found || VerifyPassword("pw", parsed) {
		t.Fatal("an OIDC session identity was accepted as a LABEL_TOOL_USERS name:hash entry")
	}
}
