// Package scheduler ties the snmp (fetch) and store (persist) packages
// together: polling one device end-to-end, and running that on a
// concurrent, timeout-bounded loop across every seeded device.
package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/dns"
	"github.com/tyandl/switchgopher/internal/snmp"
	"github.com/tyandl/switchgopher/internal/store"
)

// splitHostPort supports devices.Address being either a bare host/IP
// (default SNMP port 161) or "host:port" for non-standard setups (e.g. a
// local snmpd test instance, which runs unprivileged on a high port).
func splitHostPort(address string) (host string, port uint16) {
	h, p, err := net.SplitHostPort(address)
	if err != nil {
		return address, 161
	}
	n, err := strconv.ParseUint(p, 10, 16)
	if err != nil {
		return h, 161
	}
	return h, uint16(n)
}

// deviceTarget is the minimal subset of a devices row the poller needs.
// The SNMPv3 fields are all optional - empty/unset means the device polls
// as v2c via Community, exactly as before SNMPv3 support existed. See
// buildSession for how these map to gosnmp's v3 API.
type deviceTarget struct {
	Device    string
	Address   string
	Community string

	SnmpVersion    string
	Username       string
	SecurityLevel  string
	AuthProtocol   string
	AuthPassphrase string
	PrivProtocol   string
	PrivPassphrase string
	ContextName    string
}

// deviceTargetColumns is the shared column list for scanning a
// deviceTarget - loadDeviceTargets/loadRouterTargets/PollByName all need
// the identical set, kept in one place rather than repeated three times.
const deviceTargetColumns = `Device, Address, Community, SnmpVersion, Username, SecurityLevel, AuthProtocol, AuthPassphrase, PrivProtocol, PrivPassphrase, ContextName`

// scanDeviceTarget scans one row (from either *sql.Row or *sql.Rows - both
// implement Scan) into a deviceTarget, in deviceTargetColumns' exact
// order. The SNMPv3 columns are nullable (NULL on any device row that
// predates this migration and hasn't been re-seeded since), so they're
// scanned via sql.NullString first - same NULL-tolerance fix already
// needed for ports.IfType/PoeClass elsewhere in this project.
func scanDeviceTarget(row interface{ Scan(dest ...any) error }) (deviceTarget, error) {
	var t deviceTarget
	var snmpVersion, username, securityLevel, authProtocol, authPassphrase, privProtocol, privPassphrase, contextName sql.NullString
	err := row.Scan(&t.Device, &t.Address, &t.Community, &snmpVersion, &username, &securityLevel,
		&authProtocol, &authPassphrase, &privProtocol, &privPassphrase, &contextName)
	if err != nil {
		return t, err
	}
	t.SnmpVersion = snmpVersion.String
	t.Username = username.String
	t.SecurityLevel = securityLevel.String
	t.AuthProtocol = authProtocol.String
	t.AuthPassphrase = authPassphrase.String
	t.PrivProtocol = privProtocol.String
	t.PrivPassphrase = privPassphrase.String
	t.ContextName = contextName.String
	return t, nil
}

