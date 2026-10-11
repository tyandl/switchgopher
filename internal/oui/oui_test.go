package oui

import "testing"

func TestParseCSV(t *testing.T) {
	got, err := ParseCSV("testdata/oui_sample.csv")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		oui  string
		want string
	}{
		{"E80AB9", "Cisco Systems, Inc"},
		{"0014C9", "Cisco Systems Inc"},
		{"AABBCC", "Test, With Comma Inc."},
	}
	for _, c := range cases {
		if got[c.oui] != c.want {
			t.Errorf("ParseCSV()[%q] = %q, want %q", c.oui, got[c.oui], c.want)
		}
	}
	if _, ok := got["FFFFFF"]; ok {
		t.Errorf("ParseCSV() should not have an entry for an OUI not in the file")
	}
	if len(got) != len(cases) {
		t.Errorf("ParseCSV() returned %d entries, want %d", len(got), len(cases))
	}
}

func TestParseCSVMissingFile(t *testing.T) {
	if _, err := ParseCSV("testdata/does-not-exist.csv"); err == nil {
		t.Error("expected an error for a missing file")
	}
}
