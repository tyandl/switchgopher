package discover

import (
	"testing"

	"github.com/tyandl/switchgopher/internal/model"
)

func TestClassify(t *testing.T) {
	ethernetPorts := func(n int) []model.Port {
		ports := make([]model.Port, n)
		for i := range ports {
			ports[i].IfType = ifTypeEthernetCsmacd
		}
		return ports
	}

	cases := []struct {
		name           string
		services       int64
		servicesOK     bool
		ports          []model.Port
		wantSwitchlike bool
	}{
		{"datalink bit set", 2, true, nil, true},
		{"datalink + other bits set", 2 | 4, true, nil, true},
		{"only routing bit set, no ports fallback", 4, true, nil, false},
		{"only routing bit set, fallback saves it", 4, true, ethernetPorts(2), true},
		{"sysServices unavailable, enough ethernet ports", 0, false, ethernetPorts(2), true},
		{"sysServices unavailable, too few ethernet ports", 0, false, ethernetPorts(1), false},
		{"sysServices unavailable, no ports at all", 0, false, nil, false},
		{"sysServices zero but marked ok, falls through to ports", 0, true, ethernetPorts(3), true},
	}
	for _, c := range cases {
		if got := classify(c.services, c.servicesOK, c.ports); got != c.wantSwitchlike {
			t.Errorf("%s: classify(%d, %v, %d ports) = %v, want %v", c.name, c.services, c.servicesOK, len(c.ports), got, c.wantSwitchlike)
		}
	}
}

func TestDeviceNameFor(t *testing.T) {
	cases := []struct {
		name, address, want string
	}{
		{"switch1", "10.0.0.1", "switch1"},
		{"", "10.0.0.1", "10-0-0-1"},
	}
	for _, c := range cases {
		if got := deviceNameFor(c.name, c.address); got != c.want {
			t.Errorf("deviceNameFor(%q, %q) = %q, want %q", c.name, c.address, got, c.want)
		}
	}
}

func TestHostsInCIDR(t *testing.T) {
	cases := []struct {
		name      string
		cidr      string
		wantCount int
		wantFirst string
		wantLast  string
	}{
		{"/24 skips network and broadcast", "192.0.2.0/24", 254, "192.0.2.1", "192.0.2.254"},
		{"/30 skips network and broadcast", "192.0.2.0/30", 2, "192.0.2.1", "192.0.2.2"},
		{"/31 has no distinct network/broadcast - both included", "192.0.2.0/31", 2, "192.0.2.0", "192.0.2.1"},
		{"/32 is just the one address", "192.0.2.5/32", 1, "192.0.2.5", "192.0.2.5"},
	}
	for _, c := range cases {
		hosts, err := hostsInCIDR(c.cidr)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if len(hosts) != c.wantCount {
			t.Errorf("%s: got %d hosts, want %d (%v)", c.name, len(hosts), c.wantCount, hosts)
			continue
		}
		if hosts[0] != c.wantFirst {
			t.Errorf("%s: first host = %q, want %q", c.name, hosts[0], c.wantFirst)
		}
		if hosts[len(hosts)-1] != c.wantLast {
			t.Errorf("%s: last host = %q, want %q", c.name, hosts[len(hosts)-1], c.wantLast)
		}
	}
}

func TestHostsInCIDRRejectsIPv6(t *testing.T) {
	if _, err := hostsInCIDR("2001:db8::/32"); err == nil {
		t.Error("expected an error for an IPv6 CIDR, got nil")
	}
}

func TestHostsInCIDRRejectsInvalid(t *testing.T) {
	if _, err := hostsInCIDR("not-a-cidr"); err == nil {
		t.Error("expected an error for an invalid CIDR, got nil")
	}
}
