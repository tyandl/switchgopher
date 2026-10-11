# SwitchGopher TODO

Consolidated list of known gaps and deferred work, as of 2026-09-13. Two
sources: a feature comparison against the original Perl `switchmap` (the
tool this project is a Go rewrite of - the original source is not part of
this repo; https://github.com/dotysan/switchmap is a public mirror), and
gaps already flagged inline in the code/commit history.

This file is a design log, one section per feature or decision, newest work
near the end. For an overview of how the code fits together read
[ARCHITECTURE.md](ARCHITECTURE.md) first; for how to work on it, see
[CONTRIBUTING.md](../CONTRIBUTING.md).

## Note on architecture vs. switchmap

switchmap is a batch script (cron-run) that polls switches and writes static
HTML/CSV files to a directory served by any web server - no live app, no
database. SwitchGopher is a live Go web server backed by SQLite, querying
in real time on every request. That's a deliberate difference, not a gap -
but it means a few switchmap "features" below are really its own
report-generation modules with no equivalent in spirit, not just missing
code.

## Vendor-pluggable SNMP fetch registry (2026-09, Phases 1-3 done)

`internal/snmp` was rewritten from a fixed, linear call chain
(`FetchSystemInfo` → `IdentifyDevice` → `FetchPorts` → `FetchNodes` →
`FetchCDP`) into a registry of small per-source fetch interfaces, so a
vendor-specific implementation (Cisco, Juniper, Ubiquiti, ...) can override
one data group without touching orchestration code. Interfaces are grouped
by SNMP source, not by Go struct field, e.g. one `PortsFetcher` interface
covers State+Speed+Label together since all three come from the same
ifTable/ifXTable walk in every vendor - a literal per-field split would
multiply SNMP round-trips for no benefit, since vendors override at the
MIB-table level, not the individual-field level.

**Mechanics** (`internal/snmp/registry.go`/`groups.go`/`accessors.go`/
`vendorkey.go`/`fetch.go`): a single map keyed by `(GroupName, Vendor)`,
typed `RegisterX`/`resolveX` wrappers per group for compile-time safety,
`DeriveVendorKey` maps a device's `sysObjectID` enterprise-number prefix
(e.g. `1.3.6.1.4.1.9` → Cisco) to a vendor key - a different, more
reliable signal than the old `IdentifyDevice`'s gosmi tree-walk, which
answers "what's the friendly model name" (silently empty if unresolvable),
not "which vendor bucket to pick an override from." `resolve` tries the
device's vendor first, falls back to `"generic"`, and returns `ok=false`
if neither is registered - never fatal, matching every other "not
populated" field's graceful-degrade behavior. `FetchAll` is the new single
entry point `scheduler.pollDevice` calls, replacing the five separate
`snmp.*` calls it used to make.

Vendor-specific and generic (standard-MIB) implementations live in
subpackages under `internal/snmp/drivers/` (named `drivers/`, not
`vendor/` - Go reserves a literal `vendor/` directory for dependency
vendoring, which collides with this) and self-register via `init()`, the
same idiom `database/sql` drivers use - see
`internal/snmp/drivers/README.md` for the extension-point steps. All 16
group interfaces were declared in Phase 1 (`internal/snmp/groups.go`);
Phase 2 gave 8 more of them a real implementation, so 12 of the 16 are now
live - the remaining 4 (Phase 3, below) sit ready with zero registry-layer
work left to do once someone implements them.

**Phase 1 (done)**: `internal/snmp/drivers/generic/` implements Identity,
Ports, Nodes, and CDP - adapted from the old `IdentifyDevice`/
`entityChassisModel`, `FetchPorts`, `FetchNodes`, `FetchCDP` verbatim
(moved, not rewritten). Output confirmed identical to pre-refactor
behavior via a live regression poll against a local `snmpd` test instance.
Shared decoding helpers (`PduString`, `MacFromParts`,
`DecodeQBridgeIndex`, `DecodePlainBridgeIndex`) moved to a new
dependency-free `internal/snmp/snmputil` package so both core `internal/snmp`
and every vendor package can use them without a cycle. `resolve()` also
gained a log line for "nothing registered for this group" (neither
vendor-specific nor generic) - previously indistinguishable from "not
built yet" vs. a genuinely missing registration (e.g. an `init()` typo).

**Phase 2 (done)** - `EntityModules`, `PortCounters`, `PortAggregation`,
`SpanningTree`, `IsLayer3`, `VlanNames`, `PortPoE`, `Temperature`, all as
generic-tier (standard-MIB) implementations, plus a scope gap found and
fixed along the way: `model.Device.SerialNum`/`HardwareVersion`/
`FirmwareVersion` had no registry group at all in the original 16 -
they're the same `entPhysicalTable` chassis-row walk `IdentityResolver`
already does, so `ResolveIdentity` was extended to also set them rather
than adding a redundant 17th interface.

- `EntityModules`/`PortCounters`/`SpanningTree`/`IsLayer3`/`VlanNames` -
  standard MIBs already vendored, every OID re-verified against the
  actual file text (not memory) before implementing.
- `PortAggregation` - **caught and fixed a real correctness bug in the
  original design**: naively treating every active `ifStackTable` row as
  "aggregation" is wrong, since that table is used for other kinds of
  interface stacking too (e.g. a VLAN sub-interface over a trunk port).
  Fixed by filtering to pairs where the higher-layer interface's `ifType`
  is `ieee8023adLag(161)` - confirmed present in the vendored
  `IANAifType-MIB` - which is what actually identifies a real
  EtherChannel/LACP aggregate.
- `PortPoE` - needed vendoring `POWER-ETHERNET-MIB` (now in
  `resources/mibs/`). Real, documented limitation: `pethPsePortEntry` is
  indexed by `(pethPsePortGroupIndex, pethPsePortIndex)`, not `ifIndex` -
  confirmed in the actual RFC 3621 text, which defines no formal mapping
  between the two. Implemented as a best-effort heuristic (treat
  `pethPsePortIndex` as `ifIndex` only when `pethPsePortGroupIndex == 1`,
  the common non-modular case; skip any other group entirely rather than
  guess) - unverified against real PoE hardware, since the local `snmpd`
  test simulator doesn't implement this MIB at all.
- `Temperature` - needed vendoring `ENTITY-SENSOR-MIB` (now in
  `resources/mibs/`). The value/scale/precision combination formula is
  the standard RFC 3433 interpretation, not something spelled out
  verbatim in the MIB text itself (it only says the three fields "must be
  examined together") - flagged in code as applying general convention,
  not a verified-word-for-word citation. Also unverified against real
  hardware, same reason as PoE.
- Both new MIBs `IMPORT SnmpAdminString FROM SNMP-FRAMEWORK-MIB`, which
  isn't vendored - confirmed empirically (actually running
  `gosmi.LoadModule` against them) that gosmi resolves this leniently
  without the defining module physically present, so it wasn't added.
- `store.UpsertDevice`'s `SET` clause and `store.ReplacePorts`' `INSERT`
  now write all of the above; new `store.ReplaceModules` (mirrors
  `ReplacePorts`' delete+reinsert pattern) writes `EntityModules`'
  output - the `modules` table had no write function at all before this.
  No schema migration needed - every column already existed via earlier
  `ensureColumn` migrations.
- Verified live against a local `snmpd` instance: all 8 groups resolved
  without error: real port counters captured, and graceful zero/empty
  results everywhere the local host doesn't implement a given MIB
  (`Temp`, `StpTopChanges`, `Modules`, `PortPoE`, `VlanNames`,
  `AggregatePort` all correctly empty) - confirms the best-effort
  skip-on-failure design works as intended, not just that it compiles.

**Phase 3 (done)** - `PortDuplex`, `PortVlanMembership`, `CpuMemory`,
`ConfigManagement`. Before writing any OIDs from memory, the actual
switchmap Perl source this project rewrites was found on disk
and read directly -
it has real, battle-tested logic for the first two that turned out to
be meaningfully different from what this project's own earlier doc
comments had guessed:

- **`PortDuplex` isn't Cisco-specific after all.** switchmap tries
  **EtherLike-MIB's `dot3StatsDuplexStatus`** - a real IEEE-standard,
  cross-vendor MIB - *first*, before any Cisco MIB. Cisco-specific
  fallbacks only exist for hardware old enough to predate that MIB's
  support (Catalyst 1900/2900XL/3500XL/5000-in-CatOS era), none of
  which this project's seed data represents. Implemented as
  **generic-tier only** (`internal/snmp/drivers/generic/duplex.go`),
  skipping the legacy Cisco cascade entirely.
- **`PortVlanMembership`'s real source isn't the trunk-bitmap MIB.**
  The original guess (`CISCO-VTP-MIB`'s `vlanTrunkPortVlansEnabled`
  bitmap) turned out to be switchmap's *separate* `VlansOnTrunk` report
  feature (every VLAN allowed on a trunk), not the single access-VLAN
  value `model.Port.Vlan` needs. The real cascade, copied verbatim from
  switchmap's `Constants.pm`/`PopulatePorts.pm`: `vmVlan`
  (`CISCO-VLAN-MEMBERSHIP-MIB`) for the access-port VLAN, overridden
  unconditionally by `vlanTrunkPortNativeVlan` (`CISCO-VTP-MIB`)
  whenever it isn't the default (1) - this is how a trunk port's
  native VLAN becomes its effective "Vlan." `AuxVlan` (voice VLAN):
  `vmVoiceVlanId`, skipping the sentinel `4096`
  ("not configured"). switchmap's own further fallback,
  `c2900PortVoiceVlanId` (`CISCO-C2900-MIB`), wasn't implemented -
  that MIB isn't even in the LibreNMS mirror this project sources
  vendored MIBs from, confirming it's genuinely obsolete, not just
  old.
- `CpuMemory`/`ConfigManagement` - no switchmap precedent (it never
  tracked either). `CISCO-PROCESS-MIB`'s `cpmCPUTotal5minRev` (not the
  older `cpmCPUTotal5min` - `STATUS deprecated` in the vendored MIB
  text) and `CISCO-MEMORY-POOL-MIB`'s `ciscoMemoryPoolUtilization5Min`
  for the `processor(1)` pool (the one type "required to be supported
  by all devices" per the MIB itself) give both as 0-100 percentages.
  `ConfigManagement`'s `ccmHistoryRunningLastChanged` is a `TimeTicks`
  relative to the device's own uptime, not a Unix timestamp - converted
  by also fetching the device's current `sysUpTime` and computing the
  delta (a second, redundant `Get`, chosen over changing
  `ConfigManagementFetcher`'s interface signature to keep every group
  interface uniform).
- New `internal/snmp/drivers/cisco/` package - the first tier actually
  registered under a real vendor key (`"cisco"`) instead of
  `VendorGeneric`. 5 more Cisco MIBs vendored
  (`CISCO-VTP-MIB`/`CISCO-VLAN-MEMBERSHIP-MIB`/`CISCO-PROCESS-MIB`/
  `CISCO-MEMORY-POOL-MIB`/`CISCO-CONFIG-MAN-MIB`) plus standard
  `EtherLike-MIB` - several import from other unvendored MIBs
  (`INET-ADDRESS-MIB`, `RMON-MIB`, `HCNUM-TC`, `CISCO-QOS-PIB-MIB`,
  `SNMP-FRAMEWORK-MIB`) - confirmed empirically, same as Phase 2's
  `POWER-ETHERNET-MIB`/`ENTITY-SENSOR-MIB`, that gosmi tolerates all of
  them missing.
- `handlers_switches.go`/`handlers_ports.go`'s hardcoded
  `Duplex: "unknown"` replaced with a real `duplexLabel` function
  reading the actual column - the SQL already selected `p.Duplex` in
  `handlers_switches.go` (just never used it); `handlers_ports.go`
  needed the column added to its query too.
- `ports.Vlan`/`AuxVlan` carry a `FOREIGN KEY REFERENCES vlans(Vlan)` -
  a Cisco-reported VLAN ID has no guarantee a `vlans` row already
  exists for it (config may not declare it, and Q-BRIDGE-MIB
  vlan-name discovery is a separate walk that may not cover the same
  ID). `store.ReplacePorts` now backfills a nameless placeholder
  `vlans` row (`INSERT OR IGNORE ... Source = 'discovered'`) for every
  distinct VLAN ID a poll's ports reference, before inserting the
  ports themselves, so the constraint can't fail regardless of insert
  order.

**Bug found and fixed, affecting Phase 1/2 code too**: `gosnmp.BulkWalk`
requires its root OID **without** a trailing dot - passing one (the
convention every column OID constant in this codebase uses, so
`strings.TrimPrefix` against a returned PDU's `Name` correctly isolates
its index suffix) makes it silently match **zero rows**, even when real
data exists at that column. Confirmed directly against a live `snmpd`
instance: walking `ipAdEntIfIndex` with a trailing dot found nothing;
without one, found every row. This affected every group added in
Phases 2-3 that walked a single column directly rather than a whole
table prefix (`PortAggregation`, `IsLayer3`, `PortPoE`, `VlanNames`,
`Temperature`, `PortDuplex`, `PortVlanMembership`, `CpuMemory`) -
their "empty" Phase 2 live-test results were plausible (this project's
test host genuinely doesn't implement most of those MIBs) but not
actually *proven*, since the bug would have produced the same empty
result either way. Fixed uniformly via a new
`snmputil.BulkWalkColumn(s, columnOID, walker)` helper (trims the
trailing dot before calling `BulkWalk`) - every affected call site now
goes through it, and `PortDuplex` re-tested live afterward to confirm
real data is now actually captured (it was: the test machine's physical and
virtual interfaces returned real duplex values that a raw `snmpwalk` against
the same OID also showed, previously silently missed).

**Juniper/Ubiquiti**: get a documented extension point
(`internal/snmp/drivers/README.md` + a pre-seeded comment in
`vendorkey.go`'s `enterpriseVendors` map) rather than empty stub packages
or invented OIDs - no research exists for either vendor, and an empty
package with no `init()` registrations would read as "already done" when
blank-imported for nothing.

## Port admin-state and trunk/access mode (2026-09, done)

Two more fields filled in before the Stats page rebuild (deferred to its
own later plan) - both cheap additions since each reuses a table walk an
existing fetch group already performs, so neither needed a new SNMP
round trip or a new registry group:

- **`ports.AdminState`** (`ifAdminStatus`, IF-MIB, standard,
  `ifEntry.7`) - added to the generic `PortsFetcher`'s existing `ifTable`
  walk (`internal/snmp/drivers/generic/ports.go`), alongside
  `State`/`Speed`/`Label`.
- **`ports.IsTrunk`** (`vlanTrunkPortDynamicStatus`, `CISCO-VTP-MIB`,
  `vlanTrunkPortEntry.14`) - added to the Cisco
  `PortVlanMembershipFetcher`'s existing `vlanTrunkPortTable` walk
  (`internal/snmp/drivers/cisco/vlanmembership.go`), alongside
  `vlanTrunkPortNativeVlan`. Its own MIB description says it's already
  "a result of vlanTrunkPortDynamicState and the ifOperStatus of the
  trunk port itself" - the device has already resolved admin+link state
  into one authoritative answer, so no second object needed
  fetching/merging by hand. Cisco-only - there's no standard,
  cross-vendor MIB for trunk/access mode. A plain `bool` defaulting
  `false` is fully honest either way it arises: `vlanTrunkPortTable`'s
  own scope is "the local system's VLAN trunk ports" (an access port has
  no row here at all - the mirror image of `vmVlan` excluding trunk
  ports), and an explicit `notTrunking(2)` reading both correctly mean
  "not currently trunking."
- UI: `ports.tmpl` gained Mode/Admin State columns, `port.tmpl`'s Row
  table gained the same two (via the already-generic `infoRow`
  rendering, no template change needed there) - matching the fetch-to-
  displayed-label depth `Duplex` got in the Cisco-tier work, not
  schema-only.
- Verified live against a local `snmpd` instance: `AdminState` matched a
  raw `snmpwalk` of the same OID exactly; `IsTrunk` gracefully stayed
  `false` (no Cisco vendor match on a generic Linux agent, same as every
  other Cisco-only group against this test host).

**Explicitly deferred** (each its own mechanism, not an SNMP walk against
the polled switch at all): `nodes.Ip`, MAC-vendor lookup, and
`nodes.Hostname` via reverse DNS are all done, each documented separately
below - as is the Stats page rebuild that was waiting on them (see its own
section further down).

## nodes.Ip via router ARP-walk (2026-09, done)

Grounded in the actual switchmap Perl source (`GetArp.pl`, read directly,
not worked from memory) - confirms the architecture already suspected: a
completely standalone job, decoupled from the main per-switch poll, that
walks a **hand-maintained list of routers**
(`@ThisSite::routers` in site config, not auto-detected) and writes a
flat MAC/IP lookup the main report generator later reads. **Decided**
(three-way question): identify routers to ARP-poll via *both* an
explicit config declaration (new `devices.IsRouter`, mirroring
switchmap's own proven hand-listed-routers design) *and* the existing
auto-detected `devices.IsLayer3` flag - `WHERE IsRouter = 1 OR
IsLayer3 = 1`.

- `ipNetToMediaPhysAddress` (RFC1213-MIB, standard, already vendored,
  `.1.3.6.1.2.1.4.22.1.2.`) - switchmap tries the legacy `atTable` first,
  falling back to this only if that fails; `atTable` predates and is
  superseded by `ipNetToMediaTable` even within RFC1213 itself, so
  (matching every other "skip untestable legacy MIBs" decision this
  session) only `ipNetToMediaPhysAddress` was implemented. Its `INDEX`
  is `{ifIndex, ipAddress}`, so walking this one column directly yields
  the IP from the OID suffix (strip the column prefix and the leading
  `ifIndex` component - what's left is already a correctly-formatted
  dotted-decimal IP, no reassembly needed) and the MAC from the PDU's
  *value* as raw bytes - the first MAC decode in this codebase pulled
  from a value rather than an OID-index suffix, hence a new
  `snmputil.MacFromBytes`.
- Deliberately *not* a registry group (`internal/snmp/arp.go`'s
  `FetchArpTable` is a plain top-level function) - the vendor-registry
  machinery is for per-device poll enrichment merged into
  `model.Device`/`model.Port`, and ARP data isn't about the device being
  polled at all, it's what a router's cache says about *other* hosts
  entirely. Standard MIB, no vendor variation, so no registry needed.
- New `store.UpdateNodeIPs` updates **every** historical sighting row for
  a MAC, not just the most recent - `nodes` has no separate "current
  known state" concept and this project doesn't track per-sighting
  historical IP changes, so this is a documented simplification (a very
  old sighting could show a MAC's *current* IP rather than whatever it
  had back then). Can only enrich existing sightings - `nodes` rows
  require a valid `(Port, Device)` FK, so ARP data alone can never create
  a new node row for a MAC with no FDB-observed sighting on any
  monitored switch.
- New third scheduler ticker (`pollArp`/`loadRouterTargets`, alongside
  the existing poll/prune ones), own configurable interval
  (`arp_poll_interval_seconds`, default 300s, same `config.Int` pattern
  as every other interval).
- Verified live against a local `snmpd` instance: `FetchArpTable`'s
  output matched a raw `snmpwalk` of the same OID exactly (4 real ARP
  entries), and `UpdateNodeIPs` correctly overwrote a test node row's
  `Ip` column.

**Bug found while touching `Seed()` for the new `IsRouter` field, not
fixed (explicitly deferred by decision)**: `Seed()`'s
`INSERT OR REPLACE INTO devices/vlans/buildings/categories` statements
only list config-sourced columns. SQLite's `INSERT OR REPLACE` deletes
and re-inserts the whole row on a primary-key conflict - confirmed
empirically - so any column *not* in that list (every polled field:
`ChassisModel`, `SysObjectID`, `IsLayer3`, `Cpu`, `Memory`, `SerialNum`,
etc.) gets silently wiped back to `NULL` every time `-seed` runs again
after real polling has happened. Pre-existing (not introduced this
session), affects every `Seed()`-loaded table, not just `devices`. Fix
would be switching to an upsert that only touches config-sourced columns
on conflict (e.g. `ON CONFLICT(...) DO UPDATE SET <config columns
only>`), left for its own separate pass rather than bundled into the
ARP-walk work that surfaced it.

## MAC-vendor (OUI) lookup (2026-09, done)

Grounded in switchmap's own reference design (`OuiCodes.pm`/
`OuiCodes.txt`/`UpdateOuiCodes.pl`, read directly) - a flat, vendored
`<OUI> <org name>` lookup table, not a per-poll SNMP fetch. This project
uses the IEEE's current official CSV distribution
(`https://standards-oui.ieee.org/oui/oui.csv`) instead of replicating
switchmap's regex parse of the older human-readable `oui.txt`, since the
CSV is easier and more robust to parse. Scope matches switchmap's own:
only the classic 24-bit `MA-L` OUI (the first 3 MAC octets) is looked up -
the newer 28-bit/36-bit `MA-M`/`MA-S` extended allocations aren't handled,
a documented limitation, not an oversight.

**Design note**: the first pass loaded the parsed CSV into an in-memory Go
map at startup (mirroring `internal/config`'s pattern) and looked vendors
up in Go per rendered row. Reconsidered mid-session - the schema here is
static (two columns, never changes), so a real `oui` table costs nothing
extra to refresh (still just "re-download the CSV," now followed by a
`-seed` re-import instead of a process restart) and unlocks plain SQL
joins in the report queries, plus `sqlite3`-level inspectability and,
later, vendor-breakdown aggregates for the still-deferred Stats page
rebuild. Switched to that before landing:

- `oui` table (`Oui TEXT PRIMARY KEY, OrgName TEXT NOT NULL`) in
  `internal/store`'s schema - reference data, not site config, so it's
  seeded separately from `Seed()`'s config-file-driven tables.
- `internal/oui.ParseCSV(path) (map[string]string, error)` is now a pure
  parser (no package-level state) - `store.SeedOui` calls it and bulk-
  imports the result via a prepared statement in one transaction (`INSERT
  OR REPLACE` is safe here, unlike the documented `Seed()` bug below,
  since `oui` has only the two columns this import ever sets - no
  separately-polled column exists to wipe).
- `resources/oui/oui.csv` (vendored, ~40,200 rows) + `resources/oui/
  README.md` documenting the source, the `MA-L`-only scope, and the
  manual refresh process (re-download the CSV, re-run `-seed`), mirroring
  `resources/mibs/README.md`'s precedent.
- Report queries (`handlers_switches.go`'s Ports report,
  `handlers_ports.go`'s node-history table and single Port page's node
  list) now `LEFT JOIN oui o ON o.Oui = upper(replace(substr(n.Mac, 1, 8),
  ':', ''))` instead of calling a Go lookup function per row. A 4th
  suspected call site, `search.tmpl`, was checked and doesn't actually
  have a NIC column at all - an earlier TODO.md claim to the contrary was
  stale and has been corrected.
- Verified live against a throwaway instance and the real tracked
  `resources/inventory.sqlite`: ran the real `-seed` path (40,196 rows
  imported), then a test node's MAC temporarily set to a known
  IEEE-registered OUI (`E8:0A:B9:...`) correctly resolved to "Cisco
  Systems, Inc" via the join on both the Ports report and Node History
  page. Test MAC reverted afterward; dump diff against `HEAD` showed only
  the new `oui` table's schema and data as a difference, confirming
  nothing else in the tracked DB moved.

## nodes.Hostname via reverse DNS (2026-09, done)

Grounded in switchmap's own `MacIpTables.pm` (`GetHostNames`/
`GetHostNamesThreaded`, read directly) - a PTR lookup per known IP,
run as its own pass rather than a per-device SNMP fetch. switchmap's own
comment on this exact problem: unthreaded reverse lookups at its site took
over 12 hours; threaded, under 20 minutes - so this project's equivalent
(`internal/dns.Resolve`) uses a bounded-concurrency worker pool rather
than looking up one IP at a time.

- `internal/dns.Resolve(ctx, r Resolver, ips map[string]string,
  concurrency int, perLookupTimeout time.Duration, domainSuffix string)
  map[string]string` - `Resolver` is the one-method subset of
  `*net.Resolver` this needs, so tests use a fake instead of hitting real
  DNS. A miss is simply absent from the result (not recorded blank),
  matching switchmap's own hit/miss counting rather than overwriting a
  previously-known hostname with nothing. `domainSuffix`, if set, is
  stripped from the end of each resolved name (case-insensitive, with or
  without a leading dot) - the Go equivalent of switchmap's
  `s/$ThisSite::DnsDomain$//`.
- Folded into `pollArp` right after the existing `store.UpdateNodeIPs`
  call, reusing the same ARP-table mac->ip map for both - mirrors
  `MacIpTables.pm` filling `MacIpAddr` and `MacHostName` from the same
  pass, rather than a separate ticker/config interval for DNS.
- New `store.UpdateNodeHostnames` mirrors `UpdateNodeIPs` exactly (same
  "update every historical sighting row for a MAC" simplification, same
  existing-sighting-only limitation - see `UpdateNodeIPs`'s doc comment).
- New config keys (`config.Int`/new `config.String` accessor, same
  default-fallback pattern as every other interval in this project):
  `dns_concurrency` (default 20), `dns_lookup_timeout_seconds` (default
  3), `dns_domain_suffix` (default `""`, no stripping).
- Verified live: a scratch program confirmed `internal/dns.Resolve`
  against real public DNS (`8.8.8.8` -> `dns.google`, `1.1.1.1` ->
  `one.one.one.one`); a second scratch program confirmed
  `store.UpdateNodeHostnames` writes correctly against the real tracked
  `resources/inventory.sqlite` (reverted afterward, dump-diff confirmed
  byte-identical to `HEAD`). No live router was available to exercise the
  full `pollArp` path end-to-end with real ARP data - unit tests cover
  `internal/dns.Resolve`'s concurrency, miss-handling, and domain-suffix
  logic against a fake resolver instead.

## SNMPv3 support (2026-10, done)

`scheduler.pollDevice` hardcoded `gosnmp.Version2c`; `devices` only had a
`Community` column. `gosnmp` (already a dependency) implements v3 fully -
confirmed by reading its real source (`v3.go`/`v3_usm.go`), not assumed.
Per-device, not global: `devices.SnmpVersion` (`'2c'` or `'3'`, default
`'2c'`) lets a mixed fleet poll some devices as v2c and others as v3
side by side, with zero config/code changes for devices that stay on v2c.

- New nullable `devices` columns (`Username`, `SecurityLevel`,
  `AuthProtocol`, `AuthPassphrase`, `PrivProtocol`, `PrivPassphrase`,
  `ContextName`), config-sourced via `configuration/devices.json` exactly
  like `Community`/`Address` already are - optional fields, absence means
  v2c behavior, unchanged for every pre-existing device.
  `SecurityLevel` values: `noAuthNoPriv`/`authNoPriv`/`authPriv`.
  `AuthProtocol`: `MD5`/`SHA`/`SHA224`/`SHA256`/`SHA384`/`SHA512`.
  `PrivProtocol`: `DES`/`AES`/`AES192`/`AES256`/`AES192C`/`AES256C` -
  all gosnmp's own enum names verbatim, not reinvented, so they stay
  greppable against its source. Credentials stay plaintext in SQLite,
  same as `Community` already is - no new at-rest exposure, just more of
  it.
- `Community` stays `TEXT NOT NULL` rather than migrated to nullable -
  SQLite has no `ALTER COLUMN`; dropping that constraint would need a
  full table-rebuild migration (`ports`/`modules` both FK-reference
  `devices(Device)`) for a purely cosmetic win. A v3-only device just
  gets `Community = ''`, which `buildSession` never reads on the v3
  branch.
- New `internal/scheduler.buildSession` (shared by `pollDevice` and
  `pollArp`, which previously duplicated session construction inline)
  branches on `SnmpVersion`. Returns a clear error - not a silent
  wrong-auth connection - for a v3 device with an unrecognized or missing
  `SecurityLevel`/protocol; surfaces as the same per-device poll-failure
  log line a `Connect()` error already produces.
- **Engine discovery required zero new code**: `gosnmp`'s
  `negotiateInitialSecurityParameters` (`v3.go`) runs automatically and
  transparently inside the library's own send path on a session's first
  real request - not something `Connect()` or caller code has to
  trigger. v3 works correctly without any caching of
  `engineID`/`engineBoots`/`engineTime`.
- **Decided: no engine-param caching across polls** - `pollDevice`
  already builds a brand-new session per poll with no cross-cycle reuse
  for *any* device, v2c or v3; adding v3-specific caching would be
  inconsistent with how every other device is handled, and the discovery
  round-trip already happens inside the same `Timeout`-bounded exchange.
  Left as a possible future optimization, not a correctness gap.
- **Decided: VACM partial-failure differentiation not built** - a v3
  device that authenticates but gets a restricted OID subtree back just
  surfaces as today's generic per-group "didn't resolve" pattern
  (already non-fatal/logged throughout `fetch.go`). No special "this
  might be a permissions issue, not a bug" handling exists; noting this
  explicitly rather than silently skipping it.
- **A real, forced fix surfaced while scoping this**: `store.Seed()`
  built its `devices` INSERT via raw `fmt.Sprintf("...'%s'...")` string
  concatenation (no escaping) - pre-existing, and out of scope on its
  own, but v3 passphrases routinely contain characters that break that
  immediately, not theoretically. Converted just the `devices` INSERT to
  a parameterized `Prepare`/`Exec` loop, mirroring `SeedOui`'s existing
  correct pattern - necessary for v3 to function at all, not a stylistic
  improvement. `buildings`/`categories`/`vlans`' identical pre-existing
  pattern is untouched (not worsened by this change, not required to
  build this feature, and left for its own separate pass if it matters -
  same "surfaced a bug, fixed only what blocked the actual feature"
  discipline as the `AggregatePort`/`PoeClass` fixes elsewhere in this
  document).
- Verified live end-to-end against a throwaway `snmpd` configured with a
  real v3 user (`createUser`/`rouser`, `authPriv`, SHA+AES) - including
  an auth passphrase containing a literal single quote
  (`AuthPass'123`), specifically to prove the `Seed()` fix: seeded it
  through the real `configuration/devices.json` -> `Seed()` ->
  parameterized-INSERT path without corruption, then a real `-pollonce`
  succeeded and wrote real polled data. Also verified: a deliberately
  wrong passphrase fails cleanly (`"incoming packet is not authentic"`,
  logged, non-zero exit, no crash); an existing v2c device still polls
  correctly unaffected; and two devices with different `SnmpVersion`
  values coexist in the same `devices` table (the actual mixed-fleet
  case). Reverted all test data (DB and `devices.json`) afterward;
  confirmed the tracked DB's dump matches `HEAD` except the intentional
  new `devices` columns (`oui` row order differs between any two
  `-seed` runs due to Go map iteration order, a pre-existing, harmless
  nondeterminism unrelated to this change - verified by content, not
  order, same as every prior schema-adding phase).

## Data-completeness gaps (2026-09, done)

Both items formerly listed here are done:

- **Info page's Firmware field** - turned out to already be fully fetched
  and stored (`identityResolver.ResolveIdentity`'s Entity MIB chassis-row
  walk sets `dev.FirmwareVersion`, `UpsertDevice` persists it) - the gap
  was purely a wiring bug: `infoHandler`'s query never selected
  `FirmwareVersion` and hardcoded the row to `""` with a stale "needs an
  Entity MIB module walk we haven't built" comment. Fixed by selecting
  and displaying the already-populated column - no new SNMP work needed.
  Verified live: blank on the generic Linux test host (its Entity MIB
  doesn't expose chassis firmware data - a real, expected "device doesn't
  report this" case, not a bug), then confirmed the render path itself
  works by temporarily setting a real value directly in the tracked DB,
  reverted after.
- **Ports page's "Since" column** - new `ports.Since INTEGER` (Unix
  timestamp), computed from IF-MIB `ifLastChange` (`ifEntry.9`, standard -
  already covered by `FetchPorts`'s existing `ifTable` `BulkWalk`, so one
  more dispatch case, not a new walk) converted via the device's current
  `sysUpTime` - same TimeTicks-delta approach, and same "second redundant
  Get rather than changing the group interface signature" reasoning, as
  `Device.ConfigUpdated`/`FetchConfigUpdated`. Best-effort: a failed
  `sysUpTime` Get doesn't fail the whole poll, it just leaves `Since` at
  its zero value for every port on that poll. `0` (unavailable) stores as
  `NULL`, not a literal Unix-epoch 1970 timestamp (`nullableSince`,
  mirroring `nullableInt`'s existing convention) - a real 1970 value would
  otherwise render as a nonsensical "01/01/1970" instead of blank.

**Two more pre-existing bugs surfaced by live-testing, unrelated to either
gap above but caught while verifying them - both fixed:**

- `ports.PoeClass` has held a text label (`"class0"` through `"class4"`, from
  `internal/snmp/drivers/generic/portpoe.go`'s `poeClassNames`) since the
  PoE walk landed, but the schema declared it `INTEGER` and
  `handlers_switches.go`'s `poeLabel` read it back as `sql.NullInt64` -
  meaning PoE class data has never actually worked: an empty (unpopulated)
  value stored a literal `''` that failed to scan, and a real value would
  have failed the exact same way, just with real data instead. Only
  surfaced now because this was the first live poll to exercise the Ports
  report against a device with a non-crashing `AggregatePort` (see the
  Stats page section above) - the `PoeClass` scan error was the very next
  thing blocking that same page. Fixed: schema changed to `TEXT`, `poeLabel`
  now takes `sql.NullString` and renders `"Class " + trimmed label`
  (`"class2"` -> `"Class 2"`), new `nullableString` helper (mirroring
  `nullableInt`) converts an empty label to `NULL` on write, matching
  every other optional-field convention in `ReplacePorts`.
- Confirmed via a real poll against a throwaway `snmpd`, then reverted -
  dump-diff against `HEAD` showed only the intentional `ports.IfType`/
  `ports.Since` schema additions (`oui` row order differs between any two
  `-seed` runs due to Go map iteration order, a pre-existing, harmless
  nondeterminism - verified by content, not order, same as the Stats page
  verification above).

## vlans.Subnet - config-validated, not SNMP-discovered (2026-09)

`vlans.Subnet` stays config-only (`configuration/vlans.json`, loaded by
`Seed()`) - SNMP-based discovery was investigated (a Layer-3 device's
`ipAddrTable` correlated to the VLAN's SVI, see git history for that
research if it's ever revisited) but deliberately dropped in favor of a
simpler, decided direction: **when a vlan's `Subnet` is present in config,
use it to validate observed client IPs (`nodes.Ip`) against it, rather than
ever trying to derive the subnet itself from a poll.**

Not built yet - explicitly deferred, not just undecided:

- The validation itself (does a node's IP fall inside its vlan's
  configured `Subnet` CIDR) is straightforward once there's real data to
  check - the blocker isn't the logic, it's that both inputs are mostly
  empty today: `nodes.Ip` is barely populated (8 old fixture rows, nothing
  from real polling - see the Node IP gap above) and every `Subnet` in the
  current `configuration/vlans.json` is blank.
- UI direction is decided: color the affected cell(s) to flag a mismatch,
  rather than a separate column/badge. Not implemented.
- Surface (which page(s) show this) not finalized - candidates are the
  Vlan detail page (`vlan.tmpl`'s Members table, since `Subnet` is already
  on that page) and/or wherever else `IP` already renders
  (`node.tmpl`/`ports.tmpl`/`search.tmpl`/`port.tmpl`).

**DHCP (bool) - removed entirely (2026-09).** There's no standard MIB
object for "is this subnet DHCP-served" - that's an administrative policy
fact (whether a DHCP pool or relay is configured), not protocol state a
device reports. The two theoretical proxies considered were both weak and
both would've needed an unverified vendor MIB (an `ip helper-address`
relay-config MIB, or inferring from DHCP-snooping activity, which only
signals "DHCP traffic happened recently," not "this subnet is designated
as DHCP"). Rather than keep a field with no viable path to being polled and
no other use in the app, `DHCP` was dropped from the `vlans` table
(`store.dropColumn`, the mirror of `ensureColumn`, handles the migration
for already-seeded databases), from `configuration/vlans.json`, and from
the Vlans/Vlan-detail pages that displayed it.

## vlans.Source - discovery semantics (2026-09)

`vlans.Source` (`'config'` or `'discovered'`, added alongside
the SNMP-feasibility question above) exists to let an SNMP poll safely
auto-name a vlan ID with no entry in `configuration/vlans.json`, without
ever overwriting anything config declared. Exact rules, as decided:

- A vlan ID present in `vlans.json` is `Source = 'config'` **regardless of
  whether its `Name` is empty** - `Seed()` inserts every config entry
  up front, so by the time any poll runs, "no row for this ID" and "row
  exists with an empty config-declared name" are already distinguishable
  states. `store.UpsertDiscoveredVlan` only ever fires for the former.
- A vlan ID absent from config becomes `Source = 'discovered'`, named
  after **the first non-empty name any poll reports for it** - an empty
  name is skipped rather than inserted (a discovered row shouldn't get
  permanently locked onto blank just because the first poll to see the ID
  hadn't resolved a name yet). Once a discovered row exists, every later
  call for that ID is a no-op (`INSERT OR IGNORE` against the `Vlan`
  primary key) - so if two switches disagree on an undocumented vlan's
  name, whichever names it first wins from then on, silently.
- Consequence, by design: an undocumented vlan either gets one consistent
  name everywhere it's polled (the common case - correct), or a later
  switch's differing name is dropped on the floor with nothing surfaced
  today (the "something's wrong" case the mismatch is actually useful
  for). **Open follow-up, not built**: a way to notice and flag that
  second case (and its mirror - a config-declared vlan with an empty
  `Name` that a switch *does* report a name for) rather than relying on
  someone reading raw poll logs to catch it.

## Modules table (2026-09, done)

This section previously said "still needs a poller walk + UI section" -
that text was stale (see below).

`model.Module` and its `modules` DB table (per-slot line-card info for
modular chassis switches - a Catalyst 6506/6509/4507 has swappable modules,
a fixed-config switch doesn't) sat unused since the original 2014 code,
with no comment ever explaining why. Intent reconstructed (2026-09) from
three pieces of circumstantial evidence, not an author's note:

- The `modules` table's `Module` column has always been `INTEGER`, not
  `TEXT`, despite the Go field's original "Module Name" comment - a
  composite primary key of `(Module INTEGER, Device)` only makes sense if
  `Module` is a numeric slot/index, matching Cisco's own `show module`
  numbering.
- switchmap's own `Stats.pm` (the tool this project rewrites) has this
  exact comment: `if it has modules (i.e. 6509s have modules, 3524s
  don't)` - the same device family already in this project's own seed
  data (`HQ-R2-6506-01`, `LAB-R2-6509-01`).
- The schema (`Description`/`Model`/`FwVersion`/`HwVersion`/`SwVersion`/
  `SerialNum`) maps field-for-field onto Entity MIB's `entPhysicalTable`
  (the same table already walked for chassis info) filtered to
  `entPhysicalClass == module(9)` ("e.g., plug-in card or daughter-card")
  instead of `chassis(3)` - see `model.Module`'s doc comment for the exact
  OID-to-column mapping.

**This reconstruction turned out to already be fully built** by the time
this note was revisited (2026-09) - the "what's missing" text below was
written before that work landed and nobody came back to update it once it
did. Tracing the actual code (not just this file) found the whole path
already wired end-to-end, dating to the original Phase 2 SNMP-registry
work:

- `internal/snmp/drivers/generic/entitymodules.go` walks the same
  `entPhysicalTable` used for chassis info, filtered to
  `class == module(9)`, registered as `EntityModulesFetcher`.
- `internal/snmp/fetch.go`'s `FetchAll` already resolves it and populates
  `Result.Modules`.
- `internal/scheduler/scheduler.go`'s `pollDevice` already calls
  `store.ReplaceModules(tx, target.Device, result.Modules)`.
- `internal/web/handlers_switches.go`'s `infoHandler` already queries the
  `modules` table and builds one `moduleInfo` (key/value `Row`) per
  module.
- `resources/templates/info.tmpl` already has the
  `{{ if .Modules }}{{ range .Modules }}<h2>Module {{ .Module }}</h2>...`
  section, hidden entirely when a switch has none - exactly the design
  this note used to propose as future work.

Verified live (2026-09), since this codebase has had real bugs hiding in
"looks done" code before (`AggregatePort`, `PoeClass` - see the Stats
page and Data-completeness sections above): re-polled a real device
against a throwaway `snmpd` - no crash, zero real module-class rows on
that generic Linux host (expected; its Entity MIB doesn't report a
module-class component), and the Info page correctly rendered no Module
sections. Since the local `snmpd` simulator can't produce real
module-class Entity MIB data, also hand-inserted two synthetic `modules`
rows directly into the tracked DB (with the device's `Address` reverted
first, so the background scheduler's own poll cycle couldn't overwrite
them mid-test) and confirmed both rendered as separate, correctly
populated `<h2>Module N</h2>` sections, in module-number order. Reverted
afterward - `git checkout` was sufficient (no new schema/migration this
time), confirmed byte-identical to `HEAD` via dump diff.

No real hardware validation exists for this (same accepted, deferred
limitation as the rest of the project - see Housekeeping below): whether
`entPhysicalClass == module(9)` cleanly captures just line-card slots
versus also matching noisier component types (SFP transceivers, power
supplies, fans) on real Cisco gear is unverified.

## Network-wide Stats page (2026-09, done)

Grounded in switchmap's own `Stats.pm` (read directly) - switch/module
model counts, port counts by state/type, VLAN and unique-IP counts, and a
per-switch breakdown, at `/campus/stats/` (was a pure empty stub -
`statsHandler` rendered nothing).

- New `ports.IfType INTEGER` (IF-MIB `ifType`, IANAifType-MIB values) -
  `internal/snmp/drivers/generic/ports.go`'s `FetchPorts` already walked
  the whole `ifTable` prefix in one `BulkWalk`, so this was one more
  `case` in its existing dispatch switch, not a new OID/walk.
- `internal/web/handlers_stats.go` (new) - `isAncillaryPort`/
  `isVirtualPort` mirror switchmap's `SwitchUtils::IsAncillaryPort`/
  `PopulatePorts::IsVirtualPort` type lists exactly (unit-tested). Only
  the `ifType`-based checks are replicated - switchmap also excludes a
  few ancillary ports by interface *name* pattern (`^Async`, `^CPU`,
  `^Nu\d+`, etc.), which this project can't replicate: `ports.Label` is
  ifAlias falling back to ifDescr, not the raw ifDescr alone, so there's
  no column that reliably holds the original interface name. A narrower
  filter than switchmap's, and a known limitation, not an oversight.
- `portStats` does a single pass over every port - mirroring switchmap's
  own `Stats::CollectStats` (also one loop incrementing counters),
  instead of encoding the same classification twice (once in Go, once in
  SQL `CASE WHEN`). Ancillary ports are skipped entirely from every
  count, matching switchmap's own if/else structure.
- switchmap's "active ports with empty labels" column is dropped -
  `ports.Label` already falls back to `ifDescr` when `ifAlias` is blank,
  so it's never meaningfully empty in this schema. switchmap's per-site
  subtotal rows (`getSite`, an NCAR hostname-regex) are also dropped -
  org-specific, not portable, consistent with the cabling-audit/
  PoE-allowlist bullets below.
- **Three real bugs surfaced by live-testing this against a real poll**
  (not hand-crafted test data) rather than by inspection - all fixed:
  (1) `ReplacePorts` bound `AggregatePort` directly into an INTEGER
  column without converting an empty string to `NULL` the way
  `Vlan`/`AuxVlan` already did, so a real poll of a non-aggregated port
  stored a literal `''` that then failed to scan as `sql.NullInt64` -
  fixed by renaming the existing `nullableVlan` helper to the more
  general `nullableInt` and applying it to `AggregatePort` too.
  (2) The Port by State virtual/non-virtual split double-counted: a
  virtual active port incremented both `Active` and `ActiveVirtual`
  instead of exactly one, because the classification `switch` used
  `states.X++` unconditionally plus a separate `if virtual` add-on
  instead of an if/else branch. Caught because a real generic Linux
  `snmpd`'s loopback interface (a genuinely virtual, non-ancillary
  `ifType`) exposed the double-count immediately; a purely synthetic
  test fixture might not have.
  (3) `portStats`'s query scanned `IfType`/`AdminState`/`State`/`IsTrunk`
  into plain (non-nullable) `int`s, which crashes the whole page on any
  `ports` row from before this migration - or before its device's first
  real poll - since those columns are NULL until then. Fixed by scanning
  into `sql.NullInt64` throughout; a NULL `AdminState`/`State` now falls
  into the Unknown bucket instead of erroring, and a NULL `IfType` (0) is
  correctly neither ancillary nor virtual. Caught by reverting the DB to
  its pre-poll state (with the new column freshly migrated but not yet
  populated) and re-checking the page, not by the live-poll check alone.
- Verified live: re-polled a real device against a throwaway `snmpd`
  (after clearing that device's old hand-crafted `nodes` rows, which
  referenced ifIndexes the real host doesn't have and blocked
  `ReplacePorts`'s delete-and-reinsert via the `nodes` FK - a separate,
  pre-existing edge case, not a Stats bug), then hand-verified every
  number on the rendered page against direct `sqlite3` queries over the
  same data. Reverted afterward via `-seed` against a freshly-checked-out
  copy of the tracked DB (cleaner than trying to hand-revert a real
  poll's writes) - confirmed byte-for-byte identical to `HEAD` except the
  intentional `IfType` schema addition (`oui` table row order differs
  between any two `-seed` runs due to Go map iteration order, a
  pre-existing, harmless nondeterminism unrelated to this change -
  verified by content, not order).
- `stats.tmpl`'s first draft copied switchmap's own side-by-side
  nested-table layout (an outer `<table><tr><td>` wrapping each section's
  own table). Reworked to a flat stack of `<h2>` + single `<table>`
  blocks instead, matching every other report page in this app
  (`vlans.tmpl`/`ports.tmpl`/`switches.tmpl`) - no new CSS either way, but
  consistency with the existing pages' structure was the point.

## Unused-port reclamation report (2026-09, done)

`/campus/unused/` - was blocked on "inactivity-day tracking, which ties
back to the `ifLastChange`/'Since' gap"; that gap closed with the
"Since" column work, unblocking this. Grounded in switchmap's own
`WriteUnusedDirectory.pm`/`PopulatePorts::SetDaysInactive` (read
directly):

- switchmap computes `DaysInactive` from its own hand-rolled `IdleSince`
  (written by a separate `ScanSwitch.pl` cron job that snapshots port
  state on every run and records the first time it noticed a port go
  non-active) - not from any SNMP-native MIB object. This project uses
  `ports.Since` (IF-MIB `ifLastChange`, standard, already populated)
  instead - both answer the same question ("when did this port's state
  last change"), so no new SNMP work or poller changes were needed for
  this report at all.
- Threshold: new `unused_after_days` config key (`configuration/
  preferences.json`, `config.Int` pattern, default **60** - switchmap's
  own real default, `ThisSite.pm: $UnusedAfter = 60`). Read directly in
  `internal/web` at request time (`config.Preferences` is a package-level
  global, already read the same way elsewhere in this package - no need
  to thread it through `main.go`/the router as a parameter).
- **Decided** (user): one combined, filterable page rather than
  switchmap's three separate static-file views (an index, unused-by-
  switch, unused-by-vlan) - those existed because switchmap pre-renders
  static HTML per switch/vlan, not because that structure is essential.
  Filters are Building/Room/Switch (dependent dropdowns, same "current
  selection first, then blank 'all', then the rest" convention as
  `handlers_campus.go`'s `getSelection`, adapted as a new `unusedSelection`
  since the base table differs) - VLAN filtering deliberately dropped,
  not asked for.
- Classification logic (`isUnusedPort`, `internal/web/handlers_unused.go`)
  is a small pure function, unit-tested at the exact threshold boundary:
  skip ancillary pseudo-interfaces (`isAncillaryPort`, reused directly
  from the Stats page work) and currently-Active ports
  (`adminState==1 && state==1`, same check as `handlers_stats.go`'s
  `portStats`), then compare days since `ifLastChange` against the
  threshold - `> thresholdDays`, not `>=`, matching switchmap's own
  `SecondsDead > UnusedAfter*SecondsPerDay` exactly (confirmed live: a
  port at precisely the threshold does not yet count as unused).
- Verified live against the real tracked `resources/inventory.sqlite`:
  hand-set one port's `Since` far in the past with `IfType`/`State`/
  `AdminState` values for three cases (inactive-and-eligible, still-
  Active, ancillary) and confirmed exactly the eligible one appeared,
  with the correct computed `DaysInactive`; confirmed Building/Room/
  Switch filters each narrow correctly in both directions; confirmed the
  empty-result state renders a valid page (HTTP 200, empty table), not
  an error. Reverted afterward - no schema change this time, so a plain
  `git checkout` was sufficient.

## Search feature - scope not finalized (2026-09)

`/campus/search/` (`searchHandler` in `handlers_campus.go`, wired into
the nav bar's search box via `_base.tmpl`) is **not a stub** - it already
does a real `LIKE` substring match against `nodes.Mac` and
`nodes.Hostname`, rendering a results table (Date/Switch/Port/CDP/IP/Host)
that links back to each matching switch's Ports report. Explicitly
flagged as needing more work anyway (2026-09 decision - not sure yet
exactly how much, captured here to flesh out later), in two directions:

- **More searchable fields** - today only `Mac`/`Hostname` match. Natural
  candidates: device/switch name (`devices.Device`), port label/
  description (`ports.Label`), IP address (`nodes.Ip`), VLAN (`vlans.Vlan`/
  `Name`). Whether these become one combined query or separate scoped
  searches isn't decided.
- **Better matching** - today's match is a plain SQL `LIKE '%term%'` (see
  `searchHandler`), which is substring-only, has no explicit case-folding
  guarantee (depends on the DB's default collation), and offers no
  exact-match option. Also currently scoped entirely to node *sightings* -
  there's no way to search for a switch directly by name/model/building
  the way `switches.tmpl`'s filter dropdowns can browse but not search.

Not yet decided: whether this becomes one unified search across every
field above, or stays MAC/hostname-focused with separate, more targeted
lookups (e.g. a switch-name search) added elsewhere instead.

## Bootstrapping discovery script (2026-10, done)

No existing switchmap precedent to ground this in - switchmap itself has
no auto-discovery mechanism either (its device list is entirely
hand-maintained in site config). New territory for both tools.

`-discover` flag on the main binary (same one-shot-flag pattern as
`-seed`/`-pollonce`), **semi-interactive** per explicit direction: every
decision (subnet, community, port, CDP seed device, naming patterns) can
be supplied via a flag for scripted use, or left blank to be prompted for
interactively (`-discover-noninteractive` fails fast naming the missing
flag instead of prompting, for cron/scripted use with a genuinely missing
required value). **Default mode is a subnet sweep** - SNMP itself is the
"scan" (a cheap `Get(sysDescr)` tells you who's listening), no external
scanning tool, no new dependency. **CDP-neighbor crawl is opt-in**
(`-discover-cdp=<already-seeded-device>`), **one hop only** - crawling
further would mean guessing credentials for unknown neighbors with no
real basis; noted here as a possible future extension, not built.

**Deliberately no configuration-file dependency at all** (explicit
direction, after an earlier version of this feature read from a
dedicated `configuration/discover-patterns.json` and from
`configuration/preferences.json`): a tool meant to bootstrap a brand-new
installation, typically run once, shouldn't need a config file prepared
before it can even start. Every tunable - probe/fetch timeouts
(`-discover-probe-timeout`/`-discover-fetch-timeout`, `time.Duration`
flags, e.g. `500ms`/`10s`), sweep concurrency (`-discover-concurrency`),
and naming patterns (below) - is a flag, with an interactive prompt as
the fallback for whichever one has no safe default and wasn't supplied.

- **Switch-likeness test** (`internal/discover.classify`): primarily
  `sysServices`' datalink/bridge bit (`.1.3.6.1.2.1.1.7.0`, RFC1213,
  standard - new, a one-off `Get` used only for this classification, not
  added to `model.Device` or any schema since it has no other use);
  falls back to counting `ethernetCsmacd(6)`-typed ports for devices that
  don't correctly report `sysServices`. Deliberately loose - a human
  reviews every candidate before it's trusted anyway.
- **Output**: a separate `configuration/discovered-devices.json`
  (`{"Devices": [...]}`, the same shape `devices.json` itself uses, so a
  reviewed entry pastes straight across), never written into
  `configuration/devices.json` directly - `Category`/`Building`/`Room`
  can't be discovered via SNMP at all (confirmed - no OID carries this
  project's own config taxonomy), so human review is unavoidable anyway.
  Already-seeded `devices.Address` values are skipped (loaded from the
  DB each run), so re-running discovery against an increasingly-known
  fleet doesn't keep re-surfacing the same switches.
- **Naming-convention patterns close the Category/Building/Room gap**
  (the key insight: no SNMP walk can supply these, but the site's own naming does):
  this project's own device names already encode exactly what SNMP
  can't discover - `HQ-R1-3560-01` = building short code - room - model -
  index. Patterns are regexes with named capture groups
  (`Device`/`Category`/`Building`/`Room`/`Model`/`Index` - an explicit,
  documented set, not arbitrary reflection-based field matching), tried
  in order against each candidate's discovered name (first match wins,
  so a site with more than one naming convention can list several).
  Each matched group sets the corresponding `Candidate` field
  **verbatim, with no resolution against anything else** - `Building`
  capturing a bare short code like `"HQ"` straight out of the device
  name is expected and fine (**this was wrong in the first version of
  this feature**, which tried to resolve a `ShortName` group through the
  `buildings` table into a full name - more machinery than asked for;
  bootstrapping only needs to get close enough to be useful, a human
  renames it later if they want). `internal/discover/patterns.go`'s
  `ApplyPatterns`/`CompilePatterns` are deliberately pure (no network/DB/
  file I/O), fully unit-tested.
- **No file holds patterns at all - flag or prompt, nothing else**
  (second correction to the first version, which read a dedicated
  `configuration/discover-patterns.json`): the repeatable
  `-discover-pattern=<regex>` flag (a custom `flag.Value` - `flag.String`
  only keeps the last occurrence of a given flag name, no good for a
  list) supplies patterns directly on the command line. If none are
  given and the run is interactive, `promptPatterns`
  (`cmd/switchgopher/discover.go`) offers a repeatable prompt loop (one
  pattern per line, blank line to stop, including stopping immediately
  with zero). Non-interactive with no flag just means zero patterns, not
  an error - they're always optional/additive.
- **The matched Building values also seed `buildings.json` itself, not
  just each device entry** (the second half of the same insight, also
  missed in the first version): `DistinctBuildings` collects every
  distinct, non-empty `Building` value across all candidates after
  patterns are applied, skips anything already present as a `Building`
  or `ShortName` in the real seeded `buildings` table (so a building the
  site already configured, even just by its short code, doesn't keep
  resurfacing on every run), and writes the result to a second file,
  `configuration/discovered-buildings.json` (`-discover-buildings-
  output`), shaped exactly like `buildings.json` itself
  (`Building`/`ShortName`/`Description`, the latter two always blank -
  nothing in a device's own name could supply them). Same "never touch
  the hand-maintained file directly" rule as `discovered-devices.json`.
- **New `internal/snmp/cdpneighbors.go`**: `FetchCdpNeighbors`, verified
  against the vendored `CISCO-CDP-MIB` text directly (not assumed) -
  `cdpCacheAddressType`/`cdpCacheAddress` are `cdpCacheEntry.3`/`.4`
  (`.1.3.6.1.4.1.9.9.23.1.2.1.1.3.`/`.4.`), the MIB's own DESCRIPTION for
  `cdpCacheAddress` explicitly says it's meant to be an address the
  neighbor accepts SNMP on. Deliberately standalone, not a registry
  group - like `arp.go`'s `FetchArpTable`, this is data about *other*
  devices. Keeps every distinct neighbor entry, unlike
  `drivers/generic/cdp.go`'s existing per-port walk for node-history
  display (which collapses to one entry per ifIndex).
- **New `scheduler.BuildSessionForDevice`**: a thin exported wrapper so
  the CDP-crawl mode can connect to an already-seeded seed device using
  whatever credentials it has on file (v2c or v3) without duplicating
  `buildSession`'s branching a third time.
- **A real bug found via live interactive testing, fixed**: the
  community prompt was unreachable - the flag's own default (`"public"`)
  meant "not supplied" and "explicitly public" were indistinguishable,
  so the prompt never fired and a typed answer was silently ignored in
  favor of the flag default. Fixed by making the flag default empty and
  handling the real default/prompt logic explicitly, and reordered
  prompts (subnet before community - the primary decision before a
  supporting detail) once both were exercised together. Caught precisely
  *because* this was tested live via real stdin piping, not just unit
  tests of the non-interactive path.
- Verified live: three throwaway `snmpd` instances on distinct loopback
  addresses, swept via the real CLI (`-discover-subnet`/`-port` lets
  this run on a non-privileged port without root) - confirmed exactly
  the right candidates came back with correct Address/Community/Model;
  confirmed the known-address skip (temporarily seeding one test
  address as already-known) correctly excluded it; confirmed both the
  interactive-prompt path (piped stdin) and the non-interactive-missing-
  flag error path. Naming patterns verified against the real seeded
  `buildings` table with two test switches named after this site's own
  short codes (`HQ`/`LAB`) - confirmed `discovered-buildings.json`
  correctly omitted both, since the site already has them configured -
  then a third switch named with a deliberately novel code (`ZZ`)
  confirmed it *does* show up once genuinely new. `-discover-pattern`
  verified against a device named `QQ99` (a shape the example
  hyphenated pattern used elsewhere in this testing can't match) -
  confirmed a CLI-supplied pattern resolved `Building`/`Room` correctly,
  and that two repeated `-discover-pattern` flags respect first-match-
  wins order. The fully interactive path (no flags at all beyond
  `-discover-port`) was run end-to-end via piped stdin - subnet,
  community, *and* a naming pattern all entered at prompts, confirmed to
  produce an identical result to the flag-driven run; separately
  confirmed non-interactive mode with no pattern flag just proceeds with
  zero patterns (no error, no prompt). The new `-discover-probe-timeout`/
  `-discover-fetch-timeout`/`-discover-concurrency` flags were exercised
  with real `time.Duration` values (`500ms`/`5s`). **CDP-crawl's positive
  (neighbor found and reachable) path is unverified against real CDP
  data** - net-snmp's stock agent doesn't implement `CISCO-CDP-MIB`
  (same honest limitation already accepted elsewhere in this project for
  CDP/Entity-MIB-module testing) - only the zero-neighbors graceful case
  was confirmed live. All test data reverted; no schema change this
  phase, confirmed via dump diff against `HEAD`.

## Schema migrations flattened, site data reset (2026-10, done)

The app had never been run against real deployed data - every column
added to `vlans`/`ports`/`devices` across 2026-09/10's development had
its `ensureColumn` migration-list entry added in lockstep with the
column going into the base `CREATE TABLE` string, so there was no
in-between state anywhere that actually needed the migration path to
reach it. With no real database to preserve, `store.Setup()`'s
`columns` migration list (~25 entries) and its one
`dropColumn(database, "vlans", "DHCP")` call were deleted outright -
`ensureColumn`/`dropColumn` were kept as infrastructure for (later deleted as unused; see git history)
whenever a genuine schema change needs to reach a database with real
data in it later.

Also reset, per explicit instruction, back to a clean bootstrap state:
`configuration/{buildings,categories,vlans,devices}.json` emptied to
valid-but-empty JSON (`{"Buildings":[]}` etc. - not deleted, since
`Seed()` fatally errors on a missing file), and
`resources/inventory.sqlite` rebuilt from scratch (deleted, then
`-seed` against the now-empty config) rather than having its rows
manually deleted - this doubles as a live confirmation that the
flattened schema actually produces a correct database from nothing.
`oui` (40,196-row IEEE vendor registry, vendored reference data, not
site data) was deliberately left populated - `-seed` always refreshes
it via `SeedOui` regardless of `-seed`'s site-data inputs.

Emptying the config to zero entries exposed a real, previously-latent
bug in `Seed()`: the buildings/categories/vlans inserts build one raw,
concatenated `INSERT ... VALUES (...), (...), ...` string by hand
(unlike the parameterized `devices` insert) - with zero rows, that left
a bare `INSERT ... VALUES` with no tuple at all, which SQLite correctly
rejects as invalid. Fixed by skipping each of those three `Exec` calls
entirely when its source list is empty (the parameterized `devices`
loop already handled zero rows fine, since it simply doesn't iterate).

`documentation/TODO.md`'s own "Housekeeping" list still carries
`configuration/preferences.json`'s dead `diffs_dir` personal-path value
as a separate, pre-existing item - intentionally not touched by this
reset, since it's a tuning/preferences concern, not device/building/
category/vlan site data. (`diffs_dir` and `sorted_by_category`, both unused by
any code, were deleted from `preferences.json` afterward.)

## Web UI login (2026-10, shared login done; per-user accounts TODO)

The UI used to have a Logout link but no authentication at all - every
page, config, and diff was open to anyone who could reach the port.
Now `internal/web/auth.go` puts one **shared login** (a single username
and password) in front of everything except `/login/`, `/logout/`, and
the static `.css`/`.js` files. A successful login sets an HttpOnly,
SameSite=Lax session cookie (`Secure` when served over TLS) holding an
expiry plus an HMAC-SHA256 signature; it lasts 12 hours. The signing
key is random per process, so a restart logs everyone out. `?next=` is
restricted to same-site paths (no open redirect), and bcrypt runs even
for a wrong username so timing doesn't reveal which half failed.

Configuration is flags/env only: `-auth-user` (default `admin`) and
`-auth-password-hash` or `SWITCHGOPHER_PASSWORD_HASH` (preferred - keeps
the hash out of the process list). Generate a hash with
`echo 'password' | switchgopher -hash-password`. The server **refuses to
start** with no valid hash rather than running open. New dependency:
`golang.org/x/crypto/bcrypt`.

Known limits: plain HTTP sends the password in clear text, so run it
behind TLS (or a TLS-terminating proxy) anywhere untrusted; there is no
login rate limit beyond a 500ms delay per failed attempt; logout is a
GET (so a third-party page can force a logout, a nuisance, not a
breach); and there is no CSRF token on state-changing requests, which
is moot today because the UI has no state-changing forms besides login.

**TODO: per-user accounts via LDAP or SSO.** A shared login gives no
audit trail and no way to revoke one person. For LDAP, replace the
bcrypt check in `LoginHandler` with a bind against the directory
(`github.com/go-ldap/ldap/v3`: search for the user's DN, then re-bind
with the submitted password) and optionally restrict access to a
group via `memberOf`; keep the same signed cookie, but store the
username in it. For SSO, drop the login form and use OIDC
(`github.com/coreos/go-oidc` plus `golang.org/x/oauth2`), or SAML if
the site requires it: redirect to the identity provider, validate the
returned ID token, and issue the same session cookie from the claims.
Either way, `Auth.Middleware` and the cookie layer stay as they are;
only credential verification changes.

## Docker (2026-10, done)

`Dockerfile` (multi-stage; `CGO_ENABLED=0` works because
`modernc.org/sqlite` is pure Go; runs as a non-root user), `.dockerignore`,
and `docker-compose.yml`. The only code change was a `-db` flag (default
`resources/inventory.sqlite`, unchanged for local runs) so the database can
live on a `/data` volume instead of in the disposable image; the image's
entrypoint passes `-db /data/inventory.sqlite`. `configuration/` is
bind-mounted (it is hand-edited input) and `resources/configs/` is a named
volume (saved device configs). The tracked `inventory.sqlite` is deliberately
*not* copied into the image - first run is
`docker compose run --rm switchgopher -seed`, which creates the schema,
seeds from `configuration/*.json`, and loads the OUI registry.

The web login hash comes from `.env` (gitignored; keep the single quotes
around the value, since a bcrypt hash contains `$`). The container has a
`wget` healthcheck against the public `/login/` page. Verified by building
the image, seeding a fresh volume, logging in over the published port, and
confirming the healthcheck and non-root user. Not verified: the
`docker-compose.yml` itself (only equivalent `docker run` commands were
exercised, since port 8000 may be in use by a local dev server), and SNMP
reachability from inside a container to real devices - it is outbound UDP
and should work on the default bridge network, but any device ACL that
allows only the host's address will need the container's NAT'd source
(or `network_mode: host`).

## inventory.sqlite no longer tracked (2026-10, done)

`resources/inventory.sqlite` (and its `-wal`/`-shm`/`-journal` files) is now
gitignored and removed from the index. It is fully rebuildable and, once in
real use, holds site data (MACs, IPs, hostnames, and the SNMP credentials
`Seed()` copies in), and a binary file that changes on every poll is a poor
fit for git. A fresh clone creates it with
`go run ./cmd/switchgopher -seed`. Earlier entries in this file that mention
testing against "the tracked" database describe how things worked at the
time.

## Example config files (2026-10, done)

The four site-data files (`configuration/{buildings,categories,vlans,devices}.json`)
are no longer tracked or shipped: they hold site data, and `devices.json`
holds SNMP credentials. Instead the repo ships `*.example.json` copies with
fictional entries (`HQ`/`LAB`, RFC 5737 documentation addresses,
`CHANGE-ME` placeholders, one v2c and one v3 device), plus `.env.example` for
the web login hash. Setup is `cp devices.example.json devices.json` (and the
same for the other three), then edit. The real files, and the discovery tool's
`configuration/discovered-*.json` output, are gitignored and also excluded by
`.dockerignore` so credentials are never baked into an image
(`docker-compose.yml` bind-mounts the directory at runtime).
`configuration/preferences.json` stays tracked - it only holds defaults.

`Seed()` now treats a missing config file as an empty list (logging it)
instead of failing, so a fresh clone can run `-seed` with no setup at all.
`seed_test.go` covers that, and also checks that every `*.example.json` is
valid `Seed()` input, including the building/category/VLAN cross-references.

## Preferences format (2026-10, done)

JSON has no comment syntax, so `configuration/preferences.json` documents
itself instead: every setting is an object with a `description` and a `value`.
The description ends with `; required: <bool>, type: <int|string>, default:
<value>`. **The loader reads only `value`** - the description is documentation
for people editing the file, so deleting it (or adding other fields) never
invalidates the config, and the required/type notes are not enforced.
`config.Load` flattens the file to a plain `name -> value` map, so the rest of
the code (`config.Int`, `config.String`, and the handlers that look a
directory up by name) is unchanged. An entry with no `value` falls back to the
default hard-coded at its call site, and a value of the wrong type is ignored
the same way (`config.Int`/`String` already did that). An unreadable or
malformed file is logged at startup and the app runs on those built-in
defaults.

This also put the four settings the code had always read but the file never
listed (`arp_poll_interval_seconds`, `dns_concurrency`,
`dns_lookup_timeout_seconds`, `dns_domain_suffix`) into the file, with the
same defaults as before.

## SNMP credentials from environment variables (2026-10, done)

A credential in `configuration/devices.json` can be a `${NAME}` placeholder
instead of the secret (`Community`, `Username`, `AuthPassphrase`,
`PrivPassphrase`). `Seed()` is unchanged and stores the placeholder text in
the `devices` table as-is, so **the database never holds the secret**. The
placeholder is expanded at poll time, in `buildSession` (`internal/scheduler/
secrets.go`), which every SNMP session goes through (device polls, ARP polls,
and the discovery tool's CDP crawl). A value with no `${` is used literally,
so existing plaintext credentials keep working. `$${` is an escape for a
literal `${`. A placeholder whose variable is unset or empty fails that
poll with an error naming the device, field, and variable (never the value),
rather than polling with an empty credential.

Consequences: the variables must be set in the server's environment whenever
it runs (`docker-compose.yml` already passes `.env` through; `.env.example`
lists the example ones), and rotating a credential means changing the variable
and restarting - no re-seed. The database file is no longer sensitive for
credentials, though it still holds MACs, IPs, and hostnames. Values that are
not placeholders remain plaintext in `devices.json` and SQLite, so migrate the
real files to placeholders before relying on this.

## Mobile browser support (TODO)

The UI has only been viewed in a desktop browser. It needs testing on a phone
and tablet browser, and mobile-specific styling is likely required. Known
problems from reading the code, not yet confirmed on a device:

- `base.css` gives `body` `min-width: 750px` and `.line` a fixed `width:
  670px`, so a phone viewport will scroll sideways.
- `_base.tmpl` has no `<meta name="viewport">` tag, so phones will render at
  desktop width and shrink the page.
- There are no `@media` queries anywhere in the stylesheets.
- The Ports table has 17 columns, and the nav bar is a single row of inline
  items with the search box in it; both are likely to overflow.

Likely work: add the viewport meta tag, replace the fixed widths with fluid
ones, make wide tables scroll horizontally inside their own container (or
collapse less useful columns on small screens), and wrap the nav on narrow
screens. Check every page, including login, Help and the per-switch tabs.

## First-run seeding and startup errors (2026-10, done)

A database that has never been seeded used to start empty until someone ran
`-seed`. Now any run other than `-seed` checks `store.NeedsSeed` - true when
the `oui` table is empty, which only a seed ever fills - and seeds
automatically (configuration plus the OUI list). That check is safe because a
never-seeded database has no polled data for `Seed()`'s `INSERT OR REPLACE` to
wipe. It happens once; after that, edits to `configuration/*.json` still need
an explicit `-seed`, which always re-seeds. A failed automatic seed is fatal,
with a message. Also, `main.go` used to ignore the result of `store.Setup()`;
a schema-setup failure (for example a read-only database file) now stops
startup with the error.

## Code review: cleanliness, bugs, security (2026-10)

Tools: `gofmt`, `go vet`, `staticcheck`, `govulncheck`, plus a manual read of
the SQL, file-serving, auth and server code.

Fixed:
- **`Seed()` built SQL by string concatenation** for buildings, categories
  and VLANs. A name containing a quote (`O'Brien Hall`) failed the seed, and
  the config files could inject SQL. All inserts are now parameterized.
- **File endpoints took any string preference as a folder.** The `folder`
  parameter is now limited to `config_dir` and `dated_config_dir`.
- **No HTTP server timeouts** (slow-client connection hogging). Added
  header/read/write/idle timeouts.
- **Session cookie was not `Secure` behind a TLS proxy.** It is now also
  marked `Secure` when the proxy sends `X-Forwarded-Proto: https`.
- **No SQLite busy timeout**, so the poller's writes could make web reads
  fail with "database is locked". `busy_timeout` is 5s.
- **Baseline response headers** added: `X-Content-Type-Options: nosniff`,
  `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`.
- **Cleanup:** removed unused `ensureColumn`/`dropColumn`, redundant `return`s,
  deprecated `io/ioutil`, and a copy-pasted handler comment. `staticcheck` is clean.
- Go bumped to 1.27.2 (go.mod, Dockerfile, README) and `golang.org/x/crypto`
  to v0.58.0. `govulncheck` now reports nothing that affects the code; before,
  it found 30 standard-library issues on Go 1.26.0.

Remaining, not changed:
- **`staticcheck`:** its latest release cannot yet analyze Go 1.27 code (it
  fails reading the compiler's export data), so the last clean run was on Go
  1.26. `go vet` passes on 1.27. Re-run it when a newer release supports 1.27.
- **Login:** logout is a GET (a third-party page can force a logout), and
  failed logins are only slowed by a 500ms delay, not rate limited.
- **Error text:** handlers return `err.Error()` to the browser on database
  errors, which can reveal internals to a logged-in user.
- **`getSelection`/`unusedSelection`** put a column name into SQL. Every
  caller passes a fixed literal, so it is safe today, but a new caller passing
  user input would be an injection.
- Only Go files have a formatter; the templates, CSS, JSON and Markdown are
  hand-formatted.

## Features switchmap has that SwitchGopher doesn't (yet)

- **Cabling/documentation hygiene audits** - `VerifyPortLabels.pm` and
  `WriteSuspiciousPortsFile.pm` flag port labels that don't match a
  controlled naming convention or a port's physical building. Org-specific
  logic (hardcoded building codes etc.) - would need a documented naming
  convention for this deployment before it's buildable at all.
- **PoE device allowlist auditing** - `WritePoePortsFile.pm` flags
  unapproved PoE devices/MACs against a configured allowlist. SwitchGopher
  now polls PoE state (`PortPoE`, Phase 2) but has no allowlist concept or
  auditing report built on top of it.
- **CSV/bulk export** - `WriteCsvDirectory.pm`. No export feature exists.

## Stretch goal: 802.1X user identity (2026-09, unverified)

Neither tool has this, and it's not part of any current plan - worth
exploring later. On a switch that implements 802.1X port authentication,
the actual authenticated *username* (not just a MAC) may be obtainable
via SNMP, not just inferable indirectly:

- **IEEE8021-PAE-MIB** (802.1X Port Access Entity MIB, standard) -
  `dot1xAuthSessionUserName` gives the authenticated identity (the
  RADIUS/EAP username) for a given port/session;
  `dot1xAuthAuthControlledPortStatus` (authorized/unauthorized) gives
  live per-port authentication state; `dot1xAuthSessionTable` gives
  session identifiers, typically alongside the authenticating MAC.
- Real caveats, not yet resolved:
  - Support is inconsistent across vendors and even across product
    lines from the same vendor - some switches only implement the
    authorized/unauthorized status table, not the username table.
  - Cisco has historically also exposed overlapping data through its
    own proprietary MIBs (e.g. pieces of `CISCO-AUTH-FRAMEWORK-MIB`, for
    its unified 802.1X/MAB/webauth framework) rather than relying solely
    on the standard PAE MIB - on Cisco gear, both may need checking.
- Nothing here has been vendored or verified against real MIB text -
  unlike everything else in this document, this is genuinely
  *unresearched*, not just unimplemented. Treat every object name above
  as a lead to verify, not a citation, before building anything.

## Neither tool has

- **Config pulling/diffing** - despite being in SwitchGopher's own 2014
  mockup (`SwitchGopher.pdf` never actually specced this page, but the app
  grew Config/Diffs tabs anyway), no config-fetching code exists in
  switchmap's codebase either. SwitchGopher is arguably ahead here: the UI
  (graceful failure, external-directory display, a real diff algorithm) is
  built and working (see `resources/configs/README.md`), just not the fetch
  mechanism. Two known paths forward, deliberately deferred:
  - SNMP-triggered TFTP backup via `CISCO-CONFIG-COPY-MIB` (fits the
    project's SNMP-everything approach, but is a real *write* operation to
    network hardware - bigger blast radius than anything built so far, and
    entirely unvalidated without real gear).
  - SSH pull (`show running-config`) - needs per-device SSH credentials
    (schema change) and a new dependency (`golang.org/x/crypto/ssh`).

## Housekeeping / small items

- **Real hardware validation** - everything has only been checked against a
  local net-snmp `snmpd` simulator (can't exercise FDB/CDP tables at all)
  plus unit tests. Per explicit decision, this is an *accepted deferred
  risk*, not a blocker - no Cisco hardware is available to test against
  right now. Don't treat this as something to chase; revisit if/when real
  gear becomes available.
