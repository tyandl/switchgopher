package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestUsageMentionsEveryFlagOnce(t *testing.T) {
	var out bytes.Buffer
	writeUsage(&out)
	text := out.String()

	flag.VisitAll(func(f *flag.Flag) {
		if strings.HasPrefix(f.Name, "test.") {
			return // registered by the testing package
		}
		if n := strings.Count(text, "\n  -"+f.Name+"\n") + strings.Count(text, "\n  -"+f.Name+" "); n != 1 {
			t.Errorf("flag -%s appears %d times in the usage text, want exactly 1 (add it to usageGroups?)", f.Name, n)
		}
	})
}

func TestUsageHasOverviewExamplesAndEnvironment(t *testing.T) {
	var out bytes.Buffer
	writeUsage(&out)
	text := out.String()
	for _, want := range []string{"Usage:", "Examples:", "Environment:", "SWITCHGOPHER_PASSWORD_HASH", "README.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("usage text is missing %q", want)
		}
	}
}
