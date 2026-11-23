// Package urlcheck holds the one check for whether a service-to-service URL would send a shared
// secret in plain text to the outside, so every client that sends one warns about the same URLs.
package urlcheck

import (
	"net"
	"net/url"
	"strings"
)

// InsecureExternal reports whether raw sends data unencrypted (plain HTTP) to
// a host that is not clearly internal. Loopback, private IPs, and short service
// names (docker/k8s, e.g. "server-core") are treated as internal/trusted and do
// not warn; a dotted public hostname or public IP over HTTP does.
func InsecureExternal(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true // unparseable: warn rather than silently accept a bad config
	}
	if u.Scheme == "https" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return true // e.g. a scheme-less "server-core:8080", which is malformed: warn
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return false // loopback or a short service name (docker/k8s)
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsPrivate()
	}
	return true
}