func loadDeviceTargets(db *sql.DB) ([]deviceTarget, error) {
	rows, err := db.Query(`SELECT ` + deviceTargetColumns + ` FROM devices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []deviceTarget
	for rows.Next() {
		t, err := scanDeviceTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// loadRouterTargets returns the devices to ARP-poll for nodes.Ip -
// config-declared (devices.IsRouter, mirroring switchmap's own
// hand-maintained router list) or auto-detected (devices.IsLayer3, from
// the IsLayer3Resolver fetch group).
func loadRouterTargets(db *sql.DB) ([]deviceTarget, error) {
	rows, err := db.Query(`SELECT ` + deviceTargetColumns + ` FROM devices WHERE IsRouter = 1 OR IsLayer3 = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []deviceTarget
	for rows.Next() {
		t, err := scanDeviceTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// PollByName looks up a single already-seeded device by name and polls it
// once. Meant for fast manual iteration during development/verification
// (e.g. against a local snmpd test instance), independent of the scheduler's
// device list or interval.
func PollByName(ctx context.Context, db *sql.DB, deviceName string, timeout time.Duration, maxPerMac int) error {
	row := db.QueryRow(`SELECT `+deviceTargetColumns+` FROM devices WHERE Device = ?`, deviceName)
	target, err := scanDeviceTarget(row)
	if err != nil {
		return fmt.Errorf("loading device %q: %w", deviceName, err)
	}
	return pollDevice(ctx, db, target, timeout, maxPerMac)
}

// BuildSessionForDevice looks up an already-seeded device by name and
// builds a *gosnmp.GoSNMP session for it using whatever credentials it
// has on file (v2c or v3) - the one piece of pollDevice/pollArp's logic
// a caller outside this package needs (the discovery tool's CDP-crawl
// mode, to connect to a named seed device), without duplicating
// buildSession's v2c/v3 branching a third time.
func BuildSessionForDevice(ctx context.Context, db *sql.DB, deviceName string, timeout time.Duration) (*gosnmp.GoSNMP, error) {
	row := db.QueryRow(`SELECT `+deviceTargetColumns+` FROM devices WHERE Device = ?`, deviceName)
	target, err := scanDeviceTarget(row)
	if err != nil {
		return nil, fmt.Errorf("loading device %q: %w", deviceName, err)
	}
	return buildSession(ctx, target, timeout)
}

// v3SecurityLevels/v3AuthProtocols/v3PrivProtocols map devices.SecurityLevel/
// AuthProtocol/PrivProtocol's stored strings to gosnmp's own enum names
// (kept identical to gosnmp's names, not reinvented, so they stay
// greppable against its source/docs).
var v3SecurityLevels = map[string]gosnmp.SnmpV3MsgFlags{
	"noAuthNoPriv": gosnmp.NoAuthNoPriv,
	"authNoPriv":   gosnmp.AuthNoPriv,
	"authPriv":     gosnmp.AuthPriv,
}

var v3AuthProtocols = map[string]gosnmp.SnmpV3AuthProtocol{
	"MD5":    gosnmp.MD5,
	"SHA":    gosnmp.SHA,
	"SHA224": gosnmp.SHA224,
	"SHA256": gosnmp.SHA256,
	"SHA384": gosnmp.SHA384,
	"SHA512": gosnmp.SHA512,
}

var v3PrivProtocols = map[string]gosnmp.SnmpV3PrivProtocol{
	"DES":     gosnmp.DES,
	"AES":     gosnmp.AES,
	"AES192":  gosnmp.AES192,
	"AES256":  gosnmp.AES256,
	"AES192C": gosnmp.AES192C,
	"AES256C": gosnmp.AES256C,
}

// buildSession constructs the *gosnmp.GoSNMP for one poll - shared by
// pollDevice and pollArp, which otherwise would have had to duplicate the
// v3 branch below. target.SnmpVersion == "3" selects v3; anything else
// (including "2c"/"" - every device predating SNMPv3 support) keeps
// today's v2c behavior unchanged. Returns an error for a v3 device with
// an unrecognized or missing SecurityLevel/protocol rather than silently
// connecting with the wrong security parameters - surfaces as the same
// per-device poll-failure log line a Connect() error already produces,
// not a crash.
//
// Engine discovery (engineID/engineBoots/engineTime) is NOT cached here -
// gosnmp's negotiateInitialSecurityParameters (v3.go) already runs that
// handshake automatically and transparently inside the session's first
// real request, so v3 works correctly without it. pollDevice already
// builds a brand-new session per poll with no cross-cycle reuse for any
// device, v2c or v3, so caching engine params would be inconsistent with
// how every other device is handled - left as a possible future
// optimization, not a correctness gap.
func buildSession(ctx context.Context, target deviceTarget, timeout time.Duration) (*gosnmp.GoSNMP, error) {
	target, err := resolveSecrets(target)
	if err != nil {
		return nil, err
	}
	host, port := splitHostPort(target.Address)
	session := &gosnmp.GoSNMP{
		Target:  host,
		Port:    port,
		Timeout: timeout,
		Retries: 1,
		Context: ctx,
	}

	if target.SnmpVersion != "3" {
		session.Version = gosnmp.Version2c
		session.Community = target.Community
		return session, nil
	}

	flags, ok := v3SecurityLevels[target.SecurityLevel]
	if !ok {
		return nil, fmt.Errorf("device %s: unrecognized SecurityLevel %q for SNMPv3", target.Device, target.SecurityLevel)
	}
	usp := &gosnmp.UsmSecurityParameters{UserName: target.Username}
	if flags&gosnmp.AuthNoPriv > 0 { // required for both authNoPriv and authPriv
		proto, ok := v3AuthProtocols[target.AuthProtocol]
		if !ok {
			return nil, fmt.Errorf("device %s: unrecognized AuthProtocol %q for SNMPv3", target.Device, target.AuthProtocol)
		}
		usp.AuthenticationProtocol = proto
		usp.AuthenticationPassphrase = target.AuthPassphrase
	}
	if flags == gosnmp.AuthPriv {
		proto, ok := v3PrivProtocols[target.PrivProtocol]
		if !ok {
			return nil, fmt.Errorf("device %s: unrecognized PrivProtocol %q for SNMPv3", target.Device, target.PrivProtocol)
		}
		usp.PrivacyProtocol = proto
		usp.PrivacyPassphrase = target.PrivPassphrase
	}

	session.Version = gosnmp.Version3
	session.SecurityModel = gosnmp.UserSecurityModel
	session.MsgFlags = flags
	session.SecurityParameters = usp
	session.ContextName = target.ContextName
	return session, nil
}

// pollDevice fetches everything for one device over SNMP and writes it back
// to the database in a single transaction.
func pollDevice(ctx context.Context, db *sql.DB, target deviceTarget, timeout time.Duration, maxPerMac int) error {
	session, err := buildSession(ctx, target, timeout)
	if err != nil {
		return err
	}
	if err := session.Connect(); err != nil {
		return fmt.Errorf("connect %s: %w", target.Device, err)
	}
	defer session.Close()

	result, err := snmp.FetchAll(session)
	if err != nil {
		return fmt.Errorf("fetchAll %s: %w", target.Device, err)
	}
	dev := result.Device
	dev.Device = target.Device
	dev.Address = target.Address
	dev.CommunityString = target.Community

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx %s: %w", target.Device, err)
	}
	defer tx.Rollback()

	if err := store.UpsertDevice(tx, dev); err != nil {
		return fmt.Errorf("upsertDevice %s: %w", target.Device, err)
	}
	if err := store.ReplacePorts(tx, target.Device, result.Ports); err != nil {
		return fmt.Errorf("replacePorts %s: %w", target.Device, err)
	}
	if err := store.InsertNodes(tx, target.Device, result.Nodes, result.Ports, maxPerMac); err != nil {
		return fmt.Errorf("insertNodes %s: %w", target.Device, err)
	}
	if err := store.ReplaceModules(tx, target.Device, result.Modules); err != nil {
		return fmt.Errorf("replaceModules %s: %w", target.Device, err)
	}
	for vlan, name := range result.VlanNames {
		if err := store.UpsertDiscoveredVlan(tx, vlan, name); err != nil {
			return fmt.Errorf("upsertDiscoveredVlan %s: %w", target.Device, err)
		}
	}

	return tx.Commit()
}

// dnsOptions bundles the reverse-DNS pass's tunables (see internal/dns.Resolve)
// so pollArp/StartScheduler don't need a long, easy-to-misorder positional
// parameter list for them.
type dnsOptions struct {
	concurrency  int
	timeout      time.Duration
	domainSuffix string
}

// pollArp walks one router's ARP cache, backfills nodes.Ip for any matching
// MAC already sighted on a monitored switch, then reverse-DNS-resolves that
// same ARP table's IPs to backfill nodes.Hostname too - mirroring
// switchmap's own MacIpTables.pm, which fills both MacIpAddr and
// MacHostName from the same pass. Deliberately separate from pollDevice -
// it targets a different, usually much smaller set of devices (see
// loadRouterTargets), on its own schedule, and doesn't write anything to
// the devices/ports/modules tables at all, so it doesn't need pollDevice's
// per-device transaction.
func pollArp(ctx context.Context, db *sql.DB, target deviceTarget, timeout time.Duration, dnsOpts dnsOptions) error {
	session, err := buildSession(ctx, target, timeout)
	if err != nil {
		return err
	}
	if err := session.Connect(); err != nil {
		return fmt.Errorf("connect %s: %w", target.Device, err)
	}
	defer session.Close()

	arp, err := snmp.FetchArpTable(session)
	if err != nil {
		return fmt.Errorf("fetchArpTable %s: %w", target.Device, err)
	}
	if err := store.UpdateNodeIPs(db, arp); err != nil {
		return fmt.Errorf("updateNodeIPs %s: %w", target.Device, err)
	}

	hostnames := dns.Resolve(ctx, net.DefaultResolver, arp, dnsOpts.concurrency, dnsOpts.timeout, dnsOpts.domainSuffix)
	if err := store.UpdateNodeHostnames(db, hostnames); err != nil {
		return fmt.Errorf("updateNodeHostnames %s: %w", target.Device, err)
	}
	return nil
}

// StartScheduler periodically polls every device in the devices table,
// bounded to `concurrency` simultaneous polls so a batch of slow/unreachable
// devices can't stall a whole cycle. Runs an immediate pass at startup (so
// there's live data without waiting a full interval), then re-queries the
// device list on every tick so newly-added devices are picked up without a
// restart. Also runs store.Prune on its own, much coarser interval, so the
// nodes table's growth stays bounded (see store.InsertNodes/store.Prune for
// why that table needs dedup and a per-MAC cap at write time as well as
// age-based pruning over time). Also runs pollArp on its own interval
// against loadRouterTargets' (usually much smaller) device list, to
// backfill nodes.Ip and, via a reverse-DNS pass over that same ARP data,
// nodes.Hostname - see pollArp's doc comment for why that's separate from
// the main poll. Intended to run as its own goroutine, started once from
// main().
func StartScheduler(db *sql.DB, interval, timeout time.Duration, concurrency int, pruneInterval time.Duration, retain time.Duration, maxPerMac int, arpInterval time.Duration, dnsConcurrency int, dnsTimeout time.Duration, dnsDomainSuffix string) {
	dnsOpts := dnsOptions{concurrency: dnsConcurrency, timeout: dnsTimeout, domainSuffix: dnsDomainSuffix}
	poll := func() {
		targets, err := loadDeviceTargets(db)
		if err != nil {
			log.Println("startScheduler: loading devices:", err)
			return
		}

		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		for _, t := range targets {
			wg.Add(1)
			sem <- struct{}{}
			go func(t deviceTarget) {
				defer wg.Done()
				defer func() { <-sem }()

				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				if err := pollDevice(ctx, db, t, timeout, maxPerMac); err != nil {
					log.Printf("poll %s: %v", t.Device, err)
				}
			}(t)
		}
		wg.Wait()
	}

	prune := func() {
		n, err := store.Prune(db, retain)
		if err != nil {
			log.Println("startScheduler: pruning nodes:", err)
			return
		}
		if n > 0 {
			log.Printf("startScheduler: pruned %d old node sightings\n", n)
		}
	}

	arp := func() {
		targets, err := loadRouterTargets(db)
		if err != nil {
			log.Println("startScheduler: loading router targets:", err)
			return
		}
		for _, t := range targets {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			if err := pollArp(ctx, db, t, timeout, dnsOpts); err != nil {
				log.Printf("pollArp %s: %v", t.Device, err)
			}
			cancel()
		}
	}

	// prune before the first poll - poll can take minutes across a large,
	// partly-unreachable device list, and there's no reason retention
	// cleanup should wait behind that.
	prune()
	poll()
	arp()
	pollTicker := time.NewTicker(interval)
	pruneTicker := time.NewTicker(pruneInterval)
	arpTicker := time.NewTicker(arpInterval)
	for {
		select {
		case <-pollTicker.C:
			poll()
		case <-pruneTicker.C:
			prune()
		case <-arpTicker.C:
			arp()
		}
	}
}
