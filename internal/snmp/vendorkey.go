package snmp

import "strings"

// enterpriseVendors maps an SNMP enterprise-number OID prefix (the
// ".1.3.6.1.4.1.<n>" arc IANA assigns each organization) to the Vendor key
// used to select registry overrides. This is a different, more reliable
// signal than IdentityResolver's gosmi tree-walk: that answers "what's
// this device's friendly model name" (silently empty if no MIB happens to
// resolve it), not "which vendor bucket should pick an override" - this
// table answers the latter regardless of which MIBs are loaded.
//
// Only Cisco is seeded here because it's the only vendor with any research
// or vendored MIBs in this project so far. Adding a real vendor package
// (see internal/snmp/drivers/README.md) means adding its enterprise number
// here too - entries aren't added speculatively ahead of that.
var enterpriseVendors = map[string]Vendor{
	"1.3.6.1.4.1.9": "cisco",
}

// DeriveVendorKey picks a Vendor from a device's sysObjectID. Never fails -
// an unrecognized or malformed OID resolves to VendorGeneric, matching
// every other "not populated"/"unresolvable" fallback in this package.
func DeriveVendorKey(sysObjectID string) Vendor {
	trimmed := strings.TrimPrefix(sysObjectID, ".")
	for prefix, vendor := range enterpriseVendors {
		if trimmed == prefix || strings.HasPrefix(trimmed, prefix+".") {
			return vendor
		}
	}
	return VendorGeneric
}
