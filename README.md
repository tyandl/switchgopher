# SwitchGopher

SwitchGopher polls network switches over SNMP and serves a web UI showing
what is plugged into each port: link state, speed, VLAN, PoE, CDP neighbors,
and the MAC address, IP address and hostname of the device on the other end.
It is a Go rewrite of the Perl tool switchmap.
It only reads from your network; it never changes a switch.

Pages: Devices (browse and filter switches), per-switch Ports / Info /
Configs / Diffs, Vlans, Stats, Unused ports, and search by MAC or hostname.
The in-app Help page explains how to read them.

## Screenshots

Taken against fictional mock data. All 17 page captures are in
[`documentation/screenshots`](documentation/screenshots).

**Home**

![Home page](documentation/screenshots/01-home.png)

**Devices:** browse and filter switches by category, building and room.

![Devices page](documentation/screenshots/02-devices.png)

**Ports:** one row per port, with the device seen on it (vendor, MAC, IP, hostname).

![Ports page for one switch](documentation/screenshots/03-ports.png)

**Port details:** one port's settings and traffic counters.

![Port detail page](documentation/screenshots/04-port-detail.png)

**Info:** model, firmware, location and uptime for one switch, plus its modules.

![Switch info page](documentation/screenshots/06-info.png)

**Stats:** network-wide totals.

![Stats page](documentation/screenshots/13-stats.png)

## Quick start

Requires Go 1.27 or newer.

```sh
# 1. Site configuration: copy each example and edit it for your network.
for f in buildings categories vlans devices; do
  cp configuration/$f.example.json configuration/$f.json
done

# 2. Make a login hash (see "Login" below), then start the server. The first
#    start creates the database and loads the configuration plus the
#    MAC-vendor list automatically.
echo 'choose-a-password' | go run ./cmd/switchgopher -hash-password 2>/dev/null
SWITCHGOPHER_PASSWORD_HASH='<paste the hash>' go run ./cmd/switchgopher
```

Open <http://localhost:8000/>. After changing the files in `configuration/`,
run `go run ./cmd/switchgopher -seed` to reload them (a database that has
already been seeded is not re-seeded automatically). Switches are polled every 5 minutes by default, so pages
fill in after the first poll.

**Don't have a device list?** `-discover` sweeps a management subnet over
SNMP (or crawls one hop of CDP neighbors with `-discover-cdp=<device>`) and
writes candidates to `configuration/discovered-devices.json` for you to
review and copy into `devices.json`. It prompts for anything it needs unless
you pass flags such as `-discover-subnet`; `-discover-pattern` extracts
building/room/model from device names with a regex. Run
`go run ./cmd/switchgopher -help` for every `-discover-*` flag.

## Configuration

| File | Purpose |
|---|---|
| `configuration/buildings.json` | Building names and short codes |
| `configuration/categories.json` | Device categories (also the config-folder names) |
| `configuration/vlans.json` | VLAN IDs, names, subnets |
| `configuration/devices.json` | The switches to poll, with their SNMP credentials |
| `configuration/preferences.json` | Polling intervals, retention, paths, DNS/ARP settings |

The first four hold site data and credentials, so only the
`*.example.json` copies are tracked; the real files are gitignored. Each
example has a `Notes` entry explaining its fields; the app ignores `Notes`.

`preferences.json` is tracked. Each setting is an object with a
`description` (which ends in `required`, `type` and `default`) and a `value`.
The app reads only `value`, so you can delete the descriptions. Remove a
`value` to fall back to the built-in default.

### SNMP credentials

`devices.json` supports SNMPv2c (`Community`) and SNMPv3 (`SnmpVersion: "3"`
plus `Username`, `SecurityLevel`, `AuthProtocol`, `AuthPassphrase`,
`PrivProtocol`, `PrivPassphrase`); the two can be mixed per device.

To keep secrets out of files and the database, write a credential as a
`${NAME}` placeholder:

```json
{ "Device": "HQ-R1-3560-01", "Community": "${CORE_COMMUNITY}" }
```

The database stores the placeholder as written, and the value is read from
the environment variable `NAME` each time the device is polled. This applies
to `Community`, `Username`, `AuthPassphrase` and `PrivPassphrase`.

- A variable that is unset or empty makes that device's poll fail with an
  error naming the device and variable.
- Write `$${` for a literal `${`.
- Plain values still work, but are stored in plaintext in `devices.json` and
  the database.
- The variables must be set in the server's environment whenever it runs.
  To rotate a credential, change the variable and restart; no re-seed is
  needed.

### Login

The web UI sits behind one shared username and password, using a signed
session cookie that lasts 12 hours. The server refuses to start without a
password hash.

- Generate a bcrypt hash: `echo 'password' | go run ./cmd/switchgopher -hash-password`
- Provide it with `SWITCHGOPHER_PASSWORD_HASH` (preferred, keeps it out of
  the process list) or `-auth-password-hash`. Single-quote it in a shell,
  because the hash contains `$`.
- The username is `admin` unless you pass `-auth-user`.

Traffic is plain HTTP, so run it behind TLS anywhere untrusted. A TLS-terminating
proxy should send `X-Forwarded-Proto: https`, which marks the session cookie `Secure`. Per-user
accounts (LDAP or SSO) are on the to-do list.

## Docker

```sh
cp .env.example .env    # set SWITCHGOPHER_PASSWORD_HASH and the credential variables
docker compose up -d
```

The first start seeds the database automatically. After editing the files in
`configuration/`, reload them with `docker compose run --rm switchgopher -seed`.

The database lives in a volume at `/data`, `configuration/` is bind-mounted,
and saved device configs use a second volume. The real configuration files
are excluded from the image. If your devices only accept SNMP from the
host's address, you may need `network_mode: host`.

## Command-line flags

| Flag | Meaning |
|---|---|
| `-seed` | Load `configuration/*.json` and the OUI list into the database, then exit |
| `-port` | HTTP port (default 8000) |
| `-db` | SQLite file (default `resources/inventory.sqlite`) |
| `-auth-user`, `-auth-password-hash` | Login username and bcrypt hash |
| `-hash-password` | Print a bcrypt hash for a password read from stdin, then exit |
| `-pollonce=<device>` | Poll one device once and exit (handy for testing) |
| `-discover` and `-discover-*` | Device discovery; see above |

## Development

```sh
go build ./... && go vet ./... && go test ./...
```

Design notes, decisions and open items live in
[`documentation/TODO.md`](documentation/TODO.md). Vendor-specific SNMP
support is pluggable; see `internal/snmp/drivers/README.md`.
Not yet validated against real Cisco hardware, only a local net-snmp
simulator and unit tests.
