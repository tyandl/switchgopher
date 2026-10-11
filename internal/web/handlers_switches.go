package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// poeLabel implements the ports report's PoE column: power class if
// available, else active state. PoeClass is a text label ("class0"..
// "class4" - see internal/snmp/drivers/generic/portpoe.go's
// poeClassNames), reformatted here as "Class N".
func poeLabel(class sql.NullString, active sql.NullInt64) string {
	if class.Valid && class.String != "" {
		return "Class " + strings.TrimPrefix(class.String, "class")
	}
	if active.Valid {
		if active.Int64 != 0 {
			return "Active"
		}
		return "Inactive"
	}
	return ""
}

// counterLabel formats a nullable I/O counter for display, blank when
// absent - not populated yet (see model.Port's I/O counter fields).
func counterLabel(n sql.NullInt64) string {
	if !n.Valid {
		return ""
	}
	return fmt.Sprint(n.Int64)
}

// ifOperStatus values (IF-MIB) - see internal/snmp/port.go's FetchPorts,
// which is what populates ports.State.
func ifOperStatusLabel(state int64) string {
	switch state {
	case 1:
		return "Active"
	case 2:
		return "Down"
	case 3:
		return "Testing"
	case 5:
		return "Dormant"
	case 6:
		return "Not Present"
	case 7:
		return "Lower Layer Down"
	default:
		return "Unknown"
	}
}

// ifAdminStatus values (IF-MIB, standard) - see
// internal/snmp/drivers/generic/ports.go's FetchPorts, which is what
// populates ports.AdminState.
func adminStateLabel(state sql.NullInt64) string {
	switch state.Int64 {
	case 1:
		return "Up"
	case 2:
		return "Down"
	case 3:
		return "Testing"
	default:
		return "Unknown"
	}
}

// trunkLabel - see internal/snmp/drivers/cisco/vlanmembership.go's doc
// comment on why a plain bool (defaulting false/"Access") is honest for
// both a genuine access port and a non-Cisco/not-yet-polled one.
func trunkLabel(isTrunk sql.NullInt64) string {
	if isTrunk.Valid && isTrunk.Int64 != 0 {
		return "Trunk"
	}
	return "Access"
}

// Port duplex values - see internal/snmp/drivers/generic/duplex.go's doc
// comment for where these come from (EtherLike-MIB's
// dot3StatsDuplexStatus: unknown=1/half=2/full=3) plus a project-defined
// 0 for "no data returned at all," which reads identically to "unknown."
func duplexLabel(duplex sql.NullInt64) string {
	switch duplex.Int64 {
	case 2:
		return "half"
	case 3:
		return "full"
	default:
		return "unknown"
	}
}

// sinceLabel formats ports.Since (a Unix timestamp - see model.Port.Since's
// doc comment for the IF-MIB ifLastChange conversion) for the Ports
// report's "Since" column, same date format as nodeRow's Date. Blank when
// NULL - either never polled by a device new enough to report it, or a
// pre-migration row.
func sinceLabel(since sql.NullInt64) string {
	if !since.Valid {
		return ""
	}
	return time.Unix(since.Int64, 0).Format("01/02/2006")
}

