package web

import "testing"

func TestIsAncillaryPort(t *testing.T) {
	cases := []struct {
		ifType int
		want   bool
	}{
		{1, true},   // other
		{135, true}, // l2vlan
		{161, true}, // ieee8023adLag
		{6, false},  // ethernetCsmacd - a normal physical port
		{24, false}, // softwareLoopback - virtual, but not ancillary
		{0, false},  // unset/unpolled
	}
	for _, c := range cases {
		if got := isAncillaryPort(c.ifType); got != c.want {
			t.Errorf("isAncillaryPort(%d) = %v, want %v", c.ifType, got, c.want)
		}
	}
}

func TestIsVirtualPort(t *testing.T) {
	cases := []struct {
		ifType int
		want   bool
	}{
		{22, true},  // proprietary serial
		{24, true},  // softwareLoopback
		{49, true},  // aal5
		{53, true},  // propVirtual
		{81, true},  // Digital Signal Level 0
		{100, true}, // voice recEive and transMit
		{101, true}, // voice Foreign Exchange Office
		{102, true}, // voice Foreign Exchange Station
		{103, true}, // voice encapsulation
		{108, true}, // pppMultilinkBundle
		{6, false},  // ethernetCsmacd - a normal physical port
		{1, false},  // other - ancillary, but not virtual
		{0, false},  // unset/unpolled
	}
	for _, c := range cases {
		if got := isVirtualPort(c.ifType); got != c.want {
			t.Errorf("isVirtualPort(%d) = %v, want %v", c.ifType, got, c.want)
		}
	}
}
