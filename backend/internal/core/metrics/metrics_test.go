package metrics

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/P-PrPas/CT-Flow/backend/internal/testsupport"

	"github.com/P-PrPas/CT-Flow/backend/internal/infra/store"
	"github.com/P-PrPas/CT-Flow/backend/internal/infra/vpe"
)

// The vectors Python produced. This is the readiness signal the whole tool turns
// on, and backend/tools/metrics.py still exists and is still called by
// tools/experiment_conf.py -- so these two implementations have to keep agreeing, and
// this file is what makes that true rather than hoped.
type metricsVectors struct {
	IoU []struct {
		A    [4]float64 `json:"a"`
		B    [4]float64 `json:"b"`
		Want float64    `json:"want"`
	} `json:"iou"`
	Cases []struct {
		Name string                     `json:"name"`
		GT   map[string][]store.Box     `json:"gt"`
		Pred map[string][]vpe.Detection `json:"pred"`
		Want json.RawMessage            `json:"want"`
	} `json:"cases"`
}

func load(t *testing.T) metricsVectors {
	t.Helper()
	raw, err := os.ReadFile(testsupport.MustBackendFile("tests/testdata/metrics_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v metricsVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestIoUMatchesPython(t *testing.T) {
	for _, c := range load(t).IoU {
		if got := IoU(c.A, c.B); math.Abs(got-c.Want) > 1e-12 {
			t.Errorf("IoU(%v, %v) = %v, want %v", c.A, c.B, got, c.Want)
		}
	}
}

// Comparing the serialised result, not the struct: field for field is what the
// frontend receives, and it catches a missing or renamed key as well as a wrong
// number.
func TestEvaluateMatchesPython(t *testing.T) {
	for _, c := range load(t).Cases {
		// Python iterates the ground-truth dict in insertion order, which the
		// JSON preserves; Go maps have none, so the order is passed explicitly.
		order := sortedKeys(c.GT)
		got := Evaluate(c.GT, order, c.Pred, DefaultIoU)

		gotJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var gotAny, wantAny any
		if err := json.Unmarshal(gotJSON, &gotAny); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(c.Want, &wantAny); err != nil {
			t.Fatal(err)
		}
		if !equalJSON(gotAny, wantAny) {
			t.Errorf("%s:\ngot  %s\nwant %s", c.Name, gotJSON, c.Want)
		}
	}
}

// The three rules a looser matcher would get wrong, stated on their own so a
// failure names the rule rather than a case number.
func TestMatchingRules(t *testing.T) {
	box := [4]float64{10, 10, 50, 50}

	t.Run("class must agree", func(t *testing.T) {
		got := Evaluate(
			map[string][]store.Box{"a": {{Cls: "x", Box: box}}}, []string{"a"},
			map[string][]vpe.Detection{"a": {{Cls: "y", Box: box, Conf: 0.9}}}, DefaultIoU)
		if got.Overall.TP != 0 || got.Overall.FP != 1 || got.Overall.FN != 1 {
			t.Errorf("a perfectly placed box of the wrong class scored %+v, want no match", got.Overall)
		}
	})

	t.Run("a truth box is claimed once", func(t *testing.T) {
		got := Evaluate(
			map[string][]store.Box{"a": {{Cls: "x", Box: box}}}, []string{"a"},
			map[string][]vpe.Detection{"a": {
				{Cls: "x", Box: box, Conf: 0.95},
				{Cls: "x", Box: box, Conf: 0.6},
			}}, DefaultIoU)
		if got.Overall.TP != 1 || got.Overall.FP != 1 {
			t.Errorf("two predictions on one truth box scored %+v, want one hit and one false positive",
				got.Overall)
		}
	})

	t.Run("confidence order decides", func(t *testing.T) {
		// The better-placed box is the less confident one. Highest confidence
		// claims the truth box first, so the exact match becomes the FP.
		got := Evaluate(
			map[string][]store.Box{"a": {{Cls: "x", Box: box}}}, []string{"a"},
			map[string][]vpe.Detection{"a": {
				{Cls: "x", Box: box, Conf: 0.5},
				{Cls: "x", Box: [4]float64{12, 12, 52, 52}, Conf: 0.99},
			}}, DefaultIoU)
		if len(got.PerImage) != 1 || len(got.PerImage[0].Pred) != 2 {
			t.Fatalf("unexpected shape: %+v", got.PerImage)
		}
		if !got.PerImage[0].Pred[0].Matched || got.PerImage[0].Pred[1].Matched {
			t.Error("the more confident prediction should have claimed the truth box")
		}
	})

	t.Run("threshold is inclusive at 0.5", func(t *testing.T) {
		// Exactly half overlap: 0.5 must count, since the comparison is >=.
		gt := [4]float64{0, 0, 100, 100}
		half := [4]float64{0, 0, 100, 200} // IoU = 10000/20000 = 0.5
		got := Evaluate(
			map[string][]store.Box{"a": {{Cls: "x", Box: gt}}}, []string{"a"},
			map[string][]vpe.Detection{"a": {{Cls: "x", Box: half, Conf: 0.9}}}, DefaultIoU)
		if got.Overall.TP != 1 {
			t.Errorf("IoU exactly at the threshold scored %+v, want a match", got.Overall)
		}
	})
}

// An empty everything must be zeroes, not a divide by zero.
func TestEmptyIsZeroNotPanic(t *testing.T) {
	got := Evaluate(map[string][]store.Box{"a": {}}, []string{"a"},
		map[string][]vpe.Detection{}, DefaultIoU)
	if got.Overall.F1 != 0 || got.Images != 1 || len(got.PerClass) != 0 {
		t.Errorf("empty evaluation = %+v", got)
	}
}

func sortedKeys(m map[string][]store.Box) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// equalJSON compares decoded JSON with a tolerance on numbers: two languages
// computing the same ratio can differ in the last bit, and that is not a
// difference anyone can act on.
func equalJSON(a, b any) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		return ok && math.Abs(av-bv) < 1e-12
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			other, ok := bv[k]
			if !ok || !equalJSON(v, other) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equalJSON(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
