package snmp

import "testing"

func TestDeriveVendorKeyCisco(t *testing.T) {
	// A real Cisco sysObjectID (Catalyst 3560), enterprise arc 1.3.6.1.4.1.9.
	if got := DeriveVendorKey(".1.3.6.1.4.1.9.1.716"); got != "cisco" {
		t.Errorf("got %q, want cisco", got)
	}
}

func TestDeriveVendorKeyCiscoNoLeadingDot(t *testing.T) {
	if got := DeriveVendorKey("1.3.6.1.4.1.9.1.716"); got != "cisco" {
		t.Errorf("got %q, want cisco", got)
	}
}

func TestDeriveVendorKeyUnknownFallsBackToGeneric(t *testing.T) {
	// net-snmp's own enterprise arc (1.3.6.1.4.1.8072) - the local test
	// simulator's sysObjectID - has no registered vendor.
	if got := DeriveVendorKey(".1.3.6.1.4.1.8072.3.2.10"); got != VendorGeneric {
		t.Errorf("got %q, want %q", got, VendorGeneric)
	}
}

func TestDeriveVendorKeyEmpty(t *testing.T) {
	if got := DeriveVendorKey(""); got != VendorGeneric {
		t.Errorf("got %q, want %q", got, VendorGeneric)
	}
}

func TestDeriveVendorKeyDoesNotPrefixMatchUnrelatedArc(t *testing.T) {
	// 1.3.6.1.4.1.90 should NOT match the "1.3.6.1.4.1.9" (Cisco) prefix -
	// this guards against a naive strings.HasPrefix(trimmed, prefix) bug.
	if got := DeriveVendorKey(".1.3.6.1.4.1.90.1.1"); got != VendorGeneric {
		t.Errorf("got %q, want %q (90 must not prefix-match 9)", got, VendorGeneric)
	}
}
