// Package history stores one point per Evaluate run, next to the bank it
// measured (T-07).
//
// On disk so the learning curve survives a browser, a machine, and a colleague.
// Ported from backend/services/history.py.
//
// A point is opaque here: the frontend decides what it puts in one
// (lib/history.ts), and this only has to store and return them in order. That
// is why it is json.RawMessage rather than a struct -- adding a field to the
// curve should not require a backend change.
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Max points kept. Enough for a long project's worth of Evaluate runs, and
// bounded so the file cannot grow without limit.
const Max = 200

func Path(stateDir string) string {
	return filepath.Join(stateDir, "_bank", "eval_history.json")
}

// Read returns the recorded points, oldest first.
//
// A missing file is an empty history, and so is a corrupt one: a truncated
// learning curve is a nicety to lose, never a reason to fail the request that
// asked for it.
func Read(stateDir string) []json.RawMessage {
	raw, err := os.ReadFile(Path(stateDir))
	if err != nil {
		return []json.RawMessage{}
	}
	var points []json.RawMessage
	if err := json.Unmarshal(raw, &points); err != nil {
		return []json.RawMessage{}
	}
	if points == nil {
		return []json.RawMessage{}
	}
	return points
}

// Append adds one point and returns the history as it now stands.
//
// ponytail: read-modify-write, no lock. Two people evaluating the same project
// in the same second would drop a point -- take the bank's lock through the
// sidecar if that ever stops being hypothetical.
func Append(stateDir string, point json.RawMessage) ([]json.RawMessage, error) {
	p := Path(stateDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	points := append(Read(stateDir), point)
	if len(points) > Max {
		points = points[len(points)-Max:]
	}
	body, err := json.Marshal(points)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return nil, err
	}
	return points, nil
}

// Clear drops the whole history. A missing file is already cleared.
func Clear(stateDir string) error {
	err := os.Remove(Path(stateDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
