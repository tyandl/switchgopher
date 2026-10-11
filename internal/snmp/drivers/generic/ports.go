package generic

import (
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

const (
	oidIfTablePrefix  = ".1.3.6.1.2.1.2.2.1"
	oidIfDescr        = ".1.3.6.1.2.1.2.2.1.2."
	oidIfAdminStatus  = ".1.3.6.1.2.1.2.2.1.7."
	oidIfSpeed        = ".1.3.6.1.2.1.2.2.1.5."
	oidIfOperStatus   = ".1.3.6.1.2.1.2.2.1.8."
	oidIfLastChange   = ".1.3.6.1.2.1.2.2.1.9."
	oidIfXTablePrefix = ".1.3.6.1.2.1.31.1.1.1"
	oidIfAlias        = ".1.3.6.1.2.1.31.1.1.1.18."
	oidIfHighSpeed    = ".1.3.6.1.2.1.31.1.1.1.15."

	// oidSysUpTime - same scalar, same TimeTicks-delta conversion approach
	// as cisco/configmanagement.go's FetchConfigUpdated (see that file's
	// doc comment for why this is a second, redundant Get rather than a
	// changed group interface). Declared separately here since it's a
	// different package - not worth sharing a private const across two
	// otherwise-independent driver packages for one OID string.
	oidSysUpTime = ".1.3.6.1.2.1.1.3.0"
)

func init() {
	snmp.RegisterPorts(snmp.VendorGeneric, portsFetcher{})
}

type portsFetcher struct{}

// FetchPorts walks ifTable/ifXTable and returns one Port per interface,
// keyed by ifIndex. Device is left blank here - callers writing to the DB
// already know which device this is and pass it separately.
func (portsFetcher) FetchPorts(s *gosnmp.GoSNMP) ([]model.Port, error) {
	descr := map[string]string{}
	alias := map[string]string{}
	speed := map[string]int{}
	highSpeed := map[string]int{}
	status := map[string]int{}
	adminStatus := map[string]int{}
	ifType := map[string]int{}
	lastChange := map[string]int64{}

	collect := func(pdu gosnmp.SnmpPDU) error {
		switch {
		case strings.HasPrefix(pdu.Name, oidIfDescr):
			descr[strings.TrimPrefix(pdu.Name, oidIfDescr)] = snmputil.PduString(pdu)
		case strings.HasPrefix(pdu.Name, oidIfAlias):
			alias[strings.TrimPrefix(pdu.Name, oidIfAlias)] = snmputil.PduString(pdu)
		case strings.HasPrefix(pdu.Name, oidIfSpeed):
			speed[strings.TrimPrefix(pdu.Name, oidIfSpeed)] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		case strings.HasPrefix(pdu.Name, oidIfHighSpeed):
			highSpeed[strings.TrimPrefix(pdu.Name, oidIfHighSpeed)] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		case strings.HasPrefix(pdu.Name, oidIfOperStatus):
			status[strings.TrimPrefix(pdu.Name, oidIfOperStatus)] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		case strings.HasPrefix(pdu.Name, oidIfAdminStatus):
			adminStatus[strings.TrimPrefix(pdu.Name, oidIfAdminStatus)] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		case strings.HasPrefix(pdu.Name, oidIfType):
			ifType[strings.TrimPrefix(pdu.Name, oidIfType)] = int(gosnmp.ToBigInt(pdu.Value).Int64())
		case strings.HasPrefix(pdu.Name, oidIfLastChange):
			lastChange[strings.TrimPrefix(pdu.Name, oidIfLastChange)] = gosnmp.ToBigInt(pdu.Value).Int64()
		}
		return nil
	}

	if err := s.BulkWalk(oidIfTablePrefix, collect); err != nil {
		return nil, err
	}
	if err := s.BulkWalk(oidIfXTablePrefix, collect); err != nil {
		return nil, err
	}

	// ifLastChange is TimeTicks - "the value of sysUpTime at the time the
	// interface entered its current operational state" - relative to the
	// device's own uptime clock, not a Unix timestamp. Converting needs the
	// device's *current* sysUpTime as a reference point, fetched here via a
	// second, redundant Get - same approach and same reasoning as
	// cisco/configmanagement.go's FetchConfigUpdated (see oidSysUpTime's
	// doc comment). Best-effort: a failed Get here shouldn't fail the
	// whole poll just because the "Since" column can't be computed, so
	// sinceUnix (below) is left as the zero func on any error, leaving
	// every port's Since at its zero value rather than returning an error.
	sinceUnix := func(int64) int64 { return 0 }
	if result, err := s.Get([]string{oidSysUpTime}); err == nil && len(result.Variables) == 1 {
		v := result.Variables[0]
		if v.Type != gosnmp.NoSuchObject && v.Type != gosnmp.NoSuchInstance {
			uptimeTicks := gosnmp.ToBigInt(v.Value).Int64()
			sinceUnix = func(lastChangeTicks int64) int64 {
				elapsed := time.Duration(uptimeTicks-lastChangeTicks) * 10 * time.Millisecond // TimeTicks are hundredths of a second
				return time.Now().Add(-elapsed).Unix()
			}
		}
	}

	ports := make([]model.Port, 0, len(descr))
	for idx, name := range descr {
		label := alias[idx]
		if label == "" {
			label = name
		}
		sp := highSpeed[idx] // ifXTable's ifHighSpeed is already in Mbps
		if sp == 0 {
			sp = speed[idx] / 1_000_000 // ifTable's ifSpeed is in bps
		}
		ports = append(ports, model.Port{
			Port:       idx,
			State:      status[idx],
			AdminState: adminStatus[idx],
			Speed:      sp,
			Label:      label,
			IfType:     ifType[idx],
			Since:      sinceUnix(lastChange[idx]),
		})
	}
	return ports, nil
}
