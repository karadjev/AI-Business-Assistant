package network

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// DNS is checked and dialled by its validated IP, preventing private targets and rebinding.
func SafeClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect forbidden") }, Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, errors.New("invalid destination")
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil || len(ips) == 0 {
			return nil, errors.New("destination unavailable")
		}
		for _, a := range ips {
			if !publicIP(a.IP) {
				return nil, errors.New("private destination forbidden")
			}
		}
		return (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}}}
}
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		_, subnet, _ := net.ParseCIDR(block)
		if subnet.Contains(ip) {
			return false
		}
	}
	return true
}
