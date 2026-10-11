// Package config loads configuration/preferences.json into a shared,
// process-wide map - preserving the app's existing global-preferences
// pattern, just behind a package boundary instead of a bare package-main var.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Preferences holds each setting's value from configuration/preferences.json,
// keyed by name. JSON numbers land as float64 - use Int to read one as an
// integer.
var Preferences map[string]interface{}

// entry is how each setting is written in preferences.json. Only Value is
// read: description, type and required are documentation for people editing
// the file, so deleting them never invalidates the config.
type entry struct {
	Value interface{} `json:"value"`
}

// Load reads the preferences file at path and fills Preferences with each
// entry's value. An entry with no value is skipped, so callers fall back to
// their built-in default.
func Load(path string) error {
	prefFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening preferences: %w", err)
	}
	defer prefFile.Close()

	var entries map[string]entry
	if err := json.NewDecoder(prefFile).Decode(&entries); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	prefs := make(map[string]interface{}, len(entries))
	for key, e := range entries {
		if e.Value != nil {
			prefs[key] = e.Value
		}
	}
	Preferences = prefs
	return nil
}

// Int reads an integer preference (JSON numbers decode as float64), falling
// back to def when absent, of the wrong type, or when Preferences itself is
// nil (e.g. Load failed - Load already logged that).
func Int(key string, def int) int {
	if v, ok := Preferences[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return def
}

// String reads a string preference, falling back to def when absent, of the
// wrong type, or when Preferences itself is nil (e.g. Load failed - Load
// already logged that).
func String(key, def string) string {
	if v, ok := Preferences[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}
