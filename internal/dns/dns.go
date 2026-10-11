// Package dns resolves nodes' IP addresses to hostnames via reverse-DNS
// (PTR) lookups - the Go equivalent of switchmap's own MacIpTables.pm
// GetHostNames/GetHostNamesThreaded, read directly from the real switchmap
// source. A bounded-concurrency pass over a mac -> ip map, run on its own
// schedule alongside the ARP walk that produces that map (see
// internal/scheduler's pollArp) - not an SNMP fetch group, since this data
// comes from DNS, not a polled device.
package dns

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Resolver is the subset of *net.Resolver this package needs, so tests can
// substitute a fake instead of hitting real DNS. *net.Resolver (including
// net.DefaultResolver) already satisfies this.
type Resolver interface {
	LookupAddr(ctx context.Context, addr string) (names []string, err error)
}

// Resolve reverse-looks-up every IP in ips (mac -> ip), returning mac ->
// hostname for each successful lookup - a miss is simply absent from the
// result, not recorded as blank, mirroring switchmap's own "count hits vs
// misses" behavior rather than overwriting a previously-known hostname with
// nothing.
//
// Runs up to concurrency lookups at once: switchmap's own comment on this
// exact problem says unthreaded reverse lookups at its site took over 12
// hours, and threading brought that under 20 minutes - a bounded worker
// pool here is this project's equivalent. Each lookup is bounded by
// perLookupTimeout so one slow or dead nameserver can't stall the whole
// pass (also switchmap's own concern - see its $DnsIsBroken workaround for
// a subnet whose DNS server was down).
//
// domainSuffix, if non-empty, is stripped from the end of each resolved
// name (case-insensitive, with or without a leading dot) - switchmap does
// the same with $ThisSite::DnsDomain so hostnames display short instead of
// fully-qualified.
func Resolve(ctx context.Context, r Resolver, ips map[string]string, concurrency int, perLookupTimeout time.Duration, domainSuffix string) map[string]string {
	type pair struct{ mac, ip string }
	work := make(chan pair)
	hostnames := map[string]string{}
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				lctx, cancel := context.WithTimeout(ctx, perLookupTimeout)
				names, err := r.LookupAddr(lctx, p.ip)
				cancel()
				if err != nil || len(names) == 0 {
					continue
				}
				host := stripDomainSuffix(strings.TrimSuffix(names[0], "."), domainSuffix)
				mu.Lock()
				hostnames[p.mac] = host
				mu.Unlock()
			}
		}()
	}
	for mac, ip := range ips {
		work <- pair{mac, ip}
	}
	close(work)
	wg.Wait()

	return hostnames
}

// stripDomainSuffix removes a trailing domain suffix from host, matching
// switchmap's own `s/$ThisSite::DnsDomain$//` - case-insensitive, and
// tolerant of the suffix being written with or without a leading dot.
func stripDomainSuffix(host, suffix string) string {
	if suffix == "" {
		return host
	}
	suffix = strings.TrimPrefix(suffix, ".")
	lowerHost := strings.ToLower(host)
	lowerSuffix := "." + strings.ToLower(suffix)
	if strings.HasSuffix(lowerHost, lowerSuffix) {
		return host[:len(host)-len(lowerSuffix)]
	}
	return host
}
