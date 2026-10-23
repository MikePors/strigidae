package github

import (
	"net"
	"strings"
)

// NormalizeHost returns host in the form used to compare GitHub hosts:
// lower-case, without a port, and with GitHub's SSH-over-443 alias
// (ssh.github.com) mapped to the web host, so every spelling of one host
// compares equal.
func NormalizeHost(host string) string {
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname // any port is a transport detail, not part of the host
	}
	host = strings.ToLower(host)
	if host == "ssh.github.com" {
		return "github.com"
	}
	return host
}
