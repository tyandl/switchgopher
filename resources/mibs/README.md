# Vendored MIBs

Curated, minimal set needed to identify devices (sysObjectID / Entity MIB),
walk interfaces, and walk MAC forwarding tables. Vendored in-repo so builds
are reproducible and offline-capable — Ubuntu's `libsnmp-base` package
deliberately ships only NET-SNMP/UCD MIBs (licensing), and Cisco's MIBs
aren't Debian-packaged at all.

## Sources

- IETF/IANA standard MIBs (`SNMPv2-*`, `IF-MIB`, `IANAifType-MIB`,
  `ENTITY-MIB`, `BRIDGE-MIB`, `RFC1213-MIB`, `P-BRIDGE-MIB`, `Q-BRIDGE-MIB`,
  `POWER-ETHERNET-MIB`, `ENTITY-SENSOR-MIB`, `EtherLike-MIB`):
  pulled from LibreNMS's curated MIB collection —
  `https://github.com/librenms/librenms/tree/master/mibs`. `POWER-ETHERNET-MIB`
  and `ENTITY-SENSOR-MIB` `IMPORT SnmpAdminString FROM
  SNMP-FRAMEWORK-MIB`, which isn't vendored here - confirmed (empirically,
  by actually running `gosmi.LoadModule` against them) that gosmi resolves
  this leniently without needing the defining module physically present,
  so `SNMP-FRAMEWORK-MIB` wasn't added. `EtherLike-MIB` has no unresolved
  imports.
- Cisco MIBs (`cisco/CISCO-SMI`, `cisco/CISCO-TC`, `cisco/CISCO-PRODUCTS-MIB`,
  `cisco/CISCO-CDP-MIB`, `cisco/CISCO-VTP-MIB`,
  `cisco/CISCO-VLAN-MEMBERSHIP-MIB`, `cisco/CISCO-PROCESS-MIB`,
  `cisco/CISCO-MEMORY-POOL-MIB`, `cisco/CISCO-CONFIG-MAN-MIB`): same
  LibreNMS collection, under `resources/mibs/cisco/` — originally published
  by Cisco for exactly this kind of redistribution. `CISCO-VTP-MIB` and
  `CISCO-CONFIG-MAN-MIB` also import from unvendored `INET-ADDRESS-MIB`
  and `SNMP-FRAMEWORK-MIB`; `CISCO-VTP-MIB` additionally from
  unvendored `RMON-MIB`; `CISCO-PROCESS-MIB` from unvendored `HCNUM-TC`
  and `SNMP-FRAMEWORK-MIB`; `CISCO-MEMORY-POOL-MIB` from unvendored
  `CISCO-QOS-PIB-MIB` (for the `Percent` type) - all confirmed
  (empirically, same as above) to load fine without those present.
  `CISCO-C2900-MIB` (switchmap's own fallback source for
  `c2900PortVoiceVlanId` on pre-IOS 2900/3524-class switches) isn't in
  the LibreNMS mirror at all - genuinely obsolete, not just old - so
  that fallback wasn't implemented or vendored.
- `NET-SNMP-MIB`, `NET-SNMP-TC`: copied from this machine's
  `/usr/share/snmp/mibs/` (installed by the `libsnmp-base` package). Needed
  to resolve the sysObjectID reported by a local net-snmp `snmpd` test
  instance (`.1.3.6.1.4.1.8072.3.2.10`).

## Refreshing

Re-run the same `curl` fetches against
`https://raw.githubusercontent.com/librenms/librenms/master/mibs/<NAME>`
(no file extension) for the IETF set, and `.../mibs/cisco/<NAME>` for Cisco.

## Load order

gosmi resolves `IMPORTS` against its search path automatically, but every
module referenced by an `IMPORTS` clause must physically exist here. Load
order followed in `initMIBs()`: `SNMPv2-SMI` → `SNMPv2-TC` → `SNMPv2-CONF` →
`SNMPv2-MIB` → `IANAifType-MIB` → `IF-MIB` → `ENTITY-MIB` → `RFC1213-MIB` →
`BRIDGE-MIB` → `P-BRIDGE-MIB` → `Q-BRIDGE-MIB` → `CISCO-SMI` → `CISCO-TC` →
`CISCO-PRODUCTS-MIB` → `CISCO-CDP-MIB` → `NET-SNMP-TC` → `NET-SNMP-MIB` →
`POWER-ETHERNET-MIB` → `ENTITY-SENSOR-MIB` → `EtherLike-MIB` →
`CISCO-VTP-MIB` → `CISCO-VLAN-MEMBERSHIP-MIB` (depends on `CISCO-VTP-MIB`
for `VlanIndex`, loaded first) → `CISCO-PROCESS-MIB` →
`CISCO-MEMORY-POOL-MIB` → `CISCO-CONFIG-MAN-MIB`.
