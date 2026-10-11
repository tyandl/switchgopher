// Package model holds the data types shared between the SNMP-fetch layer
// (internal/snmp) and the persistence layer (internal/store). It has no
// dependencies of its own, which is what lets those two packages depend on
// it without depending on each other.
package model

import "time"

type Device struct {
	Device          string // device's hostname
	Address         string // device's IP
	CommunityString string // the snmp community string

	ChassisModel       string // Entity MIB entPhysicalModelName, or sysObjectID resolved via gosmi
	ProductDescription string // .1.3.6.1.2.1.1.1.0 (sysDescr)
	ProductName        string // same source as ChassisModel

	SysName     string // .1.3.6.1.2.1.1.5.0
	SysObjectID string // .1.3.6.1.2.1.1.2.0
	SysContact  string // .1.3.6.1.2.1.1.4.0
	SysLocation string // .1.3.6.1.2.1.1.6.0
	SysUptime   string // .1.3.6.1.2.1.1.3.0

	// Not populated this pass - kept deliberately vendor/OID-agnostic since
	// how each gets populated is still an open question:
	//  - SerialNum/HardwareVersion/FirmwareVersion: likely Entity MIB
	//    entPhysicalSerialNum/HardwareRev/FirmwareRev, the same
	//    entPhysicalTable already walked for ChassisModel.
	//  - Temp: likely ENTITY-SENSOR-MIB (not yet vendored).
	//  - Cpu/Memory: no cross-vendor standard MIB exists; would need a
	//    vendor-specific one (e.g. Cisco's CISCO-PROCESS-MIB/
	//    CISCO-MEMORY-POOL-MIB).
	//  - ConfigUpdated: likely a vendor-specific config-management MIB
	//    (e.g. Cisco's CISCO-CONFIG-MAN-MIB ccmHistoryRunningLastChanged).
	//  - StpTopChanges: BRIDGE-MIB's dot1dStpTopChanges (already vendored).
	SerialNum       string
	HardwareVersion string
	FirmwareVersion string
	Temp            float64
	Cpu             int
	Memory          int
	ConfigUpdated   int64
	StpTopChanges   int64

	// IsLayer3 - not populated this pass. Distinguishes "is this device
	// actively routing between VLANs" from "is this hardware capable of
	// routing at all" - SNMP can only answer the former; the latter would
	// need a hand-maintained list of known L3-capable models keyed off
	// ChassisModel/SysObjectID, which isn't worth building unless the
	// live signal below turns out to be insufficient.
	//
	// Proposed rule: ipForwarding (RFC1213-MIB, already vendored,
	// .1.3.6.1.2.1.4.1.0) == forwarding(1), AND more than one VLAN
	// interface (IF-MIB ifType == l2vlan(135), IANAifType-MIB, already
	// vendored) has an address bound in ipAddrTable. Both conditions
	// matter: ipForwarding alone isn't enough, since even a plain
	// L2-managed switch commonly has one IP-bound VLAN interface (a
	// management address on Vlan1) while still reporting
	// not-forwarding(2) - so that case is already excluded by
	// ipForwarding. The real gotcha is the reverse: a device could report
	// forwarding(1) with only that same single management interface and
	// no genuine inter-VLAN routing happening, hence requiring more than
	// one IP-bound VLAN interface as corroborating evidence.
	// sysServices (.1.3.6.1.2.1.1.7.0, standard system group, layer-3 bit
	// = 4) is a weaker secondary signal - known in practice to be
	// inconsistently maintained by vendors, not worth relying on alone.
	IsLayer3 bool
}

