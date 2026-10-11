package generic

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
)

// dot1dStpTopChanges (BRIDGE-MIB, dot1dStp.4 = mib-2.17.2.4, scalar Counter32).
const oidDot1dStpTopChanges = ".1.3.6.1.2.1.17.2.4.0"

func init() {
	snmp.RegisterSpanningTree(snmp.VendorGeneric, spanningTreeFetcher{})
}

type spanningTreeFetcher struct{}

// FetchStpTopChanges returns a device's spanning-tree topology-change
// count since last reset. Not every device implements BRIDGE-MIB's
// spanning-tree group (e.g. the local net-snmp test simulator doesn't) -
// that's a normal Get failure here, not something worth wrapping in a
// special case; the caller already treats every group's error as
// best-effort/skip.
func (spanningTreeFetcher) FetchStpTopChanges(s *gosnmp.GoSNMP) (int64, error) {
	result, err := s.Get([]string{oidDot1dStpTopChanges})
	if err != nil {
		return 0, fmt.Errorf("fetchStpTopChanges: %w", err)
	}
	if len(result.Variables) == 0 {
		return 0, fmt.Errorf("fetchStpTopChanges: no value returned")
	}
	// SNMPv2c reports a missing scalar per-variable (Type NoSuchObject/
	// NoSuchInstance), not as a transport-level error from Get itself -
	// worth a real error here rather than silently returning a 0 that
	// would look identical to "0 topology changes recorded."
	v := result.Variables[0]
	if v.Type == gosnmp.NoSuchObject || v.Type == gosnmp.NoSuchInstance {
		return 0, fmt.Errorf("fetchStpTopChanges: not implemented by this device")
	}
	return gosnmp.ToBigInt(v.Value).Int64(), nil
}
