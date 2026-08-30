// Package images lists the images directly inside a folder.
//
// Shared by the pool and the test set, since both are just "images directly
// inside this directory, matching config.ImageExts". Ported from
// the FastAPI service's services/images.py.
package images

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/P-PrPas/CT-Flow/backend/internal/platform/config"
)

// List returns absolute paths, sorted, for the images directly inside dir. Not
// recursive, and a missing or unreadable directory is an empty list rather than
// an error -- callers report "no images in <dir>" either way.
//
// Selection is by extension only, deliberately matching the Python original:
// whether the bytes really decode is a separate and much more expensive
// question, asked only where it matters (upload validation).
func List(dir string) []string {
	if dir == "" {
		return []string{}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if config.ImageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	// Sorted by full path: the frontend shows them in this order and the
	// smoke test indexes into it, so it is part of the contract.
	sort.Strings(out)
	return out
}

// ListCached is List with a per-directory cache, keyed on the folder's mtime --
// which changes exactly when an entry is added or removed, i.e. exactly when the
// set of images could differ. GET /api/pool would otherwise readdir tens of
// thousands of files on every page of every scroll.
//
// The returned slice is shared with the cache and with other callers: treat it
// as read-only (the pool handler filters it into new slices, never mutates it).
//
// ponytail: in-process map + sync.Mutex, one API process. Same "no horizontal
// scale" ceiling as the job and claim trackers -- when they move, this moves.
func ListCached(dir string) []string {
	if dir == "" {
		return []string{}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return []string{}
	}
	listMu.Lock()
	defer listMu.Unlock()
	if c, ok := listCache[dir]; ok && c.mtime.Equal(info.ModTime()) {
		return c.paths
	}
	paths := List(dir)
	listCache[dir] = cachedList{mtime: info.ModTime(), paths: paths}
	return paths
}

type cachedList struct {
	mtime time.Time
	paths []string
}

var (
	listMu    sync.Mutex
	listCache = map[string]cachedList{}
)
