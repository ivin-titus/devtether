package netutil

import (
	"errors"
	"net"
	"strings"
)

// NormalizeHost normalizes the host header by removing brackets, ports, and trailing dots.
func NormalizeHost(host string) string {
	// First, strip IPv6 `[` and `]` literals only when both are present.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	// Then, use net.SplitHostPort to strip the port (handling the case where there is no port).
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else {
		var addrErr *net.AddrError
		if !errors.As(err, &addrErr) || (addrErr.Err != "missing port in address" && addrErr.Err != "too many colons in address") {
			host = ""
		}
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}

	// Sync DNS and Proxy FQDN stripping: remove trailing dot.
	host = strings.TrimSuffix(host, ".")

	return strings.ToLower(host)
}
