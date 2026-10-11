package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/tyandl/switchgopher/internal/discover"
	"github.com/tyandl/switchgopher/internal/scheduler"
	"github.com/tyandl/switchgopher/internal/snmp"
)

// Discovery flags - see documentation/TODO.md for the full design. This
// tool bootstraps a brand-new installation, typically run once -
// deliberately no configuration-file dependency at all (not
// configuration/preferences.json, not a dedicated patterns file): every
// decision is either a flag or an interactive prompt.
var (
	discoverMode            = flag.Bool("discover", false, "Run device discovery (subnet sweep by default, or CDP crawl via -discover-cdp) and write candidates to -discover-output, then exit")
	discoverSubnet          = flag.String("discover-subnet", "", "CIDR to sweep for candidate switches, e.g. 192.0.2.0/24 (prompted if empty, unless -discover-noninteractive)")
	discoverCommunity       = flag.String("discover-community", "", "SNMPv2c community string to try during discovery (defaults to \"public\" if not supplied, prompted for first if interactive)")
	discoverPort            = flag.Int("discover-port", 161, "SNMP port to probe during discovery")
	discoverCDP             = flag.String("discover-cdp", "", "Device name (already seeded) to CDP-crawl one hop from, instead of a subnet sweep")
	discoverOutput          = flag.String("discover-output", "configuration/discovered-devices.json", "Where to write discovered candidate devices - never configuration/devices.json itself, which stays hand-maintained")
	discoverBuildingsOutput = flag.String("discover-buildings-output", "configuration/discovered-buildings.json", "Where to write the distinct Building values found by naming patterns - never configuration/buildings.json itself")
	discoverNonInteractive  = flag.Bool("discover-noninteractive", false, "Error out instead of interactively prompting for a missing required discovery parameter")
	discoverProbeTimeout    = flag.Duration("discover-probe-timeout", 2*time.Second, "Timeout for the initial liveness probe against each candidate address")
	discoverFetchTimeout    = flag.Duration("discover-fetch-timeout", 10*time.Second, "Timeout for the fuller SNMP identify pass against a responding candidate")
	discoverConcurrency     = flag.Int("discover-concurrency", 20, "How many hosts to probe at once during a subnet sweep")
	discoverPatternFlags    patternList
)

func init() {
	flag.Var(&discoverPatternFlags, "discover-pattern", "Regex with named groups (Device/Category/Building/Room/Model/Index) to extract fields from each discovered device's name - repeatable (pass -discover-pattern multiple times); prompted for interactively instead if none given")
}

// patternList implements flag.Value so -discover-pattern can be repeated
// on the command line (flag.String only keeps the last occurrence of a
// given flag name, no good for a list).
type patternList []string

// String implements flag.Value.
func (p *patternList) String() string {
	return strings.Join(*p, ", ")
}

// Set implements flag.Value; each occurrence of the flag appends one pattern.
func (p *patternList) Set(value string) error {
	*p = append(*p, value)
	return nil
}

// runDiscovery dispatches to discover.Sweep or discover.CrawlCDP
// (-discover-cdp opts into the latter), applies any configured naming
// patterns, and writes the result to -discover-output. Candidates are
// never merged into configuration/devices.json directly - Category/
// Building/Room (when no pattern resolves them) and, for CDP-discovered
// neighbors, credentials, need a human's review first.
func runDiscovery(database *sql.DB) {
	if err := snmp.InitMIBs("resources/mibs"); err != nil {
		fatal("cannot load the MIB files", "err", err)
	}

	probeTimeout := *discoverProbeTimeout
	fetchTimeout := *discoverFetchTimeout
	concurrency := *discoverConcurrency

	known, err := loadKnownAddresses(database)
	if err != nil {
		fatal("discovery: cannot load known devices", "err", err)
	}

	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	// communityOrDefault has a safe, reasonable default ("public") -
	// unlike subnet below, it's never a hard requirement: missing +
	// non-interactive just uses the default silently, missing +
	// interactive offers a prompt (showing that default, so Enter
	// accepts it). Called after whichever primary decision (subnet or
	// CDP seed) each branch makes first, so prompts appear in a natural
	// order - the main decision, then supporting detail.
	communityOrDefault := func() string {
		if *discoverCommunity != "" {
			return *discoverCommunity
		}
		if *discoverNonInteractive {
			return "public"
		}
		return promptString(reader, "SNMPv2c community to try", "public")
	}

	var candidates []discover.Candidate

	if *discoverCDP != "" {
		session, err := scheduler.BuildSessionForDevice(ctx, database, *discoverCDP, fetchTimeout)
		if err != nil {
			fatal("discovery failed", "err", err)
		}
		if err := session.Connect(); err != nil {
			fatal("discovery: cannot connect to the CDP seed device", "device", *discoverCDP, "err", err)
		}
		defer session.Close()

		candidates, err = discover.CrawlCDP(ctx, session, communityOrDefault(), uint16(*discoverPort), probeTimeout, fetchTimeout, known)
		if err != nil {
			fatal("discovery: CDP crawl failed", "device", *discoverCDP, "err", err)
		}
	} else {
		// Subnet has no safe default - genuinely required, so missing +
		// non-interactive is a hard error rather than a silent guess.
		subnet := *discoverSubnet
		if subnet == "" {
			if *discoverNonInteractive {
				fatal("discovery: -discover-subnet is required in non-interactive mode")
			}
			subnet = promptString(reader, "Management subnet to sweep (CIDR)", "")
			if subnet == "" {
				fatal("discovery: a subnet is required")
			}
		}

		candidates, err = discover.Sweep(ctx, subnet, communityOrDefault(), uint16(*discoverPort), probeTimeout, fetchTimeout, concurrency, known)
		if err != nil {
			fatal("discovery: subnet sweep failed", "err", err)
		}
	}

	// No configuration-file source for patterns at all (see the var
	// block's doc comment) - -discover-pattern (repeatable) if given,
	// else an interactive prompt loop, else none at all in non-
	// interactive mode (patterns are always optional/additive, never
	// required - zero patterns just means every candidate's Category/
	// Building/Room/Model/Index stays whatever Sweep/CrawlCDP already
	// found, usually blank).
	var patterns []*regexp.Regexp
	if len(discoverPatternFlags) > 0 {
		patterns, err = discover.CompilePatterns(discoverPatternFlags)
		if err != nil {
			fatal("discovery: invalid -discover-pattern", "err", err)
		}
	} else if !*discoverNonInteractive {
		patterns = promptPatterns(reader)
	}
	candidates = discover.ApplyPatterns(candidates, patterns)

	if err := writeCandidates(*discoverOutput, candidates); err != nil {
		fatal("discovery: cannot write the results", "file", *discoverOutput, "err", err)
	}
	fmt.Printf("discovered %d candidate device(s), wrote %s\n", len(candidates), *discoverOutput)

	knownBuildings, err := loadKnownBuildings(database)
	if err != nil {
		fatal("discovery: cannot load known buildings", "err", err)
	}
	buildings := discover.DistinctBuildings(candidates, knownBuildings)
	if len(buildings) > 0 {
		if err := writeBuildings(*discoverBuildingsOutput, buildings); err != nil {
			fatal("discovery: cannot write the buildings", "file", *discoverBuildingsOutput, "err", err)
		}
		fmt.Printf("found %d new building(s) from naming patterns, wrote %s\n", len(buildings), *discoverBuildingsOutput)
	}

	fmt.Println("review Category/Building/Room (and, for CDP-discovered neighbors, credentials) before merging into configuration/devices.json - a pattern-matched Building may just be a short code (e.g. \"HQ\"), not the full name; same for anything in", *discoverBuildingsOutput)
}

