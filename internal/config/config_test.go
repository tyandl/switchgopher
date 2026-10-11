package config

import (
	"os"
	"path/filepath"
	"testing"
)

func load(t *testing.T, src string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	Preferences = nil
	return Load(path)
}

func TestLoadReadsOnlyValue(t *testing.T) {
	// Full entry, and entries with every documentation field deleted.
	err := load(t, `{
		"n": {"description": "d; default: 1", "type": "integer", "required": true, "value": 7},
		"s": {"value": "x"},
		"extra": {"value": 3, "unknown_field": true}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if Int("n", 0) != 7 || String("s", "") != "x" || Int("extra", 0) != 3 {
		t.Errorf("got %v", Preferences)
	}
}

func TestLoadIgnoresEntriesWithoutValue(t *testing.T) {
	// "required": true and a mismatched type are documentation only.
	if err := load(t, `{"n": {"type": "string", "required": true}}`); err != nil {
		t.Fatal(err)
	}
	if got := Int("n", 42); got != 42 {
		t.Errorf("Int = %d, want caller default 42", got)
	}
}

func TestShippedPreferencesLoad(t *testing.T) {
	Preferences = nil
	if err := Load("../../configuration/preferences.json"); err != nil {
		t.Fatalf("shipped preferences.json does not load: %v", err)
	}
	if Int("poll_interval_seconds", 0) != 300 || String("dated_config_dir", "") == "" {
		t.Errorf("unexpected values: %v", Preferences)
	}
}