type Port struct {
	Port       string // ifIndex - the only universally-present integer identifier
	Device     string // Device Port is on
	State      int    // ifOperStatus
	AdminState int    // ifAdminStatus (IF-MIB, standard) - up(1)/down(2)/testing(3)
	Speed      int    // Mbps
	Duplex     int    // EtherLike-MIB dot3StatsDuplexStatus (standard) - unknown(1)/half(2)/full(3), 0 if no data at all
	Vlan       string // access vlan - Cisco vmVlan, overridden by vlanTrunkPortNativeVlan for trunk ports (see internal/snmp/drivers/cisco/vlanmembership.go)
	AuxVlan    string // auxiliary (voice) vlan - Cisco vmVoiceVlanId
	IsTrunk    bool   // Cisco vlanTrunkPortDynamicStatus - false covers both "genuinely access" and "not a Cisco device/not yet polled," see vlanmembership.go's doc comment for why that's still honest
	Label      string // ifAlias, falling back to ifDescr
	IfType     int    // IF-MIB ifType (standard, IANAifType-MIB values) - lets a caller distinguish a real physical/logical interface from a pseudo-interface (e.g. ethernetCsmacd(6) vs. other(1)/l2vlan(135)/ieee8023adLag(161)); see internal/web/handlers_stats.go's isAncillaryPort/isVirtualPort
	Since      int64  // Unix timestamp - IF-MIB ifLastChange (standard, ifEntry.9), the last time this port's operational state changed. TimeTicks relative to the device's own uptime clock, converted using the device's current sysUpTime at fetch time (same approach as Device.ConfigUpdated). 0 if unavailable.

	// PoE - not populated this pass. PoeActive would come from
	// pethPsePortDetectionStatus == deliveringPower(3) (POWER-ETHERNET-MIB,
	// standard); PoeClass from pethPsePortPowerClassifications (also
	// standard, but only valid while a PD is being powered, and caps out at
	// class4 - doesn't cover 802.3bt's higher classes).
	PoeActive bool
	PoeClass  string

	// AggregatePort - not populated this pass. Set on a member port to the
	// ifIndex (i.e. another row's Port value, same Device) of the logical
	// aggregate it belongs to - blank on the aggregate's own row and on
	// ports that aren't aggregated. Would come from IF-MIB's ifStackTable
	// (already vendored): for a member port's ifIndex as
	// ifStackLowerLayer, the paired non-zero ifStackHigherLayer is its
	// aggregate - the same standard, protocol-agnostic mechanism
	// switchmap's own PopulateEtherChannels.pm uses.
	AggregatePort string

	// I/O counters - not populated this pass. Cumulative totals since the
	// interface's last reset/reinitialization (see ifCounterDiscontinuityTime),
	// not per-interval rates - a caller wanting "errors in the last hour"
	// needs to diff two polls itself. InPackets/OutPackets from
	// ifHCInUcastPkts/ifHCOutUcastPkts (Counter64, IF-MIB ifXTable);
	// In/OutErrors and In/OutDiscards from ifInErrors/ifOutErrors/
	// ifInDiscards/ifOutDiscards (Counter32, IF-MIB ifTable). All standard,
	// already vendored, and already walked by FetchPorts for State/Speed/
	// Description - adding these OIDs to that same walk is the natural next
	// step whenever polling for this lands.
	InPackets   int64
	OutPackets  int64
	InErrors    int64
	OutErrors   int64
	InDiscards  int64
	OutDiscards int64
}

type Node struct {
	Mac       string    // mac address in 00:00:00:00:00:00 form
	Ip        string    // ipv4 address - not populated this pass (would need an ARP-table walk)
	Hostname  string    // not populated this pass (would need reverse DNS)
	TimeStamp time.Time // Time this entry was made.
	Port      string    // the ifIndex this node was attached to

	// Best-effort CDP info, backfilled by snmp.FetchCDP - most devices (and
	// the stock net-snmp simulator used for local testing) don't implement
	// CISCO-CDP-MIB, so these are commonly empty.
	CdpDeviceId   string // .1.3.6.1.4.1.9.9.23.1.2.1.1.6
	CdpDevicePort string // .1.3.6.1.4.1.9.9.23.1.2.1.1.7
	CdpPlatform   string // .1.3.6.1.4.1.9.9.23.1.2.1.1.8
}

// Module is one line card/daughter-card slot in a modular chassis switch
// (e.g. a Catalyst 6506/6509/4507, several of which are in the real seeded
// device list) - not populated this pass, and never has been; this struct
// and its DB table have existed unused since the original 2014 code with no
// comment explaining why, so the intent below is reconstructed from
// circumstantial evidence rather than a note left by the original author:
//   - the modules table's Module column has always been INTEGER, not TEXT,
//     despite the Go field's original "Module Name" comment - a composite
//     primary key of (Module INTEGER, Device) only makes sense if Module is
//     a numeric slot/index, matching Cisco's own "show module" numbering.
//   - switchmap's own Stats.pm (the tool this project rewrites) has this
//     exact comment: "if it has modules (i.e. 6509s have modules, 3524s
//     don't)" - the same device family already in this project's seed data.
//
// The schema already fits Entity MIB's entPhysicalTable (same table walked
// for chassis info) filtered to entPhysicalClass == module(9) ("e.g.,
// plug-in card or daughter-card") instead of chassis(3):
//   - Module      <- entPhysicalIndex
//   - Description <- entPhysicalDescr
//   - Model       <- entPhysicalModelName
//   - FwVersion / HwVersion / SwVersion <- entPhysicalFirmwareRev /
//     entPhysicalHardwareRev / entPhysicalSoftwareRev
//   - SerialNum   <- entPhysicalSerialNum
type Module struct {
	Module      string // entPhysicalIndex of this module within the chassis
	Device      string // Device Name (matches device ID)
	Description string
	Model       string
	FwVersion   string
	HwVersion   string
	SwVersion   string
	SerialNum   string
}

// Vlan is initialized from a config file.
type Vlan struct {
	Vlan int
	Name string
}