// promptPatterns offers an interactive prompt loop for naming patterns
// when none were given via -discover-pattern - repeatable the same way
// the flag is, one pattern per line, a blank line to stop (including
// stopping immediately with zero patterns, which is a completely valid
// choice: patterns are always optional).
func promptPatterns(reader *bufio.Reader) []*regexp.Regexp {
	fmt.Println("Naming pattern(s) - regex with named groups (Device/Category/Building/Room/Model/Index) to extract fields from each discovered device's name, e.g. ^(?P<Building>[A-Za-z]+)-(?P<Room>[A-Za-z0-9]+)-(?P<Model>[A-Za-z0-9]+)-(?P<Index>[A-Za-z0-9]+)$")
	var raw []string
	for i := 1; ; i++ {
		line := promptString(reader, fmt.Sprintf("Pattern %d (blank to stop)", i), "")
		if line == "" {
			break
		}
		raw = append(raw, line)
	}
	patterns, err := discover.CompilePatterns(raw)
	if err != nil {
		fatal("discovery failed", "err", err)
	}
	return patterns
}

// promptString prompts once on stdout and reads one line from reader -
// a single shared reader across an entire discovery run, not one per
// call, so a line typed (or pasted) ahead of being prompted for isn't
// silently dropped by a fresh bufio.Reader's own internal buffering.
func promptString(reader *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// loadKnownAddresses returns the host (port stripped, if present) of
// every already-seeded device's Address - Sweep/CrawlCDP skip these, so
// re-running discovery against an increasingly-known fleet doesn't keep
// re-surfacing already-tracked switches.
func loadKnownAddresses(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT Address FROM devices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	known := map[string]bool{}
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		known[host] = true
	}
	return known, rows.Err()
}

// loadKnownBuildings returns every Building and ShortName value already
// in the seeded buildings table, combined into one set - DistinctBuildings
// uses this to skip a pattern-matched value that's already configured
// (even just by its short code), so it doesn't keep resurfacing on every
// discovery run.
func loadKnownBuildings(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT Building, ShortName FROM buildings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	known := map[string]bool{}
	for rows.Next() {
		var building string
		var shortName sql.NullString
		if err := rows.Scan(&building, &shortName); err != nil {
			return nil, err
		}
		known[building] = true
		if shortName.Valid && shortName.String != "" {
			known[shortName.String] = true
		}
	}
	return known, rows.Err()
}

// writeCandidates writes {"Devices": [...]} - the same shape
// configuration/devices.json itself uses, so a reviewed entry can be
// copy-pasted straight across.
func writeCandidates(path string, candidates []discover.Candidate) error {
	out := struct {
		Devices []discover.Candidate
	}{Devices: candidates}
	data, err := json.MarshalIndent(out, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// writeBuildings writes {"Buildings": [...]} - the same shape
// configuration/buildings.json itself uses.
func writeBuildings(path string, buildings []discover.BuildingCandidate) error {
	out := struct {
		Buildings []discover.BuildingCandidate
	}{Buildings: buildings}
	data, err := json.MarshalIndent(out, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
