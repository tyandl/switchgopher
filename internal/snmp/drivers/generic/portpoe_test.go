package generic

import "testing"

func TestPethPortToIfIndexGroupOne(t *testing.T) {
	ifIndex, ok := pethPortToIfIndex("1.24")
	if !ok || ifIndex != "24" {
		t.Errorf("got %q, %v, want \"24\", true", ifIndex, ok)
	}
}

func TestPethPortToIfIndexSkipsOtherGroups(t *testing.T) {
	// Group != 1 (modular/stacked chassis) has no confirmed correlation to
	// ifIndex - must be skipped, not guessed at.
	if _, ok := pethPortToIfIndex("2.24"); ok {
		t.Error("expected not ok for a non-1 PSE group index")
	}
}

func TestPethPortToIfIndexMalformed(t *testing.T) {
	if _, ok := pethPortToIfIndex("1"); ok {
		t.Error("expected not ok for a suffix with only one component")
	}
	if _, ok := pethPortToIfIndex("1.not-a-number"); ok {
		t.Error("expected not ok for a non-numeric port component")
	}
}
