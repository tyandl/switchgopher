// Package generic holds the standard-MIB (vendor-agnostic) implementation
// of every fetch group - the fallback every device gets when no
// vendor-specific override is registered, and today the only tier that's
// actually wired into snmp.FetchAll. Each file's init() registers its
// implementation under snmp.VendorGeneric.
package generic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
	"github.com/sleepinggenius2/gosmi"
	"github.com/sleepinggenius2/gosmi/types"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/snmp"
)

func init() {
	snmp.RegisterIdentity(snmp.VendorGeneric, identityResolver{})
}

type identityResolver struct{}

// ResolveIdentity fills in ChassisModel/ProductName/SerialNum/
// HardwareVersion/FirmwareVersion, mirroring the cascade the original
// Perl switchmap used: prefer the Entity MIB's chassis entry when a
// device implements it (most accurate, live data - and the only source
// for SerialNum/HardwareVersion/FirmwareVersion), otherwise fall back to
// resolving sysObjectID against the vendored vendor MIBs via gosmi for
// ChassisModel/ProductName alone.
func (identityResolver) ResolveIdentity(s *gosnmp.GoSNMP, dev *model.Device) error {
	if row, ok := chassisRow(s); ok {
		name := row.ModelName
		if name == "" {
			name = row.Descr
		}
		dev.ChassisModel = name
		dev.ProductName = name
		dev.SerialNum = row.SerialNum
		dev.HardwareVersion = row.HardwareRev
		dev.FirmwareVersion = row.FirmwareRev
		return nil
	}

	oid, err := parseOid(dev.SysObjectID)
	if err != nil {
		return fmt.Errorf("resolveIdentity %s: %w", dev.Device, err)
	}
	node, err := gosmi.GetNodeByOID(oid)
	if err != nil {
		// Unresolvable vendor OID (no matching MIB vendored) is not fatal -
		// ChassisModel just stays empty for this device.
		return nil
	}
	dev.ChassisModel = node.Name
	dev.ProductName = node.Name
	return nil
}

// chassisRow finds the chassis-class row in the Entity MIB's
// physical-entity table. Not every device implements the Entity MIB (ok
// is false when the walk finds nothing usable), in which case the caller
// falls back to a sysObjectID lookup - which can't recover
// SerialNum/HardwareVersion/FirmwareVersion, only ChassisModel/ProductName.
func chassisRow(s *gosnmp.GoSNMP) (entPhysicalRow, bool) {
	for _, row := range walkEntPhysicalTable(s) {
		if row.Class != entPhysicalClassChassis {
			continue
		}
		if row.ModelName != "" || row.Descr != "" {
			return row, true
		}
	}
	return entPhysicalRow{}, false
}

// parseOid converts a dotted-decimal OID string (as returned by gosnmp, e.g.
// ".1.3.6.1.4.1.9.1.716") into the type gosmi's lookup functions expect.
func parseOid(s string) (types.Oid, error) {
	s = strings.TrimPrefix(s, ".")
	parts := strings.Split(s, ".")
	oid := make(types.Oid, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("parseOid %q: %w", s, err)
		}
		oid = append(oid, types.SmiSubId(n))
	}
	return oid, nil
}
