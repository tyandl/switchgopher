package web

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"
)

// serving the home page!
func homeHandler(w http.ResponseWriter, req *http.Request) {
	p := map[string]Any{
		"Title":   "Home",
		"Version": Version,
	}
	if err := templates.ExecuteTemplate(w, "home.tmpl", &p); err != nil {
		internalError(w, req, err)
		return
	}
}

// handles the page for selecting switches
func switchesHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Devices",
		"Version": Version,
	}
	var urlQuery = map[string]string{
		"Category": "",
		"Building": "",
		"Room":     "",
		"ModelNum": "",
		"Num":      "",
	}
	for k := range urlQuery {
		_, ok := req.URL.Query()[k]
		if ok {
			urlQuery[k] = req.URL.Query()[k][0]
		}
	}

	p["Device"] = getDevices(db, urlQuery)

	for _, v := range []string{"Category", "Building", "Room", "ModelNum", "Num"} {
		p[v] = getSelection(db, v, urlQuery)
	}

	// render the page and send the response.
	if err := templates.ExecuteTemplate(w, "switches.tmpl", &p); err != nil {
		internalError(w, req, err)
		return
	}
}

// vlansHandler renders /campus/vlans/: every VLAN with its device and port counts
// and utilization. Query: sort=Vlan|Name, and unused=1 to include VLANs that no
// port uses.
func vlansHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Vlans",
		"Version": Version,
	}
	type Vlan struct {
		Vlan        string
		Name        string
		Subnet      string
		Switches    int
		Ports       int
		Utilization string
	}

	sortBy := req.URL.Query().Get("sort")
	if sortBy != "Name" {
		sortBy = "Vlan"
	}
	p["Sort"] = sortBy

	// showUnused round-trips through a hidden+checkbox pair sharing the name
	// "unused" (see vlans.tmpl) - a GET form has no way to distinguish "box
	// left unchecked" from "box never submitted", so the hidden field always
	// sends "0" and the checkbox, only present when checked, sends "1" after
	// it in the query string. Default (first visit, neither present) is to
	// show everything, matching the mockup's default-checked state.
	showUnused := true
	if vals, ok := req.URL.Query()["unused"]; ok && len(vals) > 0 {
		showUnused = vals[len(vals)-1] != "0"
	}
	p["ShowUnused"] = showUnused

	// Switches/Ports/ActivePorts are computed per vlan from ports currently
	// assigned to it (as an access or aux vlan). Note ports.Vlan/AuxVlan
	// aren't populated by polling yet (see internal/snmp/port.go), so these
	// will read as 0 for every vlan until that's built - not a bug here.
	rows, err := db.Query(`
		SELECT
			v.Vlan, v.Name, v.Subnet,
			(SELECT COUNT(DISTINCT p.Device) FROM ports p WHERE p.Vlan = v.Vlan OR p.AuxVlan = v.Vlan),
			(SELECT COUNT(*) FROM ports p WHERE p.Vlan = v.Vlan OR p.AuxVlan = v.Vlan),
			(SELECT COUNT(*) FROM ports p WHERE (p.Vlan = v.Vlan OR p.AuxVlan = v.Vlan) AND p.State = 1)
		FROM vlans v
		ORDER BY ` + sortBy)
	if err != nil {
		slog.Error("cannot query the VLAN list", "err", err)
		return
	}
	defer rows.Close()

	var results []Vlan
	for rows.Next() {
		var v Vlan
		var activePorts int
		if err := rows.Scan(&v.Vlan, &v.Name, &v.Subnet, &v.Switches, &v.Ports, &activePorts); err != nil {
			slog.Error("cannot read a VLAN row", "err", err)
			continue
		}
		if !showUnused && v.Ports == 0 {
			continue
		}
		if v.Ports > 0 {
			v.Utilization = fmt.Sprintf("%d%%", activePorts*100/v.Ports)
		} else {
			v.Utilization = "0%"
		}
		results = append(results, v)
	}

	p["Vlans"] = results

	if err := templates.ExecuteTemplate(w, "vlans.tmpl", &p); err != nil {
		internalError(w, req, err)
		return
	}
}

