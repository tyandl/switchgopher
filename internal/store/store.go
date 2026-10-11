// Package store owns the sqlite connection, schema, and all reads/writes -
// both the seed-data schema setup and the poller's write-back after each
// SNMP poll.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/oui"
)

// Connect attempts to connect to the database and returns the connection
// pointer and an error code. path is the SQLite file (created if missing).
func Connect(path string) (*sql.DB, error) {
	// busy_timeout makes a writer (the poller) and a reader (the web UI) wait
	// for each other instead of failing at once with "database is locked".
	// Set up a connection to the db. Connection will be established later.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", path, err)
	}
	// Pinging the db should establish a connection.
	err = db.Ping()
	if err != nil {
		return db, fmt.Errorf("connecting to database %s: %w", path, err)
	}
	// attempt to set foreign key constraint on.
	_, err = db.Exec("PRAGMA foreign_keys = YES")
	if err != nil {
		// failing to set the foreign key constraint is a fatal error to protect the
		// database's integrity...
		return db, fmt.Errorf("enabling foreign keys: %w", err)
	}
	return db, nil
}

// Setup applies the schema to the database if it is not already set up.
// this should be considered testing code and NOT used in production.
// TODO this should use a preconfigured "template" database that gets copied instead...
func Setup(database *sql.DB) error {
	var Result string
	// to verify that a table already exists, we will start by assuming it does not.
	// tables contains a map of table names to their construction command. We will query
	// the master table for the names of all existing tables and remove all matches leaving
	// only a list of tables that need to be created.
	tables := map[string]string{
		"deviceTypes": deviceTypes,
		"categories":  categories,
		"buildings":   buildings,
		"vlans":       vlans,
		"devices":     devices,
		"modules":     modules,
		"ports":       ports,
		"nodes":       nodes,
		"oui":         ouiSchema,
	}
	// query the master table for the names of all existing tables
	rows, err := database.Query(`SELECT name FROM sqlite_master WHERE type='table';`)
	if err != nil {
		return fmt.Errorf("listing existing tables: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&Result); err != nil {
			return fmt.Errorf("listing existing tables: %w", err)
		}
		delete(tables, Result)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("listing existing tables: %w", err)
	}
	// execute the creation of all non-existing tables
	for k, v := range tables {
		_, err = database.Exec(v)
		if err != nil {
			return fmt.Errorf("creating table %s: %w", k, err)
		}
	}

	// Migrations flattened (2026-10): nothing had been deployed, so every column
	// that used to be added by an ALTER TABLE is now in the CREATE TABLE strings
	// above. Once a release exists with real data, a schema change needs a
	// migration step here (PRAGMA table_info to check, then ALTER TABLE).

	return nil
}

/** DATABASE STUFF! */

const deviceTypes = `CREATE TABLE deviceTypes (
	DeviceType TEXT NOT NULL,
	PRIMARY KEY (DeviceType)
)`

const categories = `CREATE TABLE categories (
	Category TEXT NOT NULL,
	PRIMARY KEY (Category)
)`

const buildings = `CREATE TABLE buildings (
	Building TEXT NOT NULL,
	ShortName TEXT,
	Description TEXT,
	PRIMARY KEY (Building)
)`

const vlans = `CREATE TABLE vlans (
	Vlan INTEGER NOT NULL,
	Name TEXT,
	Subnet TEXT,
	Source TEXT NOT NULL DEFAULT 'config',
	PRIMARY KEY (Vlan)
)`

