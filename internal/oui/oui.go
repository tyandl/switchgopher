// Package oui parses a vendored snapshot of the IEEE's public MA-L OUI
// registry (see resources/oui/README.md for the source, scope, and refresh
// process) into a Go map, for store.SeedOui to bulk-import into the oui
// table at -seed time. The lookup itself happens in SQL (a JOIN against the
// oui table, keyed on a MAC's first 3 octets), not in Go - see
// internal/store's oui table doc comment.
package oui

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

// ParseCSV reads path (the IEEE's oui.csv format: a header row followed by
// Registry,Assignment,Organization Name,Organization Address rows) and
// returns a map of uppercase 6-hex-character OUI -> organization name.
// Only classic 24-bit MA-L assignments are meaningful here (Assignment is
// already 6 hex characters for those; anything else is skipped) - matching
// switchmap's own OuiCodes.pm scope.
func ParseCSV(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("oui: could not open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("oui: could not read header from %s: %w", path, err)
	}
	assignmentCol, orgCol := -1, -1
	for i, col := range header {
		switch col {
		case "Assignment":
			assignmentCol = i
		case "Organization Name":
			orgCol = i
		}
	}
	if assignmentCol == -1 || orgCol == -1 {
		return nil, fmt.Errorf("oui: %s missing Assignment/Organization Name columns", path)
	}

	vendors := map[string]string{}
	for {
		record, err := r.Read()
		if err != nil {
			break
		}
		if len(record) <= assignmentCol || len(record) <= orgCol {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(record[assignmentCol]))
		if len(code) != 6 {
			continue
		}
		vendors[code] = record[orgCol]
	}

	return vendors, nil
}
