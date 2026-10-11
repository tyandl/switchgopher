package scheduler

import (
	"strings"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("SG_TEST_A", "alpha")
	t.Setenv("SG_TEST_B", "b$tar")
	for in, want := range map[string]string{
		"plain":            "plain",
		"":                 "",
		"${SG_TEST_A}":     "alpha",
		"x-${SG_TEST_A}-y": "x-alpha-y",
		"${SG_TEST_B}":     "b$tar",
		"$${SG_TEST_A}":    "${SG_TEST_A}",
		"cost$5":           "cost$5",
		"a${":              "a${",
		"${not valid}":     "${not valid}",
	} {
		got, err := expandEnv(in)
		if err != nil || got != want {
			t.Errorf("expandEnv(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestExpandEnvUnsetOrEmptyIsAnError(t *testing.T) {
	t.Setenv("SG_TEST_EMPTY", "")
	for _, in := range []string{"${SG_TEST_UNSET_XYZ}", "${SG_TEST_EMPTY}"} {
		if _, err := expandEnv(in); err == nil {
			t.Errorf("expandEnv(%q): want error", in)
		}
	}
}

func TestBuildSessionExpandsPlaceholders(t *testing.T) {
	t.Setenv("SG_TEST_COMM", "s3cret")
	s, err := buildSession(t.Context(), deviceTarget{Device: "d", Address: "192.0.2.1", Community: "${SG_TEST_COMM}"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s.Community != "s3cret" {
		t.Errorf("Community = %q, want expanded value", s.Community)
	}
}

func TestBuildSessionMissingVariableNamesDeviceNotValue(t *testing.T) {
	_, err := buildSession(t.Context(), deviceTarget{Device: "core-1", Address: "192.0.2.1", Community: "${SG_TEST_UNSET_XYZ}"}, 1)
	if err == nil || !strings.Contains(err.Error(), "core-1") || !strings.Contains(err.Error(), "SG_TEST_UNSET_XYZ") {
		t.Errorf("want error naming device and variable, got %v", err)
	}
}
