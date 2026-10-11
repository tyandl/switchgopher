package web

import (
	"database/sql"
	"fmt"
	//	"log"
	"net/http"
)

// vlanMember is one port assigned to a vlan (as its access or aux vlan),
// for the membership table on vlan.tmpl.
type vlanMember struct {
	Device, Port, Role, State, Speed, Description string
}

// vlanHandler renders /campus/vlans/vlan/?vlan=<id>: one VLAN's name and subnet,
// and the ports that use it.
func vlanHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Vlan",
		"Version": Version,
	}
	result, ok := req.URL.Query()["vlan"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	vlan := result[0]
	p["Vlan"] = vlan

	var name, subnet sql.NullString
	row := db.QueryRow(`SELECT Name, Subnet FROM vlans WHERE Vlan = ?`, vlan)
	if err := row.Scan(&name, &subnet); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p["Row"] = []infoRow{
		{"Name", name.String},
		{"Subnet", subnet.String},
	}

	// A port belongs to this vlan either as its access vlan or its aux
	// (voice) vlan - list both, tagged with which.
	rows, err := db.Query(`
		SELECT Device, Port, State, Speed, Label,
		       CASE WHEN Vlan = ?1 THEN 'access' ELSE 'aux' END
		FROM ports
		WHERE Vlan = ?1 OR AuxVlan = ?1
		ORDER BY Device, Port
	`, vlan)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var members []vlanMember
	for rows.Next() {
		var device, label, role sql.NullString
		var port, state, speed sql.NullInt64
		if err := rows.Scan(&device, &port, &state, &speed, &label, &role); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		members = append(members, vlanMember{
			Device:      device.String,
			Port:        fmt.Sprint(port.Int64),
			Role:        role.String,
			State:       ifOperStatusLabel(state.Int64),
			Speed:       fmt.Sprintf("%d Mbps", speed.Int64),
			Description: label.String,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p["Members"] = members

	if err := templates.ExecuteTemplate(w, "vlan.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
