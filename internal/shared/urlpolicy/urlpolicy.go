// Package urlpolicy validates user-supplied URLs that the data plane will
// fetch (webhook / presidio / llm_classifier guards and transformers),
// guarding against SSRF into internal services. The allow-private toggle is
// set once at process startup from config; by default private, loopback,
// and link-local destinations are rejected.
package urlpolicy

import (
	"fmt"
	"net"
	"net/url"
)

// allowPrivate is set at startup by SetAllowPrivate. Startup writes happen
// before any request handling, so no synchronization is needed.
var allowPrivate bool

// SetAllowPrivate configures whether private/loopback destinations are
// permitted. Enable it only when guards intentionally target services on a
// trusted internal network (e.g. the docker-compose stack).
func SetAllowPrivate(v bool) { allowPrivate = v }

// AllowPrivate reports the current setting.
func AllowPrivate() bool { return allowPrivate }

// ValidatePublicHTTPURL checks that raw is a well-formed http(s) URL and,
// unless private destinations are allowed, that every IP its host resolves
// to is a public address. field is used in the error message.
func ValidatePublicHTTPURL(raw, field string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("%s must be a valid http(s) URL", field)
	}
	if allowPrivate {
		return nil
	}

	host := u.Hostname()
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("%s: cannot resolve host %q: %w", field, host, err)
		}
		ips = resolved
	}

	for _, ip := range ips {
		if isDisallowed(ip) {
			return fmt.Errorf("%s must not point to a private, loopback, or link-local address (%s); "+
				"set the allow-private-guard-urls option to permit internal targets", field, ip)
		}
	}
	return nil
}

// isDisallowed reports whether an IP is in a range we refuse to fetch from
// by default.
func isDisallowed(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() || // RFC 1918 + RFC 4193 (IPv6 ULA)
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		isCGNAT(ip)
}

// isCGNAT covers the RFC 6598 100.64.0.0/10 shared address space, which
// net.IP.IsPrivate does not.
func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}
