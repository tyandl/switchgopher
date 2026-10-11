package snmp

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/model"
)

// ErrNotSupported is wrapped by a fetcher's error when the device simply does
// not offer the thing being fetched (no temperature sensor, an optional MIB
// object it lacks). That is expected for many devices, so FetchAll logs it at
// Debug level instead of Warn.
var ErrNotSupported = errors.New("not supported by this device")

// logOptionalFailure logs a failed best-effort group: Debug if the device just
// does not support it, Warn for anything else (timeouts, malformed answers).
func logOptionalFailure(group, target string, err error) {
	if errors.Is(err, ErrNotSupported) {
		slog.Debug("device does not support an optional feature", "group", group, "target", target, "reason", err)
		return
	}
	slog.Warn("optional SNMP fetch failed; continuing without it", "group", group, "target", target, "err", err)
}

// Result is everything FetchAll gathered for one device in one poll.
type Result struct {
	Device    model.Device
	Ports     []model.Port
	Nodes     []model.Node
	Modules   []model.Module
	VlanNames map[int]string // VLAN ID -> name; doesn't merge into Device/Port, see VlanNamesFetcher's doc comment
}

// FetchAll polls one device: bootstraps system info to determine its
// vendor, then resolves and calls each registered group's implementation
// (vendor-specific override if one exists, else the generic/standard-MIB
// one, else skipped - see resolve). Ports and Nodes failing is fatal
// (matching today's pollDevice); every other group is best-effort,
// logged and skipped on error or absence, so an unregistered or failing
// group never blocks the rest of a poll.
func FetchAll(snmp *gosnmp.GoSNMP) (Result, error) {
	dev, err := FetchSystemInfo(snmp)
	if err != nil {
		return Result{}, fmt.Errorf("fetchAll: fetchSystemInfo: %w", err)
	}
	vendor := DeriveVendorKey(dev.SysObjectID)

	if r, ok := resolveIdentity(vendor); ok {
		if err := r.ResolveIdentity(snmp, &dev); err != nil {
			logOptionalFailure("resolveIdentity", snmp.Target, err)
		}
	}

	var ports []model.Port
	if f, ok := resolvePorts(vendor); ok {
		if ports, err = f.FetchPorts(snmp); err != nil {
			return Result{}, fmt.Errorf("fetchAll: fetchPorts: %w", err)
		}
	}

	// Position index for merging per-port group results (PortCounters/
	// PortAggregation/PortPoE) into the ports slice by ifIndex, built once
	// up front rather than per group.
	posByIfIndex := make(map[string]int, len(ports))
	for i, p := range ports {
		posByIfIndex[p.Port] = i
	}

	if f, ok := resolvePortCounters(vendor); ok {
		counters, err := f.FetchPortCounters(snmp)
		if err != nil {
			logOptionalFailure("fetchPortCounters", snmp.Target, err)
		}
		for ifIndex, c := range counters {
			if pos, ok := posByIfIndex[ifIndex]; ok {
				ports[pos].InPackets = c.InPackets
				ports[pos].OutPackets = c.OutPackets
				ports[pos].InErrors = c.InErrors
				ports[pos].OutErrors = c.OutErrors
				ports[pos].InDiscards = c.InDiscards
				ports[pos].OutDiscards = c.OutDiscards
			}
		}
	}

	if f, ok := resolvePortAggregation(vendor); ok {
		aggregation, err := f.FetchPortAggregation(snmp)
		if err != nil {
			logOptionalFailure("fetchPortAggregation", snmp.Target, err)
		}
		for ifIndex, aggregate := range aggregation {
			if pos, ok := posByIfIndex[ifIndex]; ok {
				ports[pos].AggregatePort = aggregate
			}
		}
	}

	if f, ok := resolvePortPoE(vendor); ok {
		poe, err := f.FetchPortPoE(snmp)
		if err != nil {
			logOptionalFailure("fetchPortPoE", snmp.Target, err)
		}
		for ifIndex, p := range poe {
			if pos, ok := posByIfIndex[ifIndex]; ok {
				ports[pos].PoeActive = p.Active
				ports[pos].PoeClass = p.Class
			}
		}
	}

	if f, ok := resolvePortDuplex(vendor); ok {
		duplex, err := f.FetchPortDuplex(snmp)
		if err != nil {
			logOptionalFailure("fetchPortDuplex", snmp.Target, err)
		}
		for ifIndex, d := range duplex {
			if pos, ok := posByIfIndex[ifIndex]; ok {
				ports[pos].Duplex = d
			}
		}
	}

	if f, ok := resolvePortVlanMembership(vendor); ok {
		membership, err := f.FetchPortVlanMembership(snmp)
		if err != nil {
			logOptionalFailure("fetchPortVlanMembership", snmp.Target, err)
		}
		for ifIndex, m := range membership {
			if pos, ok := posByIfIndex[ifIndex]; ok {
				ports[pos].Vlan = m.Vlan
				ports[pos].AuxVlan = m.AuxVlan
				ports[pos].IsTrunk = m.IsTrunk
			}
		}
	}

	var modules []model.Module
	if f, ok := resolveEntityModules(vendor); ok {
		if modules, err = f.FetchEntityModules(snmp); err != nil {
			logOptionalFailure("fetchEntityModules", snmp.Target, err)
		}
	}

	if f, ok := resolveSpanningTree(vendor); ok {
		if dev.StpTopChanges, err = f.FetchStpTopChanges(snmp); err != nil {
			logOptionalFailure("fetchStpTopChanges", snmp.Target, err)
		}
	}

	if f, ok := resolveIsLayer3(vendor); ok {
		if dev.IsLayer3, err = f.ResolveIsLayer3(snmp); err != nil {
			logOptionalFailure("resolveIsLayer3", snmp.Target, err)
		}
	}

	if f, ok := resolveTemperature(vendor); ok {
		if dev.Temp, err = f.FetchTemperature(snmp); err != nil {
			logOptionalFailure("fetchTemperature", snmp.Target, err)
		}
	}

	if f, ok := resolveCpuMemory(vendor); ok {
		if dev.Cpu, dev.Memory, err = f.FetchCpuMemory(snmp); err != nil {
			logOptionalFailure("fetchCpuMemory", snmp.Target, err)
		}
	}

	if f, ok := resolveConfigManagement(vendor); ok {
		if dev.ConfigUpdated, err = f.FetchConfigUpdated(snmp); err != nil {
			logOptionalFailure("fetchConfigUpdated", snmp.Target, err)
		}
	}

	var vlanNames map[int]string
	if f, ok := resolveVlanNames(vendor); ok {
		if vlanNames, err = f.FetchVlanNames(snmp); err != nil {
			logOptionalFailure("fetchVlanNames", snmp.Target, err)
		}
	}

	var nodes []model.Node
	if f, ok := resolveNodes(vendor); ok {
		if nodes, err = f.FetchNodes(snmp); err != nil {
			return Result{}, fmt.Errorf("fetchAll: fetchNodes: %w", err)
		}
	}
	if f, ok := resolveCDP(vendor); ok {
		f.FetchCDP(snmp, nodes)
	}

	return Result{Device: dev, Ports: ports, Nodes: nodes, Modules: modules, VlanNames: vlanNames}, nil
}
