package snmp

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// ipNetToMediaPhysAddress (RFC1213-MIB, standard, already vendored):
// ip.22 (ipNetToMediaTable) .1 (Entry) .2 = .1.3.6.1.2.1.4.22.1.2. -
// ipNetToMediaEntry's INDEX is { ipNetToMediaIfIndex,
// ipNetToMediaNetAddress }, so walking this one column directly yields
// PDUs whose OID suffix is "{ifIndex}.{octet1}.{octet2}.{octet3}.{octet4}"
// - after stripping the column prefix and the leading ifIndex component,
// the remaining suffix is already a correctly-formatted dotted-decimal
// IP string, no reassembly needed. The MAC comes from the PDU's *value*
// as raw bytes (a PhysAddress OCTET STRING), not the OID suffix - unlike
// every FDB-table MAC decode elsewhere in this codebase.
//
// switchmap's own GetArp.pl (the tool this project rewrites) tries the
// legacy atTable first, falling back to ipNetToMediaPhysAddress only if
// that fails - atTable predates and is superseded by ipNetToMediaTable
// even within RFC1213 itself, so (matching every other "skip untestable
// legacy MIBs" decision in this project) only ipNetToMediaPhysAddress is
// implemented here.
const oidIpNetToMediaPhysAddress = ".1.3.6.1.2.1.4.22.1.2."

// FetchArpTable walks a device's ARP cache and returns a MAC -> IP
// mapping. Deliberately not a registry group (see internal/snmp/groups.go) -
// that machinery is for per-device poll enrichment merged into
// model.Device/model.Port, and ARP data isn't about the device being
// polled at all, it's what a router's cache says about *other* hosts
// entirely. Standard MIB, no vendor variation, so no registry is needed
// here either.
func FetchArpTable(s *gosnmp.GoSNMP) (map[string]string, error) {
	arp := map[string]string{}
	err := snmputil.BulkWalkColumn(s, oidIpNetToMediaPhysAddress, func(pdu gosnmp.SnmpPDU) error {
		suffix := strings.TrimPrefix(pdu.Name, oidIpNetToMediaPhysAddress)
		_, ip, ok := strings.Cut(suffix, ".")
		if !ok {
			return nil
		}
		b, ok := pdu.Value.([]byte)
		if !ok {
			return nil
		}
		mac, ok := snmputil.MacFromBytes(b)
		if !ok {
			return nil
		}
		arp[mac] = ip
		return nil
	})
	if err != nil {
		return nil, err
	}
	return arp, nil
}
