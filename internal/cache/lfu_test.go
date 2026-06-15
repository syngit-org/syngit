package cache

import "testing"

func TestLFUSetGet(t *testing.T) {
	c := NewLFU[string, int](4)
	c.Set("a", 1)

	got, ok := c.Get("a")
	if !ok || got != 1 {
		t.Fatalf("Get(a) = (%d, %v), want (1, true)", got, ok)
	}

	if _, ok := c.Get("missing"); ok {
		t.Fatalf("Get(missing) = true, want false")
	}
}

func TestLFUSetOverwrites(t *testing.T) {
	c := NewLFU[string, int](4)
	c.Set("a", 1)
	c.Set("a", 2)

	if got, ok := c.Get("a"); !ok || got != 2 {
		t.Fatalf("Get(a) = (%d, %v), want (2, true)", got, ok)
	}
	if len(c.entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (overwrite must not add a second entry)", len(c.entries))
	}
}

func TestLFUDelete(t *testing.T) {
	c := NewLFU[string, int](4)
	c.Set("a", 1)
	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Fatalf("Get(a) after Delete = true, want false")
	}
	c.Delete("a") // deleting an absent key is a no-op
}

func TestLFUEvictsLeastFrequentlyUsed(t *testing.T) {
	c := NewLFU[string, int](2)
	c.Set("a", 1)
	c.Set("b", 2)

	// Make "a" more frequently used than "b", then insert "c" to force eviction.
	c.Get("a")
	c.Get("a")
	c.Set("c", 3)

	if _, ok := c.Get("b"); ok {
		t.Fatalf("b should have been evicted as least-frequently-used")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatalf("a should have survived eviction")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatalf("c should be present (just inserted)")
	}
}

func TestLFUNeverEvictsJustInserted(t *testing.T) {
	c := NewLFU[string, int](1)
	c.Set("a", 1)
	c.Set("b", 2) // must evict "a", never the freshly-inserted "b"

	if _, ok := c.Get("a"); ok {
		t.Fatalf("a should have been evicted")
	}
	if got, ok := c.Get("b"); !ok || got != 2 {
		t.Fatalf("Get(b) = (%d, %v), want (2, true)", got, ok)
	}
}

func TestLFUAgingHalvesCounters(t *testing.T) {
	c := NewLFU[string, int](2)
	c.Set("a", 1)
	c.Set("b", 2)

	// Drive "a" up to the aging cap; the cap is hit during a Get, which then
	// halves every counter. The cache must remain consistent afterwards.
	for i := uint32(0); i < useCountCap; i++ {
		c.Get("a")
	}
	if c.entries["a"].useCount > useCountCap/2+1 {
		t.Fatalf("aging did not halve a's counter: useCount=%d", c.entries["a"].useCount)
	}
	if got, ok := c.Get("a"); !ok || got != 1 {
		t.Fatalf("Get(a) = (%d, %v), want (1, true)", got, ok)
	}
}

func TestLFUNilIsDisabled(t *testing.T) {
	c := NewLFU[string, int](0) // disabled => nil
	if c != nil {
		t.Fatalf("NewLFU(0) = %v, want nil", c)
	}

	// All methods must be safe no-ops on a nil cache.
	c.Set("a", 1)
	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Fatalf("Get on disabled cache = true, want false")
	}
}
