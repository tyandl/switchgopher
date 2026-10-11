package cisco

import "testing"

func TestNativeVlanOverridesDefault(t *testing.T) {
	if nativeVlanOverrides(1) {
		t.Error("expected the default native vlan (1) not to override")
	}
}

func TestNativeVlanOverridesNonDefault(t *testing.T) {
	if !nativeVlanOverrides(42) {
		t.Error("expected a non-default native vlan to override")
	}
}

func TestVoiceVlanConfigured(t *testing.T) {
	if !voiceVlanConfigured(100) {
		t.Error("expected a real voice vlan value to be considered configured")
	}
}

func TestVoiceVlanUnconfiguredSentinel(t *testing.T) {
	if voiceVlanConfigured(4096) {
		t.Error("expected the 4096 sentinel to be considered not configured")
	}
}
