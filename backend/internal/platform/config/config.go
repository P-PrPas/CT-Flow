// Package config holds the app-wide settings and the one security control
// every request path depends on.
//
// Mode decides which filesystem the directory picker walks:
//
//	local -> the server runs on your own PC, so browsing the server IS browsing
//	         your PC. Roots = all drives.
//	vm    -> the server runs on a shared VM. Only VMDataRoot is browsable, and
//	         your datasets have to live there (a mounted share, usually).
//
// Set with the LABEL_TOOL_MODE env var. Ported from backend/config.py and
// backend/deps.py -- see docs/history/REFACTOR_PLAN.md.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is read once at startup. Nothing here changes while the process runs,
// which is why it is passed by value and never guarded by a lock.
type Config struct {
	Mode        string // "local" | "vm"
	VMDataRoot  string
	ModelsDir   string
	MaxUploadMB float64
}

// StateDirName is the fixed subfolder of a project's image folder that holds
// the prompt bank, the eval history and the events log. A project is just the
// one folder of images the user picks -- everything else lives in here or in
// PostgreSQL, so nothing needs a second folder selection.
//
// The inference sidecar deliberately does not know this name: it is sent an
// already-joined state_dir, so the convention has exactly one owner.
const StateDirName = ".ctflow"

// LabelColors is what the frontend cycles through per class index.
var LabelColors = []string{
	"#7dd8ff", "#f97316", "#34d399", "#a78bfa",
	"#fbbf24", "#fb7185", "#22d3ee", "#84cc16",
}

// DefaultConf is the confidence threshold evaluate/autolabel use when the
// request does not name one.
const DefaultConf = 0.25

// ImageExts is the set of extensions treated as images, lowercased. Matching
// is on extension only -- whether the bytes really decode is a separate check
// and the one that actually decides (see the upload handler).
var ImageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".bmp": true}

func Load() Config {
	return Config{
		Mode:        strings.ToLower(env("LABEL_TOOL_MODE", "local")),
		VMDataRoot:  env("LABEL_TOOL_VM_ROOT", "/opt/mount/project"),
		ModelsDir:   env("MODELS_DIR", "models"),
		MaxUploadMB: envFloat("LABEL_TOOL_MAX_UPLOAD_MB", 25),
	}
}

// BrowseRoots is what the folder picker starts from.
func (c Config) BrowseRoots() []string {
	if c.Mode == "vm" {
		return []string{c.VMDataRoot}
	}
	return []string{"/"}
}

// PathAllowed is a trust boundary, not a convenience: the browser can send any
// path string, so vm mode confines it to VMDataRoot. local mode is single-user
// on your own PC and allows everything.
//
// Three ways to get this wrong, all of them tried by someone at some point:
//
//   - strings.HasPrefix lets "/opt/mount/projectX" through when the root is
//     "/opt/mount/project". Comparison has to be by path component.
//   - filepath.Clean alone does not follow symlinks, so a link planted inside
//     the root that points at /etc still escapes.
//   - EvalSymlinks fails outright on a path that does not exist yet, which
//     the upload handler legitimately sends (it creates its destination).
//
// resolvePath below handles all three.
func (c Config) PathAllowed(p string) bool {
	if c.Mode != "vm" {
		return true
	}
	target, err := resolvePath(p)
	if err != nil {
		return false
	}
	root, err := resolvePath(c.VMDataRoot)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// StateDir is where a project's prompt bank, eval history and events log live.
// The caller has already run inputDir through PathAllowed; a subfolder of an
// allowed path is always itself allowed, so this needs no check of its own.
func StateDir(inputDir string) string {
	return filepath.Join(inputDir, StateDirName)
}

// resolvePath is Python's Path.resolve(strict=False): absolute, symlinks
// followed as far as they exist, and whatever does not exist yet appended
// verbatim.
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	// Walk up to the deepest ancestor that exists, resolve that, then put the
	// missing tail back on. A path that exists resolves on the first pass.
	rest := ""
	cur := abs
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil // nothing along this path exists; Abs already cleaned it
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
