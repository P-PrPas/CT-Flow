package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The cases from docs/history/REFACTOR_PLAN.md section 4.2. Every one of these is a way
// a naive port of config.path_allowed() lets a request out of the root, and the
// browser picks the string, so each is reachable by anyone who can load the UI.
func TestPathAllowedVMMode(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "project")
	mustMkdir(t, filepath.Join(root, "dataset"))

	// A sibling whose name merely starts with the root's: the HasPrefix trap.
	mustMkdir(t, filepath.Join(tmp, "projectX"))

	// A symlink planted inside the root pointing out of it: the Clean-only trap.
	outside := filepath.Join(tmp, "secrets")
	mustMkdir(t, outside)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// ...and one pointing back inside, which must still be allowed.
	if err := os.Symlink(filepath.Join(root, "dataset"), filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}

	c := Config{Mode: "vm", VMDataRoot: root}
	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{"the root itself", root, true},
		{"a folder inside", filepath.Join(root, "dataset"), true},
		{"a path that does not exist yet (upload creates its destination)",
			filepath.Join(root, "new", "upload"), true},
		{"the state dir of a project inside", StateDir(filepath.Join(root, "dataset")), true},
		{"a symlink inside pointing back inside", filepath.Join(root, "inside"), true},

		{"a sibling directory sharing the root's name prefix", filepath.Join(tmp, "projectX"), false},
		{"a sibling's contents", filepath.Join(tmp, "projectX", "steal.jpg"), false},
		{"the parent", tmp, false},
		{"traversal out with ..", filepath.Join(root, "..", "secrets"), false},
		{"traversal out with .. to a missing file",
			filepath.Join(root, "..", "secrets", "nope.txt"), false},
		{"an unrelated absolute path", "/etc/passwd", false},
		{"a symlink inside pointing out", filepath.Join(root, "escape"), false},
		{"through a symlink inside pointing out", filepath.Join(root, "escape", "creds"), false},
		{"empty", "", false},
	} {
		if got := c.PathAllowed(tc.path); got != tc.want {
			t.Errorf("PathAllowed(%s) = %v, want %v -- %s", tc.path, got, tc.want, tc.name)
		}
	}
}

// local mode is the "server runs on your own PC" case: browsing the server is
// browsing your own machine, so there is nothing to confine it to.
func TestPathAllowedLocalMode(t *testing.T) {
	c := Config{Mode: "local", VMDataRoot: "/opt/mount/project"}
	for _, p := range []string{"/etc/passwd", "", "../..", "C:\\Users"} {
		if !c.PathAllowed(p) {
			t.Errorf("local mode should allow %q", p)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	for _, k := range []string{"LABEL_TOOL_MODE", "LABEL_TOOL_VM_ROOT", "MODELS_DIR", "LABEL_TOOL_MAX_UPLOAD_MB"} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.Mode != "local" {
		t.Errorf("default mode = %q, want local (backend/config.py's default)", c.Mode)
	}
	if c.VMDataRoot != "/opt/mount/project" || c.MaxUploadMB != 25 {
		t.Errorf("unexpected defaults: %+v", c)
	}

	// A cap that does not parse must fall back rather than becoming 0, which
	// would silently reject every upload.
	t.Setenv("LABEL_TOOL_MAX_UPLOAD_MB", "not-a-number")
	if got := Load().MaxUploadMB; got != 25 {
		t.Errorf("unparseable cap = %v, want the 25 MB fallback", got)
	}
	t.Setenv("LABEL_TOOL_MODE", "VM")
	if Load().Mode != "vm" {
		t.Error("mode should be lowercased, as config.py does")
	}
}

func TestBrowseRoots(t *testing.T) {
	if got := (Config{Mode: "vm", VMDataRoot: "/data"}).BrowseRoots(); len(got) != 1 || got[0] != "/data" {
		t.Errorf("vm roots = %v, want [/data]", got)
	}
	if got := (Config{Mode: "local"}).BrowseRoots(); len(got) != 1 || got[0] != "/" {
		t.Errorf("local roots = %v, want [/]", got)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}
