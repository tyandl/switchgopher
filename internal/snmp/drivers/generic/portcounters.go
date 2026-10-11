package generic

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
)

const (
	oidIfInDiscards     = ".1.3.6.1.2.1.2.2.1.13."
	oidIfInErrors       = ".1.3.6.1.2.1.2.2.1.14."
	oidIfOutDiscards    = ".1.3.6.1.2.1.2.2.1.19."
	oidIfOutErrors      = ".1.3.6.1.2.1.2.2.1.20."
	oidIfHCInUcastPkts  = ".1.3.6.1.2.1.31.1.1.1.7."
	oidIfHCOutUcastPkts = ".1.3.6.1.2.1.31.1.1.1.11."
)

func init() {
	snmp.RegisterPortCounters(snmp.VendorGeneric, portCountersFetcher{})
}

type portCountersFetcher struct{}

// FetchPortCounters returns each interface's cumulative I/O counters,
// keyed by ifIndex - InPackets/OutPackets from ifXTable's Counter64
// ifHCInUcastPkts/ifHCOutUcastPkts, the rest from ifTable's Counter32
// columns.
func (portCountersFetcher) FetchPortCounters(s *gosnmp.GoSNMP) (map[string]snmp.PortCounters, error) {
	counters := map[string]snmp.PortCounters{}

	get := func(idx string) snmp.PortCounters { return counters[idx] }

	collect := func(pdu gosnmp.SnmpPDU) error {
		v := gosnmp.ToBigInt(pdu.Value).Int64()
		switch {
		case strings.HasPrefix(pdu.Name, oidIfInDiscards):
			idx := strings.TrimPrefix(pdu.Name, oidIfInDiscards)
			c := get(idx)
			c.InDiscards = v
			counters[idx] = c
		case strings.HasPrefix(pdu.Name, oidIfInErrors):
			idx := strings.TrimPrefix(pdu.Name, oidIfInErrors)
			c := get(idx)
			c.InErrors = v
			counters[idx] = c
		case strings.HasPrefix(pdu.Name, oidIfOutDiscards):
			idx := strings.TrimPrefix(pdu.Name, oidIfOutDiscards)
			c := get(idx)
			c.OutDiscards = v
			counters[idx] = c
		case strings.HasPrefix(pdu.Name, oidIfOutErrors):
			idx := strings.TrimPrefix(pdu.Name, oidIfOutErrors)
			c := get(idx)
			c.OutErrors = v
			counters[idx] = c
		case strings.HasPrefix(pdu.Name, oidIfHCInUcastPkts):
			idx := strings.TrimPrefix(pdu.Name, oidIfHCInUcastPkts)
			c := get(idx)
			c.InPackets = v
			counters[idx] = c
		case strings.HasPrefix(pdu.Name, oidIfHCOutUcastPkts):
			idx := strings.TrimPrefix(pdu.Name, oidIfHCOutUcastPkts)
			c := get(idx)
			c.OutPackets = v
			counters[idx] = c
		}
		return nil
	}

	if err := s.BulkWalk(oidIfTablePrefix, collect); err != nil {
		return nil, err
	}
	if err := s.BulkWalk(oidIfXTablePrefix, collect); err != nil {
		return nil, err
	}
	return counters, nil
}
