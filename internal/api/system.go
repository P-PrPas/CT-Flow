package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/P-PrPas/CT-Flow/internal/config"
	"github.com/P-PrPas/CT-Flow/internal/images"
	"github.com/P-PrPas/CT-Flow/internal/models"
)

type configResponse struct {
	Mode         string               `json:"mode"`
	Roots        []string             `json:"roots"`
	Colors       []string             `json:"colors"`
	Models       []models.PublicEntry `json:"models"`
	DefaultModel string               `json:"default_model"`
}

// GetConfig reports the deployment's mode, what the folder picker can reach,
// the per-class box colours, and which checkpoints are selectable.
//
// Always reachable, even signed out -- the UI needs it before it can draw a
// login box -- and it is also the container healthcheck, which is why it
// touches neither the database nor the inference sidecar.
func (s *Server) GetConfig(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, configResponse{
		Mode:         s.Cfg.Mode,
		Roots:        s.Cfg.BrowseRoots(),
		Colors:       config.LabelColors,
		Models:       s.Catalog.Public(),
		DefaultModel: s.Catalog.Default,
	})
	return nil
}

type browseDir struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type browseResponse struct {
	Path string `json:"path"`
	// Pointer so an unavailable parent serialises as null, not "" -- DirPicker
	// tests it for null to decide whether to draw an "up" control.
	Parent *string     `json:"parent"`
	Dirs   []browseDir `json:"dirs"`
	Images int         `json:"images"`
	Roots  []string    `json:"roots"`
}

// Browse backs the directory picker: the subdirectories of `path` plus how many
// images each level holds, so a dataset folder is distinguishable from a
// container folder.
func (s *Server) Browse(w http.ResponseWriter, r *http.Request) error {
	path := r.URL.Query().Get("path")
	if path == "" {
		// The initial call. Answered without touching the filesystem at all.
		writeJSON(w, http.StatusOK, browseResponse{
			Path: "", Parent: nil, Dirs: []browseDir{}, Images: 0, Roots: s.Cfg.BrowseRoots(),
		})
		return nil
	}
	p, err := s.checkedPath(path)
	if err != nil {
		return err
	}
	info, statErr := os.Stat(p)
	if statErr != nil || !info.IsDir() {
		return errStatus(http.StatusNotFound, "not a directory")
	}

	dirs := []browseDir{}
	// A directory we are not allowed to read is skipped silently rather than
	// failing the request: one unreadable folder must not make the picker
	// unusable for everything beside it.
	if entries, err := os.ReadDir(p); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, browseDir{Name: e.Name(), Path: filepath.Join(p, e.Name())})
			}
		}
	}

	var parent *string
	if up := filepath.Dir(p); up != p && s.Cfg.PathAllowed(up) {
		parent = &up
	}
	writeJSON(w, http.StatusOK, browseResponse{
		Path: p, Parent: parent, Dirs: dirs,
		Images: len(images.List(p)), Roots: s.Cfg.BrowseRoots(),
	})
	return nil
}

// GetImage streams the raw image file at `path` -- what the browser's <img src>
// points at. The response is image bytes, not JSON.
func (s *Server) GetImage(w http.ResponseWriter, r *http.Request) error {
	p, err := s.checkedPath(r.URL.Query().Get("path"))
	if err != nil {
		return err
	}
	info, statErr := os.Stat(p)
	if statErr != nil || info.IsDir() {
		return errStatus(http.StatusNotFound, "image not found")
	}
	http.ServeFile(w, r, p)
	return nil
}
