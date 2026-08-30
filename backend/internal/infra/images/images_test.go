package images

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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

// resetCache clears the package-global so each ListCached test starts clean.
func resetCache(t *testing.T) {
	t.Helper()
	listMu.Lock()
	listCache = map[string]cachedList{}
	listMu.Unlock()
}

func TestListCachedServesTheCacheUntilTheFolderMtimeMoves(t *testing.T) {
	resetCache(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := ListCached(dir)
	if len(first) != 1 {
		t.Fatalf("first read = %v", first)
	}

	// Add a file but pin the folder mtime back to what the cache recorded: a
	// slow directory-attribute cache on a mounted share looks exactly like
	// this, and the cache is allowed to be a few seconds stale.
	at := mustStatMtime(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "b.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, at, at); err != nil {
		t.Fatal(err)
	}
	if got := ListCached(dir); len(got) != 1 {
		t.Fatalf("mtime unchanged but ListCached re-read: %v", got)
	}

	// Now let the mtime move -- the real "an image was added" signal.
	moved := at.Add(2 * time.Second)
	if err := os.Chtimes(dir, moved, moved); err != nil {
		t.Fatal(err)
	}
	if got := ListCached(dir); len(got) != 2 {
		t.Fatalf("mtime moved but ListCached kept the stale list: %v", got)
	}
}

func TestListCachedReReadsAfterTheTTL(t *testing.T) {
	resetCache(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ListCached(dir) // populate

	// Age the cache entry's read time past the TTL without touching the clock.
	listMu.Lock()
	c := listCache[dir]
	c.read = c.read.Add(-listTTL - time.Second)
	listCache[dir] = c
	listMu.Unlock()

	if err := os.WriteFile(filepath.Join(dir, "b.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	at := mustStatMtime(t, dir)
	_ = os.Chtimes(dir, at, at) // keep mtime frozen so only the TTL can trigger

	if got := ListCached(dir); len(got) != 2 {
		t.Fatalf("TTL elapsed but ListCached served the stale list: %v", got)
	}
}

func TestListCachedEvictsTheOldestReadPastTheCap(t *testing.T) {
	resetCache(t)
	base := t.TempDir()
	dirs := make([]string, listCacheN+5)
	for i := range dirs {
		d := filepath.Join(base, "p"+strconv.Itoa(i))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		dirs[i] = d
		ListCached(d)
		// Space the recorded read times so "oldest" is unambiguous.
		listMu.Lock()
		c := listCache[d]
		c.read = c.read.Add(time.Duration(i) * time.Millisecond)
		listCache[d] = c
		listMu.Unlock()
	}

	listMu.Lock()
	n := len(listCache)
	_, firstStillThere := listCache[dirs[0]]
	_, lastThere := listCache[dirs[len(dirs)-1]]
	listMu.Unlock()

	if n > listCacheN {
		t.Fatalf("cache holds %d entries, cap is %d", n, listCacheN)
	}
	if firstStillThere {
		t.Error("the oldest-read entry survived eviction")
	}
	if !lastThere {
		t.Error("the newest entry was evicted")
	}
}

func mustStatMtime(t *testing.T, dir string) time.Time {
	t.Helper()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}
