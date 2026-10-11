package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "INFO": slog.LevelInfo, "": slog.LevelInfo,
		"warn": slog.LevelWarn, "Warning": slog.LevelWarn, " error ": slog.LevelError,
	} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("ParseLevel(verbose) should fail")
	}
}

func TestSetupFiltersByLevel(t *testing.T) {
	defer slog.SetDefault(slog.Default())
	var buf bytes.Buffer
	if err := Setup(&buf, "warn", "text"); err != nil {
		t.Fatal(err)
	}
	slog.Debug("hidden debug")
	slog.Info("hidden info")
	slog.Warn("shown warn", "device", "core-1")
	slog.Error("shown error")

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Errorf("messages below the level were logged:\n%s", out)
	}
	if !strings.Contains(out, "shown warn") || !strings.Contains(out, "device=core-1") || !strings.Contains(out, "shown error") {
		t.Errorf("expected warn and error lines with attributes:\n%s", out)
	}
}

func TestSetupJSON(t *testing.T) {
	defer slog.SetDefault(slog.Default())
	var buf bytes.Buffer
	if err := Setup(&buf, "debug", "json"); err != nil {
		t.Fatal(err)
	}
	slog.Info("hello", "count", 3, "took", 1500*time.Millisecond)

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if rec["took"] != "1.5s" {
		t.Errorf("a duration should be written as text, got %v", rec["took"])
	}
	if rec["msg"] != "hello" || rec["level"] != "INFO" || rec["count"] != float64(3) {
		t.Errorf("unexpected record: %v", rec)
	}
}

func TestSetupRejectsBadValues(t *testing.T) {
	var buf bytes.Buffer
	if err := Setup(&buf, "loud", "text"); err == nil {
		t.Error("bad level should fail")
	}
	if err := Setup(&buf, "info", "xml"); err == nil {
		t.Error("bad format should fail")
	}
}
