// Package discover implements the bootstrapping discovery tool: finding
// candidate switches before they're hand-entered into
// configuration/devices.json. Two independent mechanisms - Sweep (default,
// a management-subnet scan using SNMP itself as the "scan," no external
// tool or new dependency) and CrawlCDP (opt-in, one hop from an
// already-known seed device's CDP neighbors) - see documentation/TODO.md
// for why neither switchmap nor this project had this before, and the
// design decisions behind both.
package discover

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/snmp"
)

// Candidate is one discovered device, shaped to match store.Seed()'s
// local Device struct field-for-field - a reviewed entry can be
// copy-pasted straight into configuration/devices.json with no
// reshaping. SnmpVersion/v3 credential fields are deliberately absent:
// v3 credentials can't be discovered or guessed, only v2c
// community-based reachability can.
type Candidate struct {
	Device    string
	Category  string
	Building  string
	Room      string
	Model     string
	Index     string
	Address   string
	Community string
}

const (
	oidSysDescr    = ".1.3.6.1.2.1.1.1.0"
	oidSysServices = ".1.3.6.1.2.1.1.7.0"

	// sysServicesDatalink is RFC1213's sysServices bit for "performs
	// services at the datalink/subnetwork layer" (bit 2, value 2) - the
	// standard, textbook signal for "this device bridges/switches
	// traffic," distinct from bit 1 (physical/repeater) or bit 3
	// (internet/routing).
	sysServicesDatalink = 2

	// ifTypeEthernetCsmacd is IANAifType-MIB's ethernetCsmacd(6) - used
	// as a fallback switch-likeness signal for devices that don't
	// correctly report sysServices.
	ifTypeEthernetCsmacd        = 6
	minEthernetPortsForFallback = 2
)

// probe is the actual "scan": a quick, short-timeout Get(sysDescr) over a
// fresh v2c session. Returns the live, already-connected session on
// success (reused for the fuller identify step that follows, avoiding a
// second connect) or nil, false - any error (unreachable, wrong
// community, doesn't speak SNMP at all) is treated identically, since
// discovery only cares about "responds or doesn't."
func probe(ctx context.Context, addr string, port uint16, community string, timeout time.Duration) (*gosnmp.GoSNMP, bool) {
	session := &gosnmp.GoSNMP{
		Target:    addr,
		Port:      port,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   timeout,
		Retries:   0,
		Context:   ctx,
	}
	if err := session.Connect(); err != nil {
		return nil, false
	}
	if _, err := session.Get([]string{oidSysDescr}); err != nil {
		session.Close()
		return nil, false
	}
	return session, true
}

// classify is the switch-likeness test, deliberately loose - a human
// reviews every candidate before it's trusted anyway. Primary signal:
// sysServices' datalink bit. Fallback, for devices that don't correctly
// report sysServices: at least minEthernetPortsForFallback real
// (ethernetCsmacd) ports in the already-fetched result. Deliberately a
// pure function, no network - fetchSysServices (below) does the actual
// Get, kept separate so this is unit-testable without a live session.
func classify(sysServices int64, sysServicesOK bool, ports []model.Port) bool {
	if sysServicesOK && sysServices&sysServicesDatalink != 0 {
		return true
	}
	count := 0
	for _, p := range ports {
		if p.IfType == ifTypeEthernetCsmacd {
			count++
		}
	}
	return count >= minEthernetPortsForFallback
}

// fetchSysServices is classify's one piece of network I/O, split out so
// classify itself stays pure. ok is false for a Get failure or a device
// that doesn't implement sysServices at all (NoSuchObject/NoSuchInstance) -
// classify treats that identically to "bit not set," falling through to
// the port-count heuristic.
func fetchSysServices(session *gosnmp.GoSNMP) (services int64, ok bool) {
	resp, err := session.Get([]string{oidSysServices})
	if err != nil || len(resp.Variables) != 1 {
		return 0, false
	}
	v := resp.Variables[0]
	if v.Type == gosnmp.NoSuchObject || v.Type == gosnmp.NoSuchInstance {
		return 0, false
	}
	return gosnmp.ToBigInt(v.Value).Int64(), true
}

