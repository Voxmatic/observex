package f61

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"
)

// ErrBlockedDestination: every address the target resolves to is non-public
// and the vantage is not configured for private targets.
var ErrBlockedDestination = errors.New("probe/f61: destination is not a public address")

// blockedPrefixes are refused unless private targets are allowed (PROPOSED
// EGRESS-1, SSRF guard). netip's IsPrivate/IsLoopback/IsLinkLocal*/
// IsMulticast/IsUnspecified cover the rest.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),  // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, incl. broadcast
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64: may embed a private IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64
	netip.MustParsePrefix("2001:db8::/32"),  // documentation
	netip.MustParsePrefix("fec0::/10"),      // deprecated site-local
}

// PublicAddress reports whether ip may be probed by a vantage that is not
// allowed private targets.
func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// Resolver resolves a host name. *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// GuardedDialer dials probe targets by vetted IP address. The name is
// resolved once and the connection is made to that address, so a DNS answer
// cannot change between the check and the dial (DNS rebinding). SNI and
// hostname verification still use the configured host, because tlscert sets
// them from the endpoint, not from the dialled address.
type GuardedDialer struct {
	AllowPrivate bool
	Resolver     Resolver
	Dialer       *net.Dialer
}

// Dial matches tlscert.DialFunc.
func (g GuardedDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	d := g.Dialer
	if d == nil {
		d = &net.Dialer{Timeout: 30 * time.Second}
	}
	var candidates []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		candidates = []netip.Addr{ip}
	} else {
		r := g.Resolver
		if r == nil {
			r = net.DefaultResolver
		}
		candidates, err = r.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
	}
	var lastErr error = ErrBlockedDestination
	for _, ip := range candidates {
		if !g.AllowPrivate && !PublicAddress(ip) {
			continue
		}
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, &net.OpError{Op: "dial", Net: network, Err: lastErr}
}
