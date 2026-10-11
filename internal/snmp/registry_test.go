package snmp

import "testing"

// testGroup/testVendor* keep this test's registrations isolated from the
// real, package-level GroupName/Vendor constants (and from generic's own
// init() registrations, which have already run by the time tests execute).
const testGroup GroupName = "test-group-registry"

type testImpl struct{ name string }

func TestRegisterResolveFallsBackToGeneric(t *testing.T) {
	Register(testGroup, VendorGeneric, testImpl{"generic-impl"})

	impl, ok := resolve(testGroup, Vendor("some-vendor-with-no-override"))
	if !ok {
		t.Fatal("expected fallback to generic to succeed")
	}
	if impl.(testImpl).name != "generic-impl" {
		t.Errorf("got %v, want generic-impl", impl)
	}
}

func TestRegisterResolvePrefersVendorSpecific(t *testing.T) {
	const group GroupName = "test-group-prefers-vendor"
	Register(group, VendorGeneric, testImpl{"generic-impl"})
	Register(group, Vendor("acme"), testImpl{"acme-impl"})

	impl, ok := resolve(group, Vendor("acme"))
	if !ok {
		t.Fatal("expected resolve to succeed")
	}
	if impl.(testImpl).name != "acme-impl" {
		t.Errorf("got %v, want acme-impl (vendor-specific should win over generic)", impl)
	}
}

func TestResolveNotFound(t *testing.T) {
	if _, ok := resolve(GroupName("no-such-group-at-all"), VendorGeneric); ok {
		t.Error("expected ok=false for an unregistered group")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	const group GroupName = "test-group-duplicate"
	Register(group, VendorGeneric, testImpl{"first"})

	defer func() {
		if recover() == nil {
			t.Error("expected Register to panic on a duplicate (group,vendor) registration")
		}
	}()
	Register(group, VendorGeneric, testImpl{"second"})
}
