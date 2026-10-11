# MAC-vendor (OUI) lookup data

`oui.csv` is a vendored snapshot of the IEEE's public MA-L (24-bit,
classic) OUI registry, used to resolve a MAC address's first 3 octets to
the manufacturer that registered them (the `NIC` column on the Ports
report, Node History, and single Port page).

## Source

`https://standards-oui.ieee.org/oui/oui.csv` - format:
`Registry,Assignment,Organization Name,Organization Address`, one row per
assignment (e.g. `MA-L,E80AB9,"Cisco Systems, Inc",...`). `internal/oui`
only reads the `Assignment` and `Organization Name` columns.

## Scope

Only `MA-L` (the classic 24-bit OUI, the first 3 MAC octets) is used -
matching the scope of switchmap's own `OuiCodes.pm`/`OuiCodes.txt`
reference design (read directly from the real Perl source this project
rewrites, not worked from memory). The newer, much less common 28-bit
(`MA-M`) and 36-bit (`MA-S`) extended allocations aren't looked up - a
known, documented limitation, not an oversight. A MAC whose OUI falls in
one of those ranges will simply resolve to no organization name.

## Refreshing

Re-download the file from the URL above and replace `oui.csv` - no
automated updater, same "manually refreshed, documented process" as
`resources/mibs/README.md`'s own MIB refresh instructions. The IEEE's
list grows continuously as new blocks are assigned; there's no fixed
refresh cadence, just re-fetch when MAC-vendor lookups start missing
newer hardware.
