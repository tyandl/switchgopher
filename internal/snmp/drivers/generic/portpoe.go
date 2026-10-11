package generic

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// POWER-ETHERNET-MIB (RFC 3621), verified against the vendored file:
// powerEthernetMIB = mib-2.105, pethObjects = .1, pethPsePortTable = .1.1,
// pethPsePortEntry = .1.1.1. pethPsePortDetectionStatus = entry.6,
// pethPsePortPowerClassifications = entry.10.
const (
	oidPethPsePortDetectionStatus      = ".1.3.6.1.2.1.105.1.1.1.6."
	oidPethPsePortPowerClassifications = ".1.3.6.1.2.1.105.1.1.1.10."
)

const pethPsePortDetectionStatusDeliveringPower = 3

// pethPsePortPowerClassifications enum: class0(1)..class4(5).
var poeClassNames = map[int64]string{
	1: "class0",
	2: "class1",
	3: "class2",
	4: "class3",
	5: "class4",
}

func init() {
	snmp.RegisterPortPoE(snmp.VendorGeneric, portPoEFetcher{})
}

type portPoEFetcher struct{}

// FetchPortPoE returns each interface's PoE state, keyed by ifIndex - with
// a real limitation worth documenting rather than hiding: pethPsePortEntry
// is INDEX { pethPsePortGroupIndex, pethPsePortIndex }, NOT ifIndex
// (verified against the actual RFC 3621 text - pethPsePortGroupIndex is
// "box in the stack, module in a rack," value 1 for non-modular devices;
// pethPsePortIndex is the port number within that group). Neither this
// MIB nor RFC 3621 defines a formal mapping from that pair to ifIndex.
// This treats pethPsePortIndex as a best-effort ifIndex only when
// pethPsePortGroupIndex == 1 (the common single-group/non-modular case,
// where port numbering conventionally lines up) and skips any row from a
// higher group entirely, rather than guess at a cross-reference that
// could silently attribute PoE state to the wrong port on a modular or
// stacked chassis. Unverified against real PoE hardware - the local
// snmpd test simulator doesn't implement this MIB at all.
func (portPoEFetcher) FetchPortPoE(s *gosnmp.GoSNMP) (map[string]snmp.PortPoE, error) {
	poe := map[string]snmp.PortPoE{}

	get := func(idx string) snmp.PortPoE { return poe[idx] }

	if err := snmputil.BulkWalkColumn(s, oidPethPsePortDetectionStatus, func(pdu gosnmp.SnmpPDU) error {
		suffix := strings.TrimPrefix(pdu.Name, oidPethPsePortDetectionStatus)
		ifIndex, ok := pethPortToIfIndex(suffix)
		if !ok {
			return nil
		}
		p := get(ifIndex)
		p.Active = gosnmp.ToBigInt(pdu.Value).Int64() == pethPsePortDetectionStatusDeliveringPower
		poe[ifIndex] = p
		return nil
	}); err != nil {
		return nil, err
	}

	if err := snmputil.BulkWalkColumn(s, oidPethPsePortPowerClassifications, func(pdu gosnmp.SnmpPDU) error {
		suffix := strings.TrimPrefix(pdu.Name, oidPethPsePortPowerClassifications)
		ifIndex, ok := pethPortToIfIndex(suffix)
		if !ok {
			return nil
		}
		p := get(ifIndex)
		p.Class = poeClassNames[gosnmp.ToBigInt(pdu.Value).Int64()]
		poe[ifIndex] = p
		return nil
	}); err != nil {
		return nil, err
	}

	return poe, nil
}

// pethPortToIfIndex applies the group==1 heuristic documented above to a
// "{group}.{port}" OID suffix.
func pethPortToIfIndex(suffix string) (string, bool) {
	parts := strings.SplitN(suffix, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	group, ok := parseSuffixInt(parts[0])
	if !ok || group != 1 {
		return "", false
	}
	if _, ok := parseSuffixInt(parts[1]); !ok {
		return "", false
	}
	return parts[1], true
}
