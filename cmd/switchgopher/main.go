// Command switchgopher polls network switches over SNMP and serves a web UI
// showing what's plugged into each port - a Go rewrite of the classic Perl
// "switchmap" tool.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tyandl/switchgopher/internal/config"
	"github.com/tyandl/switchgopher/internal/scheduler"
	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/store"
	"github.com/tyandl/switchgopher/internal/web"

	// Blank-imported so every vendor fetch package's init() runs and
	// registers its implementations into internal/snmp's registry - see
	// internal/snmp/drivers/README.md.
	_ "github.com/tyandl/switchgopher/internal/snmp/drivers/register"
)

// pollOnce is set via -pollonce=<device> for fast manual iteration during
// development (e.g. against a local snmpd test instance), polling a single
// already-seeded device once and exiting instead of starting the scheduler
// and web server.
var pollOnce = flag.String("pollonce", "", "Poll a single device (by name, as already present in the devices table) once and exit, instead of starting the server")

// seed is set via -seed to (re)populate the static inventory from
// configuration/*.json and exit - how the database gets recreated if it's
// lost or rebuilt from scratch. See store.Seed's doc comment for exactly
// what it does and does not touch.
var seed = flag.Bool("seed", false, "Populate/refresh buildings, categories, vlans, and devices from configuration/*.json, then exit")

// httpPort is set via -port so more than one instance can run at once
// (e.g. a real dev server plus a throwaway one for testing) without both
// trying to bind :8000.
// dbPath lets a container keep the database on a mounted volume instead of
// inside the (disposable) image.
var dbPath = flag.String("db", "resources/inventory.sqlite", "Path to the SQLite database file")

var httpPort = flag.Int("port", 8000, "HTTP port to listen on")

// Shared login for the web UI - see internal/web/auth.go. The hash can also
// come from SWITCHGOPHER_PASSWORD_HASH, which keeps it out of the process list.
var authUser = flag.String("auth-user", "admin", "Username for the shared web UI login")
var authHash = flag.String("auth-password-hash", "", "bcrypt hash of the web UI password (or set SWITCHGOPHER_PASSWORD_HASH); generate one with -hash-password")
var hashPassword = flag.Bool("hash-password", false, "Read a password from stdin, print its bcrypt hash for -auth-password-hash, then exit")

// seedAll loads configuration/*.json and the MAC-vendor (OUI) list.
func seedAll(database *sql.DB) error {
	if err := store.Seed(database); err != nil {
		return err
	}
	if err := store.SeedOui(database, "resources/oui/oui.csv"); err != nil {
		return fmt.Errorf("oui: %w", err)
	}
	return nil
}

func main() {
	flag.Parse()

	if *hashPassword {
		fmt.Fprint(os.Stderr, "Password: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		hash, err := web.HashPassword(strings.TrimRight(line, "\r\n"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(hash)
		return
	}

	// attempt to establish a connection to the database.
	database, err := store.Connect(*dbPath)
	if err != nil {
		// if dbglvl >= DEBUG {log.Println(err)} // not needed here since err was printed already.
		log.Println("unable to establish a connection to the database. Shutting down.")
		if database != nil {
			// we need to close if the connection was setup but not successfully established.
			database.Close()
		}
		log.Fatal("Closing...")
	}
	defer database.Close()
	// the database connection was successful, create tables
	// (If they already exist, no changes will be made)
	if err := store.Setup(database); err != nil {
		log.Fatalf("setting up the database schema: %v", err)
	}
	if err := config.Load("configuration/preferences.json"); err != nil {
		log.Printf("preferences: %v (using built-in defaults)", err)
	}

	if *seed {
		if err := seedAll(database); err != nil {
			log.Fatalf("seed: %v", err)
		}
		fmt.Println("seeded database successfully")
		return
	}

	// A database that has never been seeded would otherwise start empty, so
	// seed it once automatically. Later changes to configuration/ still need
	// an explicit -seed.
	if fresh, err := store.NeedsSeed(database); err != nil {
		log.Fatalf("checking whether the database is seeded: %v", err)
	} else if fresh {
		log.Println("database has never been seeded; seeding from configuration/ and the OUI list")
		if err := seedAll(database); err != nil {
			log.Fatalf("automatic seed: %v (fix the problem or run with -seed to see it again)", err)
		}
	}

	if *discoverMode {
		runDiscovery(database)
		return
	}

	if err := snmp.InitMIBs("resources/mibs"); err != nil {
		log.Fatal(err)
	}
	pollTimeout := time.Duration(config.Int("poll_timeout_seconds", 10)) * time.Second
	nodeMaxPerMac := config.Int("node_max_per_mac", 10)

	if *pollOnce != "" {
		ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
		defer cancel()
		if err := scheduler.PollByName(ctx, database, *pollOnce, pollTimeout, nodeMaxPerMac); err != nil {
			log.Fatalf("poll %s: %v", *pollOnce, err)
		}
		fmt.Printf("polled %s successfully\n", *pollOnce)
		return
	}

	hash := *authHash
	if hash == "" {
		hash = os.Getenv("SWITCHGOPHER_PASSWORD_HASH")
	}
	auth, err := web.NewAuth(*authUser, hash)
	if err != nil {
		log.Fatalf("web login not configured: %v (generate a hash with: switchgopher -hash-password)", err)
	}

	pollInterval := time.Duration(config.Int("poll_interval_seconds", 300)) * time.Second
	pollConcurrency := config.Int("poll_concurrency", 10)
	pruneInterval := time.Duration(config.Int("prune_interval_hours", 24)) * time.Hour
	nodeRetention := time.Duration(config.Int("node_retention_days", 90)) * 24 * time.Hour
	arpInterval := time.Duration(config.Int("arp_poll_interval_seconds", 300)) * time.Second
	dnsConcurrency := config.Int("dns_concurrency", 20)
	dnsTimeout := time.Duration(config.Int("dns_lookup_timeout_seconds", 3)) * time.Second
	dnsDomainSuffix := config.String("dns_domain_suffix", "")
	go scheduler.StartScheduler(database, pollInterval, pollTimeout, pollConcurrency, pruneInterval, nodeRetention, nodeMaxPerMac, arpInterval, dnsConcurrency, dnsTimeout, dnsDomainSuffix)

	// start webserver
	addr := fmt.Sprintf(":%d", *httpPort)
	fmt.Printf("listening on %s\n", addr)
	// Explicit timeouts so a slow or stalled client can't hold a connection
	// open forever (http.ListenAndServe sets none).
	srv := &http.Server{
		Addr:              addr,
		Handler:           web.NewRouter(database, auth),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal("Unable to serve web page!")
	}
}
