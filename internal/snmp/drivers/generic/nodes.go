package generic

import (
	"fmt"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

const (
	// dot1qTpFdbEntry INDEX is { dot1qFdbId, dot1qTpFdbAddress } - VLAN-aware,
	// preferred when present.
	oidDot1qTpFdbPort = ".1.3.6.1.2.1.17.7.1.2.2.1.2"
	// dot1dTpFdbEntry INDEX is just { dot1dTpFdbAddress } - older/simpler
	// fallback for devices/agents that don't implement Q-BRIDGE-MIB.
	oidDot1dTpFdbPort = ".1.3.6.1.2.1.17.4.3.1.2"
	// maps a dot1dBasePort number (the FDB tables' port unit) to the ifIndex
	// used everywhere else (ports table, ifTable).
	oidDot1dBasePortIfIndex = ".1.3.6.1.2.1.17.1.4.1.2"
)

func init() {
	snmp.RegisterNodes(snmp.VendorGeneric, nodesFetcher{})
}

type nodesFetcher struct{}

// FetchNodes walks the bridge forwarding-database tables to learn which MAC
// addresses were last seen on which port, preferring the VLAN-aware
// Q-BRIDGE-MIB table and falling back to the plain BRIDGE-MIB table when
// that's empty.
func (nodesFetcher) FetchNodes(s *gosnmp.GoSNMP) ([]model.Node, error) {
	basePortToIfIndex := basePortIfIndexMap(s)
	now := time.Now()

	nodes := walkFdbTable(s, oidDot1qTpFdbPort, snmputil.DecodeQBridgeIndex, basePortToIfIndex, now)
	if len(nodes) == 0 {
		nodes = walkFdbTable(s, oidDot1dTpFdbPort, snmputil.DecodePlainBridgeIndex, basePortToIfIndex, now)
	}
	return nodes, nil
}

func basePortIfIndexMap(s *gosnmp.GoSNMP) map[string]string {
	m := map[string]string{}
	_ = s.BulkWalk(oidDot1dBasePortIfIndex, func(pdu gosnmp.SnmpPDU) error {
		basePort := strings.TrimPrefix(pdu.Name, oidDot1dBasePortIfIndex+".")
		m[basePort] = fmt.Sprint(gosnmp.ToBigInt(pdu.Value).Int64())
		return nil
	})
	return m
}

// decodeIndexFunc extracts a MAC address from the portion of an FDB entry's
// OID that follows the table's own column prefix.
type decodeIndexFunc func(indexSuffix string) (mac string, ok bool)

func walkFdbTable(s *gosnmp.GoSNMP, columnOid string, decode decodeIndexFunc, basePortToIfIndex map[string]string, seenAt time.Time) []model.Node {
	var nodes []model.Node
	_ = s.BulkWalk(columnOid, func(pdu gosnmp.SnmpPDU) error {
		suffix := strings.TrimPrefix(pdu.Name, columnOid+".")
		mac, ok := decode(suffix)
		if !ok {
			return nil
		}
		basePort := fmt.Sprint(gosnmp.ToBigInt(pdu.Value).Int64())
		ifIndex := basePort
		if mapped, ok := basePortToIfIndex[basePort]; ok {
			ifIndex = mapped
		}
		nodes = append(nodes, model.Node{Mac: mac, Port: ifIndex, TimeStamp: seenAt})
		return nil
	})
	return nodes
}
