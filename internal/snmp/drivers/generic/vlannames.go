package generic

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// dot1qVlanStaticName (Q-BRIDGE-MIB: qBridgeMIB.7 -> dot1dBridge.7,
// qBridgeMIBObjects.1, dot1qVlan.4, dot1qVlanStaticTable.3,
// dot1qVlanStaticEntry.1 = .1.3.6.1.2.1.17.7.1.4.3.1.1.) - indexed
// directly by dot1qVlanIndex (the VLAN ID itself), no decoding needed.
const oidDot1qVlanStaticName = ".1.3.6.1.2.1.17.7.1.4.3.1.1."

func init() {
	snmp.RegisterVlanNames(snmp.VendorGeneric, vlanNamesFetcher{})
}

type vlanNamesFetcher struct{}

// FetchVlanNames returns a device-local view of configured VLAN IDs and
// names. Not every device implements Q-BRIDGE-MIB (e.g. the local
// net-snmp test simulator doesn't) - an empty result is normal, not an
// error.
func (vlanNamesFetcher) FetchVlanNames(s *gosnmp.GoSNMP) (map[int]string, error) {
	names := map[int]string{}
	err := snmputil.BulkWalkColumn(s, oidDot1qVlanStaticName, func(pdu gosnmp.SnmpPDU) error {
		suffix := strings.TrimPrefix(pdu.Name, oidDot1qVlanStaticName)
		vlan, ok := parseSuffixInt(suffix)
		if !ok {
			return nil
		}
		names[vlan] = snmputil.PduString(pdu)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}