const devices = `CREATE TABLE devices (
	Device TEXT NOT NULL,
	Category TEXT,
	Building TEXT,
	Room TEXT,
	ModelNum TEXT,
	Num TEXT,
	Address TEXT NOT NULL,
	Community TEXT NOT NULL,
	Type TEXT NOT NULL DEFAULT ('generic'),
	ChassisModel TEXT,
	ProductDescription TEXT,
	ProductName TEXT,
	SysName TEXT,
	SysObjectID TEXT,
	SysContact TEXT,
	SysLocation TEXT,
	SysUptime INTEGER,
	SerialNum TEXT,
	HardwareVersion TEXT,
	FirmwareVersion TEXT,
	Temp REAL,
	Cpu INTEGER,
	Memory INTEGER,
	ConfigUpdated INTEGER,
	StpTopChanges INTEGER,
	IsLayer3 INTEGER,
	IsRouter INTEGER NOT NULL DEFAULT 0,
	SnmpVersion TEXT NOT NULL DEFAULT '2c',
	Username TEXT,
	SecurityLevel TEXT,
	AuthProtocol TEXT,
	AuthPassphrase TEXT,
	PrivProtocol TEXT,
	PrivPassphrase TEXT,
	ContextName TEXT,
	PRIMARY KEY (Device),
	FOREIGN KEY (Type) REFERENCES deviceTypes(DeviceType)
)`

const modules = `CREATE TABLE modules (
	Module INTEGER NOT NULL,
	Device TEXT NOT NULL,
	Description TEXT,
	Model TEXT,
	FwVersion TEXT,
	HwVersion TEXT,
	SwVersion TEXT,
	SerialNum TEXT,
	PRIMARY KEY (Module, Device),
	FOREIGN KEY (Device) REFERENCES devices(Device)
)`

const ports = `CREATE TABLE ports (
	Port INTEGER NOT NULL,
	Device TEXT NOT NULL,
	State INTEGER,
	AdminState INTEGER,
	Speed INTEGER,
	Duplex INTEGER,
	IsTrunk INTEGER,
	IfType INTEGER,
	Since INTEGER,
	Vlan INTEGER,
	AuxVlan INTEGER,
	Label TEXT,
	PoeActive INTEGER,
	PoeClass TEXT,
	AggregatePort INTEGER,
	InPackets INTEGER,
	OutPackets INTEGER,
	InErrors INTEGER,
	OutErrors INTEGER,
	InDiscards INTEGER,
	OutDiscards INTEGER,
	PRIMARY KEY (Port, Device),
	FOREIGN KEY (Device) REFERENCES devices(Device),
	FOREIGN KEY (Vlan) REFERENCES vlans(Vlan),
	FOREIGN KEY (AuxVlan) REFERENCES vlans(Vlan)
)`

const nodes = `CREATE TABLE nodes (
	Mac INTEGER NOT NULL,
	Ip TEXT NOT NULL,
	Hostname TEXT,
	TimeStamp INTEGER NOT NULL,
	Port INTEGER,
	Device TEXT,

	CdpDeviceId TEXT,
	CdpDevicePort TEXT,
	CdpPlatform TEXT,
	PRIMARY KEY (Mac, TimeStamp),
	FOREIGN KEY (Port, Device) REFERENCES ports(Port, Device)
)`

// oui is reference data, not site config - a vendored snapshot of the
// IEEE's public MA-L registry (see resources/oui/README.md), refreshed by
// SeedOui re-importing resources/oui/oui.csv, not by hand-editing rows.
// Report queries join against it directly (Mac's first 3 octets, upper-
// cased and de-colonized, against Oui) rather than looking it up in Go.
const ouiSchema = `CREATE TABLE oui (
	Oui TEXT NOT NULL,
	OrgName TEXT NOT NULL,
	PRIMARY KEY (Oui)
)`

