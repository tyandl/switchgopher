package web

import (
	"database/sql"
	"fmt"
	//	"log"
	"net/http"
	"time"
)

// counterRow is one In/Out pair for the I/O counters table on port.tmpl.
type counterRow struct{ Metric, In, Out string }

func portHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Port",
		"Version": Version,
	}
	deviceResult, ok := req.URL.Query()["device"]
	if !ok || deviceResult[0] == "" {
		redirectHome(w, req)
		return
	}
	portResult, ok := req.URL.Query()["port"]
	if !ok || portResult[0] == "" {
		redirectHome(w, req)
		return
	}
	device := deviceResult[0]
	port := portResult[0]
	p["Device"] = device
	p["Port"] = port

	var state, adminState, speed, duplex, isTrunk, vlan, auxVlan, poeActive sql.NullInt64
	var inPackets, outPackets, inErrors, outErrors, inDiscards, outDiscards sql.NullInt64
	var label, poeClass sql.NullString
	row := db.QueryRow(`
		SELECT State, AdminState, Speed, Duplex, IsTrunk, Vlan, AuxVlan, PoeActive, PoeClass,
		       InPackets, OutPackets, InErrors, OutErrors, InDiscards, OutDiscards, Label
		FROM ports WHERE Device = ? AND Port = ?
	`, device, port)
	if err := row.Scan(&state, &adminState, &speed, &duplex, &isTrunk, &vlan, &auxVlan, &poeActive, &poeClass,
		&inPackets, &outPackets, &inErrors, &outErrors, &inDiscards, &outDiscards, &label); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Vlan/AuxVlan are handled outside the generic Row table (below) rather
	// than as plain infoRow text, since - like ports.tmpl - they should link
	// to that vlan's own page.
	p["Vlan"] = vlan.Int64
	p["AuxVlan"] = auxVlan.Int64

	p["Row"] = []infoRow{
		{"Mode", trunkLabel(isTrunk)},
		{"State", ifOperStatusLabel(state.Int64)},
		{"Admin State", adminStateLabel(adminState)},
		{"Speed", fmt.Sprintf("%d Mbps", speed.Int64)},
		{"Duplex", duplexLabel(duplex)},
		{"PoE", poeLabel(poeClass, poeActive)},
		{"Description", label.String},
	}

	// I/O counters get their own In/Out table (see port.tmpl) rather than
	// the generic Label/Value Row above, since each metric has two values.
	p["Counters"] = []counterRow{
		{"Packets", counterLabel(inPackets), counterLabel(outPackets)},
		{"Errors", counterLabel(inErrors), counterLabel(outErrors)},
		{"Discards", counterLabel(inDiscards), counterLabel(outDiscards)},
	}

	// Same shape as nodeHandler's history, filtered by port instead of by
	// MAC - who's been plugged into this port over time, rather than where
	// this MAC has roamed.
	// oui is joined on the MAC's first 3 octets rather than looked up in Go
	// - see internal/store's oui table doc comment.
	rows, err := db.Query(`
		SELECT n.TimeStamp, n.CdpDeviceId, n.Mac, n.Ip, n.Hostname, o.OrgName
		FROM nodes n
		LEFT JOIN oui o ON o.Oui = upper(replace(substr(n.Mac, 1, 8), ':', ''))
		WHERE n.Device = ? AND n.Port = ?
		ORDER BY n.TimeStamp DESC
	`, device, port)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var history []nodeRow
	for rows.Next() {
		var ts int64
		var cdp, mac, ip, host, nic sql.NullString
		if err := rows.Scan(&ts, &cdp, &mac, &ip, &host, &nic); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		history = append(history, nodeRow{
			Date: time.Unix(ts, 0).Format("01/02/2006"),
			CDP:  cdp.String,
			NIC:  nic.String,
			Mac:  mac.String,
			IP:   ip.String,
			Host: host.String,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["History"] = history

	if err := templates.ExecuteTemplate(w, "port.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// nodeRow is one sighting of a MAC address, for the history table on
// node.tmpl.
type nodeRow struct{ Date, CDP, NIC, Mac, IP, Host string }

func nodeHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Node",
		"Version": Version,
	}
	result, ok := req.URL.Query()["mac"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	mac := result[0]
	p["Mac"] = mac

	// nodes accumulates a sighting per poll (see internal/store's
	// InsertNodes), so this is the full history of where this MAC has been
	// seen, newest first.
	rows, err := db.Query(`
		SELECT n.TimeStamp, n.CdpDeviceId, n.Mac, n.Ip, n.Hostname, o.OrgName
		FROM nodes n
		LEFT JOIN oui o ON o.Oui = upper(replace(substr(n.Mac, 1, 8), ':', ''))
		WHERE n.Mac = ?
		ORDER BY n.TimeStamp DESC
	`, mac)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var out []nodeRow
	for rows.Next() {
		var ts int64
		var cdp, macAddr, ip, host, nic sql.NullString
		if err := rows.Scan(&ts, &cdp, &macAddr, &ip, &host, &nic); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, nodeRow{
			Date: time.Unix(ts, 0).Format("01/02/2006"),
			CDP:  cdp.String,
			NIC:  nic.String,
			Mac:  macAddr.String,
			IP:   ip.String,
			Host: host.String,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["Row"] = out

	if err := templates.ExecuteTemplate(w, "node.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
