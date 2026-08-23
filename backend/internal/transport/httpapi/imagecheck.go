package httpapi

import (
	"image"
	"io"
	"sort"

	// Registered for their DecodeConfig side effects. bmp is not in the
	// standard library but is in config.ImageExts, so leaving it out would make
	// every .bmp silently unreadable -- and "unreadable" here means an upload
	// rejected and an export entry skipped, both without an error anyone sees.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
)

// decodableImage reports whether r starts with something the image decoders
// recognise. Header only: this answers "is this really an image", which is what
// the upload validator and the ground-truth writer ask, and neither wants the
// pixels.
func decodableImage(r io.Reader) bool {
	_, _, err := image.DecodeConfig(r)
	return err == nil
}

// imageSize is the dimensions export needs to normalise pixel coordinates.
func imageSize(r io.Reader) (int, int, bool) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// sortStrings keeps the several call sites that need sorted output honest about
// why: the frontend renders these lists in order and the smoke test compares
// them element by element.
func sortStrings(s []string) { sort.Strings(s) }
