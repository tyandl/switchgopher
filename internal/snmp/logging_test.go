package snmp

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestLogOptionalFailureLevels(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(old)

	logOptionalFailure("temperature", "192.0.2.1", fmt.Errorf("no sensor: %w", ErrNotSupported))
	logOptionalFailure("portPoE", "192.0.2.1", errors.New("timeout"))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines, got %d:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "level=DEBUG") || !strings.Contains(lines[0], "group=temperature") {
		t.Errorf("an unsupported feature should log at DEBUG:\n%s", lines[0])
	}
	if !strings.Contains(lines[1], "level=WARN") || !strings.Contains(lines[1], "group=portPoE") || !strings.Contains(lines[1], "target=192.0.2.1") {
		t.Errorf("a real failure should log at WARN with the target:\n%s", lines[1])
	}
}
