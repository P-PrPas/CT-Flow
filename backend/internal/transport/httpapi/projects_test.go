package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/claims"
)

// The decisions the projects handlers make before any database is involved:
// what a valid create request is, what an id in the path means, and how a write
// to a folder that is not a project is reported. Store stays nil, same as the
// rest of this file -- anything that needs rows is covered end to end by
// backend/tests/smoke_test.py.

func TestCreateProjectRejectsBadRequests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.png"), pngBytes(t, 4, 4))
	s := vmServer(t, root)

	cases := []struct {
		name, body string
		status     int
		detail     string
	}{
		{
			name:   "no name",
			body:   `{"name":"  ","input_dir":"` + root + `"}`,
			status: http.StatusBadRequest,
			detail: "a project needs a name",
		},
		{
			// One task type exists. Storing another would make the project a
			// promise no module can keep.
			name:   "unknown task type",
			body:   `{"name":"x","input_dir":"` + root + `","task_type":"segmentation"}`,
			status: http.StatusBadRequest,
			detail: "unknown task type: segmentation",
		},
		{
			// The path gate applies here exactly as it does everywhere else a
			// path arrives from the browser.
			name:   "folder outside the root",
			body:   `{"name":"x","input_dir":"/etc"}`,
			status: http.StatusForbidden,
			detail: "path outside " + root + " (vm mode)",
		},
		{
			name:   "folder with no images",
			body:   `{"name":"x","input_dir":"` + filepath.Join(root, "empty") + `"}`,
			status: http.StatusBadRequest,
			detail: "no images in " + filepath.Join(root, "empty"),
		},
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(tc.body))
			w := do(s, s.CreateProject, req)
			if w.Code != tc.status {
				t.Errorf("status = %d, want %d (%s)", w.Code, tc.status, w.Body.String())
			}
			if got := detail(t, w); got != tc.detail {
				t.Errorf("detail = %q, want %q", got, tc.detail)
			}
		})
	}
}

// A non-numeric id is a URL nobody can have meant, and the UI has one branch for
// "this project is gone" -- so it is the same 404, not a 400.
func TestProjectPathIDRejectsNonNumeric(t *testing.T) {
	s := localServer(t)
	for _, h := range []struct {
		name    string
		handler Handler
		method  string
	}{
		{"get", s.GetProject, http.MethodGet},
		{"update", s.UpdateProject, http.MethodPatch},
		{"delete", s.DeleteProject, http.MethodDelete},
	} {
		t.Run(h.name, func(t *testing.T) {
			req := httptest.NewRequest(h.method, "/api/projects/not-a-number", strings.NewReader(`{}`))
			req.SetPathValue("id", "not-a-number")
			w := do(s, h.handler, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404 (%s)", w.Code, w.Body.String())
			}
			if got := detail(t, w); got != "no such project" {
				t.Errorf("detail = %q, want %q", got, "no such project")
			}
		})
	}
}

// An empty PATCH is a request that would report success while changing nothing,
// which reads to the caller as "the rename worked".
func TestUpdateProjectRejectsAnEmptyChange(t *testing.T) {
	s := localServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/projects/1", strings.NewReader(`{}`))
	req.SetPathValue("id", "1")
	w := do(s, s.UpdateProject, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if got := detail(t, w); got != "nothing to update" {
		t.Errorf("detail = %q, want %q", got, "nothing to update")
	}
}

// store.ErrNoProject is raised in five write paths and turned into a response in
// exactly one place, so the status and the wording cannot drift between them.
func TestErrNoProjectBecomesA404(t *testing.T) {
	s := localServer(t)
	w := do(s, func(http.ResponseWriter, *http.Request) error {
		return store.ErrNoProject
	}, httptest.NewRequest(http.MethodPost, "/api/label", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
	if got := detail(t, w); got != "no project for this folder -- create it first" {
		t.Errorf("detail = %q", got)
	}
}

// The name a folder opened directly gets, before anyone renames it.
func TestDefaultProjectName(t *testing.T) {
	for in, want := range map[string]string{
		"/opt/mount/project/cubes_conveyor":  "cubes_conveyor",
		"/opt/mount/project/cubes_conveyor/": "cubes_conveyor",
		"/":                                  "/",
	} {
		if got := defaultProjectName(in); got != want {
			t.Errorf("defaultProjectName(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- claims (FR-49) ---------------------------------------------------------
// The tracker's own behaviour is covered in internal/platform/claims; what is
// checked here is the HTTP shape and the one thing only this layer can get
// wrong -- putting a subject on screen where a name belongs.

func TestClaimChecksThePathLikeEveryOtherEndpoint(t *testing.T) {
	root := t.TempDir()
	s := vmServer(t, root)
	s.Claims = claims.NewTracker()

	req := httptest.NewRequest(http.MethodPost, "/api/claim",
		strings.NewReader(`{"input_dir":"/etc","image":"/etc/passwd"}`))
	w := do(s, s.Claim, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (%s)", w.Code, w.Body.String())
	}
	if got := detail(t, w); got != "path outside "+root+" (vm mode)" {
		t.Errorf("detail = %q", got)
	}
}

// The 409 conflict is asserted in backend/tests/smoke_test.py, not here: it has
// to resolve the holder's subject into a name through the users table, and a
// handler that could answer that without a database would be one that prints
// subjects at people.
