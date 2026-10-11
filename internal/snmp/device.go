// Package snmp is the vendor-pluggable SNMP fetch layer: FetchAll bootstraps
// a device's system info, derives its vendor from sysObjectID
// (DeriveVendorKey), then resolves and calls each registered group's
// implementation (see groups.go/registry.go/accessors.go). Vendor-specific
// and generic/standard-MIB implementations live in subpackages under
// internal/snmp/drivers and register themselves via init() - see
// internal/snmp/drivers/README.md.
package snmp

import (
	"fmt"

	"github.com/gosnmp/gosnmp"
	"github.com/sleepinggenius2/gosmi"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

const (
	oidSysDescr    = ".1.3.6.1.2.1.1.1.0"
	oidSysObjectID = ".1.3.6.1.2.1.1.2.0"
	oidSysUptime   = ".1.3.6.1.2.1.1.3.0"
	oidSysContact  = ".1.3.6.1.2.1.1.4.0"
	oidSysName     = ".1.3.6.1.2.1.1.5.0"
	oidSysLocation = ".1.3.6.1.2.1.1.6.0"
)

// InitMIBs loads the vendored MIB set gosmi needs to resolve sysObjectID and
// Entity MIB values during device identification (see resources/mibs/README.md for the
// module list and where they came from). Failures here are fatal - a broken
// MIB set should block startup rather than silently degrade every device to
// "unknown" during polling.
func InitMIBs(mibDir string) error {
	gosmi.Init()
	gosmi.AppendPath(mibDir)
	gosmi.AppendPath(mibDir + "/cisco")

	modules := []string{
		"SNMPv2-SMI", "SNMPv2-TC", "SNMPv2-CONF", "SNMPv2-MIB",
		"IANAifType-MIB", "IF-MIB", "ENTITY-MIB", "RFC1213-MIB",
		"BRIDGE-MIB", "P-BRIDGE-MIB", "Q-BRIDGE-MIB",
		"CISCO-SMI", "CISCO-TC", "CISCO-PRODUCTS-MIB", "CISCO-CDP-MIB",
		"NET-SNMP-TC", "NET-SNMP-MIB",
		"POWER-ETHERNET-MIB", "ENTITY-SENSOR-MIB",
		"EtherLike-MIB",
		"CISCO-VTP-MIB", "CISCO-VLAN-MEMBERSHIP-MIB",
		"CISCO-PROCESS-MIB", "CISCO-MEMORY-POOL-MIB", "CISCO-CONFIG-MAN-MIB",
	}
	for _, m := range modules {
		if _, err := gosmi.LoadModule(m); err != nil {
			return fmt.Errorf("initMIBs: failed loading %s: %w", m, err)
		}
	}
	return nil
}

// FetchSystemInfo queries the standard system group. It's the one fetch
// operation that isn't a registry group (see groups.go) - it produces
// SysObjectID, which vendor-key derivation itself depends on, so it has to
// run once, unconditionally, before any group can be resolved.
// Identification fields (ChassisModel/ProductName) are left blank here -
// see IdentityResolver.
func FetchSystemInfo(snmp *gosnmp.GoSNMP) (model.Device, error) {
	result, err := snmp.Get([]string{
		oidSysDescr, oidSysObjectID, oidSysUptime, oidSysContact, oidSysName, oidSysLocation,
	})
	if err != nil {
		return model.Device{}, fmt.Errorf("fetchSystemInfo: %w", err)
	}

	var dev model.Device
	for _, pdu := range result.Variables {
		switch pdu.Name {
		case oidSysDescr:
			dev.ProductDescription = snmputil.PduString(pdu)
		case oidSysObjectID:
			dev.SysObjectID = snmputil.PduString(pdu)
		case oidSysUptime:
			dev.SysUptime = fmt.Sprint(gosnmp.ToBigInt(pdu.Value))
		case oidSysContact:
			dev.SysContact = snmputil.PduString(pdu)
		case oidSysName:
			dev.SysName = snmputil.PduString(pdu)
		case oidSysLocation:
			dev.SysLocation = snmputil.PduString(pdu)
		}
	}
	return dev, nil
}