// Seed (re)populates the static inventory tables - buildings, categories,
// vlans, devices - from the JSON files under configuration/. This is what
// lets the database be recreated from scratch: run it after Setup on an
// empty (or lost/deleted) database to restore the inventory that polling
// depends on.
//
// It deliberately does NOT touch ports or nodes: those hold SNMP-derived
// state (see internal/scheduler), not configuration, and config files carry
// nothing for them. Because devices is repopulated via INSERT OR REPLACE
// with only the config-file columns, re-running Seed against a database that
// already has polled data will blank out that device's identification
// fields (ChassisModel, SysObjectID, etc.) until the next poll cycle refills
// them - expected for a "rebuild the inventory" operation, not a bug.
func Seed(database *sql.DB) error {
	type Building struct {
		Building    string
		ShortName   string
		Description string
	}
	type Category struct {
		Category string
	}
	type Device struct {
		Device    string
		Category  string
		Building  string
		Room      string
		Model     string
		Index     string
		Address   string
		Community string
		IsRouter  bool // optional; identifies a device to ARP-poll for nodes.Ip

		// SNMPv3 (all optional; absent/empty = today's v2c behavior via
		// Community above). See internal/scheduler's buildSession for how
		// these map to gosnmp's v3 API, and documentation/TODO.md for the
		// expected SecurityLevel/AuthProtocol/PrivProtocol value strings.
		SnmpVersion    string
		Username       string
		SecurityLevel  string
		AuthProtocol   string
		AuthPassphrase string
		PrivProtocol   string
		PrivPassphrase string
		ContextName    string
	}
	type Vlan struct {
		Vlan   int
		Name   string
		Subnet string
	}

	var buildingsFile struct{ Buildings []Building }
	var categoriesFile struct{ Categories []Category }
	var vlansFile struct{ Vlans []Vlan }
	var devicesFile struct{ Devices []Device }

	for _, f := range []struct {
		path string
		dest any
	}{
		{"configuration/buildings.json", &buildingsFile},
		{"configuration/categories.json", &categoriesFile},
		{"configuration/vlans.json", &vlansFile},
		{"configuration/devices.json", &devicesFile},
	} {
		file, err := os.Open(f.path)
		if errors.Is(err, os.ErrNotExist) {
			// The real files are gitignored (only *.example.json is
			// shipped), so a missing one just means "nothing configured".
			slog.Warn("config file not found; treating it as empty", "file", f.path)
			continue
		}
		if err != nil {
			return fmt.Errorf("opening %s: %w", f.path, err)
		}
		err = json.NewDecoder(file).Decode(f.dest)
		file.Close()
		if err != nil {
			return fmt.Errorf("decoding %s: %w", f.path, err)
		}
	}

	if _, err := database.Exec(`
	INSERT OR REPLACE INTO deviceTypes (DeviceType)
	VALUES
	('generic');
	`); err != nil {
		return fmt.Errorf("inserting into deviceTypes: %w", err)
	}

	// Every insert below is parameterized: names come from the config files
	// and may contain quotes (O'Brien Hall), which must be data, never SQL.
	for _, bldg := range buildingsFile.Buildings {
		if _, err := database.Exec(`INSERT OR REPLACE INTO buildings (Building, ShortName, Description) VALUES (?, ?, ?)`,
			bldg.Building, bldg.ShortName, bldg.Description); err != nil {
			return fmt.Errorf("inserting building %s: %w", bldg.Building, err)
		}
	}
	for _, cat := range categoriesFile.Categories {
		if _, err := database.Exec(`INSERT OR REPLACE INTO categories (Category) VALUES (?)`, cat.Category); err != nil {
			return fmt.Errorf("inserting category %s: %w", cat.Category, err)
		}
	}
	for _, vlan := range vlansFile.Vlans {
		if _, err := database.Exec(`INSERT OR REPLACE INTO vlans (Vlan, Name, Subnet, Source) VALUES (?, ?, ?, 'config')`,
			vlan.Vlan, vlan.Name, vlan.Subnet); err != nil {
			return fmt.Errorf("inserting vlan %d: %w", vlan.Vlan, err)
		}
	}

	devicesStmt, err := database.Prepare(`
		INSERT OR REPLACE INTO devices (
			Device, Category, Building, Room, ModelNum, Num, Address, Community, IsRouter,
			SnmpVersion, Username, SecurityLevel, AuthProtocol, AuthPassphrase, PrivProtocol, PrivPassphrase, ContextName
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing devices insert: %w", err)
	}
	defer devicesStmt.Close()

	for _, dev := range devicesFile.Devices {
		isRouter := 0
		if dev.IsRouter {
			isRouter = 1
		}
		snmpVersion := dev.SnmpVersion
		if snmpVersion == "" {
			snmpVersion = "2c"
		}
		if _, err := devicesStmt.Exec(
			dev.Device, dev.Category, dev.Building, dev.Room, dev.Model, dev.Index, dev.Address, dev.Community, isRouter,
			snmpVersion, dev.Username, dev.SecurityLevel, dev.AuthProtocol, dev.AuthPassphrase, dev.PrivProtocol, dev.PrivPassphrase, dev.ContextName,
		); err != nil {
			return fmt.Errorf("inserting device %s: %w", dev.Device, err)
		}
	}

	slog.Info("seeded site data",
		"buildings", len(buildingsFile.Buildings), "categories", len(categoriesFile.Categories),
		"vlans", len(vlansFile.Vlans), "devices", len(devicesFile.Devices))
	return nil
}

// NeedsSeed reports whether the database has never been seeded. -seed always
// loads the OUI registry and nothing else writes that table, so an empty oui
// table means no seed has run yet - a safe signal to seed automatically,
// since a database that has never been seeded has no polled data for
// Seed()'s INSERT OR REPLACE to wipe.
func NeedsSeed(database *sql.DB) (bool, error) {
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM oui`).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}

