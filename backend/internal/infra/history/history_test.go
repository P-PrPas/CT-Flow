package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func pt(s string) json.RawMessage { return json.RawMessage(s) }

func TestReadMissingIsEmptyNotNil(t *testing.T) {
	got := Read(t.TempDir())
	if got == nil || len(got) != 0 {
		t.Fatalf("missing history = %#v, want an empty slice", got)
	}
}

func TestAppendCreatesThenAccumulatesInOrder(t *testing.T) {
	dir := t.TempDir()

	if _, err := Append(dir, pt(`{"f1":0.1}`)); err != nil {
		t.Fatal(err)
	}
	got, err := Append(dir, pt(`{"f1":0.2}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || string(got[0]) != `{"f1":0.1}` || string(got[1]) != `{"f1":0.2}` {
		t.Fatalf("history = %s, want the two points oldest-first", got)
	}
	// It is on disk, at the documented path, and reads back the same.
	if _, err := os.Stat(Path(dir)); err != nil {
		t.Fatalf("no file at %s: %v", Path(dir), err)
	}
	if back := Read(dir); len(back) != 2 || string(back[1]) != `{"f1":0.2}` {
		t.Fatalf("reload = %s", back)
	}
}

func TestAppendCapsAtMaxKeepingTheNewest(t *testing.T) {
	dir := t.TempDir()
	var last []json.RawMessage
	for i := 0; i < Max+50; i++ {
		var err error
		last, err = Append(dir, pt(`{"i":`+strconv.Itoa(i)+`}`))
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(last) != Max {
		t.Fatalf("kept %d points, want %d", len(last), Max)
	}
	// The oldest 50 are gone; the window is the newest Max.
	if string(last[0]) != `{"i":50}` || string(last[Max-1]) != `{"i":`+strconv.Itoa(Max+49)+`}` {
		t.Fatalf("window = [%s .. %s]", last[0], last[Max-1])
	}
}

// A truncated learning curve is a nicety to lose, never a reason to fail the
// request that asked for it.
func TestReadCorruptFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Read(dir); len(got) != 0 {
		t.Fatalf("corrupt history read as %s, want empty", got)
	}
	// ...and Append recovers by writing a fresh one-point history over it.
	got, err := Append(dir, pt(`{"f1":0.9}`))
	if err != nil || len(got) != 1 {
		t.Fatalf("Append over corrupt = %s (err %v)", got, err)
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	if _, err := Append(dir, pt(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := Clear(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("file still there after Clear: %v", err)
	}
	// Clearing an already-clear history is a no-op, not an error.
	if err := Clear(dir); err != nil {
		t.Errorf("Clear on a missing file = %v", err)
	}
}
