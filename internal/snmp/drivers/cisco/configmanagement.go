package cisco

import (
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
)

// No switchmap precedent exists for config-change tracking - this OID
// comes from reading the actual vendored MIB text directly.
//
// ccmHistoryRunningLastChanged (CISCO-CONFIG-MAN-MIB: ciscoMgmt.43 ->
// ciscoConfigManMIBObjects.1 (ccmHistory) -> entry.1), scalar (.0
// instance). Its SYNTAX is TimeTicks - "the value of sysUpTime when the
// running configuration was last changed" - i.e. relative to the
// device's own uptime clock, not a Unix timestamp. Converting to one
// needs the device's *current* sysUpTime as a reference point, which
// this fetches itself (a second, redundant Get) rather than changing
// ConfigManagementFetcher's interface signature to also accept the
// value FetchSystemInfo already retrieved - keeps every group interface
// uniform (session only), at the cost of one extra round-trip.
const (
	oidCcmHistoryRunningLastChanged = ".1.3.6.1.4.1.9.9.43.1.1.1.0"
	oidSysUptime                    = ".1.3.6.1.2.1.1.3.0"
)

func init() {
	snmp.RegisterConfigManagement(snmp.Vendor("cisco"), configManagementFetcher{})
}

type configManagementFetcher struct{}

// FetchConfigUpdated returns the Unix timestamp of a device's last
// running-config change, computed from the TimeTicks delta between
// ccmHistoryRunningLastChanged and the device's current sysUpTime -
// accurate to within this poll's own latency, which is the best
// precision a TimeTicks-relative value can offer.
func (configManagementFetcher) FetchConfigUpdated(s *gosnmp.GoSNMP) (int64, error) {
	result, err := s.Get([]string{oidCcmHistoryRunningLastChanged, oidSysUptime})
	if err != nil {
		return 0, fmt.Errorf("fetchConfigUpdated: %w", err)
	}
	if len(result.Variables) != 2 {
		return 0, fmt.Errorf("fetchConfigUpdated: expected 2 values, got %d", len(result.Variables))
	}

	lastChanged, uptime := result.Variables[0], result.Variables[1]
	if lastChanged.Type == gosnmp.NoSuchObject || lastChanged.Type == gosnmp.NoSuchInstance {
		return 0, fmt.Errorf("fetchConfigUpdated: %w", snmp.ErrNotSupported)
	}

	lastChangedTicks := gosnmp.ToBigInt(lastChanged.Value).Int64()
	uptimeTicks := gosnmp.ToBigInt(uptime.Value).Int64()

	elapsed := time.Duration(uptimeTicks-lastChangedTicks) * 10 * time.Millisecond // TimeTicks are hundredths of a second
	return time.Now().Add(-elapsed).Unix(), nil
}
