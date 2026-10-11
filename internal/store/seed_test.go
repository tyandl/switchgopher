package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeedWithNoConfigFilesIsEmptyNotAnError(t *testing.T) {
	db := testDB(t)
	before := countRows(t, db, "devices")
	t.Chdir(t.TempDir())

	if err := Seed(db); err != nil {
		t.Fatalf("Seed with no configuration files: %v", err)
	}
	if got := countRows(t, db, "devices"); got != before {
		t.Errorf("devices count = %d, want unchanged %d", got, before)
	}
}

// The shipped *.example.json files must stay valid input for Seed, including
// the cross-references between them.
func TestSeedAcceptsExampleFiles(t *testing.T) {
	db := testDB(t)
	repo, err := filepath.Abs("../../configuration")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "configuration"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"buildings", "categories", "vlans", "devices"} {
		data, err := os.ReadFile(filepath.Join(repo, name+".example.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "configuration", name+".json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// testDB already holds fixture rows, so compare against a baseline.
	tables := []string{"buildings", "categories", "vlans", "devices"}
	before := map[string]int{}
	for _, table := range tables {
		before[table] = countRows(t, db, table)
	}
	t.Chdir(tmp)

	if err := Seed(db); err != nil {
		t.Fatalf("Seed with example files: %v", err)
	}
	for _, table := range tables {
		if got := countRows(t, db, table) - before[table]; got != 2 {
			t.Errorf("%s: seeded %d new rows, want 2", table, got)
		}
	}
}

func TestNeedsSeed(t *testing.T) {
	db := testDB(t)
	if need, err := NeedsSeed(db); err != nil || !need {
		t.Fatalf("empty oui table: NeedsSeed = %v, %v; want true", need, err)
	}
	if _, err := db.Exec(`INSERT INTO oui (Oui, OrgName) VALUES ('AABBCC', 'Test Corp')`); err != nil {
		t.Fatal(err)
	}
	if need, err := NeedsSeed(db); err != nil || need {
		t.Errorf("after OUI load: NeedsSeed = %v, %v; want false", need, err)
	}
}

// Names from the config files go into SQL, so quotes in them must be data,
// never SQL.
func TestSeedHandlesQuotesInNames(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "configuration"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"buildings":  `{"Buildings":[{"Building":"O'Brien Hall","ShortName":"OB","Description":"it's \"old\""}]}`,
		"categories": `{"Categories":[{"Category":"Staff's"}]}`,
		"vlans":      `{"Vlans":[{"Vlan":77,"Name":"Bob's VLAN","Subnet":"192.0.2.0/24"}]}`,
		"devices":    `{"Devices":[]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, "configuration", name+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	if err := Seed(db); err != nil {
		t.Fatalf("Seed with quotes in names: %v", err)
	}
	var b, c, v string
	if err := db.QueryRow(`SELECT Building FROM buildings WHERE ShortName = 'OB'`).Scan(&b); err != nil || b != "O'Brien Hall" {
		t.Errorf("building = %q, %v", b, err)
	}
	if err := db.QueryRow(`SELECT Category FROM categories WHERE Category LIKE 'Staff%'`).Scan(&c); err != nil || c != "Staff's" {
		t.Errorf("category = %q, %v", c, err)
	}
	if err := db.QueryRow(`SELECT Name FROM vlans WHERE Vlan = 77`).Scan(&v); err != nil || v != "Bob's VLAN" {
		t.Errorf("vlan = %q, %v", v, err)
	}
}

func TestConnectSetsBusyTimeout(t *testing.T) {
	db, err := Connect(filepath.Join(t.TempDir(), "x.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ms int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&ms); err != nil || ms != 5000 {
		t.Errorf("busy_timeout = %d, %v; want 5000", ms, err)
	}
}
