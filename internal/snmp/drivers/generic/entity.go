package generic

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

const (
	oidEntPhysicalTable       = ".1.3.6.1.2.1.47.1.1.1.1"
	oidEntPhysicalDescr       = ".1.3.6.1.2.1.47.1.1.1.1.2."
	oidEntPhysicalClass       = ".1.3.6.1.2.1.47.1.1.1.1.5."
	oidEntPhysicalHardwareRev = ".1.3.6.1.2.1.47.1.1.1.1.7."
	oidEntPhysicalFirmwareRev = ".1.3.6.1.2.1.47.1.1.1.1.8."
	oidEntPhysicalSoftwareRev = ".1.3.6.1.2.1.47.1.1.1.1.9."
	oidEntPhysicalSerialNum   = ".1.3.6.1.2.1.47.1.1.1.1.10."
	oidEntPhysicalModelName   = ".1.3.6.1.2.1.47.1.1.1.1.13."

	entPhysicalClassChassis = 3
	entPhysicalClassModule  = 9
)

// entPhysicalRow is one row of the Entity MIB's physical-entity table -
// shared by identity.go's chassis lookup and entitymodules.go's
// module-row collection, both of which walk the same table but filter to
// a different entPhysicalClass.
type entPhysicalRow struct {
	Descr       string
	Class       int64
	ModelName   string
	HardwareRev string
	FirmwareRev string
	SoftwareRev string
	SerialNum   string
}

// walkEntPhysicalTable walks the whole table in one BulkWalk, keyed by
// entPhysicalIndex (the OID suffix after the table prefix). Not every
// device implements the Entity MIB - a failed walk returns an empty map,
// not an error, since callers already treat "found nothing usable" as a
// normal, non-fatal outcome.
func walkEntPhysicalTable(s *gosnmp.GoSNMP) map[string]entPhysicalRow {
	rows := map[string]entPhysicalRow{}

	get := func(idx string) entPhysicalRow {
		r := rows[idx]
		return r
	}

	_ = s.BulkWalk(oidEntPhysicalTable, func(pdu gosnmp.SnmpPDU) error {
		switch {
		case strings.HasPrefix(pdu.Name, oidEntPhysicalDescr):
			idx := strings.TrimPrefix(pdu.Name, oidEntPhysicalDescr)
			r := get(idx)
			r.Descr = snmputil.PduString(pdu)
			rows[idx] = r
		case strings.HasPrefix(pdu.Name, oidEntPhysicalClass):
			idx := strings.TrimPrefix(pdu.Name, oidEntPhysicalClass)
			r := get(idx)
			r.Class = gosnmp.ToBigInt(pdu.Value).Int64()
			rows[idx] = r
		case strings.HasPrefix(pdu.Name, oidEntPhysicalHardwareRev):
			idx := strings.TrimPrefix(pdu.Name, oidEntPhysicalHardwareRev)
			r := get(idx)
			r.HardwareRev = snmputil.PduString(pdu)
			rows[idx] = r
		case strings.HasPrefix(pdu.Name, oidEntPhysicalFirmwareRev):
			idx := strings.TrimPrefix(pdu.Name, oidEntPhysicalFirmwareRev)
			r := get(idx)
			r.FirmwareRev = snmputil.PduString(pdu)
			rows[idx] = r
		case strings.HasPrefix(pdu.Name, oidEntPhysicalSoftwareRev):
			idx := strings.TrimPrefix(pdu.Name, oidEntPhysicalSoftwareRev)
			r := get(idx)
			r.SoftwareRev = snmputil.PduString(pdu)
			rows[idx] = r
		case strings.HasPrefix(pdu.Name, oidEntPhysicalSerialNum):
			idx := strings.TrimPrefix(pdu.Name, oidEntPhysicalSerialNum)
			r := get(idx)
			r.SerialNum = snmputil.PduString(pdu)
			rows[idx] = r
		case strings.HasPrefix(pdu.Name, oidEntPhysicalModelName):
			idx := strings.TrimPrefix(pdu.Name, oidEntPhysicalModelName)
			r := get(idx)
			r.ModelName = snmputil.PduString(pdu)
			rows[idx] = r
		}
		return nil
	})
	return rows
}
