package discover

import (
	"regexp"
	"testing"
)

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compiling %q: %v", pattern, err)
	}
	return re
}

func TestApplyPatternsFullMatch(t *testing.T) {
	patterns := []*regexp.Regexp{
		mustCompile(t, `^(?P<Building>[A-Za-z]+)-(?P<Room>[A-Za-z0-9]+)-(?P<Model>[A-Za-z0-9]+)-(?P<Index>[A-Za-z0-9]+)$`),
	}

	got := ApplyPatterns([]Candidate{{Device: "HQ-R1-3560-01"}}, patterns)

	// Building captures the bare short code "HQ" directly - no
	// resolution against anything else. That's expected: bootstrapping
	// only needs to get close enough to be useful, a human renames it
	// later if they want the full name instead.
	want := Candidate{Device: "HQ-R1-3560-01", Building: "HQ", Room: "R1", Model: "3560", Index: "01"}
	if got[0] != want {
		t.Errorf("ApplyPatterns() = %+v, want %+v", got[0], want)
	}
}

func TestApplyPatternsPartialGroups(t *testing.T) {
	// Pattern only captures Room - Model/Index/Building should stay blank.
	patterns := []*regexp.Regexp{mustCompile(t, `^[A-Za-z]+-(?P<Room>[A-Za-z0-9]+)-`)}

	got := ApplyPatterns([]Candidate{{Device: "HQ-R1-3560-01"}}, patterns)

	if got[0].Room != "R1" {
		t.Errorf("Room = %q, want %q", got[0].Room, "R1")
	}
	if got[0].Building != "" || got[0].Model != "" || got[0].Index != "" {
		t.Errorf("expected Building/Model/Index to stay blank, got %+v", got[0])
	}
}

func TestApplyPatternsNoMatch(t *testing.T) {
	patterns := []*regexp.Regexp{mustCompile(t, `^NEVER-MATCHES-(?P<Room>.+)$`)}

	original := Candidate{Device: "10-0-0-5"}
	got := ApplyPatterns([]Candidate{original}, patterns)

	if got[0] != original {
		t.Errorf("ApplyPatterns() with no match = %+v, want unchanged %+v", got[0], original)
	}
}

func TestApplyPatternsFirstMatchWins(t *testing.T) {
	patterns := []*regexp.Regexp{
		mustCompile(t, `^HQ-(?P<Room>[A-Za-z0-9]+)-`),                          // matches first, should win
		mustCompile(t, `^(?P<Building>[A-Za-z]+)-.*-(?P<Index>[A-Za-z0-9]+)$`), // would also match
	}

	got := ApplyPatterns([]Candidate{{Device: "HQ-R1-3560-01"}}, patterns)

	if got[0].Room != "R1" {
		t.Errorf("Room = %q, want %q (first pattern should win)", got[0].Room, "R1")
	}
	if got[0].Index != "" {
		t.Errorf("Index = %q, want empty - second pattern should never have run", got[0].Index)
	}
}

func TestApplyPatternsCategoryGroup(t *testing.T) {
	patterns := []*regexp.Regexp{mustCompile(t, `^(?P<Category>Core|Access)-`)}

	got := ApplyPatterns([]Candidate{{Device: "Core-HQ-3560-01"}}, patterns)

	if got[0].Category != "Core" {
		t.Errorf("Category = %q, want %q", got[0].Category, "Core")
	}
}

func TestCompilePatterns(t *testing.T) {
	patterns, err := CompilePatterns([]string{`^(?P<Room>[A-Za-z0-9]+)-`, `^(?P<Model>\d+)$`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patterns) != 2 {
		t.Fatalf("got %d patterns, want 2", len(patterns))
	}
	if !patterns[0].MatchString("BA-3560") || !patterns[1].MatchString("3560") {
		t.Errorf("compiled patterns didn't match as expected")
	}
}

func TestCompilePatternsEmpty(t *testing.T) {
	patterns, err := CompilePatterns(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("got %d patterns, want 0", len(patterns))
	}
}

func TestCompilePatternsInvalidRegex(t *testing.T) {
	if _, err := CompilePatterns([]string{"(unclosed"}); err == nil {
		t.Error("expected an error for an invalid regex, got nil")
	}
}

func TestDistinctBuildings(t *testing.T) {
	candidates := []Candidate{
		{Device: "a", Building: "HQ"},
		{Device: "b", Building: "HQ"}, // duplicate - should appear once
		{Device: "c", Building: "LAB"},
		{Device: "d", Building: ""}, // blank - skipped
	}

	got := DistinctBuildings(candidates, nil)

	want := []BuildingCandidate{{Building: "HQ"}, {Building: "LAB"}}
	if len(got) != len(want) {
		t.Fatalf("got %d buildings, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("buildings[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDistinctBuildingsSkipsKnown(t *testing.T) {
	candidates := []Candidate{
		{Device: "a", Building: "HQ"},  // already known by short code
		{Device: "b", Building: "LAB"}, // new
	}
	known := map[string]bool{"HQ": true}

	got := DistinctBuildings(candidates, known)

	if len(got) != 1 || got[0].Building != "LAB" {
		t.Errorf("DistinctBuildings() = %+v, want just [{LAB}]", got)
	}
}

func TestDistinctBuildingsNoneFound(t *testing.T) {
	got := DistinctBuildings([]Candidate{{Device: "a", Building: ""}}, nil)
	if len(got) != 0 {
		t.Errorf("DistinctBuildings() = %+v, want empty", got)
	}
}
