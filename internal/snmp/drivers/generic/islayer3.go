package generic

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/snmp/snmputil"
)

// ipForwarding (RFC1213-MIB, ip.1 = .1.3.6.1.2.1.4.1.0, forwarding(1)/not-forwarding(2)).
const oidIpForwarding = ".1.3.6.1.2.1.4.1.0"

// ipAdEntIfIndex (RFC1213-MIB, ipAddrEntry.2 = .1.3.6.1.2.1.4.20.1.2.) -
// the interface each IP address in ipAddrTable is bound to.
const oidIpAdEntIfIndex = ".1.3.6.1.2.1.4.20.1.2."

const ipForwardingForwarding = 1

func init() {
	snmp.RegisterIsLayer3(snmp.VendorGeneric, isLayer3Resolver{})
}

type isLayer3Resolver struct{}

// ResolveIsLayer3 implements the rule documented in model.Device.IsLayer3's
// doc comment: ipForwarding == forwarding(1) AND more than one VLAN
// interface (ifType == l2vlan(135)) has an address bound in ipAddrTable.
// Both conditions matter - ipForwarding alone doesn't rule out a plain
// L2-managed switch's single management IP on Vlan1 (which also reports
// not-forwarding, so that case is already excluded); the real gotcha is
// the reverse, a device reporting forwarding(1) with only that same
// single management interface and no genuine inter-VLAN routing, hence
// requiring more than one IP-bound VLAN interface as corroborating
// evidence.
func (isLayer3Resolver) ResolveIsLayer3(s *gosnmp.GoSNMP) (bool, error) {
	result, err := s.Get([]string{oidIpForwarding})
	if err != nil {
		return false, fmt.Errorf("resolveIsLayer3: %w", err)
	}
	if len(result.Variables) == 0 {
		return false, fmt.Errorf("resolveIsLayer3: no value returned")
	}
	v := result.Variables[0]
	if v.Type == gosnmp.NoSuchObject || v.Type == gosnmp.NoSuchInstance {
		return false, fmt.Errorf("resolveIsLayer3: ipForwarding: %w", snmp.ErrNotSupported)
	}
	if gosnmp.ToBigInt(v.Value).Int64() != ipForwardingForwarding {
		return false, nil
	}

	ifTypes := walkIfType(s)
	l2vlanInterfaces := map[string]bool{}
	for idx, t := range ifTypes {
		if t == ifTypeL2vlan {
			l2vlanInterfaces[idx] = true
		}
	}

	ipBoundL2vlanCount := 0
	_ = snmputil.BulkWalkColumn(s, oidIpAdEntIfIndex, func(pdu gosnmp.SnmpPDU) error {
		idx := fmt.Sprint(gosnmp.ToBigInt(pdu.Value).Int64())
		if l2vlanInterfaces[idx] {
			ipBoundL2vlanCount++
		}
		return nil
	})

	return ipBoundL2vlanCount > 1, nil
}
