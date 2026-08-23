package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	_ "golang.org/x/image/bmp"

	"github.com/P-PrPas/CT-Flow/internal/store"
)

// The vectors Python produced. These pin the parts two languages drift on
// without anyone noticing: the normalisation arithmetic, fixed-decimal
// formatting, COCO's id numbering, and which characters XML escapes.
type exportVectors struct {
	Names   []string               `json:"names"`
	ByImage map[string][]store.Box `json:"by_image"`
	YOLO    map[string]string      `json:"yolo"`
	COCO    map[string]any         `json:"coco"`
	VOC     map[string]string      `json:"voc"`
}

func load(t *testing.T) exportVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "backend", "testdata", "export_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v exportVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// The vectors are keyed by basename; join them against this checkout's fixture
// pool, the same way the generator did.
func absolute(v exportVectors) (map[string][]store.Box, string) {
	pool := filepath.Join("..", "..", "backend", "test_pool")
	out := make(map[string][]store.Box, len(v.ByImage))
	for name, boxes := range v.ByImage {
		out[filepath.Join(pool, name)] = boxes
	}
	return out, pool
}

// realDims reads the fixture images for real: their actual dimensions are what
// the recorded normalisation was computed from.
func realDims(t *testing.T) DimsFunc {
	t.Helper()
	return func(path string) (int, int, bool) {
		f, err := os.Open(path)
		if err != nil {
			return 0, 0, false // deleted since it was annotated -- skipped, not fatal
		}
		defer f.Close()
		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			return 0, 0, false
		}
		return cfg.Width, cfg.Height, true
	}
}

func unzip(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(body)
	}
	return out
}

func TestYOLOMatchesPython(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildYOLO(v.Names, byImage, realDims(t))
	if err != nil {
		t.Fatal(err)
	}
	got := unzip(t, raw)
	if !reflect.DeepEqual(got, v.YOLO) {
		t.Errorf("yolo export differs\ngot  %v\nwant %v", got, v.YOLO)
	}
	// The deleted image must be absent, not present and empty: a stale row is
	// skipped, and skipping it silently is the documented behaviour.
	if _, ok := got["labels/deleted_since_it_was_labelled.txt"]; ok {
		t.Error("an image that no longer exists produced a label file")
	}
}

func TestCOCOMatchesPython(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildCOCO(v.Names, byImage, realDims(t))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, v.COCO) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(v.COCO)
		t.Errorf("coco export differs\ngot  %s\nwant %s", gotJSON, wantJSON)
	}
}

func TestVOCMatchesPython(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildVOC(v.Names, byImage, realDims(t))
	if err != nil {
		t.Fatal(err)
	}
	got := unzip(t, raw)
	if !reflect.DeepEqual(got, v.VOC) {
		t.Errorf("voc export differs\ngot  %v\nwant %v", got, v.VOC)
	}
}

// Only &, < and > -- what xml.sax.saxutils.escape does. encoding/xml also
// rewrites quotes and newlines, which would make these documents differ from
// every one this tool has produced.
func TestXMLEscapeMatchesSaxutils(t *testing.T) {
	for in, want := range map[string]string{
		`a&b`:         `a&amp;b`,
		`a<b>c`:       `a&lt;b&gt;c`,
		`"quoted"`:    `"quoted"`, // quotes are left alone, unlike encoding/xml
		`it's`:        `it's`,
		"line\nbreak": "line\nbreak", // newlines too
		`&lt;`:        `&amp;lt;`,    // ampersand first, or this double-escapes
		``:            ``,
	} {
		if got := xmlEscape(in); got != want {
			t.Errorf("xmlEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

// A COCO id counts positions, not emitted images, so a skipped image leaves a
// gap. Unique is all COCO requires, and matching Python matters more than tidy.
func TestCOCOIDsSkipRatherThanRenumber(t *testing.T) {
	byImage := map[string][]store.Box{
		"/a.jpg":    {{Cls: "x", Box: [4]float64{0, 0, 1, 1}}},
		"/gone.jpg": {{Cls: "x", Box: [4]float64{0, 0, 1, 1}}},
		"/z.jpg":    {{Cls: "x", Box: [4]float64{0, 0, 1, 1}}},
	}
	dims := func(p string) (int, int, bool) {
		return 100, 100, p != "/gone.jpg"
	}
	raw, err := buildCOCO([]string{"x"}, byImage, dims)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Images []cocoImage `json:"images"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Images) != 2 {
		t.Fatalf("got %d images, want the two that still exist", len(doc.Images))
	}
	// /a.jpg is position 1, /gone.jpg takes 2 and is dropped, /z.jpg is 3.
	if doc.Images[0].ID != 1 || doc.Images[1].ID != 3 {
		t.Errorf("image ids = %d, %d; want 1 and 3 (the gap is deliberate)",
			doc.Images[0].ID, doc.Images[1].ID)
	}
}

func TestNames(t *testing.T) {
	got := Names()
	want := []string{"coco", "voc", "yolo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v (sorted, for the error message)", got, want)
	}
}
