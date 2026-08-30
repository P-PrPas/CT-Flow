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

// ListCached is List with a per-directory cache. GET /api/pool would otherwise
// readdir tens of thousands of files on every page of every scroll.
//
// Two conditions, and the second is why the first is not enough. A folder's
// mtime moves when an entry is added or removed, which is exactly when the set
// of images could differ -- on a local filesystem. LABEL_TOOL_VM_ROOT is
// documented as a mounted share, and NFS and SMB serve directory attributes out
// of a client-side cache, so that mtime can lag the actual folder by seconds.
// Without the TTL an image dropped into the folder stays invisible until
// something else happens to change it. listTTL bounds that regardless of what
// the filesystem does; the mtime check is what keeps the common case free.
//
// The returned slice is shared with the cache and with other callers: treat it
// as read-only (the pool handler filters it into new slices, never mutates it).
//
// ponytail: in-process map + sync.Mutex, one API process, and List runs under
// the lock so one slow readdir stalls the others. Same "no horizontal scale"
// ceiling as the job and claim trackers -- when they move, this moves.
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
	now := time.Now()
	if c, ok := listCache[dir]; ok && c.mtime.Equal(info.ModTime()) && now.Sub(c.read) < listTTL {
		return c.paths
	}
	paths := List(dir)
	// One entry per project holds every path in it -- a 50k folder is megabytes,
	// so this cannot grow without a bound the way a handful of counters could.
	// Evicting the oldest read is enough: the working set is the projects people
	// currently have open.
	if len(listCache) >= listCacheN {
		oldest, at := "", now
		for d, c := range listCache {
			if c.read.Before(at) || oldest == "" {
				oldest, at = d, c.read
			}
		}
		delete(listCache, oldest)
	}
	listCache[dir] = cachedList{mtime: info.ModTime(), read: now, paths: paths}
	return paths
}

const (
	// Short enough that a dropped-in image shows up on the next scroll, long
	// enough that paging through a gallery costs one readdir, not one per page.
	listTTL    = 10 * time.Second
	listCacheN = 32
)

type cachedList struct {
	mtime time.Time
	read  time.Time
	paths []string
}

var (
	listMu    sync.Mutex
	listCache = map[string]cachedList{}
)
