package store

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tyandl/switchgopher/internal/model"
)

// testDB returns a fresh in-memory database with the schema applied and one
// device/port seeded (nodes has a composite FK to ports(Port, Device), so
// InsertNodes needs a real port row to satisfy it). SetMaxOpenConns(1) keeps
// every operation on the same underlying connection - a plain ":memory:"
// DSN otherwise gives each pooled connection its own separate empty
// database.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if err := Setup(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO deviceTypes (DeviceType) VALUES ('generic')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO devices (Device, Address, Community) VALUES ('sw1', '10.0.0.1', 'public')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ports (Port, Device) VALUES (1, 'sw1')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func nodeCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// insertOne calls InsertNodes with a maxPerMac high enough to be a no-op for
// tests that aren't specifically exercising the cap (TestInsertNodesEnforcesMaxPerMac
// passes its own).
func insertOne(t *testing.T, db *sql.DB, n model.Node) {
	t.Helper()
	insertOneCapped(t, db, n, 1000)
}

func insertOneCapped(t *testing.T, db *sql.DB, n model.Node, maxPerMac int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertNodes(tx, "sw1", []model.Node{n}, []model.Port{{Port: "1"}}, maxPerMac); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertNodesDedupsIdenticalSighting(t *testing.T) {
	db := testDB(t)
	base := model.Node{Mac: "AA:BB:CC:00:00:01", Ip: "10.0.0.5", Hostname: "host1", Port: "1", TimeStamp: time.Unix(1000, 0)}

	insertOne(t, db, base)
	if got := nodeCount(t, db); got != 1 {
		t.Fatalf("after first insert: got %d rows, want 1", got)
	}

	// Same MAC, same everything, later timestamp - dedup should skip it.
	unchanged := base
	unchanged.TimeStamp = time.Unix(2000, 0)
	insertOne(t, db, unchanged)
	if got := nodeCount(t, db); got != 1 {
		t.Fatalf("after identical re-sighting: got %d rows, want 1 (dedup should have skipped it)", got)
	}

	// Something actually changed (hostname) - should insert a new row.
	changed := base
	changed.TimeStamp = time.Unix(3000, 0)
	changed.Hostname = "host2"
	insertOne(t, db, changed)
	if got := nodeCount(t, db); got != 2 {
		t.Fatalf("after changed sighting: got %d rows, want 2", got)
	}
}

func TestPruneRemovesOldSightingsOnly(t *testing.T) {
	db := testDB(t)

	old := model.Node{Mac: "AA:BB:CC:00:00:02", Port: "1", TimeStamp: time.Now().Add(-100 * 24 * time.Hour)}
	recent := model.Node{Mac: "AA:BB:CC:00:00:03", Port: "1", TimeStamp: time.Now()}
	insertOne(t, db, old)
	insertOne(t, db, recent)
	if got := nodeCount(t, db); got != 2 {
		t.Fatalf("setup: got %d rows, want 2", got)
	}

	n, err := Prune(db, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Prune removed %d rows, want 1", n)
	}

	var mac string
	if err := db.QueryRow(`SELECT Mac FROM nodes`).Scan(&mac); err != nil {
		t.Fatal(err)
	}
	if mac != recent.Mac {
		t.Fatalf("prune kept the wrong row: got %s, want %s", mac, recent.Mac)
	}
}

func TestInsertNodesEnforcesMaxPerMac(t *testing.T) {
	db := testDB(t)
	const mac = "AA:BB:CC:00:00:04"
	const maxPerMac = 3

	// Each sighting changes Hostname so dedup doesn't collapse them - five
	// genuinely distinct sightings, oldest to newest.
	for i := 0; i < 5; i++ {
		insertOneCapped(t, db, model.Node{
			Mac:       mac,
			Port:      "1",
			Hostname:  fmt.Sprintf("host%d", i),
			TimeStamp: time.Unix(int64(1000+i), 0),
		}, maxPerMac)
	}

	if got := nodeCount(t, db); got != maxPerMac {
		t.Fatalf("got %d rows, want %d (cap should have trimmed the oldest)", got, maxPerMac)
	}

	rows, err := db.Query(`SELECT Hostname FROM nodes ORDER BY TimeStamp`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var kept []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}
		kept = append(kept, h)
	}
	want := []string{"host2", "host3", "host4"} // the 3 most recent of 0..4
	if !reflect.DeepEqual(kept, want) {
		t.Fatalf("kept %v, want %v (should retain the most recent, not the oldest)", kept, want)
	}
}
