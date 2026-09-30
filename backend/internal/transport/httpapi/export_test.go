package httpapi

import "testing"

// N-02: a bundled export buffers the whole zip in memory before the response
// is written, uncapped. tooManyToBundle is the guard -- pure, so no store or
// DB is needed to check it fires exactly at the line it names.
func TestTooManyToBundle(t *testing.T) {
	if err := tooManyToBundle(maxBundledExportImages); err != nil {
		t.Fatalf("exactly the cap must still be allowed: %v", err)
	}
	err := tooManyToBundle(maxBundledExportImages + 1)
	if err == nil {
		t.Fatal("one past the cap must be refused")
	}
	he, ok := err.(*httpError)
	if !ok || he.Status != 400 {
		t.Fatalf("want a 400 httpError, got %#v", err)
	}
}
