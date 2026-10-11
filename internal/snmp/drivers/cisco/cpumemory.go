package cisco

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// No switchmap precedent exists for CPU/memory (it never tracked
// either) - these OIDs come from reading the actual vendored MIB text
// directly, not from a reference implementation.
const (
	// cpmCPUTotal5minRev (CISCO-PROCESS-MIB: ciscoMgmt.109 ->
	// ciscoProcessMIBObjects.1 (cpmCPU) -> cpmCPUTotalTable.1
	// (cpmCPUTotalEntry) -> entry.8), a Gauge32 percent (0..100).
	// Deliberately not cpmCPUTotal5min (entry.5) - that object is
	// STATUS deprecated in the vendored MIB text in favor of this one
	// (range fix: 1..100 -> 0..100). Indexed by cpmCPUTotalIndex, a
	// per-CPU identifier (not ifIndex) - a modular chassis can have more
	// than one row (one per supervisor/route processor); this takes the
	// first row found as the device's representative value rather than
	// building multi-CPU aggregation for a single int field.
	oidCpmCPUTotal5minRev = ".1.3.6.1.4.1.9.9.109.1.1.1.1.8."

	// ciscoMemoryPoolUtilization5Min (CISCO-MEMORY-POOL-MIB:
	// ciscoMgmt.48 -> ciscoMemoryPoolObjects.2
	// (ciscoMemoryPoolUtilizationTable) -> entry.2), a direct percent
	// reading - preferred over computing Used/(Used+Free) by hand from
	// ciscoMemoryPoolUsed/Free (entry.5/.6) since it's simpler and this
	// table AUGMENTS the base pool table (same index). Both tables are
	// indexed by ciscoMemoryPoolType; processor(1) - "the processor
	// memory pool" - is the one type the MIB says "is required to be
	// supported by all devices," so that's the row this reads.
	oidCiscoMemoryPoolUtilization5Min = ".1.3.6.1.4.1.9.9.48.1.2.1.2."
)

const memoryPoolTypeProcessor = "1"

func init() {
	snmp.RegisterCpuMemory(snmp.Vendor("cisco"), cpuMemoryFetcher{})
}

type cpuMemoryFetcher struct{}

// FetchCpuMemory returns a device's CPU utilization and memory usage,
// both as a 0-100 percentage. Takes the first CPU row found and the
// "processor" memory pool specifically - see the OID comments above for
// why.
func (cpuMemoryFetcher) FetchCpuMemory(s *gosnmp.GoSNMP) (cpu, memory int, err error) {
	cpuFound := false
	if walkErr := snmputil.BulkWalkColumn(s, oidCpmCPUTotal5minRev, func(pdu gosnmp.SnmpPDU) error {
		if cpuFound {
			return nil // already have a representative value
		}
		cpu = int(gosnmp.ToBigInt(pdu.Value).Int64())
		cpuFound = true
		return nil
	}); walkErr != nil {
		return 0, 0, walkErr
	}

	if walkErr := snmputil.BulkWalkColumn(s, oidCiscoMemoryPoolUtilization5Min, func(pdu gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(pdu.Name, oidCiscoMemoryPoolUtilization5Min)
		if idx != memoryPoolTypeProcessor {
			return nil
		}
		memory = int(gosnmp.ToBigInt(pdu.Value).Int64())
		return nil
	}); walkErr != nil {
		return 0, 0, walkErr
	}

	return cpu, memory, nil
}
