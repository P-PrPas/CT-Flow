// Package models is the catalog of selectable YOLOE checkpoints.
//
// The catalog itself is backend/models.json, loaded at startup rather than
// compiled in: the inference sidecar reads the same file to resolve an id to a
// weight file, and two hand-maintained copies would eventually disagree about
// which checkpoints exist. A disagreement there is not a crash -- it is
// GET /api/config advertising a model the sidecar cannot load.
//
// Ported from backend/inference/models.py.
package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Entry is one selectable checkpoint. File is the weight's filename, resolved
// against MODELS_DIR; it never reaches the frontend.
type Entry struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Size   string `json:"size"`
	File   string `json:"file"`
	Note   string `json:"note"`
}

// PublicEntry is what GET /api/config returns. Available means the weight is
// already in MODELS_DIR; false does not mean unusable, only that the first
// predict/label with it pays an auto-download (which can be slow, or fail
// outright with no route to github).
type PublicEntry struct {
	ID        string `json:"id"`
	Family    string `json:"family"`
	Size      string `json:"size"`
	Note      string `json:"note"`
	Available bool   `json:"available"`
}

type Catalog struct {
	Default   string  `json:"default"`
	Entries   []Entry `json:"catalog"`
	modelsDir string
	byID      map[string]Entry
}

// Load reads the shared catalog. path is backend/models.json unless
// MODELS_CATALOG says otherwise.
func Load(path, modelsDir string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading model catalog: %w", err)
	}
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parsing model catalog %s: %w", path, err)
	}
	if len(c.Entries) == 0 {
		return nil, fmt.Errorf("model catalog %s is empty", path)
	}
	c.modelsDir = modelsDir
	c.byID = make(map[string]Entry, len(c.Entries))
	for _, e := range c.Entries {
		c.byID[e.ID] = e
	}
	if _, ok := c.byID[c.Default]; !ok {
		return nil, fmt.Errorf("model catalog %s names default %q, which is not in it", path, c.Default)
	}
	return &c, nil
}

// CheckpointPath is where this checkpoint lives, or lands once the sidecar's
// ultralytics auto-downloads it.
func (c *Catalog) CheckpointPath(id string) (string, error) {
	e, ok := c.byID[id]
	if !ok {
		return "", fmt.Errorf("unknown model %q", id)
	}
	return filepath.Join(c.modelsDir, e.File), nil
}

func (c *Catalog) Has(id string) bool {
	_, ok := c.byID[id]
	return ok
}

// IsAvailable checks the disk on every call rather than caching: the sidecar
// downloads into the same directory while this process is running, so a cached
// answer would go stale in the one direction that matters.
func (c *Catalog) IsAvailable(id string) bool {
	p, err := c.CheckpointPath(id)
	if err != nil {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// Public is what the frontend gets -- no local file path leaks out.
func (c *Catalog) Public() []PublicEntry {
	out := make([]PublicEntry, 0, len(c.Entries))
	for _, e := range c.Entries {
		out = append(out, PublicEntry{
			ID: e.ID, Family: e.Family, Size: e.Size, Note: e.Note,
			Available: c.IsAvailable(e.ID),
		})
	}
	return out
}
