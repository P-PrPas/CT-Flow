package httpapi

import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"os"
	"strconv"
	"sync"

	"golang.org/x/image/draw"
)

// GetThumb serves a downscaled JPEG of the image at `path`, for the gallery
// grid. GET /api/image streams the whole file -- 100-300KB per conveyor frame,
// unusable by the hundred -- so a grid view has to have its own endpoint.
//
// The image decoders are registered by the blank imports in imagecheck.go
// (jpeg, png, gif, bmp), the same set config.ImageExts admits.
func (s *Server) GetThumb(w http.ResponseWriter, r *http.Request) error {
	p, err := s.checkedPath(r.URL.Query().Get("path"))
	if err != nil {
		return err
	}
	width := thumbDefaultW
	if q := r.URL.Query().Get("w"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 16 {
			return errStatus(http.StatusBadRequest, "w must be an integer >= 16")
		}
		width = min(n, thumbMaxW)
	}

	info, statErr := os.Stat(p)
	if statErr != nil || info.IsDir() {
		return errStatus(http.StatusNotFound, "image not found")
	}
	// mtime+size+width is enough: the file's bytes cannot change without one of
	// mtime or size moving, and a different width is a different rendering.
	etag := fmt.Sprintf(`"%d-%d-%d"`, info.ModTime().UnixNano(), info.Size(), width)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	key := p + "|" + strconv.Itoa(width)
	entry, ok := thumbs.get(key)
	if !ok || entry.etag != etag {
		body, err := renderThumb(p, width)
		if err != nil {
			return errStatus(http.StatusBadRequest, "cannot decode this image")
		}
		entry = &thumbEntry{key: key, body: body, etag: etag}
		thumbs.put(entry)
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(entry.body)
	return nil
}

// renderThumb decodes path, scales it to `width` (never upscaling), and
// re-encodes JPEG. ApproxBiLinear is the fast scaler -- a grid cell does not
// need CatmullRom sharpness.
func renderThumb(path string, width int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		return nil, fmt.Errorf("empty image")
	}
	dw, dh := width, width*b.Dy()/b.Dx()
	if dw >= b.Dx() { // a source smaller than the thumbnail stays its own size
		dw, dh = b.Dx(), b.Dy()
	}
	if dh < 1 {
		dh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 75}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const (
	thumbDefaultW = 200
	thumbMaxW     = 400
	thumbCacheN   = 512
)

// thumbs is process-wide: a decoded-and-scaled JPEG is the same bytes whoever
// asks, so there is nothing to isolate per request or per Server.
//
// ponytail: in-memory LRU, 512 entries (~8MB at 200px). A cold scroll past the
// tail of a 50k pool re-decodes; add a .ctflow/_thumbs/ disk cache
// (Go-owned, sha1(path)_w.jpg) only if that measurably drags.
var thumbs = newThumbCache()

type thumbEntry struct {
	key, etag string
	body      []byte
}

type thumbCache struct {
	mu sync.Mutex
	ll *list.List
	by map[string]*list.Element
}

func newThumbCache() *thumbCache {
	return &thumbCache{ll: list.New(), by: map[string]*list.Element{}}
}

func (c *thumbCache) get(key string) (*thumbEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.by[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*thumbEntry), true
	}
	return nil, false
}

func (c *thumbCache) put(e *thumbEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.by[e.key]; ok {
		el.Value = e
		c.ll.MoveToFront(el)
		return
	}
	c.by[e.key] = c.ll.PushFront(e)
	for c.ll.Len() > thumbCacheN {
		if back := c.ll.Back(); back != nil {
			c.ll.Remove(back)
			delete(c.by, back.Value.(*thumbEntry).key)
		}
	}
}
