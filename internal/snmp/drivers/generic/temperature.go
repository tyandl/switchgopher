package generic

import (
	"fmt"
	"math"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// ENTITY-SENSOR-MIB (RFC 3433), verified against the vendored file:
// entitySensorMIB = mib-2.99, entitySensorObjects = .1,
// entPhySensorTable = .1.1, entPhySensorEntry = .1.1.1 - SPARSE-AUGMENTS
// entPhysicalIndex (same index as entPhysicalTable, already walked for
// chassis/module info). Type=entry.1, Scale=entry.2, Precision=entry.3,
// Value=entry.4.
const (
	oidEntPhySensorType      = ".1.3.6.1.2.1.99.1.1.1.1."
	oidEntPhySensorScale     = ".1.3.6.1.2.1.99.1.1.1.2."
	oidEntPhySensorPrecision = ".1.3.6.1.2.1.99.1.1.1.3."
	oidEntPhySensorValue     = ".1.3.6.1.2.1.99.1.1.1.4."
)

const entPhySensorTypeCelsius = 8

// entPhySensorScale's enum is 10^(n-9) for n=1..17 (yocto..yotta,
// units(9)=10^0) - confirmed against the vendored file's enum comments.
const entPhySensorScaleUnitsOffset = 9

func init() {
	snmp.RegisterTemperature(snmp.VendorGeneric, temperatureFetcher{})
}

type temperatureFetcher struct{}

type sensorRow struct {
	Type      int64
	Scale     int64
	Precision int64
	Value     int64
	hasValue  bool
}

// FetchTemperature returns a device's chassis temperature in Celsius,
// from the first celsius(8)-type row in entPhySensorTable. The
// value/scale/precision combination formula below (value *
// 10^(scale-9) / 10^precision) is the standard RFC 3433 interpretation -
// the MIB text itself only says the three fields "must be examined
// together," without spelling out the arithmetic, so this is applying
// general SNMP convention, not something verified word-for-word against
// the vendored file. Unverified end-to-end - the local snmpd test
// simulator doesn't implement this MIB.
func (temperatureFetcher) FetchTemperature(s *gosnmp.GoSNMP) (float64, error) {
	rows := map[string]sensorRow{}

	get := func(idx string) sensorRow { return rows[idx] }

	collect := func(prefix string, set func(*sensorRow, int64)) func(gosnmp.SnmpPDU) error {
		return func(pdu gosnmp.SnmpPDU) error {
			idx := strings.TrimPrefix(pdu.Name, prefix)
			r := get(idx)
			set(&r, gosnmp.ToBigInt(pdu.Value).Int64())
			rows[idx] = r
			return nil
		}
	}

	if err := snmputil.BulkWalkColumn(s, oidEntPhySensorType, collect(oidEntPhySensorType, func(r *sensorRow, v int64) { r.Type = v })); err != nil {
		return 0, fmt.Errorf("fetchTemperature: %w", err)
	}
	if err := snmputil.BulkWalkColumn(s, oidEntPhySensorScale, collect(oidEntPhySensorScale, func(r *sensorRow, v int64) { r.Scale = v })); err != nil {
		return 0, fmt.Errorf("fetchTemperature: %w", err)
	}
	if err := snmputil.BulkWalkColumn(s, oidEntPhySensorPrecision, collect(oidEntPhySensorPrecision, func(r *sensorRow, v int64) { r.Precision = v })); err != nil {
		return 0, fmt.Errorf("fetchTemperature: %w", err)
	}
	if err := snmputil.BulkWalkColumn(s, oidEntPhySensorValue, collect(oidEntPhySensorValue, func(r *sensorRow, v int64) { r.Value = v; r.hasValue = true })); err != nil {
		return 0, fmt.Errorf("fetchTemperature: %w", err)
	}

	for _, row := range rows {
		if row.Type != entPhySensorTypeCelsius || !row.hasValue {
			continue
		}
		celsius := float64(row.Value) *
			math.Pow(10, float64(row.Scale-entPhySensorScaleUnitsOffset)) /
			math.Pow(10, float64(row.Precision))
		return celsius, nil
	}
	return 0, fmt.Errorf("fetchTemperature: no celsius sensor found: %w", snmp.ErrNotSupported)
}
