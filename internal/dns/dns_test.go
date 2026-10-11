package dns

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeResolver answers LookupAddr from a fixed ip -> names map, so tests
// don't hit real DNS. An ip absent from ptr simulates a lookup failure.
type fakeResolver struct {
	ptr map[string][]string
}

func (f fakeResolver) LookupAddr(_ context.Context, addr string) ([]string, error) {
	names, ok := f.ptr[addr]
	if !ok {
		return nil, errors.New("no such host")
	}
	return names, nil
}

func TestResolve(t *testing.T) {
	r := fakeResolver{ptr: map[string][]string{
		"10.0.0.1": {"host1.example.com."},
		"10.0.0.2": {"host2.example.com."},
		// 10.0.0.3 deliberately absent - simulates a miss.
	}}
	ips := map[string]string{
		"AA:AA:AA:AA:AA:AA": "10.0.0.1",
		"BB:BB:BB:BB:BB:BB": "10.0.0.2",
		"CC:CC:CC:CC:CC:CC": "10.0.0.3",
	}

	got := Resolve(context.Background(), r, ips, 4, time.Second, "")

	want := map[string]string{
		"AA:AA:AA:AA:AA:AA": "host1.example.com",
		"BB:BB:BB:BB:BB:BB": "host2.example.com",
	}
	if len(got) != len(want) {
		t.Fatalf("Resolve() = %v, want %v", got, want)
	}
	for mac, host := range want {
		if got[mac] != host {
			t.Errorf("Resolve()[%q] = %q, want %q", mac, got[mac], host)
		}
	}
	if _, ok := got["CC:CC:CC:CC:CC:CC"]; ok {
		t.Errorf("Resolve() should omit a miss, not record it blank: got %q", got["CC:CC:CC:CC:CC:CC"])
	}
}

func TestResolveStripsDomainSuffix(t *testing.T) {
	r := fakeResolver{ptr: map[string][]string{
		"10.0.0.1": {"host1.CAMPUS.example.com."},
	}}
	ips := map[string]string{"AA:AA:AA:AA:AA:AA": "10.0.0.1"}

	got := Resolve(context.Background(), r, ips, 1, time.Second, ".campus.example.com")

	if got["AA:AA:AA:AA:AA:AA"] != "host1" {
		t.Errorf("Resolve() with domain suffix = %q, want %q", got["AA:AA:AA:AA:AA:AA"], "host1")
	}
}

func TestStripDomainSuffix(t *testing.T) {
	cases := []struct{ host, suffix, want string }{
		{"host1.example.com", "example.com", "host1"},
		{"host1.example.com", ".example.com", "host1"},
		{"host1.EXAMPLE.com", "example.com", "host1"},
		{"host1.example.com", "", "host1.example.com"},
		{"host1.other.com", "example.com", "host1.other.com"},
		{"example.com", "example.com", "example.com"}, // no leading dot, no match
	}
	for _, c := range cases {
		if got := stripDomainSuffix(c.host, c.suffix); got != c.want {
			t.Errorf("stripDomainSuffix(%q, %q) = %q, want %q", c.host, c.suffix, got, c.want)
		}
	}
}
