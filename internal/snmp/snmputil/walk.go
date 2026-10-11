package snmputil

import (
	"strings"

	"github.com/gosnmp/gosnmp"
)

// BulkWalkColumn walks a single SNMP table column. Column OID constants
// in this codebase are conventionally written with a trailing dot (so
// strings.TrimPrefix against a returned PDU's Name correctly isolates
// its index suffix) - but gosnmp.BulkWalk's root argument must NOT have
// that trailing dot, or it silently matches zero rows even when real
// data exists at that column (confirmed against a live snmpd instance:
// walking ".1.3.6.1.2.1.4.20.1.2." found nothing, walking
// ".1.3.6.1.2.1.4.20.1.2" found every row). This wraps that trim so
// every column walk in this codebase gets it right without repeating
// the fix - and the gotcha - at every call site.
func BulkWalkColumn(s *gosnmp.GoSNMP, columnOID string, walker gosnmp.WalkFunc) error {
	return s.BulkWalk(strings.TrimSuffix(columnOID, "."), walker)
}
