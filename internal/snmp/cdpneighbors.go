package snmp

import (
	"net"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// CISCO-CDP-MIB (verified against the vendored MIB text directly, not
// assumed): cdpCacheEntry's INDEX is { cdpCacheIfIndex,
// cdpCacheDeviceIndex }, so every column here shares the same
// "{ifIndex}.{deviceIndex}" OID suffix per neighbor.
//
// cdpCacheAddress's own DESCRIPTION is explicit about why this is useful
// for discovery specifically: "If the neighbor device is SNMP-manageable,
// it is supposed to generate its CDP messages such that this address is
// one at which it will receive SNMP messages" - i.e. this is meant to be
// pollable, not just informational.
const (
	oidCdpCacheAddressType = ".1.3.6.1.4.1.9.9.23.1.2.1.1.3."
	oidCdpCacheAddress     = ".1.3.6.1.4.1.9.9.23.1.2.1.1.4."
	oidCdpCacheDeviceId    = ".1.3.6.1.4.1.9.9.23.1.2.1.1.6."
	oidCdpCachePlatform    = ".1.3.6.1.4.1.9.9.23.1.2.1.1.8."
)

// cdpCacheAddressTypeIP is CiscoNetworkProtocol's ip(1) value - the only
// address type this decodes. cdpCacheAddress's format depends on
// cdpCacheAddressType; a non-IP neighbor address (rare in practice) is
// skipped rather than guessed at.
const cdpCacheAddressTypeIP = 1

// CdpNeighbor is one entry from a device's CDP neighbor cache, with a
// usable IPv4 management address.
type CdpNeighbor struct {
	DeviceId string
	Platform string
	Address  string
}

// FetchCdpNeighbors walks a device's CDP neighbor cache and returns one
// CdpNeighbor per entry whose address is a decodable IPv4. Deliberately
// standalone, not a registry group (see groups.go) - like arp.go's
// FetchArpTable, this is data about *other* devices, not enrichment
// merged into the device being polled. Used only by the discovery
// tool's CDP-crawl mode; drivers/generic/cdp.go has its own, separate
// CDP fetch for regular polling's node-history display (DeviceId/
// DevicePort/Platform merged onto model.Node, collapsed to one entry per
// ifIndex) - this one keeps every distinct neighbor, since discovery
// cares about all of them, not "the one neighbor shown per port."
func FetchCdpNeighbors(s *gosnmp.GoSNMP) ([]CdpNeighbor, error) {
	addressTypes := map[string]int64{}
	addresses := map[string][]byte{}
	deviceIDs := map[string]string{}
	platforms := map[string]string{}

	collect := func(prefix string, set func(key string, pdu gosnmp.SnmpPDU)) func(gosnmp.SnmpPDU) error {
		return func(pdu gosnmp.SnmpPDU) error {
			set(strings.TrimPrefix(pdu.Name, prefix), pdu)
			return nil
		}
	}

	if err := snmputil.BulkWalkColumn(s, oidCdpCacheAddressType, collect(oidCdpCacheAddressType, func(key string, pdu gosnmp.SnmpPDU) {
		addressTypes[key] = gosnmp.ToBigInt(pdu.Value).Int64()
	})); err != nil {
		return nil, err
	}
	if err := snmputil.BulkWalkColumn(s, oidCdpCacheAddress, collect(oidCdpCacheAddress, func(key string, pdu gosnmp.SnmpPDU) {
		if b, ok := pdu.Value.([]byte); ok {
			addresses[key] = b
		}
	})); err != nil {
		return nil, err
	}
	if err := snmputil.BulkWalkColumn(s, oidCdpCacheDeviceId, collect(oidCdpCacheDeviceId, func(key string, pdu gosnmp.SnmpPDU) {
		deviceIDs[key] = snmputil.PduString(pdu)
	})); err != nil {
		return nil, err
	}
	if err := snmputil.BulkWalkColumn(s, oidCdpCachePlatform, collect(oidCdpCachePlatform, func(key string, pdu gosnmp.SnmpPDU) {
		platforms[key] = snmputil.PduString(pdu)
	})); err != nil {
		return nil, err
	}

	var neighbors []CdpNeighbor
	for key, addr := range addresses {
		if addressTypes[key] != cdpCacheAddressTypeIP || len(addr) != 4 {
			continue
		}
		neighbors = append(neighbors, CdpNeighbor{
			DeviceId: deviceIDs[key],
			Platform: platforms[key],
			Address:  net.IP(addr).String(),
		})
	}
	return neighbors, nil
}
