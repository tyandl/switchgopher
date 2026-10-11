package generic

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// dot3StatsDuplexStatus (EtherLike-MIB, dot3 = transmission.7,
// dot3StatsTable = dot3.2, dot3StatsEntry.19 =
// .1.3.6.1.2.1.10.7.2.1.19.) - indexed by dot3StatsIndex, which the MIB
// itself documents as "the same interface as identified by the same
// value of ifIndex," so no suffix decoding is needed. Cross-vendor
// standard, confirmed against the actual switchmap Perl source
// (PopulatePorts.pm's GetPortToDuplex) as the first thing it tries,
// before any Cisco-specific MIB - Cisco fallbacks there only exist for
// hardware old enough to predate this MIB's support (Catalyst
// 1900/2900XL/3500XL/5000-in-CatOS era), none of which this project's
// own seed data represents, so no vendor override is built for this
// group.
const oidDot3StatsDuplexStatus = ".1.3.6.1.2.1.10.7.2.1.19."

// model.Port.Duplex encoding - a clean value set defined for this
// project, not carried over from switchmap's own inconsistent per-MIB
// numeric codes (which differ table-to-table and get normalized to
// strings in Perl). Matches dot3StatsDuplexStatus's own enum directly
// (unknown=1, half=2, full=3) plus a 0 for "no data at all," consistent
// with every other "not populated" field in this project defaulting to
// zero value.
const (
	DuplexUnset   = 0
	DuplexUnknown = 1
	DuplexHalf    = 2
	DuplexFull    = 3
)

func init() {
	snmp.RegisterPortDuplex(snmp.VendorGeneric, portDuplexFetcher{})
}

type portDuplexFetcher struct{}

// FetchPortDuplex returns each interface's duplex mode, keyed by ifIndex.
func (portDuplexFetcher) FetchPortDuplex(s *gosnmp.GoSNMP) (map[string]int, error) {
	duplex := map[string]int{}
	err := snmputil.BulkWalkColumn(s, oidDot3StatsDuplexStatus, func(pdu gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(pdu.Name, oidDot3StatsDuplexStatus)
		duplex[idx] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return duplex, nil
}
