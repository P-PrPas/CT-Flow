// Package images lists the images directly inside a folder.
//
// Shared by the pool and the test set, since both are just "images directly
// inside this directory, matching config.ImageExts". Ported from
// backend/inference/images.py.
package images

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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
