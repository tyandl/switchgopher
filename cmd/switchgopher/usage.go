package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

func init() {
	flag.Usage = func() { writeUsage(flag.CommandLine.Output()) }
}

// usageGroup is one section of the -help output.
type usageGroup struct {
	title string
	flags []string
}

// usageGroups puts every flag in a section so -help reads as modes and
// their options instead of one alphabetical list. A test fails if a flag is
// registered but missing here.
var usageGroups = []usageGroup{
	{"Server (the default when no action flag is given)", []string{"port", "db", "auth-user", "auth-password-hash"}},
	{"One-shot actions (do one thing, then exit)", []string{"seed", "hash-password", "pollonce", "discover"}},
	{"Logging (applies to every mode)", []string{"log-level", "log-format"}},
	{"Discovery options (used with -discover)", []string{
		"discover-subnet", "discover-cdp", "discover-community", "discover-port",
		"discover-pattern", "discover-output", "discover-buildings-output",
		"discover-noninteractive", "discover-probe-timeout", "discover-fetch-timeout",
		"discover-concurrency",
	}},
}

const usageHeader = `SwitchGopher polls network switches over SNMP and serves a web UI showing
what is plugged into each port. It only reads from your network.

Usage:
  switchgopher [options]            start the poller and the web server
  switchgopher <action> [options]   do one thing and exit (see actions below)

Run it from the project directory: it reads configuration/ and resources/
using relative paths.
`

const usageFooter = `
Examples:
  # Make a login hash, then start the server on port 8000
  echo 'a-password' | switchgopher -hash-password
  SWITCHGOPHER_PASSWORD_HASH='<hash>' switchgopher

  # Reload configuration/*.json after editing it
  switchgopher -seed

  # Poll one device and print any error (good for debugging a device)
  switchgopher -pollonce core-1

  # Find switches on a management subnet
  switchgopher -discover -discover-subnet 192.0.2.0/24

Environment:
  SWITCHGOPHER_PASSWORD_HASH   bcrypt hash of the web login password
                               (the server will not start without one)
  SWITCHGOPHER_LOG_LEVEL       default for -log-level (debug, info, warn, error)
  SWITCHGOPHER_LOG_FORMAT      default for -log-format (text, json)
  <NAME>                       any variable referenced as ${NAME} in
                               configuration/devices.json, read when a device
                               is polled

More: README.md (setup), documentation/ARCHITECTURE.md (how it works).
`

// writeUsage prints the -help text to w.
func writeUsage(w io.Writer) {
	fmt.Fprint(w, usageHeader)
	for _, g := range usageGroups {
		fmt.Fprintf(w, "\n%s:\n", g.title)
		for _, name := range g.flags {
			if f := flag.Lookup(name); f != nil {
				writeFlag(w, f)
			}
		}
	}
	fmt.Fprint(w, usageFooter)
}

// writeFlag prints one flag in the same layout as the flag package's own
// PrintDefaults: the name and value type, then the description and default.
func writeFlag(w io.Writer, f *flag.Flag) {
	valueType, usage := flag.UnquoteUsage(f)
	var b strings.Builder
	fmt.Fprintf(&b, "  -%s", f.Name)
	if valueType != "" {
		b.WriteString(" " + valueType)
	}
	b.WriteString("\n    \t" + usage)
	if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
		if valueType == "string" {
			fmt.Fprintf(&b, " (default %q)", f.DefValue)
		} else {
			fmt.Fprintf(&b, " (default %s)", f.DefValue)
		}
	}
	fmt.Fprintln(w, b.String())
}
