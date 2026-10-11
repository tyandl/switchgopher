// Package snmputil holds small, dependency-free SNMP decoding helpers shared
// across the generic and vendor-specific fetch implementations. It depends
// only on gosnmp, never on internal/model or internal/snmp, so both the core
// registry package and every vendor package can import it without a cycle.
package snmputil

import (
	"fmt"

	"github.com/gosnmp/gosnmp"
)

// PduString normalizes an SNMP PDU value to a string regardless of whether
// gosnmp decoded it as an OctetString ([]byte) or something else.
func PduString(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
