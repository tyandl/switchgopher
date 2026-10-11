package generic

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

const (
	oidCdpCacheDeviceId   = ".1.3.6.1.4.1.9.9.23.1.2.1.1.6"
	oidCdpCacheDevicePort = ".1.3.6.1.4.1.9.9.23.1.2.1.1.7"
	oidCdpCachePlatform   = ".1.3.6.1.4.1.9.9.23.1.2.1.1.8"
)

func init() {
	snmp.RegisterCDP(snmp.VendorGeneric, cdpFetcher{})
}

type cdpFetcher struct{}

// FetchCDP is best-effort: it backfills CDP neighbor info onto nodes already
// found by a NodesFetcher, matched by ifIndex. An empty/failed walk here is
// expected on most gear, not an error - CISCO-CDP-MIB support is far from
// universal, and the local net-snmp simulator used for testing doesn't
// implement it at all. Stays in the generic tier (not a Cisco override)
// precisely because it already degrades gracefully on non-Cisco gear with
// no vendor branching needed.
func (cdpFetcher) FetchCDP(s *gosnmp.GoSNMP, nodes []model.Node) {
	deviceID := cdpColumn(s, oidCdpCacheDeviceId)
	devicePort := cdpColumn(s, oidCdpCacheDevicePort)
	platform := cdpColumn(s, oidCdpCachePlatform)

	for i := range nodes {
		nodes[i].CdpDeviceId = deviceID[nodes[i].Port]
		nodes[i].CdpDevicePort = devicePort[nodes[i].Port]
		nodes[i].CdpPlatform = platform[nodes[i].Port]
	}
}

// cdpColumn walks one cdpCacheTable column and returns it keyed by ifIndex -
// cdpCacheEntry's INDEX is { cdpCacheIfIndex, cdpCacheDeviceIndex }, so the
// ifIndex is always the first sub-identifier after the column prefix.
func cdpColumn(s *gosnmp.GoSNMP, columnOid string) map[string]string {
	m := map[string]string{}
	_ = s.BulkWalk(columnOid, func(pdu gosnmp.SnmpPDU) error {
		suffix := strings.TrimPrefix(pdu.Name, columnOid+".")
		ifIndex, _, found := strings.Cut(suffix, ".")
		if !found {
			ifIndex = suffix
		}
		m[ifIndex] = snmputil.PduString(pdu)
		return nil
	})
	return m
}
