package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/images"
	"github.com/P-PrPas/CT-Flow/backend/internal/platform/config"
)

// Upload is drag-and-drop image intake (FR-29 / T-13), so someone who knows
// what a defect looks like can bring their own images without a server path and
// an engineer to put files there for them.
//
// Three checks stand between the browser and the disk, in this order:
//  1. the destination goes through checkedPath like every other path;
//  2. the filename is reduced to a bare name -- no directory part survives;
//  3. the bytes have to decode as an image. The extension is a fast reject; the
//     decode is the one that actually decides.
func (s *Server) Upload(w http.ResponseWriter, r *http.Request) error {
	// T-13's precondition -- "no upload on a shared server without auth" -- used
	// to be a check here. It is now true by construction: the process refuses to
	// start without OIDC or LABEL_TOOL_USERS (T-27), so the condition it guarded
	// can no longer occur.

	mr, err := r.MultipartReader()
	if err != nil {
		return errStatus(http.StatusBadRequest, "expected multipart/form-data: "+err.Error())
	}

	limit := int64(s.Cfg.MaxUploadMB * 1024 * 1024)
	var dest string
	saved := []string{}
	skipped := []map[string]string{}
	skip := func(name, reason string) {
		skipped = append(skipped, map[string]string{"name": name, "reason": reason})
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errStatus(http.StatusBadRequest, "malformed upload: "+err.Error())
		}

		if part.FormName() == "dir" {
			raw, err := io.ReadAll(io.LimitReader(part, 4096))
			part.Close()
			if err != nil {
				return err
			}
			// Streaming means parts arrive in order, and the destination has to
			// be known before the first file. The frontend sends `dir` first;
			// anything else is a client bug, not a case to guess at.
			dest, err = s.checkedPath(string(raw))
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		if part.FormName() != "files" {
			part.Close()
			continue
		}
		if dest == "" {
			part.Close()
			return errStatus(http.StatusBadRequest, "the `dir` field must come before the files")
		}

		raw := part.FileName()
		// basename first: "../x.jpg" becomes "x.jpg" and lands inside the
		// destination like any other file. The traversal is neutralised, not
		// merely refused.
		name := strings.TrimSpace(filepath.Base(raw))
		switch {
		case name == "" || name == "." || name == string(filepath.Separator) ||
			strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`):
			// A backslash survives Base() on Linux, so a Windows-style path in
			// the filename is caught here rather than becoming one odd filename.
			skip(raw, "bad filename")
			part.Close()
			continue
		case !config.ImageExts[strings.ToLower(filepath.Ext(name))]:
			skip(name, "not an image file type")
			part.Close()
			continue
		}

		dst := filepath.Join(dest, name)
		if _, err := os.Stat(dst); err == nil {
			// Never overwrite: an upload must not be able to replace an image
			// someone has already labeled.
			skip(name, "already in this folder")
			part.Close()
			continue
		}

		// One byte past the cap is enough to know it is over, so an oversized
		// upload costs the cap rather than the file.
		body, err := io.ReadAll(io.LimitReader(part, limit+1))
		part.Close()
		if err != nil {
			return err
		}
		if int64(len(body)) > limit {
			skip(name, fmt.Sprintf("larger than %s MB", trimFloat(s.Cfg.MaxUploadMB)))
			continue
		}
		if !decodableImage(bytes.NewReader(body)) {
			skip(name, "not a readable image")
			continue
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return err
		}
		saved = append(saved, dst)
	}

	if dest == "" {
		return errStatus(http.StatusBadRequest, "no `dir` field in the upload")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved": saved, "skipped": skipped, "images": images.List(dest),
	})
	return nil
}

// trimFloat matches Python's f"{v:g}": 25 renders as "25", not "25.000000".
func trimFloat(v float64) string { return fmt.Sprintf("%g", v) }
