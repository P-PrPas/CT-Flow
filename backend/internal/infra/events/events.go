// Package events records session events so §7's effort metrics survive a
// browser reload.
//
// The UI already counts "how long did that image take" and "how many auto
// labels did I have to fix" while you work -- but only in React state, which
// means the answer to "is this tool actually saving us time" dies with the tab.
// This appends each event next to the bank it belongs to:
//
//	<state_dir>/_bank/events.jsonl
//
// One JSON object per line, append-only. Ported from backend/inference/events.py
// -- a text file and a loop, no database and no analytics service, because
// these are a handful of counters per labeling day and the file lands in the
// same folder as the dataset it describes.
//
// The one rule worth stating: a field is null when nothing has been recorded,
// and 0 only when something was measured and came out zero. A port that
// collapses the two passes a casual read and destroys the only thing the metric
// is for.
package events

import (
	"bufio"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Kinds the summary understands. Anything else is still recorded (the file is
// the raw log) but does not move a number.
const (
	KindSession = "session"
	KindLabel   = "label"
	KindFix     = "fix"
	KindAuto    = "auto"
)

// Event is one recorded thing. Secs is a pointer because "not timed" and "took
// zero seconds" are different, and Written is counted only on auto events.
type Event struct {
	TS      float64  `json:"ts"`
	Kind    string   `json:"kind"`
	Session string   `json:"session,omitempty"`
	Secs    *float64 `json:"secs"`
	Written int      `json:"written,omitempty"`
	User    *string  `json:"user,omitempty"`
}

func Path(stateDir string) string {
	return filepath.Join(stateDir, "_bank", "events.jsonl")
}

// Append writes one event. The timestamp is the server's clock, not the
// browser's, for the same reason the job poller reports `now`.
//
// One short line opened in append mode: concurrent writers interleave whole
// lines rather than corrupting each other on every OS this runs on.
func Append(stateDir string, e Event) error {
	p := Path(stateDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	e.TS = float64(time.Now().UnixNano()) / 1e9
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// Read returns every parseable event. A half-written last line costs one event,
// not the log -- the writer above can be interrupted mid-line.
func Read(stateDir string) []Event {
	f, err := os.Open(Path(stateDir))
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

// Summary is the §7 table. Every rate and median is a pointer so it can be
// null; see the package comment.
type Summary struct {
	Sessions                  int      `json:"sessions"`
	SessionsReachingAutolabel int      `json:"sessions_reaching_autolabel"`
	Abandonment               *float64 `json:"abandonment"`
	ManualLabels              int      `json:"manual_labels"`
	MedianLabelSecs           *float64 `json:"median_label_secs"`
	MedianTimeToFirstAutoSecs *float64 `json:"median_time_to_first_auto_secs"`
	AutoWritten               int      `json:"auto_written"`
	Corrections               int      `json:"corrections"`
	CorrectionRate            *float64 `json:"correction_rate"`
}

func Summarize(stateDir string) Summary {
	ev := Read(stateDir)

	started := map[string]bool{} // sessions that were opened
	reached := map[string]bool{} // sessions that got as far as auto-labelling
	var labelSecs, autoSecs []float64
	var manual, fixes, written int

	for _, e := range ev {
		switch e.Kind {
		case KindSession:
			if e.Session != "" {
				started[e.Session] = true
			}
		case KindLabel:
			manual++
			if e.Secs != nil {
				labelSecs = append(labelSecs, *e.Secs)
			}
		case KindAuto:
			if e.Session != "" {
				reached[e.Session] = true
			}
			written += e.Written
			if e.Secs != nil {
				autoSecs = append(autoSecs, *e.Secs)
			}
		case KindFix:
			fixes++
		}
	}

	// Intersected with `started`, so an auto-label run from a session we never
	// saw open cannot push abandonment below zero.
	reachedAndStarted := 0
	for s := range reached {
		if started[s] {
			reachedAndStarted++
		}
	}

	s := Summary{
		Sessions:                  len(started),
		SessionsReachingAutolabel: reachedAndStarted,
		ManualLabels:              manual,
		MedianLabelSecs:           median(labelSecs),
		MedianTimeToFirstAutoSecs: median(autoSecs),
		AutoWritten:               written,
		Corrections:               fixes,
	}
	if len(started) > 0 {
		s.Abandonment = ptr(pyRound(1-float64(reachedAndStarted)/float64(len(started)), 3))
	}
	if written > 0 {
		s.CorrectionRate = ptr(pyRound(float64(fixes)/float64(written), 3))
	}
	return s
}

func median(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	m := sorted[mid]
	if len(sorted)%2 == 0 {
		m = (sorted[mid-1] + sorted[mid]) / 2
	}
	return ptr(pyRound(m, 1))
}

// pyRound rounds to `places` decimals the way Python's round() does: on the
// float's exact binary value, with ties going to the even digit.
//
// math.Round(v*p)/p is the obvious port and it is wrong on reachable inputs.
// Go rounds halves away from zero, Python rounds them to even, and an exact
// tie needs nothing exotic to produce: one correction over sixteen auto-labels
// is 0.0625, which Python reports as 0.062 and the naive port as 0.063.
// Scaling by 1000 in float also introduces its own error before the rounding
// even happens. big.Rat holds the value exactly, so neither problem arises.
func pyRound(v float64, places int) float64 {
	r := new(big.Rat).SetFloat64(v)
	if r == nil { // NaN or Inf: nothing sensible to round to
		return v
	}
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil))
	r.Mul(r, scale)

	num, den := r.Num(), r.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	// QuoRem truncates toward zero, so rem carries num's sign; compare
	// magnitudes and step away from zero when the remainder is more than half,
	// or exactly half and the quotient is odd.
	twiceRem := new(big.Int).Abs(rem)
	twiceRem.Lsh(twiceRem, 1)
	switch cmp := twiceRem.Cmp(den); {
	case cmp > 0, cmp == 0 && q.Bit(0) == 1:
		if num.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	out, _ := new(big.Rat).SetFrac(q, scale.Num()).Float64()
	return out
}

func ptr(f float64) *float64 { return &f }
