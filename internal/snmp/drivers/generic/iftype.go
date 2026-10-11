package generic

import (
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// ifType (IF-MIB ifEntry.3, .1.3.6.1.2.1.2.2.1.3.) - shared by
// portaggregation.go (filtering for ieee8023adLag) and islayer3.go
// (filtering for l2vlan).
const oidIfType = ".1.3.6.1.2.1.2.2.1.3."

const (
	ifTypeIeee8023adLag = 161 // IANAifType-MIB: IEEE 802.3ad Link Aggregate
	ifTypeL2vlan        = 135 // IANAifType-MIB: Layer 2 Virtual LAN using 802.1Q
)

// walkIfType returns each interface's ifType, keyed by ifIndex.
func walkIfType(s *gosnmp.GoSNMP) map[string]int {
	ifTypes := map[string]int{}
	_ = snmputil.BulkWalkColumn(s, oidIfType, func(pdu gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(pdu.Name, oidIfType)
		ifTypes[idx] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		return nil
	})
	return ifTypes
}

// parseSuffixInt is a small helper for the OID-suffix integer parsing
// portaggregation.go (ifIndex) and vlannames.go (VLAN ID) both need.
func parseSuffixInt(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
