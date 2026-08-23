// Package metrics scores predictions against hand-drawn ground truth.
//
// Precision, recall and F1 at a fixed IoU threshold -- not mAP. This is the
// readiness signal the whole tool turns on: it is what answers "is the prompt
// bank good enough to auto-label the rest", and pool confidence only says which
// image to label next.
//
// Ported from backend/services/metrics.py. That file stays in the repo and
// stays runnable, because _experiment_conf.py (the T-01 threshold sweep) calls
// it directly against raw YOLO folders that were never .ctflow projects. So
// these two implementations live side by side on purpose -- and
// backend/testdata/metrics_cases.json is what keeps them from drifting apart.
package metrics

import (
	"sort"

	"github.com/P-PrPas/CT-Flow/internal/store"
	"github.com/P-PrPas/CT-Flow/internal/vpe"
)

// DefaultIoU is the overlap at which a prediction counts as matching a truth
// box. Fixed at 0.5 throughout the tool.
const DefaultIoU = 0.5

// IoU is intersection over union of two [x1,y1,x2,y2] boxes.
func IoU(a, b [4]float64) float64 {
	ix1, iy1 := max(a[0], b[0]), max(a[1], b[1])
	ix2, iy2 := min(a[2], b[2]), min(a[3], b[3])
	inter := max(0, ix2-ix1) * max(0, iy2-iy1)
	if inter <= 0 {
		return 0
	}
	areaA := (a[2] - a[0]) * (a[3] - a[1])
	areaB := (b[2] - b[0]) * (b[3] - b[1])
	return inter / (areaA + areaB - inter)
}

// Score is precision/recall/F1 with the counts they came from, so a reader can
// see whether a number rests on three examples or three hundred.
type Score struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
}

// TruthBox is one ground-truth box and whether anything matched it.
type TruthBox struct {
	Cls     string     `json:"cls"`
	Box     [4]float64 `json:"box"`
	Matched bool       `json:"matched"`
}

// PredBox is one prediction and whether it landed on a truth box.
type PredBox struct {
	Cls     string     `json:"cls"`
	Box     [4]float64 `json:"box"`
	Conf    float64    `json:"conf"`
	Matched bool       `json:"matched"`
}

// ImageResult is the per-image detail the Report tab draws: what was there,
// what was predicted, and which of each matched.
type ImageResult struct {
	Image string     `json:"image"`
	GT    []TruthBox `json:"gt"`
	Pred  []PredBox  `json:"pred"`
	TP    int        `json:"tp"`
	FP    int        `json:"fp"`
	FN    int        `json:"fn"`
}

type Result struct {
	Overall  Score            `json:"overall"`
	PerClass map[string]Score `json:"per_class"`
	PerImage []ImageResult    `json:"per_image"`
	Images   int              `json:"images"`
	IoU      float64          `json:"iou"`
}

// Evaluate matches predictions to ground truth greedily, highest confidence
// first, one truth box per prediction.
//
// Three rules do the work, and each is a case where a looser matcher would
// flatter the model:
//   - a prediction only matches a truth box of the same class, however well
//     placed it is;
//   - a truth box already claimed cannot be matched again, so a second
//     prediction on the same object is a false positive rather than a free
//     second hit;
//   - confidence order decides who claims what, so the model's best guess gets
//     first refusal.
//
// images is the ground-truth key order, so the caller controls the report's
// ordering; per_class is keyed by name and read sorted.
func Evaluate(gt map[string][]store.Box, gtOrder []string,
	pred map[string][]vpe.Detection, iouThr float64) Result {

	type counts struct{ tp, fp, fn int }
	perClass := map[string]*counts{}
	bucket := func(name string) *counts {
		c, ok := perClass[name]
		if !ok {
			c = &counts{}
			perClass[name] = c
		}
		return c
	}

	perImage := []ImageResult{}
	for _, path := range gtOrder {
		truths := gt[path]

		dets := append([]vpe.Detection(nil), pred[path]...)
		// Descending confidence. SliceStable so equally confident predictions
		// keep the order the model returned them in, rather than an arbitrary
		// one that could differ between runs.
		sort.SliceStable(dets, func(i, j int) bool { return dets[i].Conf > dets[j].Conf })

		gtOut := make([]TruthBox, len(truths))
		for i, t := range truths {
			gtOut[i] = TruthBox{Cls: t.Cls, Box: t.Box, Matched: false}
		}
		taken := make([]bool, len(truths))
		predOut := []PredBox{}

		for _, d := range dets {
			best, bestIoU := -1, iouThr
			for i, t := range truths {
				if taken[i] || t.Cls != d.Cls {
					continue
				}
				if v := IoU(d.Box, t.Box); v >= bestIoU {
					best, bestIoU = i, v
				}
			}
			matched := best >= 0
			if matched {
				taken[best] = true
				gtOut[best].Matched = true
				bucket(d.Cls).tp++
			} else {
				bucket(d.Cls).fp++
			}
			predOut = append(predOut, PredBox{Cls: d.Cls, Box: d.Box, Conf: d.Conf, Matched: matched})
		}
		for i, t := range truths {
			if !taken[i] {
				bucket(t.Cls).fn++
			}
		}

		img := ImageResult{Image: path, GT: gtOut, Pred: predOut}
		for _, p := range predOut {
			if p.Matched {
				img.TP++
			} else {
				img.FP++
			}
		}
		for _, g := range gtOut {
			if !g.Matched {
				img.FN++
			}
		}
		perImage = append(perImage, img)
	}

	var tp, fp, fn int
	out := make(map[string]Score, len(perClass))
	for name, c := range perClass {
		tp, fp, fn = tp+c.tp, fp+c.fp, fn+c.fn
		out[name] = prf(c.tp, c.fp, c.fn)
	}
	return Result{
		Overall: prf(tp, fp, fn), PerClass: out, PerImage: perImage,
		Images: len(gtOrder), IoU: iouThr,
	}
}

// prf reports zeroes rather than dividing by zero. A class with no predictions
// and no truth is a real state -- it scores 0, it is not an error.
func prf(tp, fp, fn int) Score {
	var p, r, f float64
	if tp+fp > 0 {
		p = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		r = float64(tp) / float64(tp+fn)
	}
	if p+r > 0 {
		f = 2 * p * r / (p + r)
	}
	return Score{Precision: p, Recall: r, F1: f, TP: tp, FP: fp, FN: fn}
}
