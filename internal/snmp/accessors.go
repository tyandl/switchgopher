package snmp

// Typed RegisterX/resolveX wrappers around the generic Register/resolve,
// one pair per group - gives vendor packages a compile error (not a
// runtime panic) for a mismatched implementation type, and gives fetch.go
// an already-asserted value to call. Only the resolveX half is exported
// as unexported (fetch.go is same-package); RegisterX is exported since
// vendor packages calling it live outside this package.

// RegisterIdentity registers impl as vendor's implementation of the Identity group.
func RegisterIdentity(vendor Vendor, impl IdentityResolver) { Register(GroupIdentity, vendor, impl) }
func resolveIdentity(vendor Vendor) (IdentityResolver, bool) {
	v, ok := resolve(GroupIdentity, vendor)
	if !ok {
		return nil, false
	}
	return v.(IdentityResolver), true
}

// RegisterPorts registers impl as vendor's implementation of the Ports group.
func RegisterPorts(vendor Vendor, impl PortsFetcher) { Register(GroupPorts, vendor, impl) }
func resolvePorts(vendor Vendor) (PortsFetcher, bool) {
	v, ok := resolve(GroupPorts, vendor)
	if !ok {
		return nil, false
	}
	return v.(PortsFetcher), true
}

// RegisterNodes registers impl as vendor's implementation of the Nodes group.
func RegisterNodes(vendor Vendor, impl NodesFetcher) { Register(GroupNodes, vendor, impl) }
func resolveNodes(vendor Vendor) (NodesFetcher, bool) {
	v, ok := resolve(GroupNodes, vendor)
	if !ok {
		return nil, false
	}
	return v.(NodesFetcher), true
}

// RegisterCDP registers impl as vendor's implementation of the CDP group.
func RegisterCDP(vendor Vendor, impl CDPFetcher) { Register(GroupCDP, vendor, impl) }
func resolveCDP(vendor Vendor) (CDPFetcher, bool) {
	v, ok := resolve(GroupCDP, vendor)
	if !ok {
		return nil, false
	}
	return v.(CDPFetcher), true
}

// RegisterEntityModules registers impl as vendor's implementation of the EntityModules group.
func RegisterEntityModules(vendor Vendor, impl EntityModulesFetcher) {
	Register(GroupEntityModules, vendor, impl)
}
func resolveEntityModules(vendor Vendor) (EntityModulesFetcher, bool) {
	v, ok := resolve(GroupEntityModules, vendor)
	if !ok {
		return nil, false
	}
	return v.(EntityModulesFetcher), true
}

// RegisterPortCounters registers impl as vendor's implementation of the PortCounters group.
func RegisterPortCounters(vendor Vendor, impl PortCountersFetcher) {
	Register(GroupPortCounters, vendor, impl)
}
func resolvePortCounters(vendor Vendor) (PortCountersFetcher, bool) {
	v, ok := resolve(GroupPortCounters, vendor)
	if !ok {
		return nil, false
	}
	return v.(PortCountersFetcher), true
}

// RegisterPortAggregation registers impl as vendor's implementation of the PortAggregation group.
func RegisterPortAggregation(vendor Vendor, impl PortAggregationFetcher) {
	Register(GroupPortAggregation, vendor, impl)
}
func resolvePortAggregation(vendor Vendor) (PortAggregationFetcher, bool) {
	v, ok := resolve(GroupPortAggregation, vendor)
	if !ok {
		return nil, false
	}
	return v.(PortAggregationFetcher), true
}

// RegisterSpanningTree registers impl as vendor's implementation of the SpanningTree group.
func RegisterSpanningTree(vendor Vendor, impl SpanningTreeFetcher) {
	Register(GroupSpanningTree, vendor, impl)
}
func resolveSpanningTree(vendor Vendor) (SpanningTreeFetcher, bool) {
	v, ok := resolve(GroupSpanningTree, vendor)
	if !ok {
		return nil, false
	}
	return v.(SpanningTreeFetcher), true
}

