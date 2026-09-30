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
	"strings"
	"testing"

	"github.com/P-PrPas/CT-Flow/backend/internal/testsupport"

	_ "golang.org/x/image/bmp"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
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
	raw, err := os.ReadFile(testsupport.MustBackendFile("tests/testdata/export_cases.json"))
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
	pool := testsupport.MustBackendFile("tests/fixtures/pool")
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

// realRead reads the fixture images' raw bytes, the same way the real handler
// does -- what the bundled export is checked against.
func realRead(t *testing.T) ReadFunc {
	t.Helper()
	return func(path string) ([]byte, bool) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, false
		}
		return raw, true
	}
}

// splitImages pulls the bundled-image entries a bundled export now carries
// apart from the annotation entries the golden vectors pinned before bundling
// existed, so the parity check only compares what Python actually produced.
// imgPrefix is "images/" for YOLO and COCO, "JPEGImages/" for VOC's own
// layout.
func splitImages(files map[string]string, imgPrefix string) (annotations, images map[string]string) {
	annotations = map[string]string{}
	images = map[string]string{}
	for name, body := range files {
		if strings.HasPrefix(name, imgPrefix) {
			images[name] = body
		} else {
			annotations[name] = body
		}
	}
	return
}

// assertImagesBundled confirms every exported image is findable under
// imgPrefix+<basename> with its exact bytes -- the actual point of bundling
// them.
func assertImagesBundled(t *testing.T, images map[string]string, byImage map[string][]store.Box, imgPrefix string) {
	t.Helper()
	for path := range byImage {
		want, err := os.ReadFile(path)
		if err != nil {
			continue // deleted-since-labelled fixture: nothing to bundle, nothing to check
		}
		got, ok := images[imgPrefix+filepath.Base(path)]
		if !ok {
			t.Errorf("%s%s missing from export", imgPrefix, filepath.Base(path))
			continue
		}
		if got != string(want) {
			t.Errorf("%s%s bytes changed in transit", imgPrefix, filepath.Base(path))
		}
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
	raw, err := buildYOLO(v.Names, byImage, realDims(t), realRead(t))
	if err != nil {
		t.Fatal(err)
	}
	annotations, images := splitImages(unzip(t, raw), "images/")
	if !reflect.DeepEqual(annotations, v.YOLO) {
		t.Errorf("yolo export differs\ngot  %v\nwant %v", annotations, v.YOLO)
	}
	assertImagesBundled(t, images, byImage, "images/")
	// The deleted image must be absent, not present and empty: a stale row is
	// skipped, and skipping it silently is the documented behaviour.
	if _, ok := annotations["labels/deleted_since_it_was_labelled.txt"]; ok {
		t.Error("an image that no longer exists produced a label file")
	}
	if _, ok := images["images/deleted_since_it_was_labelled.jpg"]; ok {
		t.Error("an image that no longer exists produced a bundled image")
	}
}

// A nil ReadFunc is how the "exclude images" export opts out of bundling --
// labels must come out exactly the same, just with no images/ entries.
func TestYOLOWithoutImages(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildYOLO(v.Names, byImage, realDims(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	annotations, images := splitImages(unzip(t, raw), "images/")
	if !reflect.DeepEqual(annotations, v.YOLO) {
		t.Errorf("yolo labels differ with images excluded\ngot  %v\nwant %v", annotations, v.YOLO)
	}
	if len(images) != 0 {
		t.Errorf("images/ entries present with a nil ReadFunc: %v", images)
	}
}

func TestCOCOMatchesPython(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildCOCO(v.Names, byImage, realDims(t), realRead(t))
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, raw)
	body, ok := files["annotations_coco.json"]
	if !ok {
		t.Fatal("coco export missing annotations_coco.json")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, v.COCO) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(v.COCO)
		t.Errorf("coco export differs\ngot  %s\nwant %s", gotJSON, wantJSON)
	}
	_, images := splitImages(files, "images/")
	assertImagesBundled(t, images, byImage, "images/")
}

func TestCOCOWithoutImages(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildCOCO(v.Names, byImage, realDims(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, raw)
	body, ok := files["annotations_coco.json"]
	if !ok {
		t.Fatal("coco export missing annotations_coco.json")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, v.COCO) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(v.COCO)
		t.Errorf("coco json differs with images excluded\ngot  %s\nwant %s", gotJSON, wantJSON)
	}
	if _, images := splitImages(files, "images/"); len(images) != 0 {
		t.Errorf("images/ entries present with a nil ReadFunc: %v", images)
	}
}

func TestVOCMatchesPython(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildVOC(v.Names, byImage, realDims(t), realRead(t))
	if err != nil {
		t.Fatal(err)
	}
	annotations, images := splitImages(unzip(t, raw), "JPEGImages/")
	if !reflect.DeepEqual(annotations, v.VOC) {
		t.Errorf("voc export differs\ngot  %v\nwant %v", annotations, v.VOC)
	}
	assertImagesBundled(t, images, byImage, "JPEGImages/")
}

func TestVOCWithoutImages(t *testing.T) {
	v := load(t)
	byImage, _ := absolute(v)
	raw, err := buildVOC(v.Names, byImage, realDims(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	annotations, images := splitImages(unzip(t, raw), "JPEGImages/")
	if !reflect.DeepEqual(annotations, v.VOC) {
		t.Errorf("voc labels differ with images excluded\ngot  %v\nwant %v", annotations, v.VOC)
	}
	if len(images) != 0 {
		t.Errorf("images/ entries present with a nil ReadFunc: %v", images)
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
	read := func(p string) ([]byte, bool) { return []byte("x"), true }
	raw, err := buildCOCO([]string{"x"}, byImage, dims, read)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Images []cocoImage `json:"images"`
	}
	if err := json.Unmarshal([]byte(unzip(t, raw)["annotations_coco.json"]), &doc); err != nil {
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
