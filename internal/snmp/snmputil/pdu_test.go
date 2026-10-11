package snmputil

import (
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestPduString(t *testing.T) {
	cases := []struct {
		pdu  gosnmp.SnmpPDU
		want string
	}{
		{gosnmp.SnmpPDU{Value: []byte("hello")}, "hello"},
		{gosnmp.SnmpPDU{Value: "already-a-string"}, "already-a-string"},
		{gosnmp.SnmpPDU{Value: 42}, "42"},
	}
	for _, c := range cases {
		if got := PduString(c.pdu); got != c.want {
			t.Errorf("PduString(%v) = %q, want %q", c.pdu.Value, got, c.want)
		}
	}
}
