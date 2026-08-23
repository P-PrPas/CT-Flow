package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type eventsVectors struct {
	Log       []map[string]any `json:"log"`
	Want      Summary          `json:"want"`
	WantEmpty Summary          `json:"want_empty"`
	TiesLog   []map[string]any `json:"ties_log"`
	WantTies  Summary          `json:"want_ties"`
}

func load(t *testing.T) eventsVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "backend", "testdata", "events_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v eventsVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// writeLog lays a log down the way Python wrote the vectors, so what is under
// test is Summarize and not this file's idea of the format.
func writeLog(t *testing.T, entries []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "_bank"), 0o755); err != nil {
		t.Fatal(err)
	}
	var body []byte
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		body = append(append(body, line...), '\n')
	}
	if err := os.WriteFile(Path(dir), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Summary holds pointers, so == compares addresses. Comparing the JSON is both
// a value comparison and the exact thing the frontend receives.
func show(t *testing.T, s Summary) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSummarizeMatchesPython(t *testing.T) {
	v := load(t)
	got, want := show(t, Summarize(writeLog(t, v.Log))), show(t, v.Want)
	if got != want {
		t.Errorf("summary = %s\nwant     %s", got, want)
	}
}

// The whole point of the pointer fields: "never measured" and "measured zero"
// are different answers, and an empty log must give the first one.
func TestSummarizeEmptyIsNullNotZero(t *testing.T) {
	v := load(t)
	got := Summarize(t.TempDir())
	if show(t, got) != show(t, v.WantEmpty) {
		t.Errorf("empty summary = %s\nwant          %s", show(t, got), show(t, v.WantEmpty))
	}
	if got.Abandonment != nil || got.CorrectionRate != nil ||
		got.MedianLabelSecs != nil || got.MedianTimeToFirstAutoSecs != nil {
		t.Error("an unmeasured rate must be null, not 0 -- a zero reads like a real measurement")
	}
}

// Rounding ties, the reason pyRound exists. math.Round(v*1000)/1000 gives 0.063
// for one fix over sixteen auto-labels; Python gives 0.062, and this fixture
// pins that.
func TestSummarizeRoundingTiesMatchPython(t *testing.T) {
	v := load(t)
	got := Summarize(writeLog(t, v.TiesLog))
	if show(t, got) != show(t, v.WantTies) {
		t.Errorf("summary = %s\nwant     %s", show(t, got), show(t, v.WantTies))
	}
	if got.CorrectionRate == nil || *got.CorrectionRate != 0.062 {
		t.Errorf("correction_rate = %v, want 0.062 -- ties round to even, as Python's round() does",
			got.CorrectionRate)
	}
}

func TestPyRoundTiesToEven(t *testing.T) {
	for _, tc := range []struct {
		v      float64
		places int
		want   float64
	}{
		{0.0625, 3, 0.062},   // exact tie, quotient even -> stays
		{0.0635, 3, 0.064},   // not an exact tie in binary; 0.0635 is just above
		{12.25, 1, 12.2},     // exact tie, down to even
		{12.35, 1, 12.3},     // 12.35 is just below in binary
		{0.5, 0, 0.0},        // classic: Python's round(0.5) is 0, not 1
		{1.5, 0, 2.0},        // ...and round(1.5) is 2
		{2.5, 0, 2.0},        // ...and round(2.5) is 2
		{-0.0625, 3, -0.062}, // sign must not change the tie rule
		{0.6666666666666666, 3, 0.667},
		{0.0, 3, 0.0},
	} {
		if got := pyRound(tc.v, tc.places); got != tc.want {
			t.Errorf("pyRound(%v, %d) = %v, want %v", tc.v, tc.places, got, tc.want)
		}
	}
}

// A writer can be interrupted mid-line. That must cost one event, not the log.
func TestReadSurvivesATruncatedLastLine(t *testing.T) {
	v := load(t)
	dir := writeLog(t, v.Log)
	f, err := os.OpenFile(Path(dir), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"kind": "label", "secs":`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if got := Summarize(dir); got.ManualLabels != v.Want.ManualLabels {
		t.Errorf("manual_labels = %d after a truncated line, want %d",
			got.ManualLabels, v.Want.ManualLabels)
	}
}

func TestAppendRoundTrips(t *testing.T) {
	dir := t.TempDir() // deliberately without _bank/: Append creates it
	secs := 12.0
	for _, e := range []Event{
		{Kind: KindSession, Session: "s1"},
		{Kind: KindLabel, Session: "s1", Secs: &secs},
		{Kind: KindAuto, Session: "s1", Written: 4},
		{Kind: KindFix, Session: "s1"},
	} {
		if err := Append(dir, e); err != nil {
			t.Fatal(err)
		}
	}
	got := Summarize(dir)
	if got.Sessions != 1 || got.ManualLabels != 1 || got.AutoWritten != 4 || got.Corrections != 1 {
		t.Errorf("round trip lost events: %+v", got)
	}
	if got.CorrectionRate == nil || *got.CorrectionRate != 0.25 {
		t.Errorf("correction_rate = %v, want 0.25", got.CorrectionRate)
	}
	// The server's clock, not the browser's -- same reason the job poller
	// reports `now`.
	for _, e := range Read(dir) {
		if e.TS <= 0 {
			t.Error("Append did not stamp a server timestamp")
		}
	}
}

// An auto-label run from a session we never saw open must not push abandonment
// below zero.
func TestUnknownSessionDoesNotBreakAbandonment(t *testing.T) {
	dir := writeLog(t, []map[string]any{
		{"kind": "session", "session": "s1"},
		{"kind": "session", "session": "s2"},
		{"kind": "auto", "session": "s1", "written": 10},
		{"kind": "auto", "session": "never-opened", "written": 5},
	})
	got := Summarize(dir)
	if got.Abandonment == nil || *got.Abandonment != 0.5 {
		t.Errorf("abandonment = %v, want 0.5", got.Abandonment)
	}
	if got.SessionsReachingAutolabel != 1 {
		t.Errorf("sessions_reaching_autolabel = %d, want 1", got.SessionsReachingAutolabel)
	}
}
