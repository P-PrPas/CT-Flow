// Tests for the HTTP layer's own decisions -- the ones no other package can
// make and no other test covers.
//
// Everything here runs against an httptest recorder with Store and VPE left
// nil, which is not laziness but the point: what is being checked is the
// response shape, the path trust boundary and the auth gate, and a handler that
// needed a database to answer those would have the boundary in the wrong place.
// The handlers that genuinely need PostgreSQL or a model are covered by
// backend/tests/smoke_test.py end to end.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/P-PrPas/CT-Flow/backend/internal/core/metrics"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/vpe"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/auth"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/config"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/jobs"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/models"
	"github.com/P-PrPas/CT-Flow/backend/internal/testsupport"
)

// discard, so a test that deliberately triggers the 500 path does not print a
// stack of noise between the results.
func testServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	catalog, err := models.Load(testsupport.MustBackendFile("models.json"), cfg.ModelsDir)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		Cfg:     cfg,
		Catalog: catalog,
		Auth:    auth.NewWithSecret("test-secret"),
		Jobs:    jobs.NewTracker(),
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// localServer confines to "/", which is as permissive as a deployment can be
// now that confinement is unconditional (T-27) -- for the tests whose subject is
// not the path gate.
func localServer(t *testing.T) *Server {
	return testServer(t, config.Config{VMDataRoot: "/", ModelsDir: "models", MaxUploadMB: 25})
}

// vmServer confines the deployment to root, which is what makes every path in a
// request a trust boundary rather than a convenience.
func vmServer(t *testing.T, root string) *Server {
	return testServer(t, config.Config{
		VMDataRoot: root, ModelsDir: "models", MaxUploadMB: 25,
	})
}

func do(s *Server, h Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handle(h)(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("body is not JSON (%d): %q", w.Code, w.Body.String())
	}
	return out
}

// detail is what lib/api.ts reads off every failed response, so it is checked by
// name in most of what follows.
func detail(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	body := decode(t, w)
	got, ok := body["detail"].(string)
	if !ok {
		t.Fatalf("no string `detail` in %v", body)
	}
	return got
}

// pngBytes is a real, decodable image. Generated rather than committed as a
// fixture: the upload validator only reads the header, so a handful of bytes
// that genuinely decode is worth more than a file someone has to go find.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeFile(t *testing.T, path string, body []byte) string {
	t.Helper()
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------------------------------------------------------------- error shape

// Every failure the frontend can see has to be {"detail": "..."} -- lib/api.ts
// throws Error(data.detail) for any non-ok response, so a handler answering in
// any other shape shows the user "undefined".
func TestHandleWritesTheDetailShape(t *testing.T) {
	s := localServer(t)

	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
		wantDetail string
	}{
		{"httpError", errStatus(http.StatusConflict, "nope"), http.StatusConflict, "nope"},
		{"not found", ErrNotFound, http.StatusNotFound, "not found"},
		// The sidecar's refusals are this API's refusals: the browser must not be
		// able to tell that a check moved out of process.
		{"sidecar error", &vpe.Error{Status: http.StatusConflict, Detail: "this project was taught with 'x'"},
			http.StatusConflict, "this project was taught with 'x'"},
		// An unexpected error is logged in full and reported as one line, so a
		// path or a query cannot leak through it.
		{"unexpected", errors.New("pq: relation \"annotations\" does not exist"),
			http.StatusInternalServerError, "internal error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := do(s, func(http.ResponseWriter, *http.Request) error { return tc.err },
				httptest.NewRequest(http.MethodGet, "/api/anything", nil))
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if got := detail(t, w); got != tc.wantDetail {
				t.Errorf("detail = %q, want %q", got, tc.wantDetail)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q, want application/json", ct)
			}
		})
	}
}

// A handler that already wrote its response and then failed must not have a
// second status stapled on: Handle only writes when the handler returns an
// error, so success is silent.
func TestHandleWritesNothingOnSuccess(t *testing.T) {
	s := localServer(t)
	w := do(s, func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("raw bytes"))
		return nil
	}, httptest.NewRequest(http.MethodGet, "/api/image", nil))

	if w.Code != http.StatusAccepted || w.Body.String() != "raw bytes" {
		t.Errorf("got %d %q, want 202 %q", w.Code, w.Body.String(), "raw bytes")
	}
}

// ------------------------------------------------------------- trust boundary

