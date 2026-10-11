package generic

import "testing"

func TestSplitStackIndex(t *testing.T) {
	higher, lower, ok := splitStackIndex("24.3")
	if !ok {
		t.Fatal("expected ok")
	}
	if higher != 24 || lower != 3 {
		t.Errorf("got higher=%d lower=%d, want 24, 3", higher, lower)
	}
}

func TestSplitStackIndexNoLowerSublayer(t *testing.T) {
	// "0.x" and "x.0" are IF-MIB's own placeholder rows for an interface
	// not involved in stacking at all - still parse fine, callers filter
	// on the zero value separately.
	higher, lower, ok := splitStackIndex("0.5")
	if !ok || higher != 0 || lower != 5 {
		t.Errorf("got higher=%d lower=%d ok=%v, want 0, 5, true", higher, lower, ok)
	}
}

func TestSplitStackIndexMalformed(t *testing.T) {
	if _, _, ok := splitStackIndex("not-a-number.3"); ok {
		t.Error("expected not ok for non-numeric component")
	}
	if _, _, ok := splitStackIndex("24"); ok {
		t.Error("expected not ok for a suffix with only one component")
	}
}

func TestParseSuffixInt(t *testing.T) {
	n, ok := parseSuffixInt("10")
	if !ok || n != 10 {
		t.Errorf("got %d, %v, want 10, true", n, ok)
	}
}

func TestParseSuffixIntInvalid(t *testing.T) {
	if _, ok := parseSuffixInt("not-a-number"); ok {
		t.Error("expected not ok for non-numeric input")
	}
}
