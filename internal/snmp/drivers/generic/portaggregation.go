package generic

import (
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// ifStackStatus (IF-MIB ifStackEntry.3, .1.3.6.1.2.1.31.1.2.1.3.) is the
// only walkable column of ifStackTable - ifStackHigherLayer/
// ifStackLowerLayer are index components (not-accessible), encoded in
// each row's OID suffix as "{higher}.{lower}" instead.
const oidIfStackStatus = ".1.3.6.1.2.1.31.1.2.1.3."

const rowStatusActive = 1

func init() {
	snmp.RegisterPortAggregation(snmp.VendorGeneric, portAggregationFetcher{})
}

type portAggregationFetcher struct{}

// FetchPortAggregation returns, for each aggregated member port, the
// ifIndex of the logical aggregate it belongs to - keyed by the member's
// own ifIndex. ifStackTable relationships aren't exclusively about
// aggregation (e.g. a VLAN sub-interface stacked over a trunk port shows
// up here too), so this filters to pairs where the higher-layer
// interface's ifType is ieee8023adLag(161) - that's what actually
// distinguishes a real EtherChannel/LACP aggregate from any other kind of
// interface stacking.
func (portAggregationFetcher) FetchPortAggregation(s *gosnmp.GoSNMP) (map[string]string, error) {
	ifTypes := walkIfType(s)
	aggregation := map[string]string{}

	err := snmputil.BulkWalkColumn(s, oidIfStackStatus, func(pdu gosnmp.SnmpPDU) error {
		suffix := strings.TrimPrefix(pdu.Name, oidIfStackStatus)
		higher, lower, ok := splitStackIndex(suffix)
		if !ok {
			return nil
		}
		if gosnmp.ToBigInt(pdu.Value).Int64() != rowStatusActive {
			return nil
		}
		if higher == 0 || lower == 0 {
			return nil // one of switchmap/IF-MIB's own "no stacking" placeholder rows
		}
		if ifTypes[strconv.Itoa(higher)] != ifTypeIeee8023adLag {
			return nil
		}
		aggregation[strconv.Itoa(lower)] = strconv.Itoa(higher)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return aggregation, nil
}

// splitStackIndex parses an ifStackEntry OID suffix "{higher}.{lower}"
// (INDEX { ifStackHigherLayer, ifStackLowerLayer }) into its two ifIndex
// values.
func splitStackIndex(suffix string) (higher, lower int, ok bool) {
	parts := strings.SplitN(suffix, ".", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	higher, ok1 := parseSuffixInt(parts[0])
	lower, ok2 := parseSuffixInt(parts[1])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return higher, lower, true
}