// The message is copied verbatim from the FastAPI original; the smoke test and
// the parity differ both compare it, so it is a contract, not a string.
func TestCheckedPathRefusalMessage(t *testing.T) {
	root := t.TempDir()
	s := vmServer(t, root)

	if _, err := s.checkedPath("/etc/passwd"); err == nil {
		t.Fatal("a path outside the root was allowed in vm mode")
	} else if he := new(httpError); !errors.As(err, &he) {
		t.Fatalf("error is %T, want *httpError", err)
	} else {
		if he.Status != http.StatusForbidden {
			t.Errorf("status = %d, want 403", he.Status)
		}
		if want := "path outside " + root + " (vm mode)"; he.Message != want {
			t.Errorf("message = %q, want %q", he.Message, want)
		}
	}
	if _, err := s.checkedPath(filepath.Join(root, "sub", "a.jpg")); err != nil {
		t.Errorf("a path inside the root was refused: %v", err)
	}
}

// checkedPaths is the list form used by score/autolabel/testset. One bad path
// has to fail the request rather than being quietly dropped from the batch --
// silently scoring 9 of 10 images is worse than refusing.
func TestCheckedPathsRefusesTheWholeList(t *testing.T) {
	root := t.TempDir()
	s := vmServer(t, root)
	good := filepath.Join(root, "a.jpg")

	if got, err := s.checkedPaths([]string{good, good}); err != nil || len(got) != 2 {
		t.Errorf("got %v (err %v), want both paths", got, err)
	}
	if _, err := s.checkedPaths([]string{good, "/etc/shadow"}); err == nil {
		t.Error("a list containing an outside path was accepted")
	}
}

// Every path-taking handler has to run its input through the boundary. This is
// the check that a new endpoint forgetting to is caught by something.
func TestPathHandlersRefuseOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	s := vmServer(t, root)
	const outside = "/etc"

	for _, tc := range []struct {
		name string
		call func() *httptest.ResponseRecorder
	}{
		{"GET /api/browse", func() *httptest.ResponseRecorder {
			return do(s, s.Browse, httptest.NewRequest(http.MethodGet, "/api/browse?path="+outside, nil))
		}},
		{"GET /api/image", func() *httptest.ResponseRecorder {
			return do(s, s.GetImage, httptest.NewRequest(http.MethodGet, "/api/image?path="+outside+"/passwd", nil))
		}},
		{"GET /api/history", func() *httptest.ResponseRecorder {
			return do(s, s.GetHistory, httptest.NewRequest(http.MethodGet, "/api/history?input_dir="+outside, nil))
		}},
		{"GET /api/events", func() *httptest.ResponseRecorder {
			return do(s, s.GetEvents, httptest.NewRequest(http.MethodGet, "/api/events?input_dir="+outside, nil))
		}},
		{"POST /api/events", func() *httptest.ResponseRecorder {
			return do(s, s.AddEvent, jsonReq(http.MethodPost, "/api/events",
				map[string]any{"input_dir": outside, "kind": "label"}))
		}},
		{"POST /api/history", func() *httptest.ResponseRecorder {
			return do(s, s.AddHistory, jsonReq(http.MethodPost, "/api/history",
				map[string]any{"input_dir": outside, "point": map[string]any{}}))
		}},
		{"DELETE /api/history", func() *httptest.ResponseRecorder {
			return do(s, s.DeleteHistory, httptest.NewRequest(http.MethodDelete, "/api/history?input_dir="+outside, nil))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.call()
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 -- %s reached the filesystem with an outside path", w.Code, tc.name)
			}
			if want := "path outside " + root + " (vm mode)"; detail(t, w) != want {
				t.Errorf("detail = %q, want %q", detail(t, w), want)
			}
		})
	}
}