// SeedOui bulk-imports resources/oui/oui.csv (see internal/oui.ParseCSV)
// into the oui table. Separate from Seed - it's vendored reference data
// refreshed by re-downloading the IEEE registry, not site configuration,
// and at ~40,000 rows it needs a prepared statement in one transaction
// rather than Seed's build-one-big-INSERT-string approach. INSERT OR
// REPLACE is safe here (unlike the documented Seed() bug for
// devices/vlans/buildings/categories) because oui has only the two columns
// this import ever sets - there's no separately-polled column for a
// conflicting row to wipe.
func SeedOui(database *sql.DB, csvPath string) error {
	vendors, err := oui.ParseCSV(csvPath)
	if err != nil {
		return err
	}

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO oui (Oui, OrgName) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for code, org := range vendors {
		if _, err := stmt.Exec(code, org); err != nil {
			return fmt.Errorf("inserting oui %s: %w", code, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("seeded OUI vendor codes", "count", len(vendors))
	return nil
}

/** POLLER WRITE-BACK. Called from scheduler.pollDevice inside one *sql.Tx
per device, so a failed poll never leaves half-updated rows. */

// UpsertDevice refreshes the identification fields on an already-seeded
// devices row. It does not INSERT - devices are expected to already exist
// (seeded from configuration/devices.json), and polling only updates what
// SNMP told us about them.
func UpsertDevice(tx *sql.Tx, dev model.Device) error {
	_, err := tx.Exec(`
		UPDATE devices SET
			ChassisModel = ?,
			ProductDescription = ?,
			ProductName = ?,
			SysName = ?,
			SysObjectID = ?,
			SysContact = ?,
			SysLocation = ?,
			SysUptime = ?,
			SerialNum = ?,
			HardwareVersion = ?,
			FirmwareVersion = ?,
			Temp = ?,
			StpTopChanges = ?,
			IsLayer3 = ?,
			Cpu = ?,
			Memory = ?,
			ConfigUpdated = ?
		WHERE Device = ?`,
		dev.ChassisModel, dev.ProductDescription, dev.ProductName,
		dev.SysName, dev.SysObjectID, dev.SysContact, dev.SysLocation, dev.SysUptime,
		dev.SerialNum, dev.HardwareVersion, dev.FirmwareVersion,
		dev.Temp, dev.StpTopChanges, dev.IsLayer3,
		dev.Cpu, dev.Memory, dev.ConfigUpdated,
		dev.Device,
	)
	return err
}

// nullableInt converts a model.Port string field that's really an integer
// (Vlan/AuxVlan/AggregatePort - all "" when not populated) to nil for an
// empty string, or the parsed int otherwise. ports.Vlan/AuxVlan carry a
// FOREIGN KEY REFERENCES vlans(Vlan), so an empty string there isn't just
// wrong, it would violate that constraint; ports.AggregatePort has no such
// FK, but binding "" into its INTEGER column still stores a literal empty
// TEXT value instead of NULL (SQLite's type affinity doesn't force the
// conversion), which then fails to scan into a sql.NullInt64 - confirmed
// live (2026-09) the first time a real poll (not hand-crafted test data)
// ever exercised this path.
func nullableInt(value string) any {
	if value == "" {
		return nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return n
}

// nullableSince converts model.Port's Since (0 when unavailable - see its
// doc comment) to nil rather than storing a literal Unix-epoch (1970)
// timestamp, which would otherwise render as a nonsensical "01/01/1970" on
// the Ports report instead of a blank cell.
func nullableSince(unix int64) any {
	if unix == 0 {
		return nil
	}
	return unix
}

// nullableString converts an optional model.Port string field (empty when
// not populated) to nil for an empty string, or the string itself
// otherwise - same "empty means NULL" convention as nullableInt/
// nullableSince, for a TEXT column instead of an INTEGER one (PoeClass:
// "class0".."class4" text labels, not numbers - see
// internal/snmp/drivers/generic/portpoe.go's poeClassNames).
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ReplacePorts swaps in a fresh snapshot of a device's ports. Full
// delete+reinsert is simpler and more correct than diffing here, since ports
// are a point-in-time SNMP snapshot, not an append-only history like nodes.
func ReplacePorts(tx *sql.Tx, device string, ports []model.Port) error {
	if _, err := tx.Exec(`DELETE FROM ports WHERE Device = ?`, device); err != nil {
		return err
	}

	// ports.Vlan/AuxVlan carry a FOREIGN KEY REFERENCES vlans(Vlan) - a
	// vendor-reported vlan ID (e.g. Cisco's vmVlan) has no guarantee a
	// vlans row already exists for it (config may not have declared it,
	// and Q-BRIDGE-MIB vlan-name discovery is a separate, independent
	// walk that may not cover the same ID) - inserting a port that
	// references one would otherwise violate the constraint and roll
	// back the whole poll. Backfilling a nameless placeholder row first
	// (INSERT OR IGNORE - a no-op against any vlan that already has one,
	// from config or prior discovery) guarantees the constraint holds
	// regardless of insert order.
	seenVlans := map[int]bool{}
	for _, p := range ports {
		for _, v := range []string{p.Vlan, p.AuxVlan} {
			id, ok := nullableInt(v).(int)
			if !ok || seenVlans[id] {
				continue
			}
			seenVlans[id] = true
			if _, err := tx.Exec(`INSERT OR IGNORE INTO vlans (Vlan, Source) VALUES (?, 'discovered')`, id); err != nil {
				return err
			}
		}
	}

	for _, p := range ports {
		portNum, err := strconv.Atoi(p.Port)
		if err != nil {
			// Non-numeric ifIndex shouldn't happen in practice - skip rather
			// than fail the whole poll over one malformed row.
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO ports (
				Port, Device, State, AdminState, Speed, Duplex, IsTrunk, IfType, Since, Vlan, AuxVlan, Label,
				PoeActive, PoeClass, AggregatePort,
				InPackets, OutPackets, InErrors, OutErrors, InDiscards, OutDiscards
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			portNum, device, p.State, p.AdminState, p.Speed, p.Duplex, p.IsTrunk, p.IfType, nullableSince(p.Since), nullableInt(p.Vlan), nullableInt(p.AuxVlan), p.Label,
			p.PoeActive, nullableString(p.PoeClass), nullableInt(p.AggregatePort),
			p.InPackets, p.OutPackets, p.InErrors, p.OutErrors, p.InDiscards, p.OutDiscards,
		); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceModules swaps in a fresh snapshot of a device's line-card
// modules, mirroring ReplacePorts' delete+reinsert pattern exactly - a
// module inventory is a point-in-time SNMP snapshot, same reasoning as
// ports.
func ReplaceModules(tx *sql.Tx, device string, modules []model.Module) error {
	if _, err := tx.Exec(`DELETE FROM modules WHERE Device = ?`, device); err != nil {
		return err
	}
	for _, m := range modules {
		moduleNum, err := strconv.Atoi(m.Module)
		if err != nil {
			// Non-numeric entPhysicalIndex shouldn't happen in practice -
			// skip rather than fail the whole poll over one malformed row.
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO modules (Module, Device, Description, Model, FwVersion, HwVersion, SwVersion, SerialNum)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			moduleNum, device, m.Description, m.Model, m.FwVersion, m.HwVersion, m.SwVersion, m.SerialNum,
		); err != nil {
			return err
		}
	}
	return nil
}

// UpsertDiscoveredVlan records a vlan ID a poll observed with no row in the
// vlans table at all - typically from a future per-device vlan-name walk
// (e.g. Q-BRIDGE-MIB's dot1qVlanStaticTable) turning up an ID with no entry
// in configuration/vlans.json. Note this only ever fires for an
// undocumented vlan: a vlan.json entry - even one with an empty Name -
// already has a row (Source = 'config') by the time any poll runs, so this
// never touches it.
//
// An empty name is deliberately skipped rather than inserted: a discovered
// row is meant to lock in on the first non-empty name any switch reports
// for that ID, so a poll that hasn't yet resolved a name shouldn't get to
// "win" that slot with blank. Once a discovered row exists, INSERT OR
// IGNORE (vlans.Vlan is the primary key) makes every later call for the
// same ID a no-op - so if two switches disagree on an undocumented vlan's
// name, whichever names it first is authoritative from then on and every
// later call is silently dropped. This function alone doesn't surface that
// disagreement (nothing here compares a dropped call's name against what's
// stored) - see documentation/TODO.md for that as an open follow-up.
//
// Not called anywhere yet; no poller code discovers vlan names or IDs this
// pass.
func UpsertDiscoveredVlan(tx *sql.Tx, vlan int, name string) error {
	if name == "" {
		return nil
	}
	_, err := tx.Exec(`
		INSERT OR IGNORE INTO vlans (Vlan, Name, Source)
		VALUES (?, ?, 'discovered')`,
		vlan, name,
	)
	return err
}

// InsertNodes appends newly-seen MAC sightings, but only when something
// actually changed since that MAC's last recorded sighting (anywhere, not
// just on this device) - port, IP, hostname, or CDP info. Without this,
// every poll cycle would insert a near-identical row for every MAC on every
// port forever, since nodes.TimeStamp being part of the primary key is what
// makes this a history table rather than a live-state one (the same
// behavior switchmap itself has) - so something has to keep that history
// proportional to actual network change instead of to poll count. See
// sightingUnchanged for the comparison, Prune for age-based deletion (the
// other periodic half of keeping this table's growth bounded), and
// enforceMaxPerMac for the third leg - a hard per-MAC cap enforced right
// here at write time rather than by a periodic sweep, so the per-MAC count
// never even temporarily exceeds it between Prune runs.
//
// nodes.Port has a composite foreign key to ports(Port, Device): a node whose
// dot1dBasePortIfIndex mapping failed (fell back to a raw base-port number
// that isn't an actual ifIndex) would violate that FK and roll back the
// *whole* per-device transaction, including otherwise-good port/device data.
// ports is the just-written snapshot for this device, used to silently skip
// any such unmappable node rather than let one bad row sink the whole poll.
func InsertNodes(tx *sql.Tx, device string, nodes []model.Node, ports []model.Port, maxPerMac int) error {
	validPorts := make(map[string]bool, len(ports))
	for _, p := range ports {
		validPorts[p.Port] = true
	}

	for _, n := range nodes {
		if !validPorts[n.Port] {
			continue
		}
		portNum, err := strconv.Atoi(n.Port)
		if err != nil {
			continue
		}

		unchanged, err := sightingUnchanged(tx, n, device, portNum)
		if err != nil {
			return err
		}
		if unchanged {
			continue
		}

		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO nodes
				(Mac, Ip, Hostname, TimeStamp, Port, Device, CdpDeviceId, CdpDevicePort, CdpPlatform)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.Mac, n.Ip, n.Hostname, n.TimeStamp.Unix(), portNum, device,
			n.CdpDeviceId, n.CdpDevicePort, n.CdpPlatform,
		); err != nil {
			return err
		}
		if err := enforceMaxPerMac(tx, n.Mac, maxPerMac); err != nil {
			return err
		}
	}
	return nil
}

// sightingUnchanged reports whether n's most recent recorded sighting
// (anywhere, not just on this device - a MAC moving to a different switch
// is itself a change worth recording) already matches what was just
// observed, so InsertNodes can skip writing a redundant row.
func sightingUnchanged(tx *sql.Tx, n model.Node, device string, portNum int) (bool, error) {
	var lastDevice, lastIp, lastHostname, lastCdpID, lastCdpPort, lastCdpPlatform sql.NullString
	var lastPort sql.NullInt64
	row := tx.QueryRow(`
		SELECT Device, Port, Ip, Hostname, CdpDeviceId, CdpDevicePort, CdpPlatform
		FROM nodes WHERE Mac = ?
		ORDER BY TimeStamp DESC LIMIT 1`,
		n.Mac,
	)
	err := row.Scan(&lastDevice, &lastPort, &lastIp, &lastHostname, &lastCdpID, &lastCdpPort, &lastCdpPlatform)
	if err == sql.ErrNoRows {
		return false, nil // never seen before - definitely a change
	}
	if err != nil {
		return false, err
	}
	return lastDevice.String == device &&
		lastPort.Int64 == int64(portNum) &&
		lastIp.String == n.Ip &&
		lastHostname.String == n.Hostname &&
		lastCdpID.String == n.CdpDeviceId &&
		lastCdpPort.String == n.CdpDevicePort &&
		lastCdpPlatform.String == n.CdpPlatform, nil
}

// Prune deletes node sightings older than keep. Dedup in InsertNodes slows
// this table's growth, but doesn't stop it - a MAC that keeps moving around,
// or just enough distinct devices over a long enough time, still grows
// without bound. Returns the number of rows removed.
func Prune(db *sql.DB, keep time.Duration) (int64, error) {
	cutoff := time.Now().Add(-keep).Unix()
	result, err := db.Exec(`DELETE FROM nodes WHERE TimeStamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateNodeIPs backfills nodes.Ip from a router's ARP table
// (mac -> ip). Deliberately updates every historical sighting row for a
// MAC, not just the most recent - nodes has no separate "current known
// state" concept, and this project doesn't track per-sighting historical
// IP changes, so this is a documented simplification (a very old
// sighting could end up showing a MAC's *current* IP rather than
// whatever it had back then), consistent with this project's existing
// tolerance for "good enough, not perfectly time-accurate" elsewhere
// (e.g. ChassisModel). Can only enrich existing sightings - nodes rows
// require a valid (Port, Device) FK, so ARP data alone can never create
// a new node row for a MAC with no FDB-observed sighting on any
// monitored switch.
func UpdateNodeIPs(db *sql.DB, arp map[string]string) error {
	for mac, ip := range arp {
		if _, err := db.Exec(`UPDATE nodes SET Ip = ? WHERE Mac = ?`, ip, mac); err != nil {
			return err
		}
	}
	return nil
}

// UpdateNodeHostnames backfills nodes.Hostname from a reverse-DNS lookup
// pass (mac -> hostname, see internal/dns.Resolve). Same "update every
// historical sighting row for a MAC" simplification, and the same
// existing-sighting-only limitation, as UpdateNodeIPs - see its doc comment.
func UpdateNodeHostnames(db *sql.DB, hostnames map[string]string) error {
	for mac, hostname := range hostnames {
		if _, err := db.Exec(`UPDATE nodes SET Hostname = ? WHERE Mac = ?`, hostname, mac); err != nil {
			return err
		}
	}
	return nil
}

// enforceMaxPerMac deletes the oldest sightings for one MAC beyond
// maxPerMac, keeping only the most recent. Called from InsertNodes right
// after writing a new row, rather than as a separate periodic sweep like
// Prune - a MAC that keeps genuinely changing (port, IP, hostname, CDP)
// every poll could otherwise accumulate rows faster than Prune's age cutoff
// would ever catch, and enforcing the cap here means the per-MAC count never
// even temporarily exceeds it between Prune runs.
func enforceMaxPerMac(tx *sql.Tx, mac string, maxPerMac int) error {
	_, err := tx.Exec(`
		DELETE FROM nodes
		WHERE Mac = ? AND ROWID NOT IN (
			SELECT ROWID FROM nodes WHERE Mac = ?
			ORDER BY TimeStamp DESC LIMIT ?
		)`,
		mac, mac, maxPerMac,
	)
	return err
}
