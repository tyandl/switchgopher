package web

import (
	"database/sql"
	"net/http"
)

// IANAifType-MIB values switchmap's own Stats.pm/SwitchUtils.pm/
// PopulatePorts.pm (the tool this project rewrites, read directly) use to
// exclude pseudo-interfaces from every port count
// (SwitchUtils::IsAncillaryPort) and to split active/inactive/disabled
// into virtual vs. non-virtual (PopulatePorts::IsVirtualPort). Only the
// ifType-based checks are replicated here - switchmap also excludes a
// handful of ancillary ports by interface *name* pattern (e.g. "^Async",
// "^CPU", "^Nu\d+"), which this project can't replicate: ports.Label is
// ifAlias falling back to ifDescr, not the raw ifDescr alone, so there's
// no column that reliably holds the original interface name. Documented
// as a known, narrower-than-switchmap gap in documentation/TODO.md.
const (
	ifTypeOther         = 1   // SwitchUtils::IsAncillaryPort's primary check
	ifTypeL2Vlan        = 135 // covers switchmap's "^VLAN-"/"^VL\d+"/"^Vl\d+" name patterns via type instead
	ifTypeIeee8023adLag = 161 // the aggregate's own logical port row, not a member port - covers switchmap's "^FEC-"/"^GEC-" name patterns via type instead
)

// virtualIfTypes mirrors PopulatePorts::IsVirtualPort's exact type list.
var virtualIfTypes = map[int]bool{
	22:  true, // proprietary serial
	24:  true, // softwareLoopback
	49:  true, // aal5
	53:  true, // propVirtual
	81:  true, // Digital Signal Level 0
	100: true, // voice recEive and transMit
	101: true, // voice Foreign Exchange Office
	102: true, // voice Foreign Exchange Station
	103: true, // voice encapsulation
	108: true, // pppMultilinkBundle
}

// isAncillaryPort reports whether ifType marks a pseudo-interface that
// switchmap excludes entirely from its stats (see the const block above).
func isAncillaryPort(ifType int) bool {
	return ifType == ifTypeOther || ifType == ifTypeL2Vlan || ifType == ifTypeIeee8023adLag
}

// isVirtualPort reports whether ifType marks a logical/non-physical
// interface, for the Port by State table's virtual/non-virtual split.
func isVirtualPort(ifType int) bool {
	return virtualIfTypes[ifType]
}

// portStateCounts is the Port by State table: active/inactive/disabled x
// virtual/non-virtual, plus a count of ports whose AdminState was neither
// up(1) nor down(2) - switchmap only logs a warning for that case
// (PopulatePorts::SetState defaults to "Unknown"); this project surfaces
// it in the UI instead, so a poller gap doesn't just silently vanish from
// the totals.
type portStateCounts struct {
	ActiveVirtual, Active     int
	InactiveVirtual, Inactive int
	DisabledVirtual, Disabled int
	Unknown                   int
}

// TotalVirtual/TotalNonVirtual must be exported - stats.tmpl calls them,
// and html/template can only invoke exported methods via reflection.
func (c portStateCounts) TotalVirtual() int {
	return c.ActiveVirtual + c.InactiveVirtual + c.DisabledVirtual
}
func (c portStateCounts) TotalNonVirtual() int {
	return c.Active + c.Inactive + c.Disabled
}

// portTypeCounts is the Port Types table - switchmap's 5th column, "active
// ports with empty labels," is dropped: ports.Label already falls back to
// ifDescr when ifAlias is blank, so it's never meaningfully empty in this
// schema (see documentation/TODO.md).
type portTypeCounts struct {
	AuxVlan, IPPhone, Trunk, EtherChannel int
}

// switchDetail is one row of the Switch Details table.
type switchDetail struct {
	Device                            string
	Active, Inactive, Disabled, Total int
}

// modelCount is one row of the Switches table (Model, Count).
type modelCount struct {
	Model string
	Count int
}

// moduleModelCount is one row of the Modules table.
type moduleModelCount struct {
	Model, Description string
	Count              int
}

func statsHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Stats",
		"Version": Version,
	}

	switches, totalSwitches, err := switchModelCounts(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["Switches"] = switches
	p["TotalSwitches"] = totalSwitches

	modules, totalModules, err := moduleModelCounts(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["Modules"] = modules
	p["TotalModules"] = totalModules

	states, types, vlanCount, details, err := portStats(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["PortStates"] = states
	p["PortTypes"] = types
	p["VlanCount"] = vlanCount
	p["SwitchDetails"] = details

	var grand switchDetail
	grand.Device = "Grand totals"
	for _, d := range details {
		grand.Active += d.Active
		grand.Inactive += d.Inactive
		grand.Disabled += d.Disabled
		grand.Total += d.Total
	}
	p["GrandTotal"] = grand

	var ipCount int
	if err := db.QueryRow(`SELECT COUNT(DISTINCT Ip) FROM nodes`).Scan(&ipCount); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["IpCount"] = ipCount

	if err := templates.ExecuteTemplate(w, "stats.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// switchModelCounts is the Switches table: how many devices report each
// ChassisModel. A blank ChassisModel (the expected case until real
// hardware is polled) groups under its own row, displayed as "(unknown)".
func switchModelCounts(db *sql.DB) ([]modelCount, int, error) {
	rows, err := db.Query(`
		SELECT ChassisModel, COUNT(*) FROM devices GROUP BY ChassisModel ORDER BY ChassisModel
	`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []modelCount
	total := 0
	for rows.Next() {
		var model sql.NullString
		var count int
		if err := rows.Scan(&model, &count); err != nil {
			return nil, 0, err
		}
		name := model.String
		if name == "" {
			name = "(unknown)"
		}
		out = append(out, modelCount{Model: name, Count: count})
		total += count
	}
	return out, total, rows.Err()
}

// moduleModelCounts is the Modules table. The modules table has been
// dormant since 2014 (see documentation/TODO.md's "Modules table"
// section) and is always empty today - this returns an empty slice in
// that case, and stats.tmpl hides the section entirely when there's
// nothing to show, matching the pattern already used on the Info page.
func moduleModelCounts(db *sql.DB) ([]moduleModelCount, int, error) {
	rows, err := db.Query(`
		SELECT Model, MAX(Description), COUNT(*) FROM modules GROUP BY Model ORDER BY Model
	`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []moduleModelCount
	total := 0
	for rows.Next() {
		var model, desc sql.NullString
		var count int
		if err := rows.Scan(&model, &desc, &count); err != nil {
			return nil, 0, err
		}
		out = append(out, moduleModelCount{Model: model.String, Description: desc.String, Count: count})
		total += count
	}
	return out, total, rows.Err()
}

// portStats does a single pass over every port - mirroring switchmap's own
// Stats::CollectStats, which is also one loop over every port incrementing
// counters as it goes, rather than computing the same classification logic
// twice (once in Go, once in SQL). Ancillary ports (see isAncillaryPort)
// are skipped entirely, matching switchmap's own if/else structure.
func portStats(db *sql.DB) (portStateCounts, portTypeCounts, int, []switchDetail, error) {
	var states portStateCounts
	var types portTypeCounts
	vlansSeen := map[int64]bool{}
	bySwitch := map[string]*switchDetail{}
	var order []string

	// IsPhone is computed here via a join against each port's most recent
	// node sighting (same correlated-subquery pattern as
	// handlers_switches.go's portsHandler) rather than in the Go loop below,
	// so the loop only has to look at one row per port.
	rows, err := db.Query(`
		SELECT p.Device, p.IfType, p.AdminState, p.State, p.IsTrunk, p.AggregatePort, p.AuxVlan, p.Vlan,
		       CASE WHEN n.CdpPlatform LIKE '%IP Phone%' THEN 1 ELSE 0 END AS IsPhone
		FROM ports p
		LEFT JOIN nodes n
			ON n.Port = p.Port AND n.Device = p.Device
			AND n.TimeStamp = (
				SELECT MAX(n2.TimeStamp) FROM nodes n2
				WHERE n2.Port = p.Port AND n2.Device = p.Device
			)
		ORDER BY p.Device
	`)
	if err != nil {
		return states, types, 0, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		// IfType/AdminState/State/IsTrunk are nullable: a ports row from
		// before this migration - or before its device's first real poll -
		// has them unset. NULL AdminState/State fall through to the
		// Unknown bucket below rather than crashing the page; NULL IfType
		// (0) is neither ancillary nor virtual, so it's treated as a
		// plain, unclassified port.
		var device string
		var ifType, adminState, state, isTrunk sql.NullInt64
		var aggregatePort, auxVlan, vlan sql.NullInt64
		var isPhone int
		if err := rows.Scan(&device, &ifType, &adminState, &state, &isTrunk, &aggregatePort, &auxVlan, &vlan, &isPhone); err != nil {
			return states, types, 0, nil, err
		}

		if isAncillaryPort(int(ifType.Int64)) {
			continue
		}

		detail, seen := bySwitch[device]
		if !seen {
			detail = &switchDetail{Device: device}
			bySwitch[device] = detail
			order = append(order, device)
		}

		virtual := isVirtualPort(int(ifType.Int64))
		switch {
		case adminState.Int64 == 2: // down
			if virtual {
				states.DisabledVirtual++
			} else {
				states.Disabled++
			}
			detail.Disabled++
		case adminState.Int64 == 1 && state.Int64 == 1: // up/up
			if virtual {
				states.ActiveVirtual++
			} else {
				states.Active++
			}
			detail.Active++
		case adminState.Int64 == 1: // up, but not currently linked
			if virtual {
				states.InactiveVirtual++
			} else {
				states.Inactive++
			}
			detail.Inactive++
		default:
			states.Unknown++
		}
		if adminState.Int64 == 1 || adminState.Int64 == 2 {
			detail.Total++
		}

		if auxVlan.Valid {
			types.AuxVlan++
		}
		if isPhone != 0 {
			types.IPPhone++
		}
		if isTrunk.Int64 != 0 {
			types.Trunk++
		}
		if aggregatePort.Valid {
			types.EtherChannel++
		}
		if vlan.Valid {
			vlansSeen[vlan.Int64] = true
		}
	}
	if err := rows.Err(); err != nil {
		return states, types, 0, nil, err
	}

	details := make([]switchDetail, 0, len(order))
	for _, device := range order {
		details = append(details, *bySwitch[device])
	}
	return states, types, len(vlansSeen), details, nil
}
