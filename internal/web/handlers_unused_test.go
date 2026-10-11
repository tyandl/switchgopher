package web

import (
	"testing"
	"time"
)

func TestIsUnusedPort(t *testing.T) {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	ninetyDaysAgo := now.AddDate(0, 0, -90).Unix()
	thirtyDaysAgo := now.AddDate(0, 0, -30).Unix()
	sixtyDaysAgo := now.AddDate(0, 0, -60).Unix()

	cases := []struct {
		name              string
		ifType            int
		adminState, state int64
		since             int64
		wantDays          int
		wantUnused        bool
	}{
		{"inactive past threshold", 6, 1, 2, ninetyDaysAgo, 90, true},
		{"active, never unused regardless of Since", 6, 1, 1, ninetyDaysAgo, 0, false},
		{"ancillary, skipped regardless of Since", 1, 1, 2, ninetyDaysAgo, 0, false},
		{"inactive but not past threshold yet", 6, 1, 2, thirtyDaysAgo, 30, false},
		{"exactly at threshold boundary - not yet unused", 6, 1, 2, sixtyDaysAgo, 60, false},
		{"admin-down (disabled) past threshold", 6, 2, 0, ninetyDaysAgo, 90, true},
	}
	for _, c := range cases {
		gotDays, gotUnused := isUnusedPort(c.ifType, c.adminState, c.state, c.since, 60, now)
		if gotUnused != c.wantUnused {
			t.Errorf("%s: isUnusedPort() unused = %v, want %v", c.name, gotUnused, c.wantUnused)
		}
		if gotUnused && gotDays != c.wantDays {
			t.Errorf("%s: isUnusedPort() days = %d, want %d", c.name, gotDays, c.wantDays)
		}
	}
}
