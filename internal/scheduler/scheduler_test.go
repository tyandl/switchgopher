package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

func TestBuildSessionV2c(t *testing.T) {
	cases := []struct {
		name        string
		snmpVersion string
	}{
		{"explicit 2c", "2c"},
		{"empty (pre-migration default)", ""},
		{"anything else that isn't literally \"3\"", "v2"},
	}
	for _, c := range cases {
		target := deviceTarget{Device: "sw1", Address: "10.0.0.1", Community: "public", SnmpVersion: c.snmpVersion}
		session, err := buildSession(context.Background(), target, time.Second)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if session.Version != gosnmp.Version2c {
			t.Errorf("%s: Version = %v, want Version2c", c.name, session.Version)
		}
		if session.Community != "public" {
			t.Errorf("%s: Community = %q, want %q", c.name, session.Community, "public")
		}
	}
}

func TestBuildSessionV3(t *testing.T) {
	target := deviceTarget{
		Device:         "sw1",
		Address:        "10.0.0.1",
		SnmpVersion:    "3",
		Username:       "monitor",
		SecurityLevel:  "authPriv",
		AuthProtocol:   "SHA",
		AuthPassphrase: "authpass123",
		PrivProtocol:   "AES",
		PrivPassphrase: "privpass123",
		ContextName:    "vrf-mgmt",
	}
	session, err := buildSession(context.Background(), target, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.Version != gosnmp.Version3 {
		t.Fatalf("Version = %v, want Version3", session.Version)
	}
	if session.SecurityModel != gosnmp.UserSecurityModel {
		t.Errorf("SecurityModel = %v, want UserSecurityModel", session.SecurityModel)
	}
	if session.MsgFlags != gosnmp.AuthPriv {
		t.Errorf("MsgFlags = %v, want AuthPriv", session.MsgFlags)
	}
	if session.ContextName != "vrf-mgmt" {
		t.Errorf("ContextName = %q, want %q", session.ContextName, "vrf-mgmt")
	}
	usp, ok := session.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		t.Fatalf("SecurityParameters is %T, want *gosnmp.UsmSecurityParameters", session.SecurityParameters)
	}
	if usp.UserName != "monitor" {
		t.Errorf("UserName = %q, want %q", usp.UserName, "monitor")
	}
	if usp.AuthenticationProtocol != gosnmp.SHA {
		t.Errorf("AuthenticationProtocol = %v, want SHA", usp.AuthenticationProtocol)
	}
	if usp.AuthenticationPassphrase != "authpass123" {
		t.Errorf("AuthenticationPassphrase = %q, want %q", usp.AuthenticationPassphrase, "authpass123")
	}
	if usp.PrivacyProtocol != gosnmp.AES {
		t.Errorf("PrivacyProtocol = %v, want AES", usp.PrivacyProtocol)
	}
	if usp.PrivacyPassphrase != "privpass123" {
		t.Errorf("PrivacyPassphrase = %q, want %q", usp.PrivacyPassphrase, "privpass123")
	}
}

func TestBuildSessionV3NoAuthNoPriv(t *testing.T) {
	target := deviceTarget{
		Device:        "sw1",
		SnmpVersion:   "3",
		Username:      "readonly",
		SecurityLevel: "noAuthNoPriv",
	}
	session, err := buildSession(context.Background(), target, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	usp := session.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if usp.AuthenticationProtocol != 0 {
		t.Errorf("AuthenticationProtocol = %v, want unset (0) for noAuthNoPriv", usp.AuthenticationProtocol)
	}
	if usp.PrivacyProtocol != 0 {
		t.Errorf("PrivacyProtocol = %v, want unset (0) for noAuthNoPriv", usp.PrivacyProtocol)
	}
}

func TestBuildSessionV3AuthNoPriv(t *testing.T) {
	target := deviceTarget{
		Device:         "sw1",
		SnmpVersion:    "3",
		Username:       "monitor",
		SecurityLevel:  "authNoPriv",
		AuthProtocol:   "MD5",
		AuthPassphrase: "authpass123",
		// PrivProtocol deliberately left unset - authNoPriv must not require it.
	}
	session, err := buildSession(context.Background(), target, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	usp := session.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if usp.AuthenticationProtocol != gosnmp.MD5 {
		t.Errorf("AuthenticationProtocol = %v, want MD5", usp.AuthenticationProtocol)
	}
	if usp.PrivacyProtocol != 0 {
		t.Errorf("PrivacyProtocol = %v, want unset (0) for authNoPriv", usp.PrivacyProtocol)
	}
}

func TestBuildSessionV3Errors(t *testing.T) {
	cases := []struct {
		name   string
		target deviceTarget
	}{
		{"unrecognized SecurityLevel", deviceTarget{SnmpVersion: "3", SecurityLevel: "bogus"}},
		{"missing SecurityLevel", deviceTarget{SnmpVersion: "3"}},
		{"unrecognized AuthProtocol", deviceTarget{SnmpVersion: "3", SecurityLevel: "authNoPriv", AuthProtocol: "bogus"}},
		{"missing AuthProtocol", deviceTarget{SnmpVersion: "3", SecurityLevel: "authPriv"}},
		{"unrecognized PrivProtocol", deviceTarget{SnmpVersion: "3", SecurityLevel: "authPriv", AuthProtocol: "SHA", PrivProtocol: "bogus"}},
		{"missing PrivProtocol", deviceTarget{SnmpVersion: "3", SecurityLevel: "authPriv", AuthProtocol: "SHA"}},
	}
	for _, c := range cases {
		if _, err := buildSession(context.Background(), c.target, time.Second); err == nil {
			t.Errorf("%s: expected an error, got nil", c.name)
		}
	}
}
