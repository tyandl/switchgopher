// Package cisco holds Cisco-specific fetch-group implementations -
// registered under snmp.Vendor("cisco"), tried before the generic tier
// for any device DeriveVendorKey resolves to that vendor (see
// internal/snmp/vendorkey.go).
package cisco

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// All three OIDs below are copied verbatim from the actual switchmap
// Perl source this project rewrites (Constants.pm) and re-verified
// against the real vendored MIB text this session - see
// documentation/TODO.md for the derivation chain and why the
// once-assumed vlanTrunkPortVlansEnabled bitmap isn't the right source
// for a port's singular VLAN.
const (
	// vmVlan (CISCO-VLAN-MEMBERSHIP-MIB): the access-port VLAN. "Trunk
	// ports won't appear in this table" per the MIB's own description.
	// Indexed directly by ifIndex.
	oidVmVlan = ".1.3.6.1.4.1.9.9.68.1.2.2.1.2."

	// vlanTrunkPortNativeVlan (CISCO-VTP-MIB): a trunk port's
	// untagged/native VLAN. Applied unconditionally as an override
	// whenever it's anything other than the default (1) - this is how a
	// trunk port's effective "Vlan" ends up set, matching switchmap's
	// own GetPortNameToVlanTable logic exactly. Indexed directly by
	// ifIndex (vlanTrunkPortIfIndex).
	oidVlanTrunkPortNativeVlan = ".1.3.6.1.4.1.9.9.46.1.6.1.1.5."

	// vmVoiceVlanId (CISCO-VLAN-MEMBERSHIP-MIB): the auxiliary/voice
	// VLAN. Sentinel 4096 means "not configured/not supported" per the
	// MIB's own description - skipped, not stored as a real value.
	// Indexed directly by ifIndex. switchmap also falls back to
	// c2900PortVoiceVlanId (CISCO-C2900-MIB) for pre-IOS 2900/3524-class
	// switches - skipped here since that MIB isn't even present in the
	// LibreNMS mirror this project sources vendored MIBs from, i.e.
	// genuinely obsolete, not just old.
	oidVmVoiceVlanId = ".1.3.6.1.4.1.9.9.68.1.5.1.1.1."

	// vlanTrunkPortDynamicStatus (CISCO-VTP-MIB): whether the port is
	// currently trunking - per its own MIB description, this is already
	// "a result of vlanTrunkPortDynamicState and the ifOperStatus of the
	// trunk port itself," so no second object needs fetching/merging by
	// hand. vlanTrunkPortTable's own scope is "the local system's VLAN
	// trunk ports" - a plain access port has no row here at all, the
	// mirror image of vmVlan excluding trunk ports. So absence-from-table
	// and an explicit notTrunking(2) both correctly mean "not currently
	// trunking," making a single bool defaulting false fully honest
	// either way. Indexed directly by ifIndex (vlanTrunkPortIfIndex).
	oidVlanTrunkPortDynamicStatus = ".1.3.6.1.4.1.9.9.46.1.6.1.1.14."
)

const (
	nativeVlanDefault     = 1
	voiceVlanUnconfigured = 4096
	trunkStatusTrunking   = 1
)

func init() {
	snmp.RegisterPortVlanMembership(snmp.Vendor("cisco"), portVlanMembershipFetcher{})
}

type portVlanMembershipFetcher struct{}

// FetchPortVlanMembership returns each interface's access and auxiliary
// (voice) VLAN, keyed by ifIndex.
func (portVlanMembershipFetcher) FetchPortVlanMembership(s *gosnmp.GoSNMP) (map[string]snmp.PortVlanMembership, error) {
	membership := map[string]snmp.PortVlanMembership{}

	get := func(idx string) snmp.PortVlanMembership { return membership[idx] }

	if err := snmputil.BulkWalkColumn(s, oidVmVlan, func(pdu gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(pdu.Name, oidVmVlan)
		m := get(idx)
		m.Vlan = snmputil.PduString(pdu)
		membership[idx] = m
		return nil
	}); err != nil {
		return nil, err
	}

	if err := snmputil.BulkWalkColumn(s, oidVlanTrunkPortNativeVlan, func(pdu gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(pdu.Name, oidVlanTrunkPortNativeVlan)
		native := gosnmp.ToBigInt(pdu.Value).Int64()
		if !nativeVlanOverrides(native) {
			return nil
		}
		m := get(idx)
		m.Vlan = snmputil.PduString(pdu)
		membership[idx] = m
		return nil
	}); err != nil {
		return nil, err
	}

	if err := snmputil.BulkWalkColumn(s, oidVmVoiceVlanId, func(pdu gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(pdu.Name, oidVmVoiceVlanId)
		voice := gosnmp.ToBigInt(pdu.Value).Int64()
		if !voiceVlanConfigured(voice) {
			return nil
		}
		m := get(idx)
		m.AuxVlan = snmputil.PduString(pdu)
		membership[idx] = m
		return nil
	}); err != nil {
		return nil, err
	}

	if err := snmputil.BulkWalkColumn(s, oidVlanTrunkPortDynamicStatus, func(pdu gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(pdu.Name, oidVlanTrunkPortDynamicStatus)
		m := get(idx)
		m.IsTrunk = gosnmp.ToBigInt(pdu.Value).Int64() == trunkStatusTrunking
		membership[idx] = m
		return nil
	}); err != nil {
		return nil, err
	}

	return membership, nil
}

// nativeVlanOverrides reports whether a trunk port's native VLAN should
// override its access-VLAN value - true for anything other than the
// default (1). Matches switchmap's own GetPortNameToVlanTable logic
// exactly: "if the native vlan is something other than 1, override the
// VLAN number that's already been set."
func nativeVlanOverrides(native int64) bool {
	return native != nativeVlanDefault
}

// voiceVlanConfigured reports whether a vmVoiceVlanId reading is a real
// configured value rather than the "not configured/not supported"
// sentinel (4096) documented in the MIB itself.
func voiceVlanConfigured(voice int64) bool {
	return voice != voiceVlanUnconfigured
}
