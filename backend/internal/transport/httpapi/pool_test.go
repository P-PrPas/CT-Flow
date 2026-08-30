package httpapi

import (
	"strconv"
	"testing"
)

func TestQueryInt(t *testing.T) {
	for _, tc := range []struct {
		in   string
		def  int
		want int
	}{
		{"", 200, 200},        // absent -> default
		{"0", 200, 0},         // an explicit zero is not "absent"
		{"50", 200, 50},       //
		{"-5", 0, -5},         // negative is parsed; the caller clamps, not this
		{"abc", 200, 200},     // garbage -> default, never an error
		{"3.5", 200, 200},     // not an int -> default
		{"999999", 1, 999999}, //
	} {
		if got := queryInt(tc.in, tc.def); got != tc.want {
			t.Errorf("queryInt(%q, %d) = %d, want %d", tc.in, tc.def, got, tc.want)
		}
	}
}

func TestSliceSet(t *testing.T) {
	got := sliceSet([]string{"a", "b", "a", "c"})
	if len(got) != 3 || !got["a"] || !got["b"] || !got["c"] || got["z"] {
		t.Fatalf("sliceSet dedupe/membership wrong: %v", got)
	}
	if s := sliceSet(nil); len(s) != 0 {
		t.Errorf("sliceSet(nil) = %v, want empty", s)
	}
}

// The thumbnail cache is a plain LRU: a hit moves the entry to the front, and
// once it is full the least-recently-used key is the one that goes.
func TestThumbCacheLRU(t *testing.T) {
	c := newThumbCache()
	put := func(k string) { c.put(&thumbEntry{key: k, body: []byte(k)}) }

	// Fill exactly to the cap.
	for i := 0; i < thumbCacheN; i++ {
		put("k" + strconv.Itoa(i))
	}
	if _, ok := c.get("k0"); !ok {
		t.Fatal("k0 fell out before the cache was over the cap")
	}

	// Touch k0 so it is now the most-recently-used, then push one past the cap.
	c.get("k0")
	put("overflow")

	if _, ok := c.get("k1"); ok {
		t.Error("k1 (now the LRU) survived the overflow")
	}
	if _, ok := c.get("k0"); !ok {
		t.Error("k0 was evicted despite being touched")
	}
	if _, ok := c.get("overflow"); !ok {
		t.Error("the entry that caused the eviction is not in the cache")
	}
	if c.ll.Len() != thumbCacheN {
		t.Errorf("cache holds %d entries, cap is %d", c.ll.Len(), thumbCacheN)
	}

	// Re-putting an existing key updates it in place, it does not grow the cache
	// or duplicate the list node.
	put("overflow")
	if c.ll.Len() != thumbCacheN {
		t.Errorf("re-put grew the cache to %d", c.ll.Len())
	}
}
