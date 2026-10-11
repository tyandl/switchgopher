package snmp

import (
	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/model"
)

// Each interface below is one coherent SNMP fetch operation - grouped by
// data source (the table/scalar cluster a vendor would plausibly override
// as a unit), not by individual Go struct field. State/Speed/Label, for
// example, always come from the same ifTable/ifXTable walk in every
// vendor, so splitting them into separate interfaces would only multiply
// SNMP round-trips with no vendor ever needing to override just one of
// them independently.
//
// SystemInfoFetcher deliberately does not exist as an interface here: it
// produces SysObjectID, which vendor-key derivation (DeriveVendorKey)
// itself depends on, so it has to run once, unconditionally, before any
// group can be resolved. It stays the concrete FetchSystemInfo function.

// --- implemented and wired into FetchAll this pass ---

// IdentityResolver fills in a device's ChassisModel/ProductName.
type IdentityResolver interface {
	ResolveIdentity(snmp *gosnmp.GoSNMP, dev *model.Device) error
}

// PortsFetcher returns one Port per interface (State/Speed/Label), keyed
// by ifIndex via model.Port.Port.
type PortsFetcher interface {
	FetchPorts(snmp *gosnmp.GoSNMP) ([]model.Port, error)
}

// NodesFetcher walks a device's bridge forwarding database to learn which
// MAC addresses were last seen on which port.
type NodesFetcher interface {
	FetchNodes(snmp *gosnmp.GoSNMP) ([]model.Node, error)
}

// CDPFetcher backfills CDP neighbor info onto nodes already found by a
// NodesFetcher, matched by ifIndex. Mutates nodes in place, matching the
// existing best-effort/silent-fail behavior.
type CDPFetcher interface {
	FetchCDP(snmp *gosnmp.GoSNMP, nodes []model.Node)
}

// --- declared now, implemented in a later phase (standard/generic MIBs only) ---

// EntityModulesFetcher returns one Module per line-card slot in a modular
// chassis (Entity MIB entPhysicalTable filtered to class == module(9),
// the sibling walk to IdentityResolver's chassis(3) filter).
type EntityModulesFetcher interface {
	FetchEntityModules(snmp *gosnmp.GoSNMP) ([]model.Module, error)
}

// PortCounters holds one interface's cumulative I/O counters.
type PortCounters struct {
	InPackets, OutPackets   int64
	InErrors, OutErrors     int64
	InDiscards, OutDiscards int64
}

// PortCountersFetcher returns PortCounters keyed by ifIndex.
type PortCountersFetcher interface {
	FetchPortCounters(snmp *gosnmp.GoSNMP) (map[string]PortCounters, error)
}

// PortAggregationFetcher returns, for each aggregated member port, the
// ifIndex of the logical aggregate it belongs to - keyed by the member's
// own ifIndex.
type PortAggregationFetcher interface {
	FetchPortAggregation(snmp *gosnmp.GoSNMP) (map[string]string, error)
}

// SpanningTreeFetcher returns a device's spanning-tree topology-change count.
type SpanningTreeFetcher interface {
	FetchStpTopChanges(snmp *gosnmp.GoSNMP) (int64, error)
}

// IsLayer3Resolver determines whether a device is actively routing between
// VLANs (see model.Device.IsLayer3's doc comment for the proposed rule).
type IsLayer3Resolver interface {
	ResolveIsLayer3(snmp *gosnmp.GoSNMP) (bool, error)
}

// PortPoE holds one interface's Power-over-Ethernet state.
type PortPoE struct {
	Active bool
	Class  string
}

// PortPoEFetcher returns PortPoE keyed by ifIndex.
type PortPoEFetcher interface {
	FetchPortPoE(snmp *gosnmp.GoSNMP) (map[string]PortPoE, error)
}

// VlanNamesFetcher returns a device-local view of configured VLAN IDs and
// names. Unlike the other groups, its output doesn't merge into a
// model.Device/model.Port - a caller feeds it to
// store.UpsertDiscoveredVlan for any ID with no configuration/vlans.json
// entry.
type VlanNamesFetcher interface {
	FetchVlanNames(snmp *gosnmp.GoSNMP) (map[int]string, error)
}

// TemperatureFetcher returns a device's chassis temperature in Celsius.
type TemperatureFetcher interface {
	FetchTemperature(snmp *gosnmp.GoSNMP) (celsius float64, err error)
}

// --- declared now, implemented in a later phase (Cisco vendor tier) ---

// PortDuplexFetcher returns each interface's duplex mode, keyed by ifIndex.
type PortDuplexFetcher interface {
	FetchPortDuplex(snmp *gosnmp.GoSNMP) (map[string]int, error)
}

// PortVlanMembership holds one interface's access and auxiliary (voice)
// VLAN assignment.
type PortVlanMembership struct {
	Vlan, AuxVlan string
	IsTrunk       bool
}

// PortVlanMembershipFetcher returns PortVlanMembership keyed by ifIndex.
type PortVlanMembershipFetcher interface {
	FetchPortVlanMembership(snmp *gosnmp.GoSNMP) (map[string]PortVlanMembership, error)
}

// CpuMemoryFetcher returns a device's CPU utilization and memory usage.
type CpuMemoryFetcher interface {
	FetchCpuMemory(snmp *gosnmp.GoSNMP) (cpu, memory int, err error)
}

// ConfigManagementFetcher returns the Unix timestamp of a device's last
// running-config change.
type ConfigManagementFetcher interface {
	FetchConfigUpdated(snmp *gosnmp.GoSNMP) (unixTime int64, err error)
}
