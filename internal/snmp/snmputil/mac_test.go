package snmputil

import "testing"

func TestMacFromParts(t *testing.T) {
	mac, ok := MacFromParts([]string{"0", "20", "145", "18", "201", "7"})
	if !ok {
		t.Fatal("expected ok")
	}
	if want := "00:14:91:12:C9:07"; mac != want {
		t.Errorf("got %s, want %s", mac, want)
	}
}

func TestMacFromPartsWrongLength(t *testing.T) {
	if _, ok := MacFromParts([]string{"1", "2", "3"}); ok {
		t.Error("expected not ok for wrong length")
	}
}

func TestMacFromPartsOutOfRange(t *testing.T) {
	if _, ok := MacFromParts([]string{"256", "0", "0", "0", "0", "0"}); ok {
		t.Error("expected not ok for out-of-range octet")
	}
	if _, ok := MacFromParts([]string{"not-a-number", "0", "0", "0", "0", "0"}); ok {
		t.Error("expected not ok for non-numeric octet")
	}
}

func TestDecodeQBridgeIndex(t *testing.T) {
	// dot1qFdbId=1 followed by MAC bytes 0.20.145.18.201.7
	mac, ok := DecodeQBridgeIndex("1.0.20.145.18.201.7")
	if !ok {
		t.Fatal("expected ok")
	}
	if want := "00:14:91:12:C9:07"; mac != want {
		t.Errorf("got %s, want %s", mac, want)
	}
}

func TestDecodeQBridgeIndexWrongLength(t *testing.T) {
	if _, ok := DecodeQBridgeIndex("1.2.3"); ok {
		t.Error("expected not ok for wrong length")
	}
}

func TestDecodePlainBridgeIndex(t *testing.T) {
	mac, ok := DecodePlainBridgeIndex("0.20.145.18.201.7")
	if !ok {
		t.Fatal("expected ok")
	}
	if want := "00:14:91:12:C9:07"; mac != want {
		t.Errorf("got %s, want %s", mac, want)
	}
}

func TestDecodePlainBridgeIndexWrongLength(t *testing.T) {
	if _, ok := DecodePlainBridgeIndex("1.2.3.4.5"); ok {
		t.Error("expected not ok for wrong length")
	}
}

func TestMacFromBytes(t *testing.T) {
	mac, ok := MacFromBytes([]byte{0x00, 0x14, 0x91, 0x12, 0xC9, 0x07})
	if !ok {
		t.Fatal("expected ok")
	}
	if want := "00:14:91:12:C9:07"; mac != want {
		t.Errorf("got %s, want %s", mac, want)
	}
}

func TestMacFromBytesWrongLength(t *testing.T) {
	if _, ok := MacFromBytes([]byte{0x00, 0x14, 0x91}); ok {
		t.Error("expected not ok for wrong length")
	}
}
