package generic

import "testing"

func TestParseOid(t *testing.T) {
	oid, err := parseOid(".1.3.6.1.4.1.9.1.716")
	if err != nil {
		t.Fatal(err)
	}
	want := []uint32{1, 3, 6, 1, 4, 1, 9, 1, 716}
	if len(oid) != len(want) {
		t.Fatalf("got %v, want %v", oid, want)
	}
	for i, w := range want {
		if uint32(oid[i]) != w {
			t.Errorf("index %d: got %d, want %d", i, oid[i], w)
		}
	}
}

func TestParseOidNoLeadingDot(t *testing.T) {
	oid, err := parseOid("1.3.6.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(oid) != 4 {
		t.Fatalf("got %v", oid)
	}
}

func TestParseOidInvalid(t *testing.T) {
	if _, err := parseOid(".1.3.x.1"); err == nil {
		t.Error("expected error for non-numeric component")
	}
}
