package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/P-PrPas/CT-Flow/backend/internal/core/export"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
)

// Export downloads this project's annotations in whichever format a training
// pipeline wants. Reads straight out of PostgreSQL; not a background job,
// because there is no inference and it is fast enough to answer inline.
func (s *Server) Export(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "yolo"
	}
	spec, ok := export.Formats[format]
	if !ok {
		return errStatus(http.StatusBadRequest,
			fmt.Sprintf("unknown format %s -- choose one of %s", pyRepr(format), pyReprList(export.Names())))
	}
	kind := q.Get("kind")
	if kind == "" {
		kind = store.KindPool
	}
	if kind != store.KindPool && kind != store.KindTestset {
		return errStatus(http.StatusBadRequest,
			fmt.Sprintf("unknown kind %s -- choose 'pool' or 'testset'", pyRepr(kind)))
	}

	inputDir, _, err := s.stateDirFor(q.Get("input_dir"))
	if err != nil {
		return err
	}
	names, err := s.Store.Classes(r.Context(), inputDir, kind)
	if err != nil {
		return err
	}
	byImage, err := s.Store.LoadAnnotations(r.Context(), inputDir, kind)
	if err != nil {
		return err
	}
	if len(names) == 0 || len(byImage) == 0 {
		return errStatus(http.StatusBadRequest,
			fmt.Sprintf("nothing to export for %s -- label something first", pyRepr(kind)))
	}

	body, err := spec.Build(names, byImage, s.imageDims)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", spec.MediaType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+spec.Filename+`"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		// Returning this would have Handle try to write a JSON error over a
		// response whose headers and body are already out -- a second
		// WriteHeader, and a zip with an error object stapled to the end of it.
		// The client is gone; the log is the only place left to say so.
		s.Log.Error("writing the export body", "path", r.URL.Path, "err", err)
	}
	return nil
}

// imageDims reads an image's size from its header. An image that has moved or
// been deleted since it was annotated reports false and gets skipped, so one
// stale row cannot make a whole dataset unexportable.
func (s *Server) imageDims(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	return imageSize(f)
}

// pyRepr and pyReprList reproduce Python's repr() for the two error messages
// that embed one. The strings are compared by the smoke test and shown to the
// user, so "unknown format 'xml'" has to stay exactly that rather than becoming
// Go's "xml" or %q's double quotes.
func pyRepr(s string) string { return "'" + strings.ReplaceAll(s, "'", `\'`) + "'" }

func pyReprList(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = pyRepr(it)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