// RegisterIsLayer3 registers impl as vendor's implementation of the IsLayer3 group.
func RegisterIsLayer3(vendor Vendor, impl IsLayer3Resolver) { Register(GroupIsLayer3, vendor, impl) }
func resolveIsLayer3(vendor Vendor) (IsLayer3Resolver, bool) {
	v, ok := resolve(GroupIsLayer3, vendor)
	if !ok {
		return nil, false
	}
	return v.(IsLayer3Resolver), true
}

// RegisterPortPoE registers impl as vendor's implementation of the PortPoE group.
func RegisterPortPoE(vendor Vendor, impl PortPoEFetcher) { Register(GroupPortPoE, vendor, impl) }
func resolvePortPoE(vendor Vendor) (PortPoEFetcher, bool) {
	v, ok := resolve(GroupPortPoE, vendor)
	if !ok {
		return nil, false
	}
	return v.(PortPoEFetcher), true
}

// RegisterVlanNames registers impl as vendor's implementation of the VlanNames group.
func RegisterVlanNames(vendor Vendor, impl VlanNamesFetcher) { Register(GroupVlanNames, vendor, impl) }
func resolveVlanNames(vendor Vendor) (VlanNamesFetcher, bool) {
	v, ok := resolve(GroupVlanNames, vendor)
	if !ok {
		return nil, false
	}
	return v.(VlanNamesFetcher), true
}

// RegisterTemperature registers impl as vendor's implementation of the Temperature group.
func RegisterTemperature(vendor Vendor, impl TemperatureFetcher) {
	Register(GroupTemperature, vendor, impl)
}
func resolveTemperature(vendor Vendor) (TemperatureFetcher, bool) {
	v, ok := resolve(GroupTemperature, vendor)
	if !ok {
		return nil, false
	}
	return v.(TemperatureFetcher), true
}

// RegisterPortDuplex registers impl as vendor's implementation of the PortDuplex group.
func RegisterPortDuplex(vendor Vendor, impl PortDuplexFetcher) {
	Register(GroupPortDuplex, vendor, impl)
}
func resolvePortDuplex(vendor Vendor) (PortDuplexFetcher, bool) {
	v, ok := resolve(GroupPortDuplex, vendor)
	if !ok {
		return nil, false
	}
	return v.(PortDuplexFetcher), true
}

// RegisterPortVlanMembership registers impl as vendor's implementation of the PortVlanMembership group.
func RegisterPortVlanMembership(vendor Vendor, impl PortVlanMembershipFetcher) {
	Register(GroupPortVlanMembership, vendor, impl)
}
func resolvePortVlanMembership(vendor Vendor) (PortVlanMembershipFetcher, bool) {
	v, ok := resolve(GroupPortVlanMembership, vendor)
	if !ok {
		return nil, false
	}
	return v.(PortVlanMembershipFetcher), true
}

// RegisterCpuMemory registers impl as vendor's implementation of the CpuMemory group.
func RegisterCpuMemory(vendor Vendor, impl CpuMemoryFetcher) { Register(GroupCpuMemory, vendor, impl) }
func resolveCpuMemory(vendor Vendor) (CpuMemoryFetcher, bool) {
	v, ok := resolve(GroupCpuMemory, vendor)
	if !ok {
		return nil, false
	}
	return v.(CpuMemoryFetcher), true
}

// RegisterConfigManagement registers impl as vendor's implementation of the ConfigManagement group.
func RegisterConfigManagement(vendor Vendor, impl ConfigManagementFetcher) {
	Register(GroupConfigManagement, vendor, impl)
}
func resolveConfigManagement(vendor Vendor) (ConfigManagementFetcher, bool) {
	v, ok := resolve(GroupConfigManagement, vendor)
	if !ok {
		return nil, false
	}
	return v.(ConfigManagementFetcher), true
}