// deviceNameFor picks a placeholder Device name for a discovered
// candidate - sysName (or, for a CDP neighbor, its advertised device ID)
// if non-empty, else the address itself with dots as hyphens. Always a
// placeholder: meant to be renamed by whoever reviews the output file,
// not a real device-naming convention guess (see patterns.go for that).
func deviceNameFor(name, address string) string {
	if name != "" {
		return name
	}
	return strings.ReplaceAll(address, ".", "-")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// hostsInCIDR enumerates every host address in an IPv4 CIDR, skipping the
// network and broadcast addresses when the prefix is wide enough to have
// distinct ones (narrower than /31) - a /31 or /32 has no meaningful
// network/broadcast split, so every address is included for those.
func hostsInCIDR(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parsing CIDR %q: %w", cidr, err)
	}
	ip4 := ipnet.IP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("only IPv4 CIDRs are supported, got %q", cidr)
	}
	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones
	total := 1 << hostBits
	network := binary.BigEndian.Uint32(ip4)
	skipEnds := hostBits >= 2

	var hosts []string
	for i := 0; i < total; i++ {
		if skipEnds && (i == 0 || i == total-1) {
			continue
		}
		addr := make(net.IP, 4)
		binary.BigEndian.PutUint32(addr, network+uint32(i))
		hosts = append(hosts, addr.String())
	}
	return hosts, nil
}

// Sweep scans every host in cidr, probing each with bounded concurrency
// and running a full identify+classify pass on anything that responds.
// known is the set of devices.Address values already seeded (loaded by
// the caller from the DB) - addresses already tracked are skipped, so
// re-running discovery against an increasingly-known fleet doesn't keep
// re-surfacing the same switches.
func Sweep(ctx context.Context, cidr, community string, port uint16, probeTimeout, fetchTimeout time.Duration, concurrency int, known map[string]bool) ([]Candidate, error) {
	hosts, err := hostsInCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	var candidates []Candidate
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, ip := range hosts {
		if known[ip] {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()

			session, ok := probe(ctx, ip, port, community, probeTimeout)
			if !ok {
				return
			}
			defer session.Close()
			session.Timeout = fetchTimeout

			result, err := snmp.FetchAll(session)
			if err != nil {
				return
			}
			services, servicesOK := fetchSysServices(session)
			if !classify(services, servicesOK, result.Ports) {
				return
			}

			mu.Lock()
			candidates = append(candidates, Candidate{
				Device:    deviceNameFor(result.Device.SysName, ip),
				Model:     result.Device.ChassisModel,
				Address:   ip,
				Community: community,
			})
			mu.Unlock()
		}(ip)
	}
	wg.Wait()
	return candidates, nil
}

// CrawlCDP walks seedSession's CDP neighbor cache (one hop only - see
// documentation/TODO.md for why this doesn't recurse further) and probes
// each neighbor with probeCommunity (the common real-world case of one
// shared community across a fleet). A neighbor that doesn't respond to
// that guess is still recorded - Model/Device come straight from its CDP
// advertisement, which needed no SNMP access to the neighbor itself -
// "found a neighbor, couldn't authenticate to it" is useful output, not
// a dead end.
func CrawlCDP(ctx context.Context, seedSession *gosnmp.GoSNMP, probeCommunity string, port uint16, probeTimeout, fetchTimeout time.Duration, known map[string]bool) ([]Candidate, error) {
	neighbors, err := snmp.FetchCdpNeighbors(seedSession)
	if err != nil {
		return nil, err
	}

	var candidates []Candidate
	for _, n := range neighbors {
		if known[n.Address] {
			continue
		}

		session, ok := probe(ctx, n.Address, port, probeCommunity, probeTimeout)
		if !ok {
			candidates = append(candidates, Candidate{
				Device:  deviceNameFor(n.DeviceId, n.Address),
				Model:   n.Platform,
				Address: n.Address,
			})
			continue
		}
		session.Timeout = fetchTimeout
		result, err := snmp.FetchAll(session)
		var classified bool
		if err == nil {
			services, servicesOK := fetchSysServices(session)
			classified = classify(services, servicesOK, result.Ports)
		}
		session.Close()
		if !classified {
			candidates = append(candidates, Candidate{
				Device:  deviceNameFor(n.DeviceId, n.Address),
				Model:   n.Platform,
				Address: n.Address,
			})
			continue
		}

		candidates = append(candidates, Candidate{
			Device:    deviceNameFor(firstNonEmpty(result.Device.SysName, n.DeviceId), n.Address),
			Model:     firstNonEmpty(result.Device.ChassisModel, n.Platform),
			Address:   n.Address,
			Community: probeCommunity,
		})
	}
	return candidates, nil
}
