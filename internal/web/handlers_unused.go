package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/tyandl/switchgopher/internal/config"
)

// unusedRow is one candidate-for-reclamation port on the Unused Ports
// report. Mirrors switchmap's own WriteUnusedRow columns (Port/VLAN/
// DaysInactive/Speed/Duplex/Label), plus Switch/Building/Room since this
// is one combined multi-switch view - switchmap's per-switch static pages
// never needed those, the switch name was each page's own heading.
type unusedRow struct {
	Switch, Building, Room, Port, Vlan string
	DaysInactive                       int
	Speed, Duplex, Description         string
}

// unusedHandler lists ports that have been continuously non-Active for
// longer than the configured unused_after_days threshold - candidates for
// reclamation. Grounded in switchmap's own WriteUnusedDirectory.pm/
// PopulatePorts::SetDaysInactive, but computed from IF-MIB ifLastChange
// (ports.Since, standard) instead of switchmap's own hand-rolled
// ScanSwitch.pl/.idlesince cron mechanism - both answer the same
// question ("when did this port's state last change"), and ifLastChange
// is already fetched and stored, so no new SNMP work was needed for this
// report.
//
// Deliberately one combined, filterable page rather than switchmap's
// three separate static-file views (an index, unused-by-switch, unused-
// by-vlan) - those existed because switchmap pre-renders static HTML per
// switch/vlan, not because that structure is essential.
func unusedHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Unused Ports",
		"Version": Version,
	}

	urlQuery := map[string]string{
		"Building": "",
		"Room":     "",
		"Device":   "",
	}
	for k := range urlQuery {
		if vals, ok := req.URL.Query()[k]; ok && len(vals) > 0 {
			urlQuery[k] = vals[0]
		}
	}

	unusedAfterDays := config.Int("unused_after_days", 60)

	// State/AdminState/IfType feed the Go-side Active/ancillary checks
	// below (mirroring handlers_stats.go's portStats, which also does one
	// broad SQL pull and applies its classification logic in Go rather
	// than encoding it as SQL CASE/WHERE). Sorted oldest-Since-first,
	// which is equivalent to days-inactive descending - the most
	// reclaimable candidates first.
	rows, err := db.Query(`
		SELECT p.Device, d.Building, d.Room, p.Port, p.Vlan, p.Speed, p.Duplex, p.Label,
		       p.Since, p.State, p.AdminState, p.IfType
		FROM ports p
		JOIN devices d ON d.Device = p.Device
		WHERE (d.Building = ?1 OR ?1 = '')
		  AND (d.Room = ?2 OR ?2 = '')
		  AND (p.Device = ?3 OR ?3 = '')
		  AND p.Since IS NOT NULL
		ORDER BY p.Since ASC
	`, urlQuery["Building"], urlQuery["Room"], urlQuery["Device"])
	if err != nil {
		internalError(w, req, err)
		return
	}
	defer rows.Close()

	var out []unusedRow
	for rows.Next() {
		var device, building, room string
		var portNum, vlan, speed, duplex, since, state, adminState, ifType sql.NullInt64
		var label sql.NullString
		if err := rows.Scan(&device, &building, &room, &portNum, &vlan, &speed, &duplex, &label,
			&since, &state, &adminState, &ifType); err != nil {
			internalError(w, req, err)
			return
		}

		daysInactive, unused := isUnusedPort(int(ifType.Int64), adminState.Int64, state.Int64, since.Int64, unusedAfterDays, time.Now())
		if !unused {
			continue
		}

		out = append(out, unusedRow{
			Switch:       device,
			Building:     building,
			Room:         room,
			Port:         fmt.Sprint(portNum.Int64),
			Vlan:         fmt.Sprint(vlan.Int64),
			DaysInactive: daysInactive,
			Speed:        fmt.Sprintf("%d Mbps", speed.Int64),
			Duplex:       duplexLabel(duplex),
			Description:  label.String,
		})
	}
	if err := rows.Err(); err != nil {
		internalError(w, req, err)
		return
	}
	p["Row"] = out
	p["Threshold"] = unusedAfterDays

	for _, name := range []string{"Building", "Room", "Device"} {
		p[name] = unusedSelection(db, name, urlQuery)
	}

	if err := templates.ExecuteTemplate(w, "unused.tmpl", &p); err != nil {
		internalError(w, req, err)
		return
	}
}

// isUnusedPort applies the Unused Ports report's classification: skip
// ancillary pseudo-interfaces (isAncillaryPort, shared with
// handlers_stats.go) and currently-Active ports (adminState==1 &&
// state==1, same check as handlers_stats.go's portStats), then compare
// how long ago the port's operational state last changed (ifLastChange,
// via ports.Since) against the configured threshold. now is passed in
// rather than read via time.Now() so this stays a pure, unit-testable
// function. daysInactive is only meaningful when unused is true.
func isUnusedPort(ifType int, adminState, state, since int64, thresholdDays int, now time.Time) (daysInactive int, unused bool) {
	if isAncillaryPort(ifType) {
		return 0, false
	}
	if adminState == 1 && state == 1 {
		return 0, false
	}
	daysInactive = int(now.Sub(time.Unix(since, 0)).Hours() / 24)
	return daysInactive, daysInactive > thresholdDays
}

// unusedSelection builds one dependent-dropdown option list (Building,
// Room, or Device) for the Unused Ports report's filter form - same
// "currently-selected value first, then a blank 'all' option, then the
// rest" convention as handlers_campus.go's getSelection, scoped to just
// Building/Room/Device (no Category/ModelNum/Num, which don't apply
// here) rather than reusing that function verbatim.
func unusedSelection(db *sql.DB, name string, urlQuery map[string]string) []string {
	temp := urlQuery[name]
	urlQuery[name] = ""
	rows, err := db.Query(`
		SELECT DISTINCT `+name+` FROM devices WHERE
					(Building = ?1 OR ?1 = '')
				AND (Room = ?2 OR ?2 = '')
				AND (Device = ?3 OR ?3 = '')
		`,
		urlQuery["Building"], urlQuery["Room"], urlQuery["Device"],
	)
	urlQuery[name] = temp
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			continue
		}
		if !containsString(results, v) {
			results = append(results, v)
		}
	}
	sort.Strings(results)

	if len(results) >= 1 {
		if urlQuery[name] == "" {
			return append([]string{""}, results...)
		}
		head := []string{urlQuery[name], ""}
		return append(head, results[1:]...)
	}
	return results
}
