package snmputil

import (
	"fmt"
	"strconv"
	"strings"
)

// MacFromParts converts 6 dotted-decimal octet strings (as split out of an
// SNMP table index) into a "00:14:91:12:C9:07"-style MAC address.
func MacFromParts(parts []string) (string, bool) {
	if len(parts) != 6 {
		return "", false
	}
	octets := make([]string, 6)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return "", false
		}
		octets[i] = fmt.Sprintf("%02X", n)
	}
	return strings.Join(octets, ":"), true
}

// DecodeQBridgeIndex handles Q-BRIDGE-MIB's dot1qTpFdbEntry INDEX
// { dot1qFdbId, dot1qTpFdbAddress }: one leading FDB-id sub-identifier
// followed by 6 MAC-address octets.
func DecodeQBridgeIndex(suffix string) (string, bool) {
	parts := strings.Split(suffix, ".")
	if len(parts) != 7 {
		return "", false
	}
	return MacFromParts(parts[1:])
}

// DecodePlainBridgeIndex handles BRIDGE-MIB's dot1dTpFdbEntry INDEX
// { dot1dTpFdbAddress }: just the 6 MAC-address octets, no VLAN/FDB-id prefix.
func DecodePlainBridgeIndex(suffix string) (string, bool) {
	return MacFromParts(strings.Split(suffix, "."))
}

// MacFromBytes converts a 6-byte PhysAddress value (as returned directly
// in an SNMP PDU's value, e.g. RFC1213-MIB's ipNetToMediaPhysAddress) into
// the same "00:14:91:12:C9:07"-style MAC address MacFromParts produces -
// a different shape of input (raw value bytes, not decimal-string OID
// index parts), since not every MAC-bearing object encodes the MAC in its
// index the way FDB tables do.
func MacFromBytes(b []byte) (string, bool) {
	if len(b) != 6 {
		return "", false
	}
	octets := make([]string, 6)
	for i, o := range b {
		octets[i] = fmt.Sprintf("%02X", o)
	}
	return strings.Join(octets, ":"), true
}