// searchRow is one sighting of a matching MAC/hostname, for search.tmpl -
// like nodeRow (handlers_ports.go) but across every switch, since a search
// isn't scoped to one MAC's own history page.
type searchRow struct{ Date, Switch, Port, CDP, IP, Host string }

// searchHandler answers the nav bar's search box (see _base.tmpl) - matches
// against Mac or Hostname, across every switch's node history.
func searchHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Search",
		"Version": Version,
	}
	term := req.URL.Query().Get("search")
	p["Search"] = term

	var out []searchRow
	if term != "" {
		like := "%" + term + "%"
		rows, err := db.Query(`
			SELECT TimeStamp, Device, Port, CdpDeviceId, Ip, Hostname
			FROM nodes
			WHERE Mac LIKE ? OR Hostname LIKE ?
			ORDER BY TimeStamp DESC
		`, like, like)
		if err != nil {
			internalError(w, req, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var ts int64
			var device, cdp, ip, host sql.NullString
			var port sql.NullInt64
			if err := rows.Scan(&ts, &device, &port, &cdp, &ip, &host); err != nil {
				internalError(w, req, err)
				return
			}
			out = append(out, searchRow{
				Date:   time.Unix(ts, 0).Format("01/02/2006"),
				Switch: device.String,
				Port:   fmt.Sprint(port.Int64),
				CDP:    cdp.String,
				IP:     ip.String,
				Host:   host.String,
			})
		}
		if err := rows.Err(); err != nil {
			internalError(w, req, err)
			return
		}
	}
	p["Row"] = out

	if err := templates.ExecuteTemplate(w, "search.tmpl", &p); err != nil {
		internalError(w, req, err)
		return
	}
}

// getDevices lists the device names that match the current Category, Building,
// Room, ModelNum and Num filters; an empty filter matches everything.
func getDevices(db *sql.DB, urlQuery map[string]string) []string {
	var Result string
	var results []string
	rows, err := db.Query(`
		SELECT Device FROM devices WHERE
					(Category = ?1 OR ?1 = "")
				AND (Building = ?2 OR ?2 = "")
				AND (Room = ?3 OR ?3 = "")
				AND (ModelNum = ?4 OR ?4 = "")
				AND (Num = ?5 OR ?5 = "")
		`,
		urlQuery["Category"],
		urlQuery["Building"],
		urlQuery["Room"],
		urlQuery["ModelNum"],
		urlQuery["Num"],
	)
	if err != nil {
		slog.Error("cannot list the devices for the filter", "err", err)
		return nil
	}
	for rows.Next() {
		rows.Scan(&Result)
		results = append(results, Result)
	}
	sort.Strings(results)
	return results
}

// getSelection builds the options for one dependent drop-down on the Devices
// page (the values of devices column name), filtered by the other drop-downs'
// current choices. name is pasted into the SQL, so it must only ever be one of
// the fixed column names the caller passes, never user input.
func getSelection(db *sql.DB, name string, urlQuery map[string]string) []string {
	var Result string
	var results []string
	temp := urlQuery[name]
	urlQuery[name] = ""
	rows, err := db.Query(`
		SELECT Distinct `+name+` FROM devices WHERE
					(Category = ?1 OR ?1 = "")
				AND (Building = ?2 OR ?2 = "")
				AND (Room = ?3 OR ?3 = "")
				AND (ModelNum = ?4 OR ?4 = "")
				AND (Num = ?5 OR ?5 = "")
		`,
		urlQuery["Category"],
		urlQuery["Building"],
		urlQuery["Room"],
		urlQuery["ModelNum"],
		urlQuery["Num"],
	)
	urlQuery[name] = temp
	if err != nil {
		slog.Error("cannot list the options for a filter drop-down", "name", name, "err", err)
		return nil
	}
	for rows.Next() {
		rows.Scan(&Result)
		if !containsString(results, Result) {
			results = append(results, Result)
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

func containsString(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
