package web

import (
	"database/sql"
	"testing"
	"time"
)

func TestPoeLabel(t *testing.T) {
	cases := []struct {
		name   string
		class  sql.NullString
		active sql.NullInt64
		want   string
	}{
		{"class wins over active", sql.NullString{String: "class2", Valid: true}, sql.NullInt64{Int64: 1, Valid: true}, "Class 2"},
		{"active only, on", sql.NullString{}, sql.NullInt64{Int64: 1, Valid: true}, "Active"},
		{"active only, off", sql.NullString{}, sql.NullInt64{Int64: 0, Valid: true}, "Inactive"},
		{"neither populated", sql.NullString{}, sql.NullInt64{}, ""},
		{"class valid but empty falls through to active", sql.NullString{String: "", Valid: true}, sql.NullInt64{Int64: 1, Valid: true}, "Active"},
	}
	for _, c := range cases {
		if got := poeLabel(c.class, c.active); got != c.want {
			t.Errorf("%s: poeLabel(%v, %v) = %q, want %q", c.name, c.class, c.active, got, c.want)
		}
	}
}

func TestIfOperStatusLabel(t *testing.T) {
	cases := map[int64]string{
		1:  "Active",
		2:  "Down",
		3:  "Testing",
		5:  "Dormant",
		6:  "Not Present",
		7:  "Lower Layer Down",
		99: "Unknown",
	}
	for in, want := range cases {
		if got := ifOperStatusLabel(in); got != want {
			t.Errorf("ifOperStatusLabel(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDuplexLabel(t *testing.T) {
	cases := []struct {
		name string
		in   sql.NullInt64
		want string
	}{
		{"half", sql.NullInt64{Int64: 2, Valid: true}, "half"},
		{"full", sql.NullInt64{Int64: 3, Valid: true}, "full"},
		{"unknown code", sql.NullInt64{Int64: 1, Valid: true}, "unknown"},
		{"no data (NULL)", sql.NullInt64{}, "unknown"},
	}
	for _, c := range cases {
		if got := duplexLabel(c.in); got != c.want {
			t.Errorf("%s: duplexLabel(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestAdminStateLabel(t *testing.T) {
	cases := []struct {
		name string
		in   sql.NullInt64
		want string
	}{
		{"up", sql.NullInt64{Int64: 1, Valid: true}, "Up"},
		{"down", sql.NullInt64{Int64: 2, Valid: true}, "Down"},
		{"testing", sql.NullInt64{Int64: 3, Valid: true}, "Testing"},
		{"no data (NULL)", sql.NullInt64{}, "Unknown"},
	}
	for _, c := range cases {
		if got := adminStateLabel(c.in); got != c.want {
			t.Errorf("%s: adminStateLabel(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestTrunkLabel(t *testing.T) {
	cases := []struct {
		name string
		in   sql.NullInt64
		want string
	}{
		{"trunking", sql.NullInt64{Int64: 1, Valid: true}, "Trunk"},
		{"not trunking", sql.NullInt64{Int64: 0, Valid: true}, "Access"},
		{"no data (NULL)", sql.NullInt64{}, "Access"},
	}
	for _, c := range cases {
		if got := trunkLabel(c.in); got != c.want {
			t.Errorf("%s: trunkLabel(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestSinceLabel(t *testing.T) {
	ts := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local).Unix()
	cases := []struct {
		name string
		in   sql.NullInt64
		want string
	}{
		{"populated", sql.NullInt64{Int64: ts, Valid: true}, "03/05/2026"},
		{"no data (NULL)", sql.NullInt64{}, ""},
	}
	for _, c := range cases {
		if got := sinceLabel(c.in); got != c.want {
			t.Errorf("%s: sinceLabel(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