// portsHandler renders /campus/switches/ports/?switch=<device>: one row per port
// with the device last seen on it, plus a table of EtherChannel (aggregate) members.
func portsHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Ports",
		"Version": Version,
	}
	result, ok := req.URL.Query()["switch"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	device := req.URL.Query()["switch"][0]
	p["Device"] = device

	type Node struct{ CDP, NIC, Mac, IP, Host string }
	type Port struct {
		Port                       string
		Aux, Vlan                  int
		State, AdminState          string
		Since                      string
		Speed, Duplex, Description string
		Mode                       string // "Trunk" or "Access" - see trunkLabel
		PoE                        string
		Aggregate                  string // this port's aggregate, if it's a member - blank otherwise
		Nodes                      []Node
	}

	// LEFT JOIN so ports with no current node sighting still render a row
	// (ports.tmpl already tolerates an empty .Nodes). The correlated
	// subquery keeps only each port's most recent sighting, since nodes
	// accumulates FDB-sighting history rather than being a live-state table
	// (see internal/store's InsertNodes).
	// oui is joined on the MAC's first 3 octets (upper-cased, colons
	// stripped) rather than looked up in Go - see internal/store's oui
	// table doc comment.
	rows, err := db.Query(`
		SELECT p.Port, p.State, p.AdminState, p.Speed, p.Duplex, p.IsTrunk, p.Since, p.Vlan, p.AuxVlan, p.PoeActive, p.PoeClass, p.AggregatePort, p.Label,
		       n.Mac, n.Ip, n.Hostname, n.CdpDeviceId, o.OrgName
		FROM ports p
		LEFT JOIN nodes n
			ON n.Port = p.Port AND n.Device = p.Device
			AND n.TimeStamp = (
				SELECT MAX(n2.TimeStamp) FROM nodes n2
				WHERE n2.Port = p.Port AND n2.Device = p.Device
			)
		LEFT JOIN oui o ON o.Oui = upper(replace(substr(n.Mac, 1, 8), ':', ''))
		WHERE p.Device = ?
		ORDER BY p.Port
	`, device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var order []string
	byPort := map[string]*Port{}
	for rows.Next() {
		var portNum, state, adminState, speed, duplex, isTrunk, since, vlan, auxVlan, poeActive, aggregatePort sql.NullInt64
		var label, mac, ip, host, cdpID, nic, poeClass sql.NullString
		if err := rows.Scan(&portNum, &state, &adminState, &speed, &duplex, &isTrunk, &since, &vlan, &auxVlan, &poeActive, &poeClass, &aggregatePort, &label, &mac, &ip, &host, &cdpID, &nic); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		key := fmt.Sprint(portNum.Int64)
		port, seen := byPort[key]
		if !seen {
			aggregate := ""
			if aggregatePort.Valid {
				aggregate = fmt.Sprint(aggregatePort.Int64)
			}
			port = &Port{
				Port:        key,
				Vlan:        int(vlan.Int64),
				Aux:         int(auxVlan.Int64),
				State:       ifOperStatusLabel(state.Int64),
				AdminState:  adminStateLabel(adminState),
				Since:       sinceLabel(since),
				Speed:       fmt.Sprintf("%d Mbps", speed.Int64),
				Duplex:      duplexLabel(duplex),
				Mode:        trunkLabel(isTrunk),
				PoE:         poeLabel(poeClass, poeActive),
				Aggregate:   aggregate,
				Description: label.String,
			}
			byPort[key] = port
			order = append(order, key)
		}
		if mac.Valid {
			port.Nodes = append(port.Nodes, Node{
				CDP:  cdpID.String,
				NIC:  nic.String,
				Mac:  mac.String,
				IP:   ip.String,
				Host: host.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsOut := make([]Port, 0, len(order))
	for _, k := range order {
		rowsOut = append(rowsOut, *byPort[k])
	}
	p["Row"] = rowsOut

	// Aggregates summary table: one row per logical port-channel, listing
	// its member ports. Built as its own query rather than derived from the
	// loop above, since a member's own row doesn't need to know about its
	// siblings - grouping by AggregatePort here is simpler than threading
	// that through the main loop.
	type Aggregate struct {
		Port    string
		Members []string
	}
	aggRows, err := db.Query(`
		SELECT AggregatePort, Port FROM ports
		WHERE Device = ? AND AggregatePort IS NOT NULL
		ORDER BY AggregatePort, Port
	`, device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer aggRows.Close()

	var aggOrder []string
	byAgg := map[string]*Aggregate{}
	for aggRows.Next() {
		var aggPort, memberPort sql.NullInt64
		if err := aggRows.Scan(&aggPort, &memberPort); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		key := fmt.Sprint(aggPort.Int64)
		agg, seen := byAgg[key]
		if !seen {
			agg = &Aggregate{Port: key}
			byAgg[key] = agg
			aggOrder = append(aggOrder, key)
		}
		agg.Members = append(agg.Members, fmt.Sprint(memberPort.Int64))
	}
	if err := aggRows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	aggregates := make([]Aggregate, 0, len(aggOrder))
	for _, k := range aggOrder {
		aggregates = append(aggregates, *byAgg[k])
	}
	p["Aggregates"] = aggregates // ports.tmpl hides this table entirely when empty

	if err := templates.ExecuteTemplate(w, "ports.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// infoRow is one label/value row in the switch info table (see info.tmpl).
type infoRow struct{ Label, Value string }

// infoHandler renders /campus/switches/info/?switch=<device>: model, firmware,
// location, contact, address and uptime, plus the chassis modules.
func infoHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Info",
		"Version": Version,
	}
	result, ok := req.URL.Query()["switch"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	device := result[0]
	p["Device"] = device

	var model, software, firmware, name, location, contact, address, uptime sql.NullString
	row := db.QueryRow(`
		SELECT ChassisModel, ProductDescription, FirmwareVersion, SysName, SysLocation, SysContact, Address, SysUptime
		FROM devices WHERE Device = ?
	`, device)
	if err := row.Scan(&model, &software, &firmware, &name, &location, &contact, &address, &uptime); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p["Row"] = []infoRow{
		{"Model", model.String},
		{"Software", software.String},
		{"Firmware", firmware.String},
		{"Name", name.String},
		{"Location", location.String},
		{"Contact", contact.String},
		{"Last Restart", restartDate(uptime.String)},
		{"IP address", address.String},
	}

	// Per-module info (line cards in a modular chassis - see model.Module's
	// doc comment). One key/value table per module; info.tmpl hides the
	// whole section when a switch has none, same as the ports report does
	// for Aggregates.
	type moduleInfo struct {
		Module string
		Row    []infoRow
	}
	modRows, err := db.Query(`
		SELECT Module, Description, Model, FwVersion, HwVersion, SwVersion, SerialNum
		FROM modules WHERE Device = ? ORDER BY Module
	`, device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer modRows.Close()

	var modules []moduleInfo
	for modRows.Next() {
		var modNum sql.NullInt64
		var description, modModel, fw, hw, sw, serial sql.NullString
		if err := modRows.Scan(&modNum, &description, &modModel, &fw, &hw, &sw, &serial); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		modules = append(modules, moduleInfo{
			Module: fmt.Sprint(modNum.Int64),
			Row: []infoRow{
				{"Model", modModel.String},
				{"Description", description.String},
				{"Hardware", hw.String},
				{"Firmware", fw.String},
				{"Software", sw.String},
				{"Serial", serial.String},
			},
		})
	}
	if err := modRows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["Modules"] = modules

	if err := templates.ExecuteTemplate(w, "info.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// restartDate turns a stored SysUptime (TimeTicks - hundredths of a second
// since boot, as fetched by snmp.FetchSystemInfo) into an approximate
// restart date. Approximate because it's computed from now, not from when
// the device was actually polled - fine for display, not for anything that
// needs precision.
func restartDate(ticks string) string {
	n, err := strconv.ParseInt(ticks, 10, 64)
	if err != nil {
		return ""
	}
	uptime := time.Duration(n) * 10 * time.Millisecond
	return time.Now().Add(-uptime).Format("01/02/2006")
}

// deviceCategory looks up a device's Category column, needed to build its
// config-file path (see resources/configs/README.md's expected layout).
func deviceCategory(db *sql.DB, device string) (string, error) {
	var category sql.NullString
	row := db.QueryRow(`SELECT Category FROM devices WHERE Device = ?`, device)
	if err := row.Scan(&category); err != nil {
		return "", err
	}
	return category.String, nil
}

// findDatedConfigs lists a device's dated config snapshots under
// dated_config_dir/<Category>/, matching <Device>.<NNN>-<NNN>.txt. A missing
// preference or directory just means no snapshots exist yet - nothing
// populates dated_config_dir itself (see resources/configs/README.md), so
// that's the normal starting state, not a failure worth erroring over.
func findDatedConfigs(category, device string) []string {
	datedDir, ok := configFolder("dated_config_dir")
	if !ok {
		return nil
	}
	files, err := os.ReadDir(safeJoin(datedDir, category))
	if err != nil {
		return nil
	}
	regex := regexp.MustCompile("^" + regexp.QuoteMeta(device) + `\.[0-9]*-[0-9]*\.txt$`)
	var dated []string
	for _, file := range files {
		if !file.IsDir() && regex.MatchString(file.Name()) {
			dated = append(dated, category+"/"+file.Name())
		}
	}
	return dated
}

// configsHandler renders /campus/switches/configs/?switch=<device>: links to the
// device's current saved config and its dated snapshots.
func configsHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Configs",
		"Version": Version,
	}
	result, ok := req.URL.Query()["switch"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	device := result[0]
	p["Device"] = device

	category, err := deviceCategory(db, device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p["Current"] = category + "/" + device + ".txt"
	p["Dated"] = findDatedConfigs(category, device)

	if err := templates.ExecuteTemplate(w, "configs.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// diffsHandler lists this device's dated snapshots, each linking to a diff
// against the current config - the single most useful comparison ("what's
// changed since the last backup"), rather than a pick-any-two-snapshots UI
// nothing has asked for.
func diffsHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Diffs",
		"Version": Version,
	}
	result, ok := req.URL.Query()["switch"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	device := result[0]
	p["Device"] = device

	category, err := deviceCategory(db, device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p["Current"] = category + "/" + device + ".txt"
	p["Dated"] = findDatedConfigs(category, device)

	if err := templates.ExecuteTemplate(w, "diffs.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
