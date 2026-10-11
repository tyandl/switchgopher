package discover

import (
	"fmt"
	"regexp"
)

// recognizedGroups is the explicit, documented set of named capture
// groups a pattern can use - deliberately not arbitrary reflection-based
// field matching, so the contract between a pattern author and this tool
// stays explicit.
var recognizedGroups = map[string]bool{
	"Device": true, "Category": true, "Building": true,
	"Room": true, "Model": true, "Index": true,
}

// CompilePatterns compiles a list of regex strings in order. This tool
// is meant to bootstrap a brand-new installation, typically run once -
// deliberately no configuration-file source for patterns at all, only
// the command line's repeatable -discover-pattern flag or an
// interactive prompt loop (see cmd/switchgopher/discover.go) feed this.
func CompilePatterns(raw []string) ([]*regexp.Regexp, error) {
	patterns := make([]*regexp.Regexp, 0, len(raw))
	for _, p := range raw {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("compiling pattern %q: %w", p, err)
		}
		patterns = append(patterns, re)
	}
	return patterns, nil
}

// ApplyPatterns tries each pattern in order against each candidate's
// Device name (first match wins, so a site with more than one historical
// naming convention can list several patterns); whichever recognized
// named groups the matching pattern defines overwrite the corresponding
// Candidate fields verbatim - whatever substring the regex captured,
// no resolution or lookup against anything else. A pattern's "Building"
// group capturing a short code (e.g. "HQ" out of "HQ-R1-3560-01") is
// expected and fine: bootstrapping only needs to get close enough to be
// useful, not exactly right - a human can rename it later. A candidate
// that matches nothing is returned unchanged - this is purely additive
// on top of whatever Sweep/CrawlCDP already filled in. Deliberately pure
// (no network, no DB) so it's unit-testable directly.
func ApplyPatterns(candidates []Candidate, patterns []*regexp.Regexp) []Candidate {
	out := make([]Candidate, len(candidates))
	for i, c := range candidates {
		out[i] = applyPatternsToOne(c, patterns)
	}
	return out
}

// applyPatternsToOne tries each pattern in order against c.Device. The first
// one that matches sets the fields its named groups captured, and the rest are
// not tried.
func applyPatternsToOne(c Candidate, patterns []*regexp.Regexp) Candidate {
	for _, re := range patterns {
		match := re.FindStringSubmatch(c.Device)
		if match == nil {
			continue
		}
		for i, name := range re.SubexpNames() {
			if i == 0 || match[i] == "" || !recognizedGroups[name] {
				continue
			}
			switch name {
			case "Device":
				c.Device = match[i]
			case "Category":
				c.Category = match[i]
			case "Building":
				c.Building = match[i]
			case "Room":
				c.Room = match[i]
			case "Model":
				c.Model = match[i]
			case "Index":
				c.Index = match[i]
			}
		}
		break
	}
	return c
}

// BuildingCandidate is one row of a discovered-buildings.json entry,
// shaped to match configuration/buildings.json's own fields exactly -
// ShortName/Description are always blank here, since a regex capture is
// just the one matched substring, never a separate short-code-to-
// full-name mapping (there's nothing in a device's own name that could
// supply a full name a short code doesn't already contain).
type BuildingCandidate struct {
	Building, ShortName, Description string
}

// DistinctBuildings collects every distinct, non-empty Building value
// across candidates (in first-seen order) - the "list of buildings" that
// seeds configuration/buildings.json, per the pattern-matched results
// across however many candidates shared the same discovered Building
// value. known is the set of Building and ShortName values already
// present in the seeded buildings table - skipped, so a building the
// site has already configured (even just by its short code) doesn't
// keep resurfacing on every discovery run.
func DistinctBuildings(candidates []Candidate, known map[string]bool) []BuildingCandidate {
	seen := map[string]bool{}
	var buildings []BuildingCandidate
	for _, c := range candidates {
		if c.Building == "" || seen[c.Building] || known[c.Building] {
			continue
		}
		seen[c.Building] = true
		buildings = append(buildings, BuildingCandidate{Building: c.Building})
	}
	return buildings
}
