package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/P-PrPas/CT-Flow/backend/internal/core/export"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
)

// maxBundledExportBytes caps the total on-disk size of the images a single
// export with images=true (the default) can bundle. spec.Build holds the
// whole zip in a bytes.Buffer before Export ever writes a byte to the
// response (N-02): RAM use runs to several times that size, with no
// per-request limit beyond docker-compose.yml's container-wide memory cap. A
// flat image count doesn't bound this -- 1500 tiny thumbnails and 1500 raw
// 20MB photos cost wildly different amounts of RAM for the same count. A
// dataset past this cap still exports fine with images=false.
//
// ponytail: sums os.Stat sizes before Build ever reads a byte -- a budget on
// bytes actually read, not the larger peak (deflate buffers, the zip
// central directory) the real request holds, but far closer than counting
// files. Move to zip.NewWriter(w) streaming straight to the response if a
// real dataset ever needs both bundled images and a bigger budget than this.
const maxBundledExportBytes = 512 << 20 // 512 MiB of source images

// tooManyToBundle refuses an images=true export whose images sum past
// maxBundledExportBytes, naming the images=false escape hatch rather than
// letting the request run and risk the api container's own memory limit
// (N-02). A path that fails to stat is skipped here exactly like
// imageDims/readImage skip it later -- a stale row must not block an export
// instead of just dropping out of it.
func tooManyToBundle(paths []string) error {
	var total int64
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	if total <= maxBundledExportBytes {
		return nil
	}
	return errStatus(http.StatusBadRequest,
		fmt.Sprintf("too much image data to bundle (%dMB > %dMB) -- retry with images=false",
			total>>20, int64(maxBundledExportBytes)>>20))
}

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
	if kind != "all" && kind != store.KindPool && kind != store.KindTestset {
		return errStatus(http.StatusBadRequest,
			fmt.Sprintf("unknown kind %s -- choose 'all', 'pool' or 'testset'", pyRepr(kind)))
	}

	inputDir, _, err := s.stateDirFor(q.Get("input_dir"))
	if err != nil {
		return err
	}
	names, byImage, err := s.classesAndAnnotations(r.Context(), inputDir, kind)
	if err != nil {
		return err
	}
	if len(names) == 0 || len(byImage) == 0 {
		return errStatus(http.StatusBadRequest,
			fmt.Sprintf("nothing to export for %s -- label something first", pyRepr(kind)))
	}

	var read export.ReadFunc
	if q.Get("images") != "false" {
		paths := make([]string, 0, len(byImage))
		for p := range byImage {
			paths = append(paths, p)
		}
		if err := tooManyToBundle(paths); err != nil {
			return err
		}
		read = s.readImage
	}
	body, err := spec.Build(names, byImage, s.imageDims, read)
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

// classesAndAnnotations resolves one kind's classes and boxes, or -- for
// "all" -- both kinds merged. Pool and testset are separate index spaces
// (classes.idx is scoped per kind), so a merge cannot just concatenate: it
// unions the class names, pool's order first, then whatever testset adds.
func (s *Server) classesAndAnnotations(ctx context.Context, inputDir, kind string) ([]string, map[string][]store.Box, error) {
	if kind != "all" {
		names, err := s.Store.Classes(ctx, inputDir, kind)
		if err != nil {
			return nil, nil, err
		}
		byImage, err := s.Store.LoadAnnotations(ctx, inputDir, kind)
		if err != nil {
			return nil, nil, err
		}
		return names, byImage, nil
	}

	poolNames, err := s.Store.Classes(ctx, inputDir, store.KindPool)
	if err != nil {
		return nil, nil, err
	}
	tsNames, err := s.Store.Classes(ctx, inputDir, store.KindTestset)
	if err != nil {
		return nil, nil, err
	}
	names := append([]string{}, poolNames...)
	for _, n := range tsNames {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}

	byImage, err := s.Store.LoadAnnotations(ctx, inputDir, store.KindPool)
	if err != nil {
		return nil, nil, err
	}
	tsByImage, err := s.Store.LoadAnnotations(ctx, inputDir, store.KindTestset)
	if err != nil {
		return nil, nil, err
	}
	for path, boxes := range tsByImage {
		// ponytail: the same absolute path existing under both kinds is a real
		// but rare edge (the unique constraint is scoped per kind, so it's not
		// forbidden) -- append rather than overwrite so neither kind's boxes
		// silently disappear from the merged export.
		byImage[path] = append(byImage[path], boxes...)
	}
	return names, byImage, nil
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

// readImage returns an image's raw bytes so export can bundle the file itself
// into the archive, not just its filename -- gated behind imageDims, so a path
// that already failed to open once isn't tried again here.
func (s *Server) readImage(path string) ([]byte, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return raw, true
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
