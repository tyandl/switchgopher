# Vendor fetch packages

Each subpackage here implements one or more of `internal/snmp`'s fetch-group
interfaces (see `internal/snmp/groups.go`) and registers itself via
`init()`, the same self-registering pattern Go's `database/sql` drivers
use. `internal/snmp/drivers/register` blank-imports every package below;
`cmd/switchgopher` blank-imports `register` once at startup so every
registration happens regardless of which vendors are actually deployed.

## `generic/`

The standard-MIB (vendor-agnostic) tier - registers under
`snmp.VendorGeneric`, the fallback every group falls back to when no
vendor-specific override exists.

## `cisco/`

Cisco-specific overrides, registered under `snmp.Vendor("cisco")` -
`PortVlanMembership`, `CpuMemory`, `ConfigManagement`. Picked up
automatically for any device `DeriveVendorKey` resolves to that vendor
(sysObjectID enterprise arc `1.3.6.1.4.1.9`).

## Adding a new vendor

1. Create a subpackage here (e.g. `cisco/`, `juniper/`, `ubiquiti/`).
2. Implement one or more `groups.go` interfaces as a struct + method(s).
3. In each file's `init()`, call the matching `snmp.RegisterX(vendor,
   impl)` (e.g. `snmp.RegisterPortVlanMembership("juniper", vlanFetcher{})`).
4. Add a blank import for the new package in
   `internal/snmp/drivers/register/register.go`.
5. Add the vendor's SNMP enterprise-number OID prefix to
   `enterpriseVendors` in `internal/snmp/vendorkey.go`, so
   `DeriveVendorKey` actually picks this vendor's overrides for matching
   devices.
6. Vendor any new MIB files the implementation needs under
   `resources/mibs/<vendor>/`, add that directory to `InitMIBs`' search path
   with `gosmi.AppendPath`, and add the modules to its module list
   (`internal/snmp/device.go`). The top-level README's "MIB files" section has
   the full steps and where to find MIBs.

Juniper (`1.3.6.1.4.1.2636`) and Ubiquiti (`1.3.6.1.4.1.41112`) have no
package here yet and no vendored MIBs - no real OID research has been done
for either, so a package would just be an empty stub with nothing to
register. Don't add one speculatively; follow the steps above when there's
an actual implementation to write.
