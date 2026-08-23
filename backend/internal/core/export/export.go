// Package export writes a project's annotations in whichever format a training
// pipeline actually wants.
//
// Ported from backend/the FastAPI export router (T-24). Pure read: nothing here writes
// state, and it does not care whether the images came from the pool or the
// held-out test set.
//
// Only three formats -- YOLO (what this tool always produced), COCO and Pascal
// VOC -- cover every consumer anyone has asked for. Add a fourth builder the day
// someone actually needs one; the dispatch is a plain map, not a registry.
//
// Coordinates come out of the database in pixels, so only YOLO and VOC reopen
// the image, and only for its dimensions. An image that has moved or been
// deleted since it was annotated is skipped rather than failing the whole
// export -- a stale row must not make the rest of a dataset unexportable.
package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
)

// Format is one supported output.
type Format struct {
	MediaType string
	Filename  string
	Build     func(names []string, byImage map[string][]store.Box, dims DimsFunc) ([]byte, error)
}

// DimsFunc reports an image's pixel dimensions, or false if it can no longer be
// read. Injected so the exporters stay testable without a filesystem.
type DimsFunc func(path string) (w, h int, ok bool)

var Formats = map[string]Format{
	"yolo": {"application/zip", "labels_yolo.zip", buildYOLO},
	"coco": {"application/json", "annotations_coco.json", buildCOCO},
	"voc":  {"application/zip", "labels_voc.zip", buildVOC},
}

// Names lists the supported formats, sorted, for the error message a bad
// request gets back.
func Names() []string {
	out := make([]string, 0, len(Formats))
	for k := range Formats {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sortedPaths fixes the iteration order. Python got this for free from a dict
// built by a query with ORDER BY i.path; a Go map has no order, and COCO image
// ids are assigned by position, so an unstable order would produce a different
// document every time.
func sortedPaths(byImage map[string][]store.Box) []string {
	paths := make([]string, 0, len(byImage))
	for p := range byImage {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// f6 and f1 match Python's f"{v:.6f}" and f"{v:.1f}": fixed decimals, not the
// shortest representation.
func f6(v float64) string { return fmt.Sprintf("%.6f", v) }
func f1(v float64) string { return fmt.Sprintf("%.1f", v) }

func buildYOLO(names []string, byImage map[string][]store.Box, dims DimsFunc) ([]byte, error) {
	idx := make(map[string]int, len(names))
	for i, n := range names {
		idx[n] = i
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipEntry(zw, "classes.txt", strings.Join(names, "\n")); err != nil {
		return nil, err
	}
	for _, path := range sortedPaths(byImage) {
		w, h, ok := dims(path)
		if !ok {
			continue
		}
		lines := make([]string, 0, len(byImage[path]))
		for _, b := range byImage[path] {
			x1, y1, x2, y2 := b.Box[0], b.Box[1], b.Box[2], b.Box[3]
			cx, cy := (x1+x2)/2/float64(w), (y1+y2)/2/float64(h)
			bw, bh := abs(x2-x1)/float64(w), abs(y2-y1)/float64(h)
			lines = append(lines, fmt.Sprintf("%d %s %s %s %s",
				idx[b.Cls], f6(cx), f6(cy), f6(bw), f6(bh)))
		}
		if err := writeZipEntry(zw, "labels/"+stem(path)+".txt", strings.Join(lines, "\n")); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type cocoImage struct {
	ID       int    `json:"id"`
	FileName string `json:"file_name"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type cocoAnnotation struct {
	ID         int        `json:"id"`
	ImageID    int        `json:"image_id"`
	CategoryID int        `json:"category_id"`
	BBox       [4]float64 `json:"bbox"`
	Area       float64    `json:"area"`
	IsCrowd    int        `json:"iscrowd"`
}

type cocoCategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func buildCOCO(names []string, byImage map[string][]store.Box, dims DimsFunc) ([]byte, error) {
	catID := make(map[string]int, len(names))
	categories := make([]cocoCategory, 0, len(names))
	for i, n := range names {
		catID[n] = i + 1 // COCO category ids are 1-based
		categories = append(categories, cocoCategory{ID: i + 1, Name: n})
	}

	imgs := []cocoImage{}
	anns := []cocoAnnotation{}
	annID := 1
	for i, path := range sortedPaths(byImage) {
		// The id counts positions, not emitted images: a skipped image consumes
		// its number and leaves a gap. That is what the Python enumerate() did,
		// and COCO only requires ids to be unique, not contiguous.
		imageID := i + 1
		w, h, ok := dims(path)
		if !ok {
			continue
		}
		imgs = append(imgs, cocoImage{ID: imageID, FileName: filepath.Base(path), Width: w, Height: h})
		for _, b := range byImage[path] {
			bw, bh := b.Box[2]-b.Box[0], b.Box[3]-b.Box[1]
			anns = append(anns, cocoAnnotation{
				ID: annID, ImageID: imageID, CategoryID: catID[b.Cls],
				BBox: [4]float64{b.Box[0], b.Box[1], bw, bh}, Area: bw * bh, IsCrowd: 0,
			})
			annID++
		}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Python wrote this with ensure_ascii=False; Go escapes <, > and & in
	// strings unless told not to, which would mangle a class name containing
	// one for no reason.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{
		"images": imgs, "annotations": anns, "categories": categories,
	}); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func buildVOC(names []string, byImage map[string][]store.Box, dims DimsFunc) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, path := range sortedPaths(byImage) {
		w, h, ok := dims(path)
		if !ok {
			continue
		}
		var objects strings.Builder
		for _, b := range byImage[path] {
			objects.WriteString("<object><name>" + xmlEscape(b.Cls) + "</name><bndbox>" +
				"<xmin>" + f1(b.Box[0]) + "</xmin><ymin>" + f1(b.Box[1]) + "</ymin>" +
				"<xmax>" + f1(b.Box[2]) + "</xmax><ymax>" + f1(b.Box[3]) + "</ymax>" +
				"</bndbox></object>")
		}
		doc := "<annotation><filename>" + xmlEscape(filepath.Base(path)) + "</filename>" +
			fmt.Sprintf("<size><width>%d</width><height>%d</height><depth>3</depth></size>", w, h) +
			objects.String() + "</annotation>"
		if err := writeZipEntry(zw, stem(path)+".xml", doc); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// xmlEscape matches Python's xml.sax.saxutils.escape: &, < and > only.
// encoding/xml's escaper also rewrites quotes and newlines, which would make
// these documents differ from every one this tool has produced so far.
// Ampersand first, or the entities introduced by the other two get re-escaped.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
}

func writeZipEntry(zw *zip.Writer, name, body string) error {
	// Deflate, matching zipfile.ZIP_DEFLATED -- the default here is Store.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(body))
	return err
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
