package httpapi

import (
	"os"
	"path/filepath"
	"testing"
)

// N-02: a bundled export buffers the whole zip in memory before the response
// is written, uncapped. tooManyToBundle is the guard -- it sums real on-disk
// sizes, so this writes actual files rather than faking a count.
func TestTooManyToBundle(t *testing.T) {
	dir := t.TempDir()
	// Truncate rather than writing real bytes: a sparse file reports the same
	// os.Stat size this cap reads, in no time and no real disk or memory for
	// what would otherwise be a 512MB+ write on every test run.
	write := func(name string, size int64) string {
		p := filepath.Join(dir, name)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		return p
	}

	atCap := []string{write("at-cap.bin", maxBundledExportBytes)}
	if err := tooManyToBundle(atCap); err != nil {
		t.Fatalf("exactly the cap must still be allowed: %v", err)
	}

	overCap := []string{write("over-cap.bin", maxBundledExportBytes+1)}
	err := tooManyToBundle(overCap)
	if err == nil {
		t.Fatal("one byte past the cap must be refused")
	}
	he, ok := err.(*httpError)
	if !ok || he.Status != 400 {
		t.Fatalf("want a 400 httpError, got %#v", err)
	}

	// A path that can't be stat'd (deleted since it was labelled) is skipped,
	// not fatal -- matching imageDims/readImage's own skip-not-fail contract.
	if err := tooManyToBundle([]string{filepath.Join(dir, "does-not-exist.bin")}); err != nil {
		t.Fatalf("an unstattable path must be skipped, not refused: %v", err)
	}
}
