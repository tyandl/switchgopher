package snmp

import (
	"fmt"
	"log"
	"sync"
)

// GroupName identifies one coherent SNMP fetch operation (see groups.go).
type GroupName string

// Vendor identifies which implementation of a group to prefer.
type Vendor string

// VendorGeneric is the standard-MIB fallback every group should register
// an implementation under wherever a cross-vendor MIB exists. resolve
// falls back to it when no vendor-specific implementation is registered.
const VendorGeneric Vendor = "generic"

const (
	GroupIdentity           GroupName = "identity"
	GroupPorts              GroupName = "ports"
	GroupNodes              GroupName = "nodes"
	GroupCDP                GroupName = "cdp"
	GroupEntityModules      GroupName = "entityModules"
	GroupPortCounters       GroupName = "portCounters"
	GroupPortAggregation    GroupName = "portAggregation"
	GroupSpanningTree       GroupName = "spanningTree"
	GroupIsLayer3           GroupName = "isLayer3"
	GroupPortPoE            GroupName = "portPoE"
	GroupVlanNames          GroupName = "vlanNames"
	GroupTemperature        GroupName = "temperature"
	GroupPortDuplex         GroupName = "portDuplex"
	GroupPortVlanMembership GroupName = "portVlanMembership"
	GroupCpuMemory          GroupName = "cpuMemory"
	GroupConfigManagement   GroupName = "configManagement"
)

type registryKey struct {
	group  GroupName
	vendor Vendor
}

var (
	registryMu sync.RWMutex
	registry   = map[registryKey]any{}
)

// Register adds an implementation for group under vendor (use
// VendorGeneric for a standard-MIB implementation available to every
// vendor). Intended to be called from a vendor package's init(), matching
// the database/sql driver-registration idiom - so a duplicate
// registration is a programming error, not a runtime condition to handle
// gracefully, and panics immediately rather than silently overwriting.
func Register(group GroupName, vendor Vendor, impl any) {
	registryMu.Lock()
	defer registryMu.Unlock()

	key := registryKey{group, vendor}
	if _, exists := registry[key]; exists {
		panic(fmt.Sprintf("snmp: Register called twice for group %q, vendor %q", group, vendor))
	}
	registry[key] = impl
}

// resolve tries vendor's implementation of group first, falling back to
// VendorGeneric's. ok is false when neither is registered - callers treat
// that identically to how every "not populated this pass" field already
// behaves: skip and leave it unpopulated, never fatal. The log line below
// is what makes that distinguishable from a genuinely missing
// registration (e.g. a typo in a vendor package's init()) rather than a
// group that's deliberately unbuilt - both would otherwise look
// identical from the outside.
func resolve(group GroupName, vendor Vendor) (any, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if impl, ok := registry[registryKey{group, vendor}]; ok {
		return impl, true
	}
	if vendor != VendorGeneric {
		if impl, ok := registry[registryKey{group, VendorGeneric}]; ok {
			return impl, true
		}
	}
	log.Printf("snmp: no implementation registered for group %q (vendor %q, no generic fallback either) - skipping", group, vendor)
	return nil, false
}
