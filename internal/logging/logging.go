// Package logging configures the process-wide logger. The program logs with
// the standard library's log/slog (slog.Debug, slog.Info, slog.Warn,
// slog.Error) and this package only chooses the minimum level and the output
// format (human-readable text or JSON lines).
//
// Levels, and what belongs at each:
//
//	Debug  detail for investigating a problem: every HTTP request, each
//	       device poll, optional features a device does not support.
//	Info   normal events an operator wants to see: startup, seeding, a summary
//	       of each poll cycle, pruning, logins.
//	Warn   something failed but the program carries on: one device did not
//	       answer, an optional fetch failed, a failed login.
//	Error  an operation failed that should not have: a database error, a
//	       handler returning 500, or a startup failure (logged just before exit).
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// ParseLevel converts "debug", "info", "warn" (or "warning") or "error",
// in any case, to a slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level %q (use debug, info, warn or error)", s)
}

// Setup makes a logger writing to w at the given minimum level and format
// ("text" or "json") the default for slog, and routes the standard log
// package's output through it at Info level, so any remaining log.Printf
// calls still appear in the same format.
func Setup(w io.Writer, level, format string) error {
	lvl, err := ParseLevel(level)
	if err != nil {
		return err
	}
	opts := &slog.HandlerOptions{
		Level: lvl,
		// slog's JSON handler writes a time.Duration as nanoseconds, which is
		// unreadable ("took": 20000000). Write it as "20ms" instead.
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindDuration {
				a.Value = slog.StringValue(a.Value.Duration().String())
			}
			return a
		},
	}

	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text", "":
		h = slog.NewTextHandler(w, opts)
	case "json":
		h = slog.NewJSONHandler(w, opts)
	default:
		return fmt.Errorf("unknown log format %q (use text or json)", format)
	}
	slog.SetDefault(slog.New(h))
	return nil
}