func jsonReq(method, target string, body any) *http.Request {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	r := httptest.NewRequest(method, target, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// A malformed body is a 400 with a reason, not a 500 -- the UI shows it.
func TestMalformedJSONBodyIs400(t *testing.T) {
	s := localServer(t)
	r := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader([]byte("{not json")))
	w := do(s, s.AddEvent, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := detail(t, w); !bytes.Contains([]byte(got), []byte("malformed request body")) {
		t.Errorf("detail = %q, want it to start with `malformed request body`", got)
	}
}

// -------------------------------------------------------------------- /config

// The one call the UI needs before it can render anything, and the container
// healthcheck: it must touch neither the database nor the sidecar. Both are nil
// here, so a handler that reached for either would panic.
func TestGetConfigTouchesNothingAndLeaksNoPaths(t *testing.T) {
	s := vmServer(t, "/opt/mount/project")
	w := do(s, s.GetConfig, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decode(t, w)

	// No `mode` field since T-27: it reported "local" vs "vm", and there is
	// only one behaviour left for it to report.
	if _, present := body["mode"]; present {
		t.Errorf("mode is still in the response: %v", body["mode"])
	}
	if roots, ok := body["roots"].([]any); !ok || len(roots) != 1 || roots[0] != "/opt/mount/project" {
		t.Errorf("roots = %v, want only the vm root", body["roots"])
	}
	if body["default_model"] == "" || body["default_model"] == nil {
		t.Error("default_model is empty")
	}
	entries, ok := body["models"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("models = %v, want the catalog", body["models"])
	}
	// PublicEntry deliberately has no File: a local weight path is not the
	// browser's business.
	for _, e := range entries {
		if _, leaked := e.(map[string]any)["file"]; leaked {
			t.Fatalf("a checkpoint's local file path reached the frontend: %v", e)
		}
	}
}

// -------------------------------------------------------------------- /browse

// The initial call, which the picker makes before the user has chosen anything.
// Answered without touching the filesystem at all, hence a root that does not
// exist.
func TestBrowseInitialCallTouchesNoFilesystem(t *testing.T) {
	s := vmServer(t, "/no/such/place")
	w := do(s, s.Browse, httptest.NewRequest(http.MethodGet, "/api/browse", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body)
	}
	body := decode(t, w)
	if body["parent"] != nil {
		t.Errorf("parent = %v, want null so DirPicker draws no `up` control", body["parent"])
	}
	if dirs, ok := body["dirs"].([]any); !ok || len(dirs) != 0 {
		t.Errorf("dirs = %v, want an empty list (not null)", body["dirs"])
	}
}

func TestBrowseListsVisibleDirsAndCountsImages(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dataset"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The state dir the tool writes into its own project folders. It must not
	// show up as somewhere to browse into.
	if err := os.MkdirAll(filepath.Join(root, config.StateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "a.jpg"), pngBytes(t, 2, 2))
	writeFile(t, filepath.Join(root, "b.png"), pngBytes(t, 2, 2))
	writeFile(t, filepath.Join(root, "notes.txt"), []byte("ignored"))

	s := vmServer(t, root)
	w := do(s, s.Browse, httptest.NewRequest(http.MethodGet, "/api/browse?path="+root, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	body := decode(t, w)

	dirs, _ := body["dirs"].([]any)
	if len(dirs) != 1 {
		t.Fatalf("dirs = %v, want only `dataset` (dotfolders are skipped)", body["dirs"])
	}
	if name := dirs[0].(map[string]any)["name"]; name != "dataset" {
		t.Errorf("dir = %v, want dataset", name)
	}
	// Counted so a dataset folder is distinguishable from a container folder.
	if body["images"] != float64(2) {
		t.Errorf("images = %v, want 2 (by extension, .txt excluded)", body["images"])
	}
	// At the root of a vm deployment there is nowhere up to go.
	if body["parent"] != nil {
		t.Errorf("parent = %v, want null at the vm root", body["parent"])
	}
}

func TestBrowseOnAFileIs404(t *testing.T) {
	root := t.TempDir()
	file := writeFile(t, filepath.Join(root, "a.jpg"), pngBytes(t, 2, 2))
	s := vmServer(t, root)

	w := do(s, s.Browse, httptest.NewRequest(http.MethodGet, "/api/browse?path="+file, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if got := detail(t, w); got != "not a directory" {
		t.Errorf("detail = %q", got)
	}
}

// --------------------------------------------------------------------- /image

func TestGetImageServesBytesAndMissesAre404(t *testing.T) {
	root := t.TempDir()
	want := pngBytes(t, 3, 4)
	file := writeFile(t, filepath.Join(root, "a.png"), want)
	s := vmServer(t, root)

	w := do(s, s.GetImage, httptest.NewRequest(http.MethodGet, "/api/image?path="+file, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	if !bytes.Equal(w.Body.Bytes(), want) {
		t.Error("served bytes differ from the file on disk")
	}

	w = do(s, s.GetImage, httptest.NewRequest(http.MethodGet,
		"/api/image?path="+filepath.Join(root, "gone.png"), nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if got := detail(t, w); got != "image not found" {
		t.Errorf("detail = %q", got)
	}
}

// A directory is not an image, and serving one would list it.
func TestGetImageRefusesADirectory(t *testing.T) {
	root := t.TempDir()
	s := vmServer(t, root)
	w := do(s, s.GetImage, httptest.NewRequest(http.MethodGet, "/api/image?path="+root, nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a directory", w.Code)
	}
}

// ---------------------------------------------------------------------- /jobs

func TestGetJobShapeAndUnknownID(t *testing.T) {
	s := localServer(t)

	r := httptest.NewRequest(http.MethodGet, "/api/jobs/nope", nil)
	r.SetPathValue("id", "nope")
	w := do(s, s.GetJob, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if got := detail(t, w); got != "unknown job" {
		t.Errorf("detail = %q", got)
	}

	id := s.Jobs.Create(7)
	s.Jobs.Tick(id, 3)
	r = httptest.NewRequest(http.MethodGet, "/api/jobs/"+id, nil)
	r.SetPathValue("id", id)
	w = do(s, s.GetJob, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	body := decode(t, w)
	for k, want := range map[string]any{
		"done": float64(3), "total": float64(7), "finished": false,
		"result": nil, "error": nil,
	} {
		if body[k] != want {
			t.Errorf("%s = %v, want %v", k, body[k], want)
		}
	}
	// The server's clock, so ProgressBar.tsx can compute an ETA without trusting
	// the browser's.
	now, ok := body["now"].(float64)
	if !ok || now <= 0 {
		t.Errorf("now = %v, want the server clock", body["now"])
	}
	if started, _ := body["started_at"].(float64); started <= 0 || started > now {
		t.Errorf("started_at = %v, want >0 and <= now (%v)", body["started_at"], now)
	}
}

// -------------------------------------------------------------------- helpers

// conf 0 is a real request -- FR-33's per-class thresholds are exercised with
// it -- so a zero value cannot stand in for "no conf sent".
func TestOrDefaultConfDistinguishesZeroFromAbsent(t *testing.T) {
	if got := orDefaultConf(nil); got != config.DefaultConf {
		t.Errorf("absent -> %v, want the default %v", got, config.DefaultConf)
	}
	zero := 0.0
	if got := orDefaultConf(&zero); got != 0 {
		t.Errorf("conf:0 -> %v, want 0", got)
	}
}

// These reproduce Python's repr() because the smoke test compares the messages
// and the user reads them: "unknown format 'xml'", not `"xml"` and not %q.
func TestPyReprMatchesPython(t *testing.T) {
	for in, want := range map[string]string{
		"yolo": "'yolo'", "": "''", "it's": `'it\'s'`,
	} {
		if got := pyRepr(in); got != want {
			t.Errorf("pyRepr(%q) = %s, want %s", in, got, want)
		}
	}
	if got := pyReprList([]string{"coco", "voc"}); got != "['coco', 'voc']" {
		t.Errorf("pyReprList = %s", got)
	}
	if got := pyReprList(nil); got != "[]" {
		t.Errorf("pyReprList(nil) = %s, want []", got)
	}
}

// f"{v:g}": 25 renders as "25", not "25.000000".
func TestTrimFloatMatchesPythonG(t *testing.T) {
	for in, want := range map[float64]string{25: "25", 0.5: "0.5", 1.25: "1.25"} {
		if got := trimFloat(in); got != want {
			t.Errorf("trimFloat(%v) = %q, want %q", in, got, want)
		}
	}
}

// The thresholds are echoed back with the numbers, so a recorded history point
// says what it was measured at. conf_by_class must be {} and never null: the
// report tab indexes into it.
func TestWithThresholdsNeverEmitsNullByClass(t *testing.T) {
	got := withThresholds(metrics.Result{IoU: 0.5}, 0.25, nil)
	if got["conf"] != 0.25 {
		t.Errorf("conf = %v", got["conf"])
	}
	byClass, ok := got["conf_by_class"].(map[string]float64)
	if !ok || byClass == nil {
		t.Fatalf("conf_by_class = %#v, want an empty map", got["conf_by_class"])
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"conf_by_class":{}`)) {
		t.Errorf("serialised as %s, want conf_by_class:{}", raw)
	}
}
