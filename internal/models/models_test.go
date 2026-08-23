package models

import (
	"os"
	"path/filepath"
	"testing"
)

// The real catalog, not a fixture: this is the file the Python sidecar loads
// too, so if it stops satisfying what the API needs, both halves are affected
// and this is the cheapest place to find out.
func catalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "backend", "models.json")
}

func TestLoadSharedCatalog(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(catalogPath(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Default != "yoloe-11s-seg" {
		t.Errorf("default = %q, want yoloe-11s-seg", c.Default)
	}
	if len(c.Entries) != 11 {
		t.Errorf("got %d checkpoints, want the 11 prompt-capable ones", len(c.Entries))
	}
	seen := map[string]bool{}
	for _, e := range c.Entries {
		if e.ID == "" || e.File == "" || e.Family == "" || e.Size == "" {
			t.Errorf("incomplete entry %+v -- the sidecar cannot load it and the UI cannot label it", e)
		}
		if filepath.Ext(e.File) != ".pt" {
			t.Errorf("%s: weight file %q is not a .pt", e.ID, e.File)
		}
		if seen[e.ID] {
			t.Errorf("duplicate model id %q", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestPublicHidesLocalPaths(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(catalogPath(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	pub := c.Public()
	if len(pub) != len(c.Entries) {
		t.Fatalf("public catalog dropped entries: %d vs %d", len(pub), len(c.Entries))
	}
	for _, e := range pub {
		if e.Available {
			t.Errorf("%s reported available from an empty MODELS_DIR", e.ID)
		}
	}

	// Availability is read from disk per call: the sidecar downloads into this
	// directory while the API is running, so a cached answer would stay false
	// forever after the first miss.
	weight, err := c.CheckpointPath(c.Default)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(weight, []byte("not really a checkpoint"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !c.IsAvailable(c.Default) {
		t.Error("a weight that appeared on disk after load must read as available")
	}
	for _, e := range c.Public() {
		if e.ID == c.Default && !e.Available {
			t.Error("Public() did not pick up the newly present weight")
		}
	}
}

func TestUnknownModel(t *testing.T) {
	c, err := Load(catalogPath(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CheckpointPath("not-a-real-model"); err == nil {
		t.Error("an unknown model id must be an error, not an empty path")
	}
	if c.Has("not-a-real-model") {
		t.Error("Has() lied about an unknown model")
	}
	if c.IsAvailable("not-a-real-model") {
		t.Error("an unknown model cannot be available")
	}
}

func TestLoadRejectsBrokenCatalog(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"not json":            "{{{",
		"empty catalog":       `{"default": "x", "catalog": []}`,
		"default not present": `{"default": "nope", "catalog": [{"id": "a", "file": "a.pt"}]}`,
	} {
		p := filepath.Join(dir, "c.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p, dir); err == nil {
			t.Errorf("%s: should have failed to load", name)
		}
	}
	if _, err := Load(filepath.Join(dir, "missing.json"), dir); err == nil {
		t.Error("a missing catalog must fail at startup, not at the first request")
	}
}
