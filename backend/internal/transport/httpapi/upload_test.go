package httpapi

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// part is one field of a multipart body. Files carry a filename, plain fields do
// not -- and the distinction matters here, because the filename is the untrusted
// string the whole upload validator exists for.
type part struct {
	field, filename string
	body            []byte
}

// The `dir` field has to arrive before the files: the request is read as a
// stream, so the destination has to be known before the first byte of a file is.
func dirPart(dest string) part { return part{field: "dir", body: []byte(dest)} }

func filePart(name string, body []byte) part {
	return part{field: "files", filename: name, body: body}
}

func multipartReq(t *testing.T, parts ...part) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		var (
			w   interface{ Write([]byte) (int, error) }
			err error
		)
		if p.filename == "" {
			w, err = mw.CreateFormField(p.field)
		} else {
			w, err = mw.CreateFormFile(p.field, p.filename)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(p.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

// skipReasons maps the skipped entries by name, so a test can say "this file was
// refused, and for this reason" without indexing into a list by position.
func skipReasons(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, entry := range decode(t, w)["skipped"].([]any) {
		e := entry.(map[string]any)
		out[e["name"].(string)] = e["reason"].(string)
	}
	return out
}

func savedNames(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	out := []string{}
	for _, p := range decode(t, w)["saved"].([]any) {
		out = append(out, filepath.Base(p.(string)))
	}
	return out
}

// T-13's precondition, in code rather than in a doc: a shared deployment that
// takes files from anyone who knows the URL is a worse problem than not having
// upload at all.
func TestUploadRefusedOnASharedServerWithoutUsers(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	root := t.TempDir()
	s := vmServer(t, root)

	w := do(s, s.Upload, multipartReq(t, dirPart(root), filePart("a.png", pngBytes(t, 2, 2))))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if got, want := detail(t, w), "set LABEL_TOOL_USERS before enabling upload on a shared server"; got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
	// And nothing landed.
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("files were written despite the refusal: %v", entries)
	}
}

// local mode is single-user on your own PC, so there is nobody to authenticate
// against and upload is allowed.
func TestUploadAllowedInLocalModeWithoutUsers(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	root := t.TempDir()
	s := localServer(t)

	w := do(s, s.Upload, multipartReq(t, dirPart(root), filePart("a.png", pngBytes(t, 2, 2))))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	if got := savedNames(t, w); len(got) != 1 || got[0] != "a.png" {
		t.Errorf("saved = %v, want [a.png]", got)
	}
}

// The filename is the untrusted string. Every one of these has been tried by
// someone; a traversal is neutralised into a plain name rather than merely
// refused, so the upload still succeeds and still lands inside the destination.
func TestUploadFilenameRules(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	root := t.TempDir()
	dest := filepath.Join(root, "pool")
	s := localServer(t)
	good := pngBytes(t, 2, 2)

	w := do(s, s.Upload, multipartReq(t,
		dirPart(dest),
		filePart("../../escape.png", good),        // traversal -> basename
		filePart(`..\windows.png`, good),          // backslash survives Base() on Linux
		filePart(".hidden.png", good),             // dotfile
		filePart("notes.txt", []byte("hello")),    // wrong extension
		filePart("lies.png", []byte("not a png")), // right extension, not an image
		filePart("keep.png", good),                // the one that should land
	))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}

	// The traversal is neutralised, not refused: it saves as a bare name.
	if got := savedNames(t, w); len(got) != 2 || got[0] != "escape.png" || got[1] != "keep.png" {
		t.Errorf("saved = %v, want [escape.png keep.png]", got)
	}
	// And it landed inside the destination, not two levels above it.
	if _, err := os.Stat(filepath.Join(dest, "escape.png")); err != nil {
		t.Errorf("escape.png is not in the destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "..", "escape.png")); err == nil {
		t.Error("a file escaped above the destination")
	}

	for name, want := range map[string]string{
		`..\windows.png`: "bad filename",
		".hidden.png":    "bad filename",
		"notes.txt":      "not an image file type",
		// The extension is a fast reject; the decode is the one that decides.
		"lies.png": "not a readable image",
	} {
		if got := skipReasons(t, w)[name]; got != want {
			t.Errorf("%s skipped as %q, want %q", name, got, want)
		}
	}
}

// One byte past the cap is enough to know it is over, so an oversized upload
// costs the cap rather than the file.
func TestUploadEnforcesTheSizeCap(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	root := t.TempDir()
	s := localServer(t)
	s.Cfg.MaxUploadMB = 0.001 // ~1 KB

	big := make([]byte, 4096)
	copy(big, pngBytes(t, 2, 2))

	w := do(s, s.Upload, multipartReq(t, dirPart(root), filePart("big.png", big)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	// f"{v:g}" -- "0.001 MB", not "0.001000 MB".
	if got, want := skipReasons(t, w)["big.png"], "larger than 0.001 MB"; got != want {
		t.Errorf("reason = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "big.png")); err == nil {
		t.Error("the oversized file was written anyway")
	}
}

// Never overwrite: an upload must not be able to replace an image someone has
// already labeled.
func TestUploadNeverOverwrites(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	root := t.TempDir()
	s := localServer(t)
	original := pngBytes(t, 7, 7)
	writeFile(t, filepath.Join(root, "a.png"), original)

	w := do(s, s.Upload, multipartReq(t, dirPart(root), filePart("a.png", pngBytes(t, 2, 2))))
	if got, want := skipReasons(t, w)["a.png"], "already in this folder"; got != want {
		t.Errorf("reason = %q, want %q", got, want)
	}
	after, err := os.ReadFile(filepath.Join(root, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Error("the existing file was replaced")
	}
}

// The destination goes through checkedPath like every other path.
func TestUploadRefusesADestinationOutsideTheRoot(t *testing.T) {
	withUser(t, "alice", "pw")
	root := t.TempDir()
	s := vmServer(t, root)
	outside := filepath.Join(t.TempDir(), "elsewhere")

	w := do(s, s.Upload, multipartReq(t, dirPart(outside), filePart("a.png", pngBytes(t, 2, 2))))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("the refused destination was created anyway")
	}
}

// Parts arrive in order, so a file before `dir` has nowhere to go. That is a
// client bug, not a case to guess at.
func TestUploadRequiresDirBeforeTheFiles(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	root := t.TempDir()
	s := localServer(t)

	w := do(s, s.Upload, multipartReq(t, filePart("a.png", pngBytes(t, 2, 2)), dirPart(root)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got, want := detail(t, w), "the `dir` field must come before the files"; got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}

	w = do(s, s.Upload, multipartReq(t, filePart("ignored", nil)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("no dir at all: status = %d, want 400", w.Code)
	}
}

func TestUploadRejectsANonMultipartBody(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	s := localServer(t)
	w := do(s, s.Upload, jsonReq(http.MethodPost, "/api/upload", map[string]string{"dir": "/tmp"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := detail(t, w); !bytes.Contains([]byte(got), []byte("expected multipart/form-data")) {
		t.Errorf("detail = %q", got)
	}
}

// The response carries the folder's images so the UI can re-render from it
// without a second round trip.
func TestUploadReturnsTheFolderListing(t *testing.T) {
	t.Setenv("LABEL_TOOL_USERS", "")
	root := t.TempDir()
	s := localServer(t)
	writeFile(t, filepath.Join(root, "existing.jpg"), pngBytes(t, 2, 2))

	w := do(s, s.Upload, multipartReq(t, dirPart(root), filePart("new.png", pngBytes(t, 2, 2))))
	images, ok := decode(t, w)["images"].([]any)
	if !ok || len(images) != 2 {
		t.Fatalf("images = %v, want both files", decode(t, w)["images"])
	}
	// Sorted by full path: the frontend shows them in this order.
	if got := fmt.Sprint(filepath.Base(images[0].(string)), filepath.Base(images[1].(string))); got != "existing.jpgnew.png" {
		t.Errorf("images out of order: %v", images)
	}
}
