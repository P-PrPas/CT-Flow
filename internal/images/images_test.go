package images

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSelectsAndSorts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"b.jpg", "a.JPEG", "c.png", "d.bmp", "e.PNG",
		"notes.txt", "weights.pt", "noext", ".hidden.jpg",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "deep.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := List(dir)
	want := []string{
		// Sorted by full path, which puts a leading dot first -- a
		// dot-prefixed image is listed here, unlike in the folder picker,
		// matching the Python original.
		filepath.Join(dir, ".hidden.jpg"),
		filepath.Join(dir, "a.JPEG"),
		filepath.Join(dir, "b.jpg"),
		filepath.Join(dir, "c.png"),
		filepath.Join(dir, "d.bmp"),
		filepath.Join(dir, "e.PNG"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

// A missing or unreadable folder is an empty list, never an error: the caller
// reports "no images in <dir>" for both, and a picker that 500s on one bad
// folder is worse than one that shows it as empty.
func TestListMissingOrEmpty(t *testing.T) {
	for name, dir := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "nope"),
		"empty":   t.TempDir(),
		"blank":   "",
		"a file":  mustFile(t),
	} {
		if got := List(dir); len(got) != 0 {
			t.Errorf("%s: got %v, want no images", name, got)
		}
	}
}

func mustFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
